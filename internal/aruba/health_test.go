package aruba

import (
	"reflect"
	"strings"
	"testing"
)

func TestSSHAlgorithms(t *testing.T) {
	aoss := ` SSH Enabled     : Yes                 Secure Copy Enabled : No
 TCP Port Number : 22                  Timeout (sec)       : 120
 Host Key Type   : RSA                 Host Key Size       : 2048
 Ciphers : aes256-ctr,aes256-cbc,aes192-ctr,3des-cbc
 MACs    : hmac-sha2-256,hmac-sha1,hmac-sha1-96
`
	c, m, k := ParseSSHAlgorithms(aoss)
	if len(c) != 4 || len(m) != 3 || k != nil {
		t.Fatalf("got %v %v %v", c, m, k)
	}
	weak := WeakSSH(Security{SSHCiphers: c, SSHMACs: m})
	if want := []string{"aes256-cbc", "3des-cbc", "hmac-sha1", "hmac-sha1-96"}; !reflect.DeepEqual(weak, want) {
		t.Fatalf("weak = %v", weak)
	}
	cx := `SSH server configuration on VRF default :
 IP Version       : IPv4 and IPv6      SSH Version  : 2.0
 TCP Port         : 22                 Grace Timeout (sec) : 120
 Ciphers         : chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,aes256-ctr
 MACs            : hmac-sha2-256-etm@openssh.com,hmac-sha2-512
 Host Key Algorithms : ssh-ed25519,rsa-sha2-512
 Key Exchange Algorithms : curve25519-sha256,diffie-hellman-group14-sha1,diffie-hellman-group-exchange-sha256
`
	c, m, k = ParseSSHAlgorithms(cx)
	if len(c) != 3 || len(m) != 2 || len(k) != 3 {
		t.Fatalf("got %v %v %v", c, m, k)
	}
	if weak := WeakSSH(Security{SSHCiphers: c, SSHMACs: m, SSHKex: k}); !reflect.DeepEqual(weak, []string{"diffie-hellman-group14-sha1"}) {
		t.Fatalf("weak = %v", weak)
	}
}

func TestLogEvents(t *testing.T) {
	logs := []LogLine{
		{Severity: "W", Text: "W 01/02/24 10:00:00 00000 auth: Invalid user name/password on SSH session User 'manager' is trying to login from 10.0.0.5"},
		{Severity: "W", Text: "W 01/02/24 10:00:05 00000 auth: Invalid user name/password on SSH session"},
		{Severity: "I", Text: "I 01/02/24 10:01:00 00076 ports: port 5 is now off-line"},
		{Severity: "I", Text: "I 01/02/24 10:01:02 00076 ports: port 5 is now on-line"},
		{Severity: "I", Text: "I 01/02/24 10:01:10 00076 ports: port 5 is now off-line"},
		{Severity: "I", Text: "I 01/02/24 10:01:12 00077 ports: port 1/A1 is now off-line"},
		{Severity: "I", Text: "2024-01-02T10:02:00.000Z sw1 intfd[123]: Event|419|LOG_INFO|AMM|1/1|Interface 1/1/7 link state changed to down"},
		{Severity: "W", Text: "2024-01-02T10:02:00.000Z sw1 aaa-utils[1]: Event|3301|LOG_WARN|AMM|1/1|User 'admin' authentication failed from 10.0.0.9"},
	}
	if n := FailedLogins(logs); n != 3 {
		t.Fatalf("failed logins = %d", n)
	}
	f := PortFlaps(logs)
	if f["5"] != 2 || f["1/A1"] != 1 || f["1/1/7"] != 1 || len(f) != 3 {
		t.Fatalf("flaps = %v", f)
	}
}

func TestConfigUsers(t *testing.T) {
	aoss := `hostname "sw"
password manager user-name "admin" sha1 "abc"
password operator user-name "viewer" sha1 "def"
aaa authentication local-user "bob" group "ops" password sha1 "x"
`
	got := ConfigUsers(FamilyAOSS, aoss)
	if want := []string{"admin (manager)", "bob (ops)", "viewer (operator)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	cx := `user admin group administrators password ciphertext AQBa
user noc group operators password ciphertext AQBb
`
	got = ConfigUsers(FamilyAOSCX, cx)
	if want := []string{"admin (administrators)", "noc (operators)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	added, removed := DiffUsers([]string{"admin (administrators)"}, got)
	if !reflect.DeepEqual(added, []string{"noc (operators)"}) || removed != nil {
		t.Fatalf("diff = %v %v", added, removed)
	}
}

func TestMACTable(t *testing.T) {
	aoss := ` Status and Counters - Port Address Table

  MAC Address       Port         VLAN
  ----------------- ------------ ----
  123456-789abc     1            10
  123456-789abd     Trk1         10
  123456-789abe     1/A1         20
`
	got := ParseMACTable(aoss)
	if len(got) != 3 || got[0].MAC != "12:34:56:78:9a:bc" || got[0].Port != "1" || got[0].VLAN != "10" || got[1].Port != "Trk1" || got[2].Port != "1/A1" || got[2].VLAN != "20" {
		t.Fatalf("got %+v", got)
	}
	cx := `MAC age-time            : 300 seconds
Number of MAC addresses : 2

MAC Address          VLAN     Type                      Port
--------------------------------------------------------------
12:34:56:78:9a:bc    10       dynamic                   1/1/5
12:34:56:78:9a:bd    20       dynamic                   lag1
`
	got = ParseMACTable(cx)
	if len(got) != 2 || got[0].Port != "1/1/5" || got[0].VLAN != "10" || got[1].Port != "lag1" || got[1].VLAN != "20" {
		t.Fatalf("got %+v", got)
	}
	if ParseMACTable("Invalid input: show mac-address") != nil {
		t.Fatal("error output should give nil")
	}
}

func TestStack(t *testing.T) {
	aoss := ` Stack ID         : 00011234-56789abc
 MAC Address      : 123456-789abc
 Stack Topology   : Chain
 Stack Status     : Active
 Uptime           : 10d 2h 3m
 Software Version : WC.16.11.0015

 Mbr
 ID  Mac Address       Model                                   Pri Status
 --- ----------------- --------------------------------------- --- ---------------
 1   123456-789abc     Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch 128 Commander
 2   123456-789abd     Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch 128 Standby
 3   123456-789abe     Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch 128 Missing
`
	st := ParseStack(aoss)
	if st == nil || st.Topology != "Chain" || len(st.Members) != 3 {
		t.Fatalf("got %+v", st)
	}
	if st.Members[0].Status != "Commander" || !st.Members[0].OK || st.Members[2].Status != "Missing" || st.Members[2].OK || st.Members[1].Model != "Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch" {
		t.Fatalf("members %+v", st.Members)
	}
	cx := `MAC Address   : 12:34:56:78:9a:bc
Secondary     : 2
Topology      : Ring
Status        : Active
Split Detection Method : None

Mbr ID  Mac Address        Type           Status
------  -----------------  -------------  ---------------
1       12:34:56:78:9a:bc  JL668A         Master
2       12:34:56:78:9a:bd  JL668A         Standby
`
	st = ParseStack(cx)
	if st == nil || st.Topology != "Ring" || len(st.Members) != 2 || st.Members[1].Status != "Standby" || st.Members[0].Model != "JL668A" {
		t.Fatalf("got %+v", st)
	}
	// issue #22: a standalone 6100 on 10.16 lists itself as the conductor
	alone := `MAC Address   : 12:34:56:78:9a:bc
Secondary     :
Topology      : Standalone
Status        : No Split
Split Detection Method : None

Mbr ID  Mac Address        Type           Status
------  -----------------  -------------  ---------------
1       12:34:56:78:9a:bc  JL677A         Conductor
`
	if st := ParseStack(alone); st != nil {
		t.Fatalf("standalone switch read as a stack: %+v", st)
	}
	two := strings.Replace(alone, "1       12:34:56:78:9a:bc  JL677A         Conductor\n", "1       12:34:56:78:9a:bc  JL677A         Conductor\n2       12:34:56:78:9a:bd  JL677A         Standby\n", 1)
	if st := ParseStack(two); st == nil || len(st.Members) != 2 || !st.Members[0].OK || !st.Members[1].OK {
		t.Fatalf("conductor/standby stack: %+v", st)
	}
	// a snapshot saved by 1.0.4: one conductor marked as failed
	old := &Stack{Topology: "Standalone", Members: []StackMember{{ID: "1", Status: "Conductor", OK: false}}}
	if old.Normalize() != nil {
		t.Fatal("saved single-member stack should normalize to nil")
	}
	pair := &Stack{Members: []StackMember{{ID: "1", Status: "Conductor"}, {ID: "2", Status: "Missing", OK: true}}}
	if n := pair.Normalize(); n == nil || !n.Members[0].OK || n.Members[1].OK {
		t.Fatalf("normalize = %+v", n)
	}
	if ParseStack("Stacking is disabled.") != nil || ParseStack("") != nil {
		t.Fatal("standalone should give nil")
	}
}

func TestUptimeSeconds(t *testing.T) {
	for in, want := range map[string]int64{
		"23 days":                              23 * 86400,
		"9 weeks, 1 day, 19 hours, 53 minutes": 64*86400 + 19*3600 + 53*60,
		"1 year, 2 days":                       367 * 86400,
		"13 mins":                              13 * 60,
		"1 day 02:03:04":                       86400 + 2*3600 + 3*60 + 4,
		"12:34:56":                             12*3600 + 34*60 + 56,
		"":                                     0,
		"unknown":                              0,
	} {
		if got := UptimeSeconds(in); got != want {
			t.Errorf("%q = %d, want %d", in, got, want)
		}
	}
}

func TestStackModel(t *testing.T) {
	st := &Stack{Members: []StackMember{{ID: "2", Status: "Standby", Model: "Aruba JL075A 3810M-16SFP+-2-slot Switch"}, {ID: "1", Status: "Commander", Model: "Aruba JL075A 3810M-16SFP+-2-slot Switch"}}}
	if st.Model() != "Aruba JL075A 3810M-16SFP+-2-slot Switch" {
		t.Fatalf("model = %q", st.Model())
	}
	vsf := ` VSF Domain ID      : 1
 MAC Address        : 123456-789abc
 VSF Topology       : Chain
 VSF Status         : Active
 Uptime             : 0d 2h 13m

 Mbr
 ID  Mac Address       Model                                      Pri Status
 --- ----------------- ------------------------------------------ --- ---------------
 1   123456-789abc     Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch   255 Commander
 2   123456-789abd     Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch   128 Standby
`
	s := ParseStack(vsf)
	if s == nil || s.Topology != "Chain" || len(s.Members) != 2 || s.Model() != "Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch" {
		t.Fatalf("vsf = %+v", s)
	}
}

// issue #27: a 3810M stack, as the switch prints it, with the model cut short
func TestStack3810M(t *testing.T) {
	out := `Stack ID         : 0005445b-ed9e9e00
MAC Address      : 445bed-9e9e65
Stack Topology   : Ring
Stack Status     : Active
Split Policy     : One-Fragment-Up
Uptime           : 157d 18h 0m
Software Version : KB.16.11.0029

 Mbr
 ID  Mac Address       Model                                 Pri Status
 --- ----------------- ------------------------------------- --- ---------------
  1  445bed-9e9e00     Aruba JL075A 3810M-16SFP+-2-slot S... 255 Commander
  2  9020c2-e26b00     Aruba JL075A 3810M-16SFP+-2-slot S... 128 Standby
`
	st := ParseStack(out)
	if st == nil || st.Topology != "Ring" || len(st.Members) != 2 {
		t.Fatalf("stack = %+v", st)
	}
	if m := st.Model(); m != "Aruba JL075A 3810M-16SFP+-2-slot" {
		t.Fatalf("model = %q", m)
	}
	if pn := PartNumbers(st.Model()); len(pn) != 1 || pn[0] != "JL075A" {
		t.Fatalf("part numbers = %v", pn)
	}
	if !st.Members[0].OK || !st.Members[1].OK || st.Members[1].Status != "Standby" {
		t.Fatalf("members = %+v", st.Members)
	}
}

