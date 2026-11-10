package aruba

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

const aossSystem = `
 Status and Counters - General System Information

  System Name        : SW-ACCESS-01
  System Contact     : noc
  System Location    : Rack A2

  MAC Age Time (sec) : 300

  Time Zone          : 240
  Daylight Time Rule : None

  Software revision  : WC.16.10.0012        Base MAC Addr      : 001122-334455
  ROM Version        : WC.16.01.0008        Serial Number      : SG12345678

  Up Time            : 23 days              Memory   - Total   : 340,525,056
  CPU Util (%)       : 7                               Free    : 170,262,528

  IP Mgmt  - Pkts Rx : 1,234,567            Packet   - Total   : 6750
             Pkts Tx : 1,000,000            Buffers    Free    : 5100
`

const cxSystem = `Hostname                        : core-cx
Description                     : FL.10.13.1000
Contact                         :
Location                        : DC1
Vendor                          : Aruba
Product Name                    : JL658A 6300M 24SFP+ 4SFP56 Swch
Chassis Serial Nbr              : SG0KKX1234
Base MAC Address                : 00:11:22:aa:bb:cc
ArubaOS-CX Version              : FL.10.13.1000
Time Zone                       : UTC
Up Time                         : 3 weeks, 2 days, 4 hours, 1 minute
CPU Util (%)                    : 4
Memory Usage (%)                : 21
`

func TestParseSystem(t *testing.T) {
	f := ParseSystem(aossSystem)
	if f.Hostname != "SW-ACCESS-01" || f.Software != "WC.16.10.0012" || f.Serial != "SG12345678" || f.CPU != 7 || f.Uptime != "23 days" {
		t.Fatalf("aos-s facts: %+v", f)
	}
	if f.Memory != 50 {
		t.Fatalf("aos-s memory = %d, want 50", f.Memory)
	}
	if NormMAC(f.BaseMAC) != "00:11:22:33:44:55" {
		t.Fatalf("mac %q", f.BaseMAC)
	}
	c := ParseSystem(cxSystem)
	if c.Hostname != "core-cx" || c.Model != "JL658A 6300M 24SFP+ 4SFP56 Swch" || c.CPU != 4 || c.Memory != 21 || c.Software != "FL.10.13.1000" {
		t.Fatalf("cx facts: %+v", c)
	}
}

func TestDetectFamily(t *testing.T) {
	if f := DetectFamily("Image stamp:    /ws/swbuildm/rel_xxx\n WC.16.10.0012", ""); f != FamilyAOSS {
		t.Fatal(f)
	}
	if f := DetectFamily("-----\nArubaOS-CX\n(c) Copyright\nVersion      : FL.10.13.1000", ""); f != FamilyAOSCX {
		t.Fatal(f)
	}
	if f := DetectFamily("Aruba Instant On 1960 24G 2XGT 2SFP+ Switch\nVersion 2.8.0", ""); f != FamilyInstantOn {
		t.Fatal(f)
	}
}

const aossSTP = `
 Multiple Spanning Trees (MST) Information

  STP Enabled   : Yes
  Force Version : MSTP-operation
  IST Mapped VLANs : 1-4094
  Switch MAC Address : 001122-334455
  Switch Priority    : 32768
  Max Age  : 20
  Max Hops : 20
  Forward Delay : 15

  Topology Change Count  : 12
  Time Since Last Change : 2 hours

  CST Root MAC Address : 0011ff-aabbcc
  CST Root Priority    : 4096
  CST Root Path Cost   : 20000
  CST Root Port        : 49

  Port  Type      Cost      Priority Role       State      Designated Bridge
  ----- --------- --------- -------- ---------- ---------- ------------------
  1     100/1000T Auto      128      Disabled   Disabled
  2     100/1000T 20000     128      Designated Forwarding 001122-334455
  49    SFP+SR    2000      128      Root       Forwarding 0011ff-aabbcc
  50    SFP+SR    2000      128      Alternate  Blocking   0011ff-112233
`

const cxSTP = `Spanning tree status    : Enabled Protocol: MSTP
MST0
  Root ID    Priority   : 4096
             MAC-Address: 00:11:22:aa:bb:cc
             This bridge is the root
             Hello time(in seconds):2  Max Age(in seconds):20
  Bridge ID  Priority  : 4096
             MAC-Address: 00:11:22:aa:bb:cc

Port         Role           State      Cost    Priority   Type
------------ -------------- ---------- ------- ---------- ----------
1/1/49       Designated     Forwarding 2000    128        P2P
1/1/50       Designated     Forwarding 2000    128        P2P

Number of topology changes    : 4
Last topology change occurred : 1234 seconds ago
`

func TestParseSTP(t *testing.T) {
	s := ParseSTP(aossSTP)
	if !s.Enabled || s.Mode != "MSTP-operation" || s.TopologyChanges != 12 || s.RootPort != "49" || s.IsRoot || s.RootPriority != "4096" || s.BridgePriority != "32768" {
		t.Fatalf("aos-s stp: %+v", s)
	}
	if len(s.Ports) != 4 || s.Ports[3].State != "Blocking" || s.Ports[2].Role != "Root" {
		t.Fatalf("aos-s stp ports: %+v", s.Ports)
	}
	c := ParseSTP(cxSTP)
	if !c.Enabled || !c.IsRoot || c.TopologyChanges != 4 || c.Mode != "MSTP" || c.RootPriority != "4096" || len(c.Ports) != 2 {
		t.Fatalf("cx stp: %+v", c)
	}
	d := ParseSTP("  STP Enabled   : No\n")
	if d.Enabled || !d.Known {
		t.Fatalf("disabled stp: %+v", d)
	}
}

const aossIf = `
  Status and Counters - Port Status

                         | Intrusion                           MDI  Flow  Bcast
  Port         Type      | Alert     Enabled Status Mode       Mode Ctrl  Limit
  ------------ --------- + --------- ------- ------ ---------- ---- ---- -----
  1            100/1000T | No        Yes     Up     1000FDx    MDIX off  0
  2            100/1000T | No        Yes     Down   1000FDx    Auto off  0
  3            100/1000T | No        No      Down   1000FDx    Auto off  0
  49           SFP+SR    | No        Yes     Up     10GigFD    NA   off  0
`

const cxIf = `--------------------------------------------------------------------------------------------------------
Port      Native  Mode   Type           Enabled Status  Reason                  Speed   Description
          VLAN                                                                  (Mb/s)
--------------------------------------------------------------------------------------------------------
1/1/1     1       access 1GbT           yes     up                              1000    Printer
1/1/2     20      access 1GbT           yes     down    Waiting for link        --      --
1/1/49    1       trunk  SFP+DA1        yes     up                              10000   uplink to core
`

func TestParseInterfaces(t *testing.T) {
	p := ParseInterfacesBrief(aossIf)
	if len(p) != 4 || !p[0].Up || p[1].Up || p[2].Enabled || p[3].Speed != "10GigFD" || p[0].Type != "100/1000T" {
		t.Fatalf("aos-s ports: %+v", p)
	}
	c := ParseInterfacesBrief(cxIf)
	if len(c) != 3 || c[0].Description != "Printer" || c[1].VLAN != "20" || c[2].Mode != "trunk" || c[2].Speed != "10000" || c[2].Description != "uplink to core" {
		t.Fatalf("cx ports: %+v", c)
	}
}

const aossLLDP = `
 LLDP Remote Device Information Detail

  Local Port   : 49
  ChassisType  : mac-address
  ChassisId    : 00 11 22 aa bb cc
  PortType     : interface-name
  PortId       : 1/1/1
  SysName      : core-cx
  System Descr : Aruba JL658A 6300M FL.10.13.1000
  PortDescr    : 1/1/1
  Pvid         :

  System Capabilities Supported  : bridge, router
  System Capabilities Enabled    : bridge, router

  Remote Management Address
     Type    : ipv4
     Address : 10.0.0.1

------------------------------------------------------------------------------
  Local Port   : 5
  ChassisType  : network-address
  ChassisId    : 10.0.5.20
  PortType     : mac-address
  PortId       : 00 aa bb cc dd ee
  SysName      : SEP00AABBCCDDEE
  System Descr : Cisco IP Phone
  PortDescr    : SW PORT
`

const cxLLDP = `Port                           : 1/1/1
Neighbor Entries               : 1
Neighbor Entries Deleted       : 0
Neighbor Chassis-Name          : SW-ACCESS-01
Neighbor Chassis-Description   : Aruba JL256A 2930F-48G-PoE+-4SFP+ Switch, revision WC.16.10.0012
Neighbor Chassis-ID            : 00:11:22:33:44:55
Neighbor Management-Address    : 10.0.0.11
Chassis Capabilities Available : Bridge, Router
Chassis Capabilities Enabled   : Bridge
Neighbor Port-ID               : 49
Neighbor Port-Desc             : 49
TTL                            : 120

Port                           : 1/1/2
Neighbor Entries               : 1
Neighbor Chassis-Name          : SW-ACCESS-02
Neighbor Chassis-ID            : 00:11:22:33:44:66
Neighbor Management-Address    : 10.0.0.12
Neighbor Port-ID               : 50
`

const aossCDP = `
 CDP neighbors information for port 7

  Port : 7
  Device ID : ap-lobby
  Address Type : IP
  Address : 10.0.9.4
  Platform : cisco AIR-AP2802I
  Capability : Router Switch
  Device Port : GigabitEthernet0
  Version : 8.10
`

func TestParseNeighbors(t *testing.T) {
	n := ParseNeighbors(aossLLDP, "lldp")
	if len(n) != 2 || n[0].LocalPort != "49" || n[0].SysName != "core-cx" || n[0].MgmtIP != "10.0.0.1" || n[0].PortID != "1/1/1" || n[1].LocalPort != "5" {
		t.Fatalf("aos-s lldp: %+v", n)
	}
	c := ParseNeighbors(cxLLDP, "lldp")
	if len(c) != 2 || c[0].SysName != "SW-ACCESS-01" || c[0].MgmtIP != "10.0.0.11" || c[0].PortID != "49" || c[1].LocalPort != "1/1/2" {
		t.Fatalf("cx lldp: %+v", c)
	}
	d := ParseNeighbors(aossCDP, "cdp")
	if len(d) != 1 || d[0].SysName != "ap-lobby" || d[0].MgmtIP != "10.0.9.4" || d[0].PortID != "GigabitEthernet0" {
		t.Fatalf("cdp: %+v", d)
	}
}

const aossCfg = `; JL256A Configuration Editor; Created on release #WC.16.10.0012
; Ver #14:01.4f.f8.1d.9b.3f.bf.bb.ef.7c.59.fc.6b.fb.9f.fc.ff.ff.37.ef:01
hostname "SW-ACCESS-01"
module 1 type jl256a
snmp-server community "public" unrestricted
sntp server priority 1 10.0.0.10
timesync sntp
logging 10.0.0.20
web-management ssl
spanning-tree
vlan 1
   name "DEFAULT_VLAN"
   untagged 1-4,6-48,50-52
   ip address dhcp-bootp
   exit
vlan 20
   name "Servers"
   tagged 49
   untagged 5
   ip address 10.20.0.11 255.255.255.0
   exit
password manager
`

const cxCfg = `!Version ArubaOS-CX FL.10.13.1000
hostname core-cx
user admin group administrators password ciphertext AQBapXX
ntp server 10.0.0.10 iburst
ntp enable
logging 10.0.0.20
ssh server vrf mgmt
https-server vrf mgmt
vlan 1,20,30
vlan 20
    name Servers
vlan 30
    name Voice
spanning-tree
interface 1/1/1
    no shutdown
    no routing
    vlan trunk native 1
    vlan trunk allowed 1,20,30
interface 1/1/5
    no shutdown
    no routing
    vlan access 20
interface 1/1/48
    no shutdown
    ip address 192.0.2.1/30
interface vlan 20
    ip address 10.20.0.1/24
`

func TestParseConfig(t *testing.T) {
	v, sec, stp := ParseConfigAOSS(aossCfg)
	if !stp || !sec.TelnetEnabled || !sec.SNMPDefault || !sec.HTTPSEnabled || len(sec.NTPServers) != 1 || sec.NTPServers[0] != "10.0.0.10" || !sec.ManagerPassword {
		t.Fatalf("aos-s sec: %+v stp=%v", sec, stp)
	}
	if len(v) != 2 || v[1].Name != "Servers" || !reflect.DeepEqual(v[1].Tagged, []string{"49"}) || len(v[0].Untagged) != 50 {
		t.Fatalf("aos-s vlans: %+v", v)
	}
	// Issue #11: J9774A/J9775A name the server with "ntp server-name".
	_, nsec, _ := ParseConfigAOSS("timesync ntp\nntp unicast\nntp server-name \"time.windows.com\" iburst\nntp server 10.0.0.11 iburst\nntp enable\n")
	if !reflect.DeepEqual(nsec.NTPServers, []string{"time.windows.com", "10.0.0.11"}) {
		t.Fatalf("aos-s ntp: %v", nsec.NTPServers)
	}
	cv, csec, cstp := ParseConfigCX(cxCfg)
	if !cstp || csec.TelnetEnabled || !csec.HTTPSEnabled || len(csec.LogServers) != 1 {
		t.Fatalf("cx sec: %+v", csec)
	}
	if len(cv) != 3 {
		t.Fatalf("cx vlans: %+v", cv)
	}
	var v20 VLAN
	for _, x := range cv {
		if x.ID == 20 {
			v20 = x
		}
	}
	if v20.Name != "Servers" || v20.IP != "10.20.0.1/24" || !reflect.DeepEqual(v20.Tagged, []string{"1/1/1"}) || !reflect.DeepEqual(v20.Untagged, []string{"1/1/5"}) {
		t.Fatalf("cx vlan20: %+v", v20)
	}
}

func TestVLANTable(t *testing.T) {
	aoss := `
  VLAN ID Name                             | Status     Voice Jumbo
  ------- -------------------------------- + ---------- ----- -----
  1       DEFAULT_VLAN                     | Port-based No    No
  20      Servers                          | Port-based No    No
`
	v := ParseVLANTable(aoss)
	if len(v) != 2 || v[1].Name != "Servers" || v[1].Status != "Port-based" {
		t.Fatalf("%+v", v)
	}
	cx := `----------------------------------------------------------------------------------------
VLAN  Name                              Status  Reason                 Type      Interfaces
----------------------------------------------------------------------------------------
1     DEFAULT_VLAN_1                    up      ok                     default   1/1/1-1/1/4
20    Servers                           up      ok                     static    1/1/1,1/1/5
`
	c := ParseVLANTable(cx)
	if len(c) != 2 || c[0].Name != "DEFAULT_VLAN_1" || c[1].Status != "up" {
		t.Fatalf("%+v", c)
	}
}

func TestEnv(t *testing.T) {
	fans := `
 Fan Information
  Num  | State       | Failures
-------+-------------+----------
Sys-1  | Fan OK      | 0
Sys-2  | Fan Failed  | 1

1 / 2 Fans in Failure State
1 / 2 Fans have been in Failure State
`
	items := ParseEnv("fan", fans)
	bad := 0
	for _, i := range items {
		if !i.OK {
			bad++
		}
	}
	if bad != 2 {
		t.Fatalf("fans: %+v", items)
	}
	psu := `
 Power Supply Status:

  PS#   Model     Serial      State           AC/DC  + V        Wattage   Max
  ----- --------- ----------- --------------- ----------------- --------- ------
  1     JL085A    CN12345678  Powered         AC 120V/240V      22        680
  2                           Not Present     -- ---------      0         0
`
	p := ParseEnv("psu", psu)
	if len(p) != 1 || !p[0].OK {
		t.Fatalf("psu: %+v", p)
	}
}

func TestPorts(t *testing.T) {
	if got := ExpandPorts("1-3,A1-A2,49"); !reflect.DeepEqual(got, []string{"1", "2", "3", "A1", "A2", "49"}) {
		t.Fatal(got)
	}
	if got := ExpandPorts("1/1/1-1/1/3"); !reflect.DeepEqual(got, []string{"1/1/1", "1/1/2", "1/1/3"}) {
		t.Fatal(got)
	}
	if got := CompressPorts([]string{"3", "1", "2", "5", "A1", "A2"}); got != "1-3,5,A1-A2" {
		t.Fatal(got)
	}
}

func TestVLANCommands(t *testing.T) {
	cur := []VLAN{{ID: 1}, {ID: 20, Tagged: []string{"49"}, Untagged: []string{"5"}}}
	cmds, err := BuildVLANCommands(FamilyAOSS, VLANChange{Action: "create", ID: 30, Name: "Voice", AddTagged: []string{"49", "50"}, AddUntagged: []string{"7"}, Save: true}, cur)
	if err != nil {
		t.Fatal(err)
	}
	want := `configure terminal|vlan 30|name "Voice"|untagged 7|tagged 49-50|exit|end|write memory`
	if strings.Join(cmds, "|") != want {
		t.Fatalf("got %s", strings.Join(cmds, "|"))
	}
	cmds, _ = BuildVLANCommands(FamilyAOSS, VLANChange{Action: "update", ID: 20, RemovePorts: []string{"49", "5"}}, cur)
	if strings.Join(cmds, "|") != "configure terminal|vlan 20|no tagged 49|no untagged 5|exit|end" {
		t.Fatalf("got %v", cmds)
	}
	if _, err := BuildVLANCommands(FamilyAOSS, VLANChange{Action: "delete", ID: 1}, cur); err == nil {
		t.Fatal("deleting vlan 1 must fail")
	}
	if _, err := BuildVLANCommands(FamilyAOSS, VLANChange{Action: "create", ID: 40, Name: `x"; reload`}, cur); err == nil {
		t.Fatal("unsafe name accepted")
	}
	cx, _ := BuildVLANCommands(FamilyAOSCX, VLANChange{Action: "create", ID: 30, AddTagged: []string{"1/1/49"}}, cur)
	if strings.Join(cx, "|") != "configure terminal|vlan 30|exit|interface 1/1/49|no routing|vlan trunk allowed 30|exit|end" {
		t.Fatalf("cx: %v", cx)
	}
}

func TestReadOnly(t *testing.T) {
	for _, ok := range []string{"show vlans", "show run | include snmp", "ping 10.0.0.1"} {
		if !ReadOnlyAllowed(ok) {
			t.Fatal(ok)
		}
	}
	for _, bad := range []string{"conf t", "reload", "show vlans\nreload", "erase startup-config"} {
		if ReadOnlyAllowed(bad) {
			t.Fatal(bad)
		}
	}
}

func TestParseBanks(t *testing.T) {
	flash := `
Image             Size (bytes) Date     Version
----------------- ------------ -------- --------------
Primary Image   :     33204214 10/18/23 WC.16.11.0015
Secondary Image :     32952136 03/10/22 WC.16.10.0012

Boot ROM Version
----------------
Primary Boot ROM Version   : WC.17.02.0006
Secondary Boot ROM Version : WC.17.02.0006

Default Boot Image   : Primary
Default Boot ROM     : Primary
`
	ver := "Image stamp:    /ws/swbuildm/rel_xxx\n                 WC.16.11.0015\n Boot Image:     Primary\n"
	b := ParseBanks(FamilyAOSS, flash, ver)
	if !b.Known || b.Primary != "WC.16.11.0015" || b.Secondary != "WC.16.10.0012" || b.Running != "primary" || b.Default != "primary" || b.Spare() != "secondary" {
		t.Fatalf("aos-s banks: %+v", b)
	}
	images := `---------------------------------------------------------------------------
ArubaOS-CX Primary Image
---------------------------------------------------------------------------
Version : FL.10.13.1000
Size    : 643 MB
Date    : 2024-01-10 08:01:31 PST
SHA-256 : 9f2a

---------------------------------------------------------------------------
ArubaOS-CX Secondary Image
---------------------------------------------------------------------------
Version : FL.10.14.0001
Size    : 650 MB

Default Image : secondary
`
	c := ParseBanks(FamilyAOSCX, images, "Version      : FL.10.13.1000\nActive Image : primary\n")
	if c.Primary != "FL.10.13.1000" || c.Secondary != "FL.10.14.0001" || c.Running != "primary" || c.Default != "secondary" || c.Spare() != "secondary" {
		t.Fatalf("cx banks: %+v", c)
	}
}

func TestConfigStatusSaved(t *testing.T) {
	cases := []struct {
		out       string
		saved, ok bool
	}{
		{"Running configuration is the same as the startup configuration.", true, true},
		{"Running configuration is same as the startup configuration.", true, true},
		{"Running configuration has not changed since last save.", true, true},
		{"The running configuration has not been changed since the last save.", true, true},
		{"Running configuration has been changed and needs to be saved.", false, true},
		{"Running configuration differs from the startup configuration.", false, true},
		{"Invalid input: config", false, false},
		{"something else entirely", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		if saved, ok := ConfigStatusSaved(c.out); saved != c.saved || ok != c.ok {
			t.Errorf("%q: got saved=%v ok=%v", c.out, saved, ok)
		}
	}
}

// A 2530-24G (J9776A) reads like any other AOS-S switch: YA firmware, no
// model in "show system", the model comes from the config header, and the
// fanless chassis rejects "show system fans".
func Test2530(t *testing.T) {
	ver := `Image stamp:    /ws/swbuildm/rel_yakima_qaoff/code/build/lakes(swbuildm_rel_yakima_qaoff_rel_yakima)
                Nov 30 2022 15:11:08
                YA.16.11.0010
                1290
Boot Image:     Primary

Boot ROM Version:    YA.15.20
Active Boot ROM:     Primary`
	if f := DetectFamily(ver, ""); f != FamilyAOSS {
		t.Fatalf("family: %s", f)
	}
	sys := `
 Status and Counters - General System Information

  System Name        : SW-MGMT-01
  System Contact     :
  System Location    : Server room

  MAC Age Time (sec) : 300

  Software revision  : YA.16.11.0010        Base MAC Addr      : 941882-1a2b3c
  ROM Version        : YA.15.20             Serial Number      : CN61FP1234

  Up Time            : 125 days             Memory   - Total   : 46,620,672
  CPU Util (%)       : 3                               Free    : 26,894,416
`
	s := Snapshot{Family: FamilyAOSS, Raw: map[string]string{
		"version": ver, "system": sys,
		"fans":   "                   ^\nInvalid input: fans",
		"config": "; J9776A Configuration Editor; Created on release #YA.16.11.0010\n; Ver #14:01.4f.f8.1d.9b.3f.bf.bb.ef.7c.59.fc.6b.fb.9f.fc.ff.ff.37.ef:01\nhostname \"SW-MGMT-01\"\n",
	}}
	parseInto(&s)
	f := s.Facts
	if f.Hostname != "SW-MGMT-01" || f.Model != "J9776A" || f.Software != "YA.16.11.0010" || f.Serial != "CN61FP1234" || f.CPU != 3 || f.Memory < 0 {
		t.Fatalf("facts: %+v", f)
	}
	if len(s.Env) != 0 {
		t.Fatalf("fanless switch got env items: %+v", s.Env)
	}
}

// Values placed into the firmware copy command are single safe tokens; an
// image name with a newline can't add another CLI command.
func TestCopyImageRejectsUnsafeValues(t *testing.T) {
	for _, file := range []string{"WC_16_11_0021\nboot set-default flash secondary\n.swi", "a b.swi", "x;reload", "x'y"} {
		if _, err := CopyImage(context.Background(), nil, FamilyAOSS, "10.0.0.5", file, "secondary", ""); err == nil {
			t.Errorf("accepted %q", file)
		}
	}
	if _, err := CopyImage(context.Background(), nil, FamilyAOSCX, "10.0.0.5", "x.swi", "primary", "mgmt\nreload"); err == nil {
		t.Error("accepted a vrf with a newline")
	}
}

func TestBootROM(t *testing.T) {
	if f := ParseSystem("  ROM Version        : WC.16.01.0008        Serial Number      : SG12345678\n"); f.BootROM != "WC.16.01.0008" {
		t.Fatalf("AOS-S boot ROM = %q", f.BootROM)
	}
	cx := "Vendor                   : Aruba\nProduct Name             : JL668A 6300F 24G\nServiceOS Version        : FL.01.14.0002\nBIOS Version             : FL.01.0001\n"
	if f := ParseSystem(cx); f.BootROM != "FL.01.14.0002" {
		t.Fatalf("AOS-CX ServiceOS = %q", f.BootROM)
	}
}
