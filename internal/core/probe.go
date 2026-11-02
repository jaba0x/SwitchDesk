package core

import (
	"bufio"
	"errors"
	"net"
	"net/http"
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
func probeHTTP(host, port string) string {
	st := probePort(host, port)
	if st != "open" {
		return st
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), probeTimeout)
	if err != nil {
		return st
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(probeTimeout))
	req, _ := http.NewRequest("GET", "http://"+host+"/", nil)
	req.Header.Set("User-Agent", "SwitchDesk")
	req.Header.Set("Connection", "close")
	if req.Write(c) != nil {
		return st
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), req)
	if err != nil {
		return st
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && strings.HasPrefix(strings.ToLower(resp.Header.Get("Location")), "https://") {
		return "redirect"
	}
	return st
}

// applyPortProbes checks Telnet (23) and plain HTTP (80) directly. What the
// switch actually answers beats what its config seems to say: a config that
// wasn't read, or a setting changed since, no longer raises a false alarm.
func applyPortProbes(s *aruba.Snapshot, host string) {
	var wg sync.WaitGroup
	var telnet, web string
	wg.Add(2)
	go func() { defer wg.Done(); telnet = probePort(host, telnetPortNum) }()
	go func() { defer wg.Done(); web = probeHTTP(host, httpPortNum) }()
	wg.Wait()
	s.Security.TelnetPort, s.Security.HTTPPort = telnet, web
	s.Security.TelnetEnabled = telnet == "open"
	if s.Family == aruba.FamilyAOSS {
		s.Security.HTTPPlain = web == "open"
	}
}
