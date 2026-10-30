package aruba

import (
	"reflect"
	"strings"
	"testing"
)

const aossPortAccessCfg = `hostname "SW-ACCESS-01"
radius-server host 10.0.0.5 key "s3cret"
radius-server host 10.0.0.6
radius-server host 10.0.0.6 dyn-authorization
aaa authentication port-access eap-radius
aaa port-access authenticator 1-3
aaa port-access authenticator 1 client-limit 2
aaa port-access authenticator 2 reauth-period 3600
aaa port-access authenticator 3 unauth-vid 99
aaa port-access authenticator 9 client-limit 4
aaa port-access authenticator active
aaa port-access mac-based 3-4
aaa port-access mac-based 4 addr-limit 3
vlan 1
   name "DEFAULT_VLAN"
   untagged 1-24
   exit
`

func TestParsePortAccessAOSS(t *testing.T) {
	pa := ParsePortAccessAOSS(aossPortAccessCfg)
	if !pa.Dot1xOn || !pa.MacAuthOn {
		t.Fatalf("global: %+v", pa)
	}
	if !reflect.DeepEqual(pa.Servers, []RadiusServer{{Host: "10.0.0.5", HasKey: true}, {Host: "10.0.0.6"}}) {
		t.Fatalf("servers: %+v", pa.Servers)
	}
	want := []PortAuth{
		{Port: "1", Dot1x: true, ClientLimit: 2},
		{Port: "2", Dot1x: true, ReauthPeriod: 3600},
		{Port: "3", Dot1x: true, MacAuth: true, UnauthVLAN: 99},
		{Port: "4", MacAuth: true, ClientLimit: 3},
	}
	// port 9 has an option but no method, so it isn't listed
	if !reflect.DeepEqual(pa.Ports, want) {
		t.Fatalf("ports: %+v", pa.Ports)
	}
}

const cxPortAccessCfg = `hostname core-cx
radius-server host 10.0.0.5 key ciphertext AQBapXX vrf mgmt
radius-server host 10.0.0.6 key ciphertext AQBapYY
aaa authentication port-access dot1x authenticator
    enable
aaa authentication port-access mac-auth enable
interface 1/1/1
    no shutdown
    aaa authentication port-access client-limit 2
    aaa authentication port-access dot1x authenticator
        enable
        reauth
        reauth-period 3600
    aaa authentication port-access mac-auth
        enable
    vlan access 20
interface 1/1/2-1/1/3
    aaa authentication port-access dot1x authenticator
        enable
        reauth-period 600
interface 1/1/4
    enable
    vlan access 20
interface vlan 20
    ip address 10.20.0.1/24
`

func TestParsePortAccessCX(t *testing.T) {
	pa := ParsePortAccessCX(cxPortAccessCfg)
	if !pa.Dot1xOn || !pa.MacAuthOn {
		t.Fatalf("global: %+v", pa)
	}
	if !reflect.DeepEqual(pa.Servers, []RadiusServer{{Host: "10.0.0.5", VRF: "mgmt", HasKey: true}, {Host: "10.0.0.6", HasKey: true}}) {
		t.Fatalf("servers: %+v", pa.Servers)
	}
	want := []PortAuth{
		{Port: "1/1/1", Dot1x: true, MacAuth: true, ClientLimit: 2, ReauthPeriod: 3600},
		// reauth-period without "reauth" doesn't re-authenticate
		{Port: "1/1/2", Dot1x: true},
		{Port: "1/1/3", Dot1x: true},
	}
	// "enable" on 1/1/4 is the port itself, not port access
	if !reflect.DeepEqual(pa.Ports, want) {
		t.Fatalf("ports: %+v", pa.Ports)
	}

	// a real 6x00 port with roles and an explicit "exit"
	pa = ParsePortAccessCX(`port-access role RoleGuest
    description Guests and failed logins
    vlan trunk native 3
    vlan trunk native name 3-Guest
    vlan trunk allowed 10,20-21
port-access role RoleLan
    vlan access 10
    reauth-period 3600
port-access role RoleVoice
    vlan access name 30-Voice
interface 1/1/15
    no shutdown
    vlan access 8
    aaa authentication port-access critical-role RoleGuest
    aaa authentication port-access preauth-role RoleGuest
    aaa authentication port-access reject-role RoleGuest
    aaa authentication port-access auth-role RoleLan
    aaa authentication port-access dot1x authenticator
        enable
    exit
interface 1/1/16
    no shutdown
`)
	roles := map[string]string{"critical-role": "RoleGuest", "preauth-role": "RoleGuest", "reject-role": "RoleGuest", "auth-role": "RoleLan"}
	if !reflect.DeepEqual(pa.Ports, []PortAuth{{Port: "1/1/15", Dot1x: true, Roles: roles}}) {
		t.Fatalf("port roles: %+v", pa.Ports)
	}
	if !reflect.DeepEqual(pa.Roles, []PortRole{
		{Name: "RoleGuest", Description: "Guests and failed logins", VLAN: 3, VLANName: "3-Guest", VLANMode: "trunk", Tagged: []int{10, 20, 21}},
		{Name: "RoleLan", VLAN: 10, VLANMode: "access"},
		{Name: "RoleVoice", VLANName: "30-Voice", VLANMode: "access"},
	}) {
		t.Fatalf("roles: %+v", pa.Roles)
	}
}

func TestBuildPortAccessRoles(t *testing.T) {
	cur := PortAccess{Dot1xOn: true, Roles: []PortRole{{Name: "RoleLan", Description: "Staff", VLAN: 10}},
		Ports: []PortAuth{{Port: "1/1/1", Dot1x: true, Roles: map[string]string{"auth-role": "RoleLan", "reject-role": "RoleLan"}}}}
	got, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{
		SetRole: &PortRole{Name: "RoleGuest", VLAN: 8},
		Ports:   []string{"1/1/1"}, Roles: map[string]string{"reject-role": "RoleGuest", "critical-role": "RoleGuest", "preauth-role": ""},
	}, cur)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"configure terminal",
		"port-access role RoleGuest", "vlan access 8", "exit",
		"interface 1/1/1",
		"aaa authentication port-access reject-role RoleGuest",
		"aaa authentication port-access critical-role RoleGuest",
		"no aaa authentication port-access preauth-role",
		"exit",
		"end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}

	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{SetRole: &PortRole{Name: "RoleLan"}}, cur)
	if !reflect.DeepEqual(got, []string{"configure terminal", "port-access role RoleLan", "no description", "no vlan access 10", "exit", "end"}) {
		t.Fatalf("clear role: %v", got)
	}

	// a trunk role with its native VLAN also set by name: a new description leaves the
	// VLAN lines alone, apart from allowing the native VLAN it was missing
	guest := PortRole{Name: "RoleGuest", VLAN: 3, VLANName: "3-Guest", VLANMode: "trunk", Tagged: []int{10}}
	cur.Roles = append(cur.Roles, guest)
	edit := guest
	edit.Description = "Guests"
	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{SetRole: &edit}, cur)
	if !reflect.DeepEqual(got, []string{"configure terminal", "port-access role RoleGuest", "description Guests", "vlan trunk allowed 3", "exit", "end"}) {
		t.Fatalf("description only: %v", got)
	}
	// "no vlan trunk native" alone is incomplete on the switch, and the name is its own line
	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{SetRole: &PortRole{Name: "RoleGuest", VLAN: 8, VLANMode: "access"}}, cur)
	want = []string{"configure terminal", "port-access role RoleGuest", "no vlan trunk native 3", "no vlan trunk native name 3-Guest", "no vlan trunk allowed 10", "vlan access 8", "exit", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native to access: %v", got)
	}

	trunk := map[string]struct {
		old, r PortRole
		want   []string
	}{
		"access to trunk": {PortRole{VLAN: 8, VLANMode: "access"}, PortRole{VLAN: 8, VLANMode: "trunk", Tagged: []int{10, 11}},
			[]string{"no vlan access 8", "vlan trunk native 8", "vlan trunk allowed 8,10-11"}},
		"tagged changes only": {PortRole{VLAN: 3, VLANName: "3-Guest", VLANMode: "trunk", Tagged: []int{3, 10, 20}}, PortRole{VLAN: 3, VLANName: "3-Guest", VLANMode: "trunk", Tagged: []int{20, 30}},
			[]string{"no vlan trunk allowed 10", "vlan trunk allowed 30"}},
		"new native replaces the old one": {PortRole{VLAN: 111, VLANName: "test", VLANMode: "trunk", Tagged: []int{111, 211}}, PortRole{VLAN: 5, VLANMode: "trunk", Tagged: []int{211}},
			[]string{"no vlan trunk native 111", "no vlan trunk native name test", "vlan trunk native 5", "no vlan trunk allowed 111", "vlan trunk allowed 5"}},
		"trunk to access": {PortRole{VLAN: 3, VLANMode: "trunk", Tagged: []int{3, 10}}, PortRole{VLAN: 5, VLANMode: "access"},
			[]string{"no vlan trunk native 3", "no vlan trunk allowed 3,10", "vlan access 5"}},
		"tagged only": {PortRole{}, PortRole{VLANMode: "trunk", Tagged: []int{40}},
			[]string{"vlan trunk allowed 40"}},
	}
	for name, tc := range trunk {
		if got := roleVLANLines(tc.old, tc.r); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v", name, got)
		}
	}
	// saving a role without changing it sends nothing
	same := PortRole{Name: "RoleLan", Description: "Staff", VLAN: 10, VLANMode: "access"}
	if _, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{SetRole: &same, Save: true}, cur); err == nil || err.Error() != "nothing changed" {
		t.Errorf("unchanged role: %v", err)
	}
	if _, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{SetRole: &PortRole{Name: "x", VLAN: 8, Tagged: []int{10}}}, cur); err == nil {
		t.Error("tagged VLANs on a single-VLAN role: expected an error")
	}

	next := AfterPortAccess(cur, PortAccessChange{Ports: []string{"1/1/1"}, Roles: map[string]string{"auth-role": "", "reject-role": ""}})
	if next.Ports[0].Roles != nil || cur.Ports[0].Roles["auth-role"] != "RoleLan" {
		t.Fatalf("after: %+v, cur: %+v", next.Ports[0], cur.Ports[0])
	}

	for name, c := range map[string]PortAccessChange{
		"role in use":     {DeleteRole: "RoleLan"},
		"unknown role":    {Ports: []string{"1/1/1"}, Roles: map[string]string{"auth-role": "Nope"}},
		"unknown slot":    {Ports: []string{"1/1/1"}, Roles: map[string]string{"fallback": "RoleLan"}},
		"bad role name":   {SetRole: &PortRole{Name: "a b"}},
		"bad description": {SetRole: &PortRole{Name: "x", Description: `say "hi"`}},
	} {
		if _, err := BuildPortAccessCommands(FamilyAOSCX, c, cur); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := BuildPortAccessCommands(FamilyAOSS, PortAccessChange{SetRole: &PortRole{Name: "x"}}, cur); err == nil {
		t.Error("roles on AOS-S: expected an error")
	}
}

func ptr[T any](v T) *T { return &v }

func TestBuildPortAccessAOSS(t *testing.T) {
	cur := PortAccess{Servers: []RadiusServer{}, MacAuthOn: true, Ports: []PortAuth{{Port: "3", MacAuth: true}}}
	got, err := BuildPortAccessCommands(FamilyAOSS, PortAccessChange{
		Ports: []string{"1", "2", "3"}, Dot1x: ptr(true), ClientLimit: ptr(2), UnauthVLAN: ptr(99), Save: true,
	}, cur)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"configure terminal",
		"aaa authentication port-access eap-radius",
		"aaa port-access authenticator 1-3",
		"aaa port-access authenticator 1-3 client-limit 2",
		"aaa port-access authenticator 1-3 unauth-vid 99",
		"aaa port-access mac-based 3 addr-limit 2",
		"aaa port-access mac-based 3 unauth-vid 99",
		"aaa port-access authenticator active",
		"end", "write memory",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}

	got, _ = BuildPortAccessCommands(FamilyAOSS, PortAccessChange{Ports: []string{"3"}, MacAuth: ptr(false)}, cur)
	if !reflect.DeepEqual(got, []string{"configure terminal", "no aaa port-access mac-based 3", "end"}) {
		t.Fatalf("mac off: %v", got)
	}

	got, _ = BuildPortAccessCommands(FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "10.0.0.5", Key: "s3cret"}}, cur)
	if got[1] != `radius-server host 10.0.0.5 key "s3cret"` || MaskSecret(got, PortAccessChange{AddServer: &RadiusServerChange{Key: "s3cret"}})[1] != `radius-server host 10.0.0.5 key "********"` {
		t.Fatalf("server: %v", got)
	}
}

func TestBuildPortAccessCX(t *testing.T) {
	cur := PortAccess{Servers: []RadiusServer{{Host: "10.0.0.5", VRF: "mgmt", HasKey: true}}}
	got, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{
		Ports: []string{"1/1/1"}, Dot1x: ptr(true), MacAuth: ptr(true), ReauthPeriod: ptr(3600), ClientLimit: ptr(0),
	}, cur)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"configure terminal",
		"aaa authentication port-access dot1x authenticator", "enable", "exit",
		"aaa authentication port-access mac-auth", "enable", "exit",
		"interface 1/1/1",
		"aaa authentication port-access dot1x authenticator", "enable", "reauth", "reauth-period 3600", "exit",
		"aaa authentication port-access mac-auth", "enable", "exit",
		"no aaa authentication port-access client-limit",
		"exit",
		"end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}

	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{RemoveServer: "10.0.0.5"}, cur)
	if got[1] != "no radius-server host 10.0.0.5 vrf mgmt" {
		t.Fatalf("remove: %v", got)
	}
	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{AddServer: &RadiusServerChange{Host: "radius.example.com", Key: "k3y", VRF: "mgmt"}}, cur)
	if got[1] != "radius-server host radius.example.com key plaintext k3y vrf mgmt" {
		t.Fatalf("add: %v", got)
	}
}

func TestBuildPortAccessCXOffClearsRoles(t *testing.T) {
	cur := PortAccess{Dot1xOn: true, MacAuthOn: true, Roles: []PortRole{{Name: "RoleLan"}, {Name: "RoleGuest"}},
		Ports: []PortAuth{{Port: "1/1/3", Dot1x: true, MacAuth: true, ClientLimit: 4, Roles: map[string]string{"auth-role": "RoleLan", "reject-role": "RoleGuest"}}}}
	// MAC authentication still uses the roles
	got, _ := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{Ports: []string{"1/1/3"}, Dot1x: ptr(false)}, cur)
	want := []string{"configure terminal", "interface 1/1/3", "no aaa authentication port-access dot1x authenticator", "exit", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dot1x off:\n%s", strings.Join(got, "\n"))
	}
	got, _ = BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{Ports: []string{"1/1/3"}, Dot1x: ptr(false), MacAuth: ptr(false)}, cur)
	want = []string{"configure terminal", "interface 1/1/3",
		"no aaa authentication port-access dot1x authenticator", "no aaa authentication port-access mac-auth",
		"no aaa authentication port-access auth-role", "no aaa authentication port-access reject-role",
		"no aaa authentication port-access client-limit", "exit", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("both off:\n%s", strings.Join(got, "\n"))
	}
	// the port stays free of the role, so the role can be deleted afterwards
	after := AfterPortAccess(cur, PortAccessChange{Ports: []string{"1/1/3"}, Dot1x: ptr(false), MacAuth: ptr(false)})
	if len(after.Ports) != 0 {
		t.Errorf("after: %+v", after.Ports)
	}
}

func TestBuildPortAccessGlobal(t *testing.T) {
	on := PortAccess{Dot1xOn: true, MacAuthOn: true, Ports: []PortAuth{{Port: "1", Dot1x: true}}}
	off := PortAccess{}
	cases := []struct {
		name string
		f    Family
		cur  PortAccess
		c    PortAccessChange
		want []string
	}{
		{"aos-s on", FamilyAOSS, off, PortAccessChange{GlobalDot1x: ptr(true)},
			[]string{"aaa authentication port-access eap-radius", "aaa port-access authenticator active"}},
		{"aos-s off", FamilyAOSS, on, PortAccessChange{GlobalDot1x: ptr(false)},
			[]string{"no aaa port-access authenticator active"}},
		{"cx dot1x off", FamilyAOSCX, on, PortAccessChange{GlobalDot1x: ptr(false)},
			[]string{"aaa authentication port-access dot1x authenticator", "no enable", "exit"}},
		{"cx mac on", FamilyAOSCX, off, PortAccessChange{GlobalMacAuth: ptr(true)},
			[]string{"aaa authentication port-access mac-auth", "enable", "exit"}},
		// turning a port on doesn't override an explicit switch-wide off
		{"cx port on, global stays off", FamilyAOSCX, off, PortAccessChange{GlobalDot1x: ptr(false), Ports: []string{"1/1/2"}, MacAuth: ptr(true)},
			[]string{"aaa authentication port-access mac-auth", "enable", "exit", "interface 1/1/2", "aaa authentication port-access mac-auth", "enable", "exit", "exit"}},
	}
	for _, tc := range cases {
		got, err := BuildPortAccessCommands(tc.f, tc.c, tc.cur)
		want := append(append([]string{"configure terminal"}, tc.want...), "end")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v\n%s", tc.name, err, strings.Join(got, "\n"))
		}
	}
	if _, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{GlobalDot1x: ptr(true)}, on); err == nil {
		t.Error("turning on what is already on should be 'nothing changed'")
	}
	if _, err := BuildPortAccessCommands(FamilyAOSS, PortAccessChange{GlobalMacAuth: ptr(false)}, on); err == nil {
		t.Error("AOS-S has no switch-wide MAC authentication")
	}
	if got := AfterPortAccess(on, PortAccessChange{GlobalDot1x: ptr(false)}); got.Dot1xOn || !got.MacAuthOn {
		t.Errorf("after: %+v", got)
	}
}

func TestPortAccessValidate(t *testing.T) {
	cur := PortAccess{Servers: []RadiusServer{{Host: "10.0.0.5"}}, Ports: []PortAuth{{Port: "1", Dot1x: true}}}
	bad := map[string]struct {
		f Family
		c PortAccessChange
	}{
		"nothing":              {FamilyAOSS, PortAccessChange{Save: true}},
		"instant on":           {FamilyInstantOn, PortAccessChange{Ports: []string{"1"}, Dot1x: ptr(true)}},
		"no ports":             {FamilyAOSS, PortAccessChange{Dot1x: ptr(true)}},
		"bad port":             {FamilyAOSS, PortAccessChange{Ports: []string{"1;reload"}, Dot1x: ptr(true)}},
		"options on open port": {FamilyAOSS, PortAccessChange{Ports: []string{"2"}, ClientLimit: ptr(2)}},
		"cx unauth vlan":       {FamilyAOSCX, PortAccessChange{Ports: []string{"1"}, UnauthVLAN: ptr(99)}},
		"limit too high":       {FamilyAOSS, PortAccessChange{Ports: []string{"1"}, ClientLimit: ptr(33)}},
		"short reauth":         {FamilyAOSS, PortAccessChange{Ports: []string{"1"}, ReauthPeriod: ptr(10)}},
		"key with space":       {FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "10.0.0.7", Key: "a b"}}},
		"key with quote":       {FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "10.0.0.7", Key: `a"b`}}},
		"duplicate server":     {FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "10.0.0.5", Key: "x"}}},
		"vrf on aos-s":         {FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "10.0.0.7", Key: "x", VRF: "mgmt"}}},
		"unknown server":       {FamilyAOSS, PortAccessChange{RemoveServer: "10.0.0.9"}},
	}
	for _, h := range []string{"", "9898.8989898.89898.7879847", "10.0.0.256", "10.0.0", "0.0.0.0", "255.255.255.255", "224.0.0.5", "127.0.0.1",
		"fe80::1%eth0", "2001:db8:::1", "-bad.example.com", "bad_name.example.com", "a..b", "radius server", "radius;reload"} {
		bad["cx host "+h] = struct {
			f Family
			c PortAccessChange
		}{FamilyAOSCX, PortAccessChange{AddServer: &RadiusServerChange{Host: h, Key: "x"}}}
	}
	bad["host name on aos-s"] = struct {
		f Family
		c PortAccessChange
	}{FamilyAOSS, PortAccessChange{AddServer: &RadiusServerChange{Host: "radius.example.com", Key: "x"}}}
	for _, h := range []string{"10.0.0.7", "2001:db8::7", "radius", "radius.example.com", "nps-01.corp.local."} {
		if _, err := BuildPortAccessCommands(FamilyAOSCX, PortAccessChange{AddServer: &RadiusServerChange{Host: h, Key: "x"}}, cur); err != nil {
			t.Errorf("host %q: %v", h, err)
		}
	}
	for name, tc := range bad {
		if _, err := BuildPortAccessCommands(tc.f, tc.c, cur); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAfterPortAccess(t *testing.T) {
	cur := PortAccess{Servers: []RadiusServer{{Host: "10.0.0.5", HasKey: true}}, MacAuthOn: true, Ports: []PortAuth{{Port: "1", Dot1x: true}, {Port: "2", MacAuth: true}}}
	got := AfterPortAccess(cur, PortAccessChange{RemoveServer: "10.0.0.5", AddServer: &RadiusServerChange{Host: "10.0.0.6", Key: "x"}, Ports: []string{"1", "3"}, Dot1x: ptr(false), MacAuth: ptr(true)})
	if !reflect.DeepEqual(got.Servers, []RadiusServer{{Host: "10.0.0.6", HasKey: true}}) {
		t.Fatalf("servers: %+v", got.Servers)
	}
	want := []PortAuth{{Port: "1", MacAuth: true}, {Port: "2", MacAuth: true}, {Port: "3", MacAuth: true}}
	if !reflect.DeepEqual(got.Ports, want) {
		t.Fatalf("ports: %+v", got.Ports)
	}
	if len(cur.Ports) != 2 || !cur.Ports[0].Dot1x {
		t.Fatalf("the current setup must not be changed: %+v", cur)
	}
}
