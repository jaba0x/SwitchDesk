package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/store"
)

func TestTopologyAndIssues(t *testing.T) {
	core := store.Switch{ID: "a", Host: "10.0.0.1", Port: 22}
	acc := store.Switch{ID: "b", Host: "10.0.0.11", Port: 22}
	coreSnap := &aruba.Snapshot{
		Family: aruba.FamilyAOSCX, CollectedAt: time.Now(),
		Facts: aruba.Facts{Hostname: "core-cx", BaseMAC: "00:11:22:aa:bb:cc", CPU: 5, Memory: 20},
		STP:   aruba.STP{Known: true, Enabled: true, IsRoot: true, BridgePriority: "4096", RootMAC: "00:11:22:aa:bb:cc"},
		Neighbors: []aruba.Neighbor{
			{Protocol: "lldp", LocalPort: "1/1/1", SysName: "SW-ACCESS-01", MgmtIP: "10.0.0.11", PortID: "49"},
			{Protocol: "lldp", LocalPort: "1/1/48", SysName: "fw01", Description: "FortiGate-100F", Capabilities: "Router"},
		},
		RunningConfig: "x", Security: aruba.Security{NTPServers: []string{"1"}, LogServers: []string{"1"}, AAA: []string{"x"}},
	}
	accSnap := &aruba.Snapshot{
		Family: aruba.FamilyAOSS, CollectedAt: time.Now(),
		Facts: aruba.Facts{Hostname: "SW-ACCESS-01", CPU: 95, Memory: 10},
		STP:   aruba.STP{Known: true, Enabled: true, RootMAC: "00:11:22:aa:bb:cc", Ports: []aruba.STPPort{{Port: "50", State: "Blocking"}}},
		Neighbors: []aruba.Neighbor{
			{Protocol: "lldp", LocalPort: "49", SysName: "core-cx", ChassisID: "00 11 22 aa bb cc", PortID: "1/1/1"},
			{Protocol: "cdp", LocalPort: "49", SysName: "core-cx", PortID: "1/1/1"},
			{Protocol: "lldp", LocalPort: "5", SysName: "SEP00AABBCCDDEE", Description: "Cisco IP Phone"},
		},
		Security:      aruba.Security{TelnetEnabled: true, HTTPPlain: true, SNMPDefault: true},
		RunningConfig: "x",
		Env:           []aruba.EnvItem{{Kind: "fan", Name: "Sys-2", Status: "Fan Failed", OK: false}},
	}
	infos := []switchInfo{{sw: core, snap: coreSnap, online: true}, {sw: acc, snap: accSnap, online: true}}
	topo := buildTopology(infos, map[string]store.Point{"a": {X: 1, Y: 2}}, false)
	if len(topo.Nodes) != 3 { // core, access, firewall (phone hidden)
		t.Fatalf("nodes: %+v", topo.Nodes)
	}
	if len(topo.Edges) != 2 {
		t.Fatalf("edges: %+v", topo.Edges)
	}
	for _, e := range topo.Edges {
		if e.From == "a" && e.To == "b" && (e.FromPort != "1/1/1" || e.ToPort != "49" || len(e.Protocols) != 2) {
			t.Fatalf("core link: %+v", e)
		}
	}
	if topo.Nodes[0].X == nil || *topo.Nodes[0].X != 1 {
		t.Fatal("layout not applied")
	}
	withPhones := buildTopology(infos, nil, true)
	if len(withPhones.Nodes) != 4 {
		t.Fatalf("with endpoints: %d", len(withPhones.Nodes))
	}

	st := &Status{Online: true, Checked: true, TopologyDelta: 3}
	is := evaluate(acc, st, accSnap, store.DefaultSettings())
	want := map[string]bool{"Telnet is enabled": false, "Default SNMP community in use": false, "Fan problem": false, "High CPU: 95%": false, "3 topology change(s) since last check": false, "No manager password set": false}
	for _, i := range is {
		if _, ok := want[i.Title]; ok {
			want[i.Title] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("missing issue %q in %+v", k, is)
		}
	}
	off := evaluate(core, &Status{Checked: true, Online: false, PingError: "timeout"}, nil, store.DefaultSettings())
	if len(off) != 1 || off[0].Severity != Critical {
		t.Fatalf("offline: %+v", off)
	}
	two := map[string]*aruba.Snapshot{"a": coreSnap, "b": {STP: aruba.STP{Enabled: true, RootMAC: "ff:ff:ff:00:00:01"}}}
	names := map[string]string{"a": "core", "b": "acc"}
	linked := Topology{Edges: []Edge{{From: "a", To: "b"}}}
	if split := networkIssues(two, names, linked); len(split) != 1 {
		t.Fatal("expected split-root issue")
	} else if !reflect.DeepEqual(split[0].SwitchIDs, []string{"a", "b"}) {
		// the site filter places the issue by these
		t.Fatalf("switch ids: %v", split[0].SwitchIDs)
	}
	// linked only through an unmanaged switch still counts as one layer 2 network
	viaExt := Topology{Edges: []Edge{{From: "a", To: "ext:x"}, {From: "b", To: "ext:x"}}}
	if split := networkIssues(two, names, viaExt); len(split) != 1 {
		t.Fatal("expected split-root issue through an unmanaged switch")
	}
	// branches joined by routing: a root each is normal
	if split := networkIssues(two, names, Topology{}); len(split) != 0 {
		t.Fatalf("routed branches flagged: %+v", split)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveCredential(store.Credential{Name: "default", Username: "admin", Password: "p@ss"})
	if err != nil || c.Password != "" {
		t.Fatal(err, c)
	}
	sw, err := s.SaveSwitch(store.Switch{Host: "10.0.0.1", CredentialID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSwitch(store.Switch{Host: "10.0.0.1"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	s2, _ := store.Open(dir)
	got, err := s2.Credential(c.ID)
	if err != nil || got.Password != "p@ss" {
		t.Fatal("password roundtrip", err, got)
	}
	if err := s2.DeleteCredential(c.ID); err == nil {
		t.Fatal("deleted credential in use")
	}
	saved, _ := s2.SaveBackup(sw.ID, "hostname a\nvlan 1", 3)
	again, _ := s2.SaveBackup(sw.ID, "hostname a\nvlan 1", 3)
	if !saved || again {
		t.Fatal("backup dedupe", saved, again)
	}
}

func TestDisabledSwitch(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := st.SaveSwitch(store.Switch{Host: "10.9.9.1"})
	b, _ := st.SaveSwitch(store.Switch{Host: "10.9.9.2"})
	m := New(st, nil)
	m.status[a.ID] = &Status{Checked: true, Online: false}
	m.status[b.ID] = &Status{Checked: true, Online: false}
	if d := m.Dashboard(); d.Offline != 2 || d.Critical != 2 {
		t.Fatalf("before: %+v", d)
	}
	st.SetDisabled([]string{b.ID}, true)
	d := m.Dashboard()
	if d.Offline != 1 || d.Disabled != 1 || d.Critical != 1 || d.Total != 2 {
		t.Fatalf("after: offline=%d disabled=%d critical=%d", d.Offline, d.Disabled, d.Critical)
	}
	m.Enqueue(b.ID)
	if m.stat(b.ID).Collecting {
		t.Fatal("paused switch was queued")
	}
}

func TestVLANOverviewEditable(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cx, _ := st.SaveSwitch(store.Switch{Host: "10.9.9.1"})
	paused, _ := st.SaveSwitch(store.Switch{Host: "10.9.9.2"})
	web, _ := st.SaveSwitch(store.Switch{Host: "10.9.9.3", Web: true})
	st.SetDisabled([]string{paused.ID}, true)
	m := New(st, nil)
	m.snaps[cx.ID] = &aruba.Snapshot{Family: aruba.FamilyAOSCX, VLANs: []aruba.VLAN{{ID: 1}}}
	m.snaps[paused.ID] = &aruba.Snapshot{Family: aruba.FamilyAOSS}
	m.snaps[web.ID] = &aruba.Snapshot{Family: aruba.FamilyInstantOn}
	got := map[string]SwitchVLANs{}
	for _, s := range m.VLANOverview() {
		got[s.ID] = s
	}
	if !got[cx.ID].Editable || got[cx.ID].Family != "AOS-CX" || len(got[cx.ID].VLANs) != 1 {
		t.Fatalf("AOS-CX switch: %+v", got[cx.ID])
	}
	if got[paused.ID].Editable || got[paused.ID].ReadOnly != "disabled" {
		t.Fatalf("disabled switch: %+v", got[paused.ID])
	}
	if got[web.ID].Editable || got[web.ID].ReadOnly == "" {
		t.Fatalf("web switch: %+v", got[web.ID])
	}
}

// Two branches joined only by routing reuse 192.168.13.0/24. A switch in the
// first branch sees a local device at 192.168.13.2, the same address as the
// managed switch in the other branch. That must not draw a link between them.
func TestTopologyOverlappingBranchIPs(t *testing.T) {
	fl2 := store.Switch{ID: "fl2", Host: "172.16.254.14"}
	digomi := store.Switch{ID: "digomi", Host: "192.168.13.2"}
	fl2Snap := &aruba.Snapshot{
		Facts: aruba.Facts{Hostname: "SW-HP-FL2-R", BaseMAC: "ec:02:73:00:00:01"},
		STP:   aruba.STP{Enabled: true, Ports: []aruba.STPPort{{Port: "24", State: "Blocking"}}},
		Neighbors: []aruba.Neighbor{
			// a local switch in FL2's branch that happens to use 192.168.13.2
			{Protocol: "lldp", LocalPort: "24", MgmtIP: "192.168.13.2", ChassisID: "c0:74:ad:ea:29:d8", Capabilities: "Bridge"},
		},
	}
	digomiSnap := &aruba.Snapshot{
		Facts: aruba.Facts{Hostname: "SW-DIGOMI-M", BaseMAC: "38:21:c7:00:00:02"},
		STP:   aruba.STP{Enabled: true, IsRoot: true},
	}
	topo := buildTopology([]switchInfo{{sw: fl2, snap: fl2Snap, online: true}, {sw: digomi, snap: digomiSnap, online: true}}, nil, false)
	for _, e := range topo.Edges {
		if (e.From == "fl2" && e.To == "digomi") || (e.From == "digomi" && e.To == "fl2") {
			t.Fatalf("drew a link across branches: %+v", e)
		}
	}
	if len(topo.Edges) != 1 || len(topo.Nodes) != 3 {
		t.Fatalf("want FL2 linked to its own neighbor: nodes %+v edges %+v", topo.Nodes, topo.Edges)
	}

	// A stack advertises a MAC we never read, but both sides report the link,
	// so it is still drawn as one link between the managed switches.
	stack := store.Switch{ID: "stack", Host: "10.1.1.1"}
	acc := store.Switch{ID: "acc", Host: "10.1.1.2"}
	stackSnap := &aruba.Snapshot{
		Facts:     aruba.Facts{Hostname: "CORE-STACK", BaseMAC: "00:00:00:00:00:aa"},
		Neighbors: []aruba.Neighbor{{Protocol: "lldp", LocalPort: "1/1/1", MgmtIP: "10.1.1.2", ChassisID: "00:00:00:00:00:bb", PortID: "49"}},
	}
	accSnap := &aruba.Snapshot{
		Facts:     aruba.Facts{Hostname: "ACC", BaseMAC: "00:00:00:00:00:bb"},
		Neighbors: []aruba.Neighbor{{Protocol: "lldp", LocalPort: "49", MgmtIP: "10.1.1.1", ChassisID: "00:00:00:00:99:99", PortID: "1/1/1"}},
	}
	topo = buildTopology([]switchInfo{{sw: stack, snap: stackSnap, online: true}, {sw: acc, snap: accSnap, online: true}}, nil, false)
	if len(topo.Nodes) != 2 || len(topo.Edges) != 1 {
		t.Fatalf("stack link: nodes %+v edges %+v", topo.Nodes, topo.Edges)
	}
}

func TestOpticIssues(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	snap := &aruba.Snapshot{
		Family: aruba.FamilyAOSCX, CollectedAt: time.Now(),
		Ports: []aruba.Port{{Name: "1/1/49", Up: true, Enabled: true}, {Name: "1/1/50", Up: true, Enabled: true}, {Name: "1/1/51", Up: false, Enabled: true}, {Name: "1/1/52", Up: true, Enabled: true}},
		Optics: []aruba.Optic{
			{Port: "1/1/49", Type: "SFP+LR", DOM: true, Rx: []float64{-14.2}, RxLowWarn: f(-14), RxLowAlarm: f(-18)},
			{Port: "1/1/50", Type: "SFP+SR", DOM: true, Rx: []float64{-19.5}, RxLowWarn: f(-10), RxLowAlarm: f(-13)},
			{Port: "1/1/51", Type: "SFP+SR", DOM: true, Rx: []float64{aruba.NoLight}, RxLowWarn: f(-10), RxLowAlarm: f(-13)}, // empty port
			{Port: "1/1/52", Type: "SFP+SR", DOM: true, Rx: []float64{-3}, RxLowWarn: f(-10), RxLowAlarm: f(-13)},
		},
	}
	is := evaluate(store.Switch{ID: "x", Host: "10.0.0.1"}, &Status{Online: true, Checked: true}, snap, store.DefaultSettings())
	var crit, warn *Issue
	for i := range is {
		switch is[i].Title {
		case "Optical power alarm on 1 port(s)":
			crit = &is[i]
		case "Optical power warning on 1 port(s)":
			warn = &is[i]
		}
	}
	if crit == nil || !strings.Contains(crit.Detail, "1/1/50") || strings.Contains(crit.Detail, "1/1/51") {
		t.Fatalf("alarm: %+v", is)
	}
	if warn == nil || !strings.Contains(warn.Detail, "1/1/49 (SFP+LR): Rx -14.2 dBm, warning at -14.0") {
		t.Fatalf("warning: %+v", warn)
	}
}
