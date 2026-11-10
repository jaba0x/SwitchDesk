// Package sshopen starts an outside SSH client for a switch (issue #23).
// No password is ever passed: the client asks for it, or uses its own keys
// and saved sessions.
package sshopen

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type Target struct {
	Name string
	Host string
	Port int
	User string
}

// Only plain host names, IPv4 and IPv6 addresses and ordinary user names go
// on a command line, so nothing can be read as an option or a second command.
var (
	hostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-_:%\[\]]*$`)
	userRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-@\\]*$`)
)

func (t Target) check() error {
	if !hostRe.MatchString(t.Host) {
		return fmt.Errorf("can't open %q in an SSH client: unusual characters in the address", t.Host)
	}
	if t.User != "" && !userRe.MatchString(t.User) {
		return fmt.Errorf("can't open an SSH client for user %q: unusual characters in the name", t.User)
	}
	if t.Port <= 0 || t.Port > 65535 {
		return errors.New("the switch has no valid SSH port")
	}
	return nil
}

// Command builds the program and arguments to run. template is the custom
// command from Settings; empty picks the computer's default terminal.
// lookPath is exec.LookPath, replaceable in tests.
func Command(t Target, template, goos string, lookPath func(string) (string, error)) ([]string, error) {
	if err := t.check(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(template) != "" {
		args, err := Split(template)
		if err != nil {
			return nil, err
		}
		r := strings.NewReplacer("{host}", t.Host, "{port}", strconv.Itoa(t.Port), "{user}", t.User, "{name}", t.Name)
		for i := range args {
			args[i] = r.Replace(args[i])
		}
		if len(args) == 0 || args[0] == "" {
			return nil, errors.New("the SSH client command in Settings is empty")
		}
		return args, nil
	}
	dest := t.Host
	if t.User != "" {
		dest = t.User + "@" + t.Host
	}
	sshArgs := []string{"ssh", "-p", strconv.Itoa(t.Port), dest}
	switch goos {
	case "darwin":
		// Terminal (or whichever app owns ssh://) opens the session
		host := t.Host
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			host = "[" + host + "]"
		}
		u := "ssh://"
		if t.User != "" {
			u += t.User + "@"
		}
		return []string{"open", u + host + ":" + strconv.Itoa(t.Port)}, nil
	case "windows":
		if _, err := lookPath("wt.exe"); err == nil {
			return append([]string{"wt.exe"}, sshArgs...), nil
		}
		// Windows' built-in OpenSSH client, in a console window of its own
		return append([]string{"ssh.exe"}, sshArgs[1:]...), nil
	default:
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
			if _, err := lookPath(term); err == nil {
				if term == "gnome-terminal" {
					return append([]string{term, "--"}, sshArgs...), nil
				}
				return append([]string{term, "-e"}, sshArgs...), nil
			}
		}
		return nil, errors.New("no terminal found; set an SSH client command in Settings")
	}
}

// Open runs the client for t without waiting for it.
func Open(t Target, template string) error {
	args, err := Command(t, template, runtime.GOOS, exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("couldn't start %s: %w", args[0], err)
	}
	go cmd.Wait()
	return nil
}

// Split breaks a command line into arguments the way a shell would for plain
// words, "double quotes" and 'single quotes'. Backslashes stay as they are,
// so Windows paths work.
func Split(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	in := false
	var quote rune
	for _, c := range s {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case c == '"' || c == '\'':
			quote, in = c, true
		case c == ' ' || c == '\t' || c == '\n':
			if in {
				args = append(args, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(c)
			in = true
		}
	}
	if quote != 0 {
		return nil, errors.New("the SSH client command has an unclosed quote")
	}
	if in {
		args = append(args, cur.String())
	}
	return args, nil
}
