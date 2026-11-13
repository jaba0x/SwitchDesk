package aruba

import (
	"reflect"
	"strings"
	"testing"
)

// Issue #32: edit port names, and take them from LLDP.

func TestBuildPortNameCommands(t *testing.T) {
	ports := []Port{{Name: "3", Description: "Printer"}, {Name: "10"}, {Name: "2"}, {Name: "Trk1"}}
	ch := PortNameChange{Names: map[string]string{"10": "AP Office", "3": "", "2": "Camera  ", "Trk1": "Uplink-SW02"}, Save: true}
	got, err := BuildPortNameCommands(FamilyAOSS, ch, ports)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"configure terminal",
		"interface 2", `name "Camera"`, "exit",
		"interface 3", "no name", "exit",
		"interface 10", `name "AP Office"`, "exit",
		"interface Trk1", `name "Uplink-SW02"`, "exit",
		"end", "write memory"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aos-s:\n%s", strings.Join(got, "\n"))
	}

	cx := []Port{{Name: "1/1/1", Description: "old"}, {Name: "1/1/2"}}
	got, err = BuildPortNameCommands(FamilyAOSCX, PortNameChange{Names: map[string]string{"1/1/1": "", "1/1/2": "dc-sw01 uplink"}}, cx)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"configure terminal", "interface 1/1/1", "no description", "exit", "interface 1/1/2", "description dc-sw01 uplink", "exit", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aos-cx:\n%s", strings.Join(got, "\n"))
	}

	for _, bad := range []PortNameChange{
		{Names: map[string]string{"99": "x"}},                     // not on the switch
		{Names: map[string]string{"10": `say "hi"`}},              // quote
		{Names: map[string]string{"10": "what?"}},                 // CLI help key
		{Names: map[string]string{"10": strings.Repeat("a", 65)}}, // too long
		{Names: map[string]string{"10": "Büro"}},                  // not ASCII
		{Names: map[string]string{"3": "Printer"}},                // nothing changes
		{Names: map[string]string{"10; reload": "x"}},             // not a port
	} {
		if _, err := BuildPortNameCommands(FamilyAOSS, bad, ports); err == nil {
			t.Errorf("accepted %v", bad.Names)
		}
	}
	if _, err := BuildPortNameCommands(FamilyInstantOn, ch, ports); err == nil {
		t.Error("instant on accepted")
	}
}

func TestSuggestPortNames(t *testing.T) {
	s := &Snapshot{Family: FamilyAOSS,
		Ports: []Port{{Name: "1"}, {Name: "2", Description: "My printer"}, {Name: "3", Description: "old-ap"}, {Name: "4"}, {Name: "5", Description: "sw-02"}, {Name: "6"}},
		Neighbors: []Neighbor{
			{Protocol: "lldp", LocalPort: "1", SysName: "AP-Lobby", Capabilities: "wlan-access-point"},
			{Protocol: "lldp", LocalPort: "2", SysName: "HP-Printer"},                    // typed by hand: kept
			{Protocol: "lldp", LocalPort: "3", SysName: "AP-Hall"},                       // was ours, neighbor changed
			{Protocol: "lldp", LocalPort: "4", SysName: "SEP001122334455"},               // phone first...
			{Protocol: "lldp", LocalPort: "4", SysName: "sw-03", Capabilities: "bridge"}, // ...switch wins
			{Protocol: "lldp", LocalPort: "5", SysName: "sw-02"},                         // already right
			{Protocol: "lldp", LocalPort: "6", SysName: ""},                              // nothing to use
			{Protocol: "lldp", LocalPort: "7", SysName: "ghost"},                         // no such port
		}}
	got := SuggestPortNames(s, map[string]string{"3": "old-ap", "2": "something else"})
	want := []PortNameSuggestion{
		{Port: "1", Name: "AP-Lobby", Neighbor: "AP-Lobby"},
		{Port: "3", Current: "old-ap", Name: "AP-Hall", Neighbor: "AP-Hall", Replaces: true},
		{Port: "4", Name: "sw-03", Neighbor: "sw-03"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if CleanPortName("  Büro \"Nord\"?\tAP ") != "Bro Nord AP" {
		t.Fatalf("clean: %q", CleanPortName("  Büro \"Nord\"?\tAP "))
	}
	if got := AfterPortNames(s.Ports, PortNameChange{Names: map[string]string{"1": "AP-Lobby", "2": ""}}); got[0].Description != "AP-Lobby" || got[1].Description != "" || s.Ports[0].Description != "" {
		t.Fatalf("after: %+v", got)
	}
}
