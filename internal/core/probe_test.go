package core

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
)

func TestPortProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, open, _ := net.SplitHostPort(ln.Addr().String())
	if got := probePort("127.0.0.1", open); got != "open" {
		t.Fatalf("listening port: %s", got)
	}
	// a port that was just freed is refused on loopback
	tmp, _ := net.Listen("tcp", "127.0.0.1:0")
	_, closed, _ := net.SplitHostPort(tmp.Addr().String())
	tmp.Close()
	if got := probePort("127.0.0.1", closed); got != "closed" {
		t.Fatalf("closed port: %s", got)
	}
	probeTimeout = 300 * time.Millisecond
	if got := probePort("192.0.2.1", "23"); got != "filtered" { // TEST-NET-1, never answers
		t.Logf("unroutable address gave %s (fine on some networks)", got)
	}
}

func TestTelnetNotAssumed(t *testing.T) {
	// the friend's case: Telnet turned off, but the config read didn't come
	// back, so the old code assumed the AOS-S default and flagged it
	_, sec, _ := aruba.ParseConfigAOSS("")
	if sec.TelnetEnabled || sec.HTTPPlain {
		t.Fatalf("empty config must not mean telnet on: %+v", sec)
	}
	// with the probe, a refused port 23 wins over a config that looks open
	snap := aruba.Snapshot{Family: aruba.FamilyAOSS, Security: aruba.Security{TelnetEnabled: true, HTTPPlain: true}}
	probeTimeout = 3 * time.Second
	free := func() string {
		l, _ := net.Listen("tcp", "127.0.0.1:0")
		_, p, _ := net.SplitHostPort(l.Addr().String())
		l.Close()
		return p
	}
	oldT, oldH := telnetPortNum, httpPortNum
	telnetPortNum, httpPortNum = free(), free() // CI machines may run a web server on 80
	defer func() { telnetPortNum, httpPortNum = oldT, oldH }()
	applyPortProbes(&snap, "127.0.0.1")
	if snap.Security.TelnetEnabled || snap.Security.HTTPPlain || snap.Security.TelnetPort == "" {
		t.Fatalf("probe should clear telnet/http: %+v", snap.Security)
	}
}

// Issue #18: with "no web-management plaintext" the switch still answers on
// port 80, but only to redirect to HTTPS. That isn't plain HTTP management.
func TestHTTPRedirectIsNotPlain(t *testing.T) {
	serve := func(h http.HandlerFunc) (string, func()) {
		s := httptest.NewServer(h)
		_, p, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
		return p, s.Close
	}
	redir, stop1 := serve(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.Host+"/", http.StatusFound)
	})
	defer stop1()
	page, stop2 := serve(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>login</html>")) })
	defer stop2()
	elsewhere, stop3 := serve(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login.html", http.StatusFound) })
	defer stop3()

	probeTimeout = 3 * time.Second
	for port, want := range map[string]string{redir: "redirect", page: "open", elsewhere: "open"} {
		if got := probeHTTP("127.0.0.1", port); got != want {
			t.Errorf("port %s: %s, want %s", port, got, want)
		}
	}

	old := httpPortNum
	defer func() { httpPortNum = old }()
	httpPortNum = redir
	snap := aruba.Snapshot{Family: aruba.FamilyAOSS, Security: aruba.Security{HTTPPlain: true}}
	applyPortProbes(&snap, "127.0.0.1")
	if snap.Security.HTTPPlain || snap.Security.HTTPPort != "redirect" {
		t.Fatalf("redirect-only port 80 still flagged: %+v", snap.Security)
	}
}
