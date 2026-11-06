package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/jaba0x/switchdesk/internal/ionlocal"
	"github.com/jaba0x/switchdesk/internal/sshx"
	"github.com/jaba0x/switchdesk/internal/store"
	"github.com/jaba0x/switchdesk/internal/tlspin"
)

// Instant On switches in local management mode have no SSH CLI. They're read
// through their web interface's XML pages with the switch's admin login.

// ErrWebOnly is returned for actions that need SSH.
var ErrWebOnly = errors.New("this Instant On switch is read through its web interface and has no SSH; make changes on the switch's web page")

func webClient(sw store.Switch) *ionlocal.Client {
	host := sw.Host
	if sw.Port != 0 && sw.Port != 80 && sw.Port != 443 {
		host = net.JoinHostPort(sw.Host, strconv.Itoa(sw.Port))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	c := ionlocal.New(host, sw.Port == 443 || sw.Port%1000 == 443, &tlspin.Pin{Want: sw.CertFP}) // 443, 8443
	c.RequireEncryption = sw.WebEncrypted
	return c
}

// rememberWeb pins what a successful sign-in showed: the certificate on first
// use, and that the switch uses encrypted login.
func (m *Manager) rememberWeb(sw store.Switch, c *ionlocal.Client) {
	if c.TLS && sw.CertFP == "" {
		if fp := c.Pin.Seen(); fp != "" {
			m.st.SetCertFP(sw.ID, fp)
		}
	}
	if c.Encrypted && !sw.WebEncrypted {
		m.st.SetWebEncrypted(sw.ID, true)
	}
}

// identityChanged reports errors that mean the switch isn't the one pinned.
func identityChanged(err error) bool {
	return errors.Is(err, tlspin.ErrChanged) || errors.Is(err, ionlocal.ErrDowngrade) || errors.Is(err, sshx.ErrHostKeyMismatch)
}

func (m *Manager) webRead(sw store.Switch) {
	st := m.stat(sw.ID)
	fail := func(err error) {
		m.mu.Lock()
		st.CollectError = err.Error()
		st.HostKeyChanged = identityChanged(err)
		st.AuthFailed = errors.Is(err, ionlocal.ErrLogin)
		st.LastCollect = time.Now()
		m.mu.Unlock()
	}
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		fail(err)
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	defer cancel()
	c := webClient(sw)
	err = c.Login(ctx, cred.Username, cred.Password)
	m.readReach(sw.ID, err)
	if err != nil {
		fail(err)
		return
	}
	m.rememberWeb(sw, c)
	snap, err := ionlocal.Collect(ctx, c)
	c.Logout(context.Background())
	if err != nil {
		fail(err)
		return
	}
	applyPortProbes(&snap, sw.Host)

	m.mu.Lock()
	prev := m.snaps[sw.ID]
	st.TopologyDelta = 0
	if prev != nil && prev.STP.Enabled && snap.STP.TopologyChanges > prev.STP.TopologyChanges {
		st.TopologyDelta = snap.STP.TopologyChanges - prev.STP.TopologyChanges
	}
	noteUserChanges(st, prev, &snap)
	m.snaps[sw.ID] = &snap
	st.LastCollect = time.Now()
	st.CollectError, st.AuthFailed, st.HostKeyChanged = "", false, false
	m.mu.Unlock()
	if m.ctx.Err() == nil {
		m.st.SaveSnapshot(sw.ID, snap)
	}
}

// testWeb signs in once for the Add switch dialog. When the port wasn't
// reachable it tries the other of 443 and 80, and says which one worked.
func (m *Manager) testWeb(sw store.Switch) (string, error) {
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if sw.Port == 0 || sw.Port == 22 {
		sw.Port = 443
	}
	// pins come from the stored switch, never from the dialog
	sw.CertFP, sw.WebEncrypted = "", false
	if o, ok := m.st.Switch(sw.ID); ok && sw.ID != "" && strings.EqualFold(o.Host, sw.Host) && o.Port == sw.Port {
		sw.CertFP, sw.WebEncrypted = o.CertFP, o.WebEncrypted
	}
	try := []int{sw.Port}
	if sw.Port == 443 {
		try = append(try, 80)
	} else if sw.Port == 80 {
		try = append(try, 443)
	}
	var lastErr error
	for _, p := range try {
		sw.Port = p
		if p != try[0] {
			// a different port is a different endpoint: no pins carried over
			sw.CertFP, sw.WebEncrypted = "", false
		}
		c := webClient(sw)
		if err := c.Login(ctx, cred.Username, cred.Password); err != nil {
			if identityChanged(err) {
				return "", err
			}
			var ne net.Error
			if errors.As(err, &ne) || strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "connection reset") {
				lastErr = err
				continue
			}
			return "", err
		}
		sys, err := c.Get(ctx, "SystemGlobalSetting")
		c.Logout(context.Background())
		if err != nil {
			return "", err
		}
		msg := fmt.Sprintf("Signed in to %s (%s)", sys.Text("systemName"), strings.SplitN(sys.Text("systemDescription"), ",", 2)[0])
		if p != try[0] {
			msg += fmt.Sprintf(". Port %d didn't answer; port %d works, so set the port to %d", try[0], p, p)
		}
		return msg, nil
	}
	return "", lastErr
}
