package core

import (
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
)

var probeTimeout = 3 * time.Second

// ports probed; tests point these at ports known to be free
var telnetPortNum, httpPortNum = "23", "80"

// probePort reports "open" when something accepts a TCP connection, "closed"
// when the switch refuses it, and "filtered" when nothing answers in time.
func probePort(host, port string) string {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), probeTimeout)
	if err == nil {
		c.Close()
		return "open"
	}
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused") {
		return "closed"
	}
	return "filtered"
}

// probeHTTP is probePort for the web port, which also tells a real web
// interface apart from one that only redirects to HTTPS. With
// "no web-management plaintext", AOS-S still answers on port 80 but only to
// send the browser to https://, so that's "redirect", not "open" (issue #18).
// It also returns the start of what port 80 answered, for the Raw output tab.
func probeHTTP(host, port string) (string, string) {
	st := probePort(host, port)
	if st != "open" {
		return st, ""
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), probeTimeout)
	if err != nil {
		return st, ""
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(probeTimeout))
	hostHdr := host
	if strings.Contains(host, ":") {
		hostHdr = "[" + host + "]"
	}
	if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: "+hostHdr+"\r\nUser-Agent: SwitchDesk\r\nAccept: */*\r\nConnection: close\r\n\r\n"); err != nil {
		return st, ""
	}
	// Read the answer as plain text instead of with net/http: the small web
	// servers on switches don't always follow HTTP to the letter (bare line
	// feeds, odd header spacing), and newer Go versions reject such answers,
	// which brought the false warning of issue #18 back.
	raw, _ := io.ReadAll(io.LimitReader(c, 16<<10))
	if httpOnlyRedirects(string(raw)) {
		return "redirect", string(raw)
	}
	return st, string(raw)
}

var (
	statusLineRe = regexp.MustCompile(`(?i)^HTTP/\d(?:\.\d)?\s+(\d{3})`)
	// <meta http-equiv="refresh" content="0;url=https://..."> or
	// location.href / location.replace("https://...") in a script
	softRedirectRe = regexp.MustCompile(`(?is)(http-equiv\s*=\s*["']?refresh[^>]*url\s*=\s*["']?\s*https://|location(?:\.href)?\s*=\s*["']https://|location\.replace\(\s*["']https://)`)
	loginFormRe    = regexp.MustCompile(`(?i)<form|type\s*=\s*["']?password`)
)

// httpOnlyRedirects says whether an answer from port 80 only sends the
// browser on to HTTPS: a 3xx with an https:// Location, or a page that does
// nothing but forward to https:// (no login form of its own).
func httpOnlyRedirects(raw string) bool {
	head, body := raw, ""
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := strings.Index(raw, sep); i >= 0 {
			head, body = raw[:i], raw[i+len(sep):]
			break
		}
	}
	lines := strings.Split(strings.ReplaceAll(head, "\r", ""), "\n")
	m := statusLineRe.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if m == nil {
		return false
	}
	code := m[1]
	if code[0] == '3' {
		for _, l := range lines[1:] {
			k, v, ok := strings.Cut(l, ":")
			if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
				return strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "https://")
			}
		}
		return false
	}
	return code == "200" && softRedirectRe.MatchString(body) && !loginFormRe.MatchString(body)
}

// applyPortProbes checks Telnet (23) and plain HTTP (80) directly. What the
// switch actually answers beats what its config seems to say: a config that
// wasn't read, or a setting changed since, no longer raises a false alarm.
func applyPortProbes(s *aruba.Snapshot, host string) {
	var wg sync.WaitGroup
	var telnet, web, answer string
	wg.Add(2)
	go func() { defer wg.Done(); telnet = probePort(host, telnetPortNum) }()
	go func() { defer wg.Done(); web, answer = probeHTTP(host, httpPortNum) }()
	wg.Wait()
	s.Security.TelnetPort, s.Security.HTTPPort = telnet, web
	if answer != "" {
		// what port 80 said, so a wrong verdict can be checked in Raw output
		if s.Raw == nil {
			s.Raw = map[string]string{}
		}
		if len(answer) > 2000 {
			answer = answer[:2000] + "\n…"
		}
		s.Raw["HTTP probe (port 80)"] = answer
	}
	s.Security.TelnetEnabled = telnet == "open"
	if s.Family == aruba.FamilyAOSS {
		s.Security.HTTPPlain = web == "open"
	}
}
