package sshopen

import (
	"os"
	"strings"
)

// Client is an SSH program SwitchDesk knows how to start. Most tools that
// open devices in an outside client either hand an ssh:// link to the OS
// (LibreNMS, Observium), offer a fixed list of known clients (NetBrain,
// Royal TS), or take any program with variables (mRemoteNG's external tools).
// SwitchDesk does all three: Automatic, the clients below, and Custom.
type Client struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Found bool   `json:"found"`
	// Command is the command line with the program's path filled in, for
	// showing in Settings.
	Command string `json:"command"`

	exe      []string // program names on PATH or full paths, the first found wins
	template string   // {exe}, {dest} (user@host), {url} (ssh://user@host:port), {host}, {port}, {user}, {name}
}

func pf(env, rest string) string {
	base := os.Getenv(env)
	if base == "" {
		return ""
	}
	return base + `\` + rest
}

func known(goos string) []Client {
	switch goos {
	case "windows":
		return []Client{
			{ID: "wt", Name: "Windows Terminal", exe: []string{"wt.exe"}, template: `{exe} ssh -p {port} {dest}`},
			{ID: "putty", Name: "PuTTY", exe: []string{"putty.exe", pf("ProgramFiles", `PuTTY\putty.exe`), pf("ProgramFiles(x86)", `PuTTY\putty.exe`)}, template: `{exe} -ssh {dest} -P {port}`},
			{ID: "kitty", Name: "KiTTY", exe: []string{"kitty.exe", pf("ProgramFiles", `KiTTY\kitty.exe`), pf("ProgramFiles(x86)", `KiTTY\kitty.exe`)}, template: `{exe} -ssh {dest} -P {port}`},
			{ID: "mobaxterm", Name: "MobaXterm", exe: []string{"MobaXterm.exe", pf("ProgramFiles(x86)", `Mobatek\MobaXterm\MobaXterm.exe`), pf("ProgramFiles", `Mobatek\MobaXterm\MobaXterm.exe`)}, template: `{exe} -newtab "ssh -p {port} {dest}"`},
			{ID: "securecrt", Name: "SecureCRT", exe: []string{"SecureCRT.exe", pf("ProgramFiles", `VanDyke Software\SecureCRT\SecureCRT.exe`)}, template: `{exe} /T /SSH2 /P {port} /L {user} {host}`},
			{ID: "openssh", Name: "OpenSSH (console window)", exe: []string{"ssh.exe"}, template: `{exe} -p {port} {dest}`},
		}
	case "darwin":
		return []Client{
			{ID: "terminal", Name: "Terminal", exe: []string{"/System/Applications/Utilities/Terminal.app"}, template: `open -a Terminal {url}`},
			{ID: "iterm", Name: "iTerm2", exe: []string{"/Applications/iTerm.app"}, template: `open -a iTerm {url}`},
			{ID: "ghostty", Name: "Ghostty", exe: []string{"/Applications/Ghostty.app"}, template: `open -na Ghostty --args -e ssh -p {port} {dest}`},
			{ID: "wezterm", Name: "WezTerm", exe: []string{"wezterm", "/Applications/WezTerm.app/Contents/MacOS/wezterm"}, template: `{exe} start -- ssh -p {port} {dest}`},
			{ID: "securecrt", Name: "SecureCRT", exe: []string{"/Applications/SecureCRT.app/Contents/MacOS/SecureCRT"}, template: `{exe} /T /SSH2 /P {port} /L {user} {host}`},
		}
	default:
		return []Client{
			{ID: "gnome-terminal", Name: "GNOME Terminal", exe: []string{"gnome-terminal"}, template: `{exe} -- ssh -p {port} {dest}`},
			{ID: "konsole", Name: "Konsole", exe: []string{"konsole"}, template: `{exe} -e ssh -p {port} {dest}`},
			{ID: "wezterm", Name: "WezTerm", exe: []string{"wezterm"}, template: `{exe} start -- ssh -p {port} {dest}`},
			{ID: "xterm", Name: "xterm", exe: []string{"xterm"}, template: `{exe} -e ssh -p {port} {dest}`},
		}
	}
}

// find returns the first of the client's programs that exists, or "".
// An entry with a path separator is a file or app bundle; anything else is
// looked up on PATH.
func (c Client) find(lookPath func(string) (string, error), stat func(string) error) string {
	for _, e := range c.exe {
		if e == "" {
			continue
		}
		if strings.ContainsAny(e, `/\`) {
			if stat(e) == nil {
				return e
			}
		} else if p, err := lookPath(e); err == nil {
			return p
		}
	}
	return ""
}

// Clients lists the known clients for goos, with the ones found on this
// computer marked.
func Clients(goos string, lookPath func(string) (string, error), stat func(string) error) []Client {
	list := known(goos)
	for i := range list {
		exe := list[i].find(lookPath, stat)
		list[i].Found = exe != ""
		shown := exe
		if shown == "" && len(list[i].exe) > 0 {
			shown = list[i].exe[0]
		}
		list[i].Command = strings.ReplaceAll(list[i].template, "{exe}", quote(shown))
	}
	return list
}

func quote(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

func fileExists(p string) error {
	_, err := os.Stat(p)
	return err
}
