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

// Placeholders a command line can use.
func (t Target) replacer() *strings.Replacer {
	dest := t.Host
	if t.User != "" {
		dest = t.User + "@" + t.Host
	}
	host := t.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	u := "ssh://"
	if t.User != "" {
		u += t.User + "@"
	}
	u += host + ":" + strconv.Itoa(t.Port)
	return strings.NewReplacer("{dest}", dest, "{url}", u, "{host}", t.Host, "{port}", strconv.Itoa(t.Port), "{user}", t.User, "{name}", t.Name)
}

// fill splits a command line and fills in the placeholders. {exe} becomes
// exe as a single argument, spaces and all.
func (t Target) fill(template, exe string) ([]string, error) {
	args, err := Split(template)
	if err != nil {
		return nil, err
	}
	r := t.replacer()
	for i := range args {
		if args[i] == "{exe}" {
			args[i] = exe
			continue
		}
		args[i] = r.Replace(args[i])
	}
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("the SSH client command in Settings is empty")
	}
	return args, nil
}

// Command builds the program and arguments to run. client is "" for
// automatic, "custom" for the command line in custom, or the ID of one of
// the known clients. A custom command saved without a client (1.0.7) still
// counts as custom.
func Command(t Target, client, custom, goos string, lookPath func(string) (string, error), stat func(string) error) ([]string, error) {
	if err := t.check(); err != nil {
		return nil, err
	}
	if client == "custom" || (client == "" && strings.TrimSpace(custom) != "") {
		if strings.TrimSpace(custom) == "" {
			return nil, errors.New("choose an SSH client in Settings, or write its command line")
		}
		return t.fill(custom, "")
	}
	if client != "" && client != "auto" {
		for _, c := range known(goos) {
			if c.ID != client {
				continue
			}
			exe := c.find(lookPath, stat)
			if exe == "" {
				return nil, fmt.Errorf("%s isn't installed on this computer; pick another SSH client in Settings", c.Name)
			}
			return t.fill(c.template, exe)
		}
		return nil, fmt.Errorf("unknown SSH client %q; pick one in Settings", client)
	}
	dest := t.Host
	if t.User != "" {
		dest = t.User + "@" + t.Host
	}
	sshArgs := []string{"ssh", "-p", strconv.Itoa(t.Port), dest}
	switch goos {
	case "darwin":
		// Terminal, or whichever app owns ssh:// links, opens the session
		return t.fill("open {url}", "")
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
		return nil, errors.New("no terminal found; pick an SSH client in Settings")
	}
}

// Open runs the client for t without waiting for it.
func Open(t Target, client, custom string) error {
	args, err := Command(t, client, custom, runtime.GOOS, exec.LookPath, fileExists)
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
