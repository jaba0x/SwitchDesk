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
		got, err := Command(tg, "", c.goos, found(c.look...))
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v: got %q, %v", c.goos, c.look, got, err)
		}
	}
	if _, err := Command(tg, "", "linux", found()); err == nil {
		t.Error("linux without a terminal should ask for a command")
	}
	v6, _ := Command(Target{Host: "fe80::1", Port: 2222, User: "admin"}, "", "darwin", found())
	if v6[1] != "ssh://admin@[fe80::1]:2222" {
		t.Errorf("ipv6 url = %s", v6[1])
	}
}

func TestCustomCommand(t *testing.T) {
	tg := Target{Name: "acc 1", Host: "sw1.example.net", Port: 2222, User: "netops"}
	got, err := Command(tg, `"C:\Program Files (x86)\Mobatek\MobaXterm\MobaXterm.exe" -newtab "ssh -p {port} {user}@{host}"`, "windows", found())
	want := []string{`C:\Program Files (x86)\Mobatek\MobaXterm\MobaXterm.exe`, "-newtab", "ssh -p 2222 netops@sw1.example.net"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, %v", got, err)
	}
	got, _ = Command(tg, "putty.exe -ssh {user}@{host} -P {port}", "windows", found())
	if !reflect.DeepEqual(got, []string{"putty.exe", "-ssh", "netops@sw1.example.net", "-P", "2222"}) {
		t.Fatalf("putty = %q", got)
	}
	if _, err := Command(tg, `putty "{host}`, "windows", found()); err == nil {
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
		if _, err := Command(tg, "", "linux", found("xterm")); err == nil {
			t.Errorf("%+v was accepted", tg)
		}
	}
}
