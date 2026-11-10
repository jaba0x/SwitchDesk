package main

import (
	"errors"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/jaba0x/switchdesk/internal/sshopen"
)

// OpenSSH opens the switch in an outside SSH client: the computer's own
// terminal, or the command set in Settings (issue #23). The password isn't
// passed on; the client asks for it or uses its own keys.
func (a *App) OpenSSH(id string) error {
	sw, ok := a.st.Switch(id)
	if !ok {
		return errors.New("switch not found")
	}
	if sw.Cloud != nil || sw.Web {
		return errors.New("this switch is read through Instant On, not SSH")
	}
	t := sshopen.Target{Name: sw.Name, Host: sw.Host, Port: sw.Port}
	if t.Port == 0 {
		t.Port = 22
	}
	if c, err := a.st.Credential(sw.CredentialID); err == nil && c.Kind == "" {
		t.User = c.Username
	}
	set := a.st.Settings()
	return sshopen.Open(t, set.SSHClient, set.SSHCommand)
}

// SSHClients lists the SSH clients SwitchDesk knows on this kind of
// computer, with the installed ones marked, for Settings.
func (a *App) SSHClients() []sshopen.Client {
	return sshopen.Clients(goruntime.GOOS, exec.LookPath, func(p string) error { _, err := os.Stat(p); return err })
}
