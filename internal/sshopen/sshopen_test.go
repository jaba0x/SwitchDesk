package sshopen

import (
	"errors"
	"reflect"
	"testing"
)

func found(names ...string) func(string) (string, error) {
	return func(n string) (string, error) {
		for _, x := range names {
			if x == n {
				return "/usr/bin/" + n, nil
			}
		}
		return "", errors.New("not found")
	}
}

func nofile(string) error { return errors.New("no such file") }

func files(paths ...string) func(string) error {
	return func(p string) error {
		for _, x := range paths {
			if x == p {
				return nil
			}
		}
		return errors.New("no such file")
	}
}

func TestKnownClients(t *testing.T) {
	tg := Target{Name: "core", Host: "10.0.0.1", Port: 22, User: "admin"}
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramFiles(x86)", `C:\Program Files (x86)`)
	moba := `C:\Program Files (x86)\Mobatek\MobaXterm\MobaXterm.exe`
	got, err := Command(tg, "mobaxterm", "", "windows", found(), files(moba))
	if err != nil || !reflect.DeepEqual(got, []string{moba, "-newtab", "ssh -p 22 admin@10.0.0.1"}) {
		t.Fatalf("mobaxterm = %q, %v", got, err)
	}
	got, _ = Command(tg, "putty", "", "windows", found("putty.exe"), nofile)
	if !reflect.DeepEqual(got, []string{"/usr/bin/putty.exe", "-ssh", "admin@10.0.0.1", "-P", "22"}) {
		t.Fatalf("putty = %q", got)
	}
	got, _ = Command(tg, "iterm", "", "darwin", found(), files("/Applications/iTerm.app"))
	if !reflect.DeepEqual(got, []string{"open", "-a", "iTerm", "ssh://admin@10.0.0.1:22"}) {
		t.Fatalf("iterm = %q", got)
	}
	if _, err := Command(tg, "securecrt", "", "windows", found(), nofile); err == nil {
		t.Fatal("a client that isn't installed should be an error")
	}
	if _, err := Command(tg, "custom", "", "windows", found(), nofile); err == nil {
		t.Fatal("custom without a command should be an error")
	}
	list := Clients("windows", found("wt.exe"), files(moba))
	byID := map[string]Client{}
	for _, c := range list {
		byID[c.ID] = c
	}
	if !byID["wt"].Found || !byID["mobaxterm"].Found || byID["putty"].Found || byID["mobaxterm"].Command != `"`+moba+`" -newtab "ssh -p {port} {dest}"` {
		t.Fatalf("clients = %+v", list)
	}
}

func TestDefaultClients(t *testing.T) {
	tg := Target{Name: "core", Host: "10.0.0.1", Port: 22, User: "admin"}
	cases := []struct {
		goos string
		look []string
		want []string
	}{
		{"darwin", nil, []string{"open", "ssh://admin@10.0.0.1:22"}},
		{"windows", []string{"wt.exe"}, []string{"wt.exe", "ssh", "-p", "22", "admin@10.0.0.1"}},
		{"windows", nil, []string{"ssh.exe", "-p", "22", "admin@10.0.0.1"}},
		{"linux", []string{"gnome-terminal"}, []string{"gnome-terminal", "--", "ssh", "-p", "22", "admin@10.0.0.1"}},
		{"linux", []string{"xterm"}, []string{"xterm", "-e", "ssh", "-p", "22", "admin@10.0.0.1"}},
	}
	for _, c := range cases {
		got, err := Command(tg, "", "", c.goos, found(c.look...), nofile)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v: got %q, %v", c.goos, c.look, got, err)
		}
	}
	if _, err := Command(tg, "", "", "linux", found(), nofile); err == nil {
		t.Error("linux without a terminal should ask for a command")
	}
	v6, _ := Command(Target{Host: "fe80::1", Port: 2222, User: "admin"}, "", "", "darwin", found(), nofile)
	if v6[1] != "ssh://admin@[fe80::1]:2222" {
		t.Errorf("ipv6 url = %s", v6[1])
	}
}

func TestCustomCommand(t *testing.T) {
	tg := Target{Name: "acc 1", Host: "sw1.example.net", Port: 2222, User: "netops"}
	got, err := Command(tg, "custom", `"C:\Program Files (x86)\Mobatek\MobaXterm\MobaXterm.exe" -newtab "ssh -p {port} {user}@{host}"`, "windows", found(), nofile)
	want := []string{`C:\Program Files (x86)\Mobatek\MobaXterm\MobaXterm.exe`, "-newtab", "ssh -p 2222 netops@sw1.example.net"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, %v", got, err)
	}
	// a 1.0.7 setting: custom command without a client
	got, _ = Command(tg, "", "putty.exe -ssh {user}@{host} -P {port}", "windows", found(), nofile)
	if !reflect.DeepEqual(got, []string{"putty.exe", "-ssh", "netops@sw1.example.net", "-P", "2222"}) {
		t.Fatalf("putty = %q", got)
	}
	if _, err := Command(tg, "custom", `putty "{host}`, "windows", found(), nofile); err == nil {
		t.Error("unclosed quote should be an error")
	}
}

func TestRejectsOddValues(t *testing.T) {
	for _, tg := range []Target{
		{Host: "-oProxyCommand=calc", Port: 22},
		{Host: "10.0.0.1 & calc", Port: 22},
		{Host: "10.0.0.1", Port: 22, User: "-oProxyCommand=x"},
		{Host: "10.0.0.1", Port: 22, User: "a b"},
		{Host: "10.0.0.1", Port: 0},
	} {
		if _, err := Command(tg, "", "", "linux", found("xterm"), nofile); err == nil {
			t.Errorf("%+v was accepted", tg)
		}
	}
}
