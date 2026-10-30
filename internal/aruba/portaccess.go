package aruba

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RadiusServer is one RADIUS server from the running-config. The shared
// secret is never read back; HasKey only says that one is set.
type RadiusServer struct {
	Host   string `json:"host"`
	VRF    string `json:"vrf,omitempty"`
	HasKey bool   `json:"hasKey"`
}

// PortAuth is the 802.1X / MAC authentication setup of one port.
type PortAuth struct {
	Port         string `json:"port"`
	Dot1x        bool   `json:"dot1x"`
	MacAuth      bool   `json:"macAuth"`
	ClientLimit  int    `json:"clientLimit,omitempty"`  // 0: switch default
	ReauthPeriod int    `json:"reauthPeriod,omitempty"` // seconds, 0: off
	UnauthVLAN   int    `json:"unauthVlan,omitempty"`   // AOS-S only
	// Roles maps each AOS-CX role slot (see RoleKinds) to the role name set for it.
	Roles map[string]string `json:"roles,omitempty"`
}

// RoleKinds are the role slots of an AOS-CX port, named as in the config:
// after a login, after a rejected login, while RADIUS can't be reached, and before a login.
var RoleKinds = []string{"auth-role", "reject-role", "critical-role", "preauth-role"}

// PortRole is an AOS-CX local user role: where a client lands once a port slot picks it.
type PortRole struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// VLANMode is "access": one untagged VLAN (vlan access), or "trunk": an untagged
	// native VLAN (vlan trunk native) plus tagged VLANs (vlan trunk allowed). "" means access.
	VLANMode string `json:"vlanMode,omitempty"`
	VLAN     int    `json:"vlan,omitempty"`     // the access or native VLAN; 0: none, or only given by name
	VLANName string `json:"vlanName,omitempty"` // from "vlan ... name <vlan-name>"
	Tagged   []int  `json:"tagged,omitempty"`   // trunk only
}

func (r PortRole) mode() string {
	if r.VLANMode == "trunk" || r.VLANMode == "native" {
		return "trunk"
	}
	return "access"
}

// roleVLANLines turns a role's VLAN setup from old into r. Only what changed is
// sent, so a VLAN the switch has by name keeps that form.
func roleVLANLines(old, r PortRole) []string {
	var out []string
	keyword := func(x PortRole) string {
		if x.mode() == "trunk" {
			return "vlan trunk native"
		}
		return "vlan access"
	}
	// AOS-CX wants the VLAN in the "no" form ("no vlan trunk native" alone is
	// incomplete), and the number and the name are separate lines.
	clear := func(x PortRole) {
		if x.VLAN > 0 {
			out = append(out, fmt.Sprintf("no %s %d", keyword(x), x.VLAN))
		}
		if x.VLANName != "" {
			out = append(out, "no "+keyword(x)+" name "+x.VLANName)
		}
	}
	if r.mode() != old.mode() {
		clear(old)
		if old.mode() == "trunk" && len(old.Tagged) > 0 {
			out = append(out, "no vlan trunk allowed "+compressInts(old.Tagged))
		}
		old = PortRole{VLANMode: r.VLANMode}
	}
	if r.VLAN != old.VLAN || r.VLANName != old.VLANName {
		clear(old)
		if r.VLANName != "" {
			out = append(out, keyword(r)+" name "+r.VLANName)
		} else if r.VLAN > 0 {
			out = append(out, fmt.Sprintf("%s %d", keyword(r), r.VLAN))
		}
	}
	if r.mode() == "trunk" {
		had, want := map[int]bool{}, map[int]bool{}
		for _, v := range old.Tagged {
			had[v] = true
		}
		var add, drop []int
		tagged := r.Tagged
		// the untagged VLAN must be allowed on the trunk too, or its traffic is dropped
		if r.VLAN > 0 && !slices.Contains(tagged, r.VLAN) {
			tagged = append(slices.Clone(tagged), r.VLAN)
			slices.Sort(tagged)
		}
		for _, v := range tagged {
			want[v] = true
			if !had[v] {
				add = append(add, v)
			}
		}
		for _, v := range old.Tagged {
			if !want[v] {
				drop = append(drop, v)
			}
		}
		if len(drop) > 0 {
			out = append(out, "no vlan trunk allowed "+compressInts(drop))
		}
		if len(add) > 0 {
			out = append(out, "vlan trunk allowed "+compressInts(add))
		}
	}
	return out
}

func compressInts(ids []int) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return CompressPorts(s)
}

// PortAccess is port authentication as configured on the switch.
type PortAccess struct {
	Servers []RadiusServer `json:"servers"`
	Roles   []PortRole     `json:"roles"` // AOS-CX only
	// Dot1xOn and MacAuthOn are the global switches; port settings do nothing without them.
	// AOS-S has no global switch for MAC authentication, so MacAuthOn is always set there.
	Dot1xOn   bool       `json:"dot1xOn"`
	MacAuthOn bool       `json:"macAuthOn"`
	Ports     []PortAuth `json:"ports"`
	// Clients is who is on the ports right now (AOS-CX only). ClientsRead says
	// the switch answered, so an empty list really means nobody.
	Clients     []PortClient `json:"clients,omitempty"`
	ClientsRead bool         `json:"clientsRead"`
	ClientsAt   time.Time    `json:"clientsAt"`
}

type portAuthSet struct {
	byName map[string]*PortAuth
	order  []string
}

func (s *portAuthSet) get(p string) *PortAuth {
	if s.byName == nil {
		s.byName = map[string]*PortAuth{}
	}
	if a := s.byName[p]; a != nil {
		return a
	}
	s.byName[p] = &PortAuth{Port: p}
	s.order = append(s.order, p)
	return s.byName[p]
}

// list drops ports that have neither method on: their leftover options do nothing.
func (s *portAuthSet) list() []PortAuth {
	out := []PortAuth{}
	for _, p := range s.order {
		if a := s.byName[p]; a.Dot1x || a.MacAuth {
			out = append(out, *a)
		}
	}
	return out
}

func addServer(list []RadiusServer, host, vrf string, key bool) []RadiusServer {
	for i := range list {
		if list[i].Host == host && list[i].VRF == vrf {
			list[i].HasKey = list[i].HasKey || key
			return list
		}
	}
	return append(list, RadiusServer{Host: host, VRF: vrf, HasKey: key})
}

func atoiOr0(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ParsePortAccessAOSS reads RADIUS servers and "aaa port-access" lines from an
// AOS-S running-config, for example:
//
//	radius-server host 10.0.0.5 key "secret"
//	aaa port-access authenticator 1-24
//	aaa port-access authenticator 1 client-limit 2
//	aaa port-access authenticator active
//	aaa port-access mac-based 25-28
func ParsePortAccessAOSS(cfg string) *PortAccess {
	pa := &PortAccess{Servers: []RadiusServer{}, Roles: []PortRole{}, MacAuthOn: true}
	var ports portAuthSet
	globalKey := false
	for _, b := range splitConfig(cfg) {
		h := b.header
		fs := strings.Fields(h)
		switch {
		case strings.HasPrefix(h, "radius-server key ") || strings.HasPrefix(h, "radius-server encrypted-key "):
			globalKey = true
		case strings.HasPrefix(h, "radius-server host ") && len(fs) >= 3:
			key := strings.Contains(h, " key ") || strings.Contains(h, " encrypted-key ")
			pa.Servers = addServer(pa.Servers, fs[2], "", key)
		case h == "aaa port-access authenticator active":
			pa.Dot1xOn = true
		case len(fs) >= 4 && fs[0] == "aaa" && fs[1] == "port-access" && (fs[2] == "authenticator" || fs[2] == "mac-based"):
			dot1x := fs[2] == "authenticator"
			for _, p := range ExpandPorts(fs[3]) {
				a := ports.get(p)
				if len(fs) == 4 {
					if dot1x {
						a.Dot1x = true
					} else {
						a.MacAuth = true
					}
					continue
				}
				if len(fs) < 6 {
					continue
				}
				switch n := atoiOr0(fs[5]); fs[4] {
				case "client-limit", "addr-limit":
					if n > a.ClientLimit {
						a.ClientLimit = n
					}
				case "reauth-period":
					a.ReauthPeriod = n
				case "unauth-vid":
					a.UnauthVLAN = n
				}
			}
		}
	}
	for i := range pa.Servers {
		pa.Servers[i].HasKey = pa.Servers[i].HasKey || globalKey
	}
	pa.Ports = ports.list()
	return pa
}

var cxPortRe = regexp.MustCompile(`^\d+/\d+/\d+`)

func isRoleKind(k string) bool {
	for _, r := range RoleKinds {
		if r == k {
			return true
		}
	}
	return false
}

// ParsePortAccessCX reads RADIUS servers and port-access settings from an
// AOS-CX running-config. Port settings sit in nested contexts under the
// interface, so this follows indentation instead of using splitConfig:
//
//	interface 1/1/1
//	    aaa authentication port-access client-limit 2
//	    aaa authentication port-access dot1x authenticator
//	        enable
//	        reauth-period 3600
func ParsePortAccessCX(cfg string) *PortAccess {
	pa := &PortAccess{Servers: []RadiusServer{}, Roles: []PortRole{}}
	var ports portAuthSet
	var iface []string
	var role *PortRole
	sub, subIndent := "", 0
	reauth := map[string]bool{}
	for _, raw := range strings.Split(cfg, "\n") {
		t := strings.TrimSpace(raw)
		if t == "" || t == "!" {
			continue
		}
		in := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if sub != "" && in <= subIndent {
			sub = ""
		}
		fs := strings.Fields(t)
		if in == 0 {
			iface = nil
			switch {
			case strings.HasPrefix(t, "interface ") && cxPortRe.MatchString(fs[1]):
				iface = ExpandPorts(fs[1])
			case t == "aaa authentication port-access dot1x authenticator":
				sub, subIndent = "global-dot1x", 0
			case t == "aaa authentication port-access mac-auth":
				sub, subIndent = "global-mac", 0
			case strings.HasPrefix(t, "port-access role ") && len(fs) == 3:
				pa.Roles = append(pa.Roles, PortRole{Name: fs[2]})
				role = &pa.Roles[len(pa.Roles)-1]
				sub, subIndent = "role", 0
			case t == "aaa authentication port-access dot1x authenticator enable":
				pa.Dot1xOn = true
			case t == "aaa authentication port-access mac-auth enable":
				pa.MacAuthOn = true
			case strings.HasPrefix(t, "radius-server host ") && len(fs) >= 3:
				vrf := ""
				for i := 3; i < len(fs)-1; i++ {
					if fs[i] == "vrf" {
						vrf = fs[i+1]
					}
				}
				pa.Servers = addServer(pa.Servers, fs[2], vrf, strings.Contains(t, " key "))
			}
			continue
		}
		if sub != "" {
			switch {
			case sub == "role" && (strings.HasPrefix(t, "vlan access ") || strings.HasPrefix(t, "vlan trunk native ")):
				// vlan access 8 | vlan access name 8-Lan | vlan trunk native 8 | vlan trunk native name 8-Lan
				rest := fs[2:]
				role.VLANMode = "access"
				if fs[1] == "trunk" {
					rest, role.VLANMode = fs[3:], "trunk"
				}
				if len(rest) == 2 && rest[0] == "name" {
					role.VLANName = rest[1]
				} else if len(rest) == 1 {
					role.VLAN = atoiOr0(rest[0])
				}
			case sub == "role" && strings.HasPrefix(t, "vlan trunk allowed ") && len(fs) == 4:
				role.VLANMode = "trunk"
				role.Tagged = append(role.Tagged, ExpandInts(fs[3])...)
			case sub == "role" && strings.HasPrefix(t, "description "):
				role.Description = strings.TrimSpace(strings.TrimPrefix(t, "description "))
			case sub == "role":
			case t == "enable" && sub == "global-dot1x":
				pa.Dot1xOn = true
			case t == "enable" && sub == "global-mac":
				pa.MacAuthOn = true
			case t == "enable":
				for _, p := range iface {
					if sub == "dot1x" {
						ports.get(p).Dot1x = true
					} else {
						ports.get(p).MacAuth = true
					}
				}
			case sub == "dot1x" && t == "reauth":
				for _, p := range iface {
					reauth[p] = true
				}
			case sub == "dot1x" && len(fs) == 2 && fs[0] == "reauth-period":
				for _, p := range iface {
					ports.get(p).ReauthPeriod = atoiOr0(fs[1])
				}
			}
			continue
		}
		if iface == nil {
			continue
		}
		switch {
		case t == "aaa authentication port-access dot1x authenticator":
			sub, subIndent = "dot1x", in
		case t == "aaa authentication port-access mac-auth":
			sub, subIndent = "mac", in
		case t == "aaa authentication port-access dot1x authenticator enable":
			for _, p := range iface {
				ports.get(p).Dot1x = true
			}
		case t == "aaa authentication port-access mac-auth enable":
			for _, p := range iface {
				ports.get(p).MacAuth = true
			}
		case strings.HasPrefix(t, "aaa authentication port-access client-limit ") && len(fs) == 5:
			for _, p := range iface {
				ports.get(p).ClientLimit = atoiOr0(fs[4])
			}
		case strings.HasPrefix(t, "aaa authentication port-access ") && len(fs) == 5 && isRoleKind(fs[3]):
			for _, p := range iface {
				a := ports.get(p)
				if a.Roles == nil {
					a.Roles = map[string]string{}
				}
				a.Roles[fs[3]] = fs[4]
			}
		}
	}
	// a re-auth period only counts once periodic re-authentication is on
	for _, p := range ports.order {
		a := ports.byName[p]
		switch {
		case !reauth[p]:
			a.ReauthPeriod = 0
		case a.ReauthPeriod == 0:
			a.ReauthPeriod = 86400 // AOS-CX default
		}
	}
	pa.Ports = ports.list()
	return pa
}

// RadiusServerChange adds one RADIUS server.
type RadiusServerChange struct {
	Host string `json:"host"`
	Key  string `json:"key"`
	VRF  string `json:"vrf"` // AOS-CX only; empty is the default VRF
}

// PortAccessChange is one port-access edit coming from the UI.
// Nil fields are left as they are.
type PortAccessChange struct {
	AddServer    *RadiusServerChange `json:"addServer"`
	RemoveServer string              `json:"removeServer"` // host
	Ports        []string            `json:"ports"`
	Dot1x        *bool               `json:"dot1x"`
	MacAuth      *bool               `json:"macAuth"`
	ClientLimit  *int                `json:"clientLimit"`  // 0 goes back to the default
	ReauthPeriod *int                `json:"reauthPeriod"` // seconds, 0 turns it off
	UnauthVLAN   *int                `json:"unauthVlan"`   // 0 removes it; AOS-S only
	// Roles sets AOS-CX role slots on the ports; an empty name takes the slot off.
	Roles map[string]string `json:"roles"`
	// SetRole creates an AOS-CX role, or changes the one with that name.
	SetRole    *PortRole `json:"setRole"`
	DeleteRole string    `json:"deleteRole"`
	// GlobalDot1x and GlobalMacAuth turn a method on or off for the whole
	// switch. AOS-S has no switch-wide setting for MAC authentication.
	GlobalDot1x   *bool `json:"globalDot1x"`
	GlobalMacAuth *bool `json:"globalMacAuth"`
	Save          bool  `json:"save"`
}

// globalWant is the switch-wide state after a change: set outright, or
// turned on because ports are given the method.
func globalWant(global, port *bool, cur bool) bool {
	if global != nil {
		return *global
	}
	return cur || (port != nil && *port)
}

var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
var numericHost = regexp.MustCompile(`^[0-9.]+$`)

// checkRadiusHost accepts a usable unicast IPv4 or IPv6 address, or on
// AOS-CX also a host name. AOS-S only takes an IP address here.
func checkRadiusHost(f Family, h string) error {
	if h == "" {
		return fmt.Errorf("enter the server's IP address or host name")
	}
	if ip, err := netip.ParseAddr(h); err == nil {
		if ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
			return fmt.Errorf("%s can't be used as a RADIUS server address", h)
		}
		return nil
	}
	if numericHost.MatchString(h) || strings.Contains(h, ":") {
		return fmt.Errorf("%q isn't a valid IP address", h)
	}
	if f != FamilyAOSCX {
		return fmt.Errorf("enter the server's IP address; this switch doesn't take a host name here")
	}
	labels := strings.Split(strings.TrimSuffix(h, "."), ".")
	if len(h) > 253 {
		return fmt.Errorf("the host name is too long")
	}
	for _, l := range labels {
		if !hostLabel.MatchString(l) {
			return fmt.Errorf("%q isn't a valid host name", h)
		}
	}
	return nil
}

var safeKey = regexp.MustCompile(`^[!#-~]{1,64}$`) // printable, no space or double quote
var safeVRF = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var safeRole = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var safeDescr = regexp.MustCompile(`^[A-Za-z0-9 _.,:/()-]{0,64}$`)

func (c PortAccessChange) portFields() bool {
	return c.Dot1x != nil || c.MacAuth != nil || c.ClientLimit != nil || c.ReauthPeriod != nil || c.UnauthVLAN != nil || len(c.Roles) > 0
}

func findRole(pa PortAccess, name string) (PortRole, bool) {
	for _, r := range pa.Roles {
		if r.Name == name {
			return r, true
		}
	}
	return PortRole{}, false
}

func (c PortAccessChange) validateRoles(f Family, cur PortAccess) error {
	if c.SetRole == nil && c.DeleteRole == "" && len(c.Roles) == 0 {
		return nil
	}
	if f != FamilyAOSCX {
		return fmt.Errorf("roles are only used on AOS-CX; AOS-S uses the unauthenticated VLAN instead")
	}
	if r := c.SetRole; r != nil {
		if !safeRole.MatchString(r.Name) {
			return fmt.Errorf("role name may use letters, digits, _ . - (max 64)")
		}
		if !safeDescr.MatchString(r.Description) {
			return fmt.Errorf("description may use letters, digits, spaces and _ . , : / ( ) - (max 64)")
		}
		if r.VLAN < 0 || r.VLAN > 4094 {
			return fmt.Errorf("VLAN ID must be 1-4094")
		}
		if r.VLANName != "" && !safeRole.MatchString(r.VLANName) {
			return fmt.Errorf("invalid VLAN name %q", r.VLANName)
		}
		if r.VLANMode != "" && r.VLANMode != "access" && r.VLANMode != "trunk" {
			return fmt.Errorf("VLAN mode must be access or trunk")
		}
		if r.mode() == "access" && len(r.Tagged) > 0 {
			return fmt.Errorf("a single-VLAN role can't have tagged VLANs; make it a trunk")
		}
		for _, v := range r.Tagged {
			if v < 1 || v > 4094 {
				return fmt.Errorf("tagged VLAN IDs must be 1-4094")
			}
		}
	}
	if c.DeleteRole != "" {
		if _, ok := findRole(cur, c.DeleteRole); !ok {
			return fmt.Errorf("role %s doesn't exist on this switch", c.DeleteRole)
		}
		var used []string
		for _, a := range cur.Ports {
			for _, name := range a.Roles {
				if name == c.DeleteRole {
					used = append(used, a.Port)
					break
				}
			}
		}
		if len(used) > 0 {
			return fmt.Errorf("role %s is used on ports %s; take it off those ports first", c.DeleteRole, CompressPorts(used))
		}
	}
	for k, name := range c.Roles {
		if !isRoleKind(k) {
			return fmt.Errorf("unknown role slot %q", k)
		}
		if name == "" {
			continue
		}
		if _, ok := findRole(cur, name); !ok && (c.SetRole == nil || c.SetRole.Name != name) {
			return fmt.Errorf("role %s doesn't exist on this switch; create it first", name)
		}
	}
	return nil
}

func (c PortAccessChange) validate(f Family, cur PortAccess) error {
	if f != FamilyAOSS && f != FamilyAOSCX {
		return fmt.Errorf("port access can only be changed on AOS-S and AOS-CX switches")
	}
	if c.AddServer == nil && c.RemoveServer == "" && c.SetRole == nil && c.DeleteRole == "" && c.GlobalDot1x == nil && c.GlobalMacAuth == nil && !c.portFields() {
		return fmt.Errorf("nothing to change")
	}
	if c.GlobalMacAuth != nil && f != FamilyAOSCX {
		return fmt.Errorf("AOS-S has no switch-wide setting for MAC authentication; it's turned on per port")
	}
	if err := c.validateRoles(f, cur); err != nil {
		return err
	}
	if s := c.AddServer; s != nil {
		if err := checkRadiusHost(f, s.Host); err != nil {
			return err
		}
		if !safeKey.MatchString(s.Key) {
			return fmt.Errorf("the shared secret must be 1-64 characters without spaces or double quotes")
		}
		if s.VRF != "" && (f != FamilyAOSCX || !safeVRF.MatchString(s.VRF)) {
			return fmt.Errorf("a VRF can only be set on AOS-CX, using letters, digits, _ and -")
		}
		for _, x := range cur.Servers {
			if strings.EqualFold(x.Host, s.Host) && x.VRF == s.VRF {
				return fmt.Errorf("RADIUS server %s is already set", s.Host)
			}
		}
	}
	if c.RemoveServer != "" {
		if _, ok := findServer(cur, c.RemoveServer); !ok {
			return fmt.Errorf("RADIUS server %s isn't set on this switch", c.RemoveServer)
		}
	}
	if !c.portFields() {
		return nil
	}
	if len(c.Ports) == 0 {
		return fmt.Errorf("select at least one port")
	}
	for _, p := range c.Ports {
		if !safePort.MatchString(p) {
			return fmt.Errorf("invalid port %q", p)
		}
	}
	maxClients := 32
	if f == FamilyAOSCX {
		maxClients = 256
	}
	if c.ClientLimit != nil && (*c.ClientLimit < 0 || *c.ClientLimit > maxClients) {
		return fmt.Errorf("client limit must be 1-%d, or 0 for the default", maxClients)
	}
	if c.ReauthPeriod != nil && *c.ReauthPeriod != 0 && (*c.ReauthPeriod < 60 || *c.ReauthPeriod > 86400) {
		return fmt.Errorf("re-authentication period must be 60-86400 seconds, or 0 to turn it off")
	}
	if c.UnauthVLAN != nil {
		if f == FamilyAOSCX && *c.UnauthVLAN != 0 {
			return fmt.Errorf("AOS-CX places unauthenticated clients through roles, not a VLAN; use the reject or critical role")
		}
		if *c.UnauthVLAN < 0 || *c.UnauthVLAN > 4094 {
			return fmt.Errorf("VLAN ID must be 1-4094")
		}
	}
	if c.ClientLimit != nil || c.ReauthPeriod != nil || c.UnauthVLAN != nil || len(c.Roles) > 0 {
		for _, a := range c.after(cur) {
			if !a.Dot1x && !a.MacAuth {
				return fmt.Errorf("turn on 802.1X or MAC authentication on port %s before changing its options", a.Port)
			}
		}
	}
	return nil
}

func findServer(pa PortAccess, host string) (RadiusServer, bool) {
	for _, s := range pa.Servers {
		if strings.EqualFold(s.Host, host) {
			return s, true
		}
	}
	return RadiusServer{}, false
}

// after returns the selected ports as they will be once c is applied.
func (c PortAccessChange) after(cur PortAccess) []PortAuth {
	have := map[string]PortAuth{}
	for _, a := range cur.Ports {
		have[a.Port] = a
	}
	var out []PortAuth
	for _, p := range c.Ports {
		a, ok := have[p]
		if !ok {
			a = PortAuth{Port: p}
		}
		if c.Dot1x != nil {
			a.Dot1x = *c.Dot1x
		}
		if c.MacAuth != nil {
			a.MacAuth = *c.MacAuth
		}
		if c.ClientLimit != nil {
			a.ClientLimit = *c.ClientLimit
		}
		if c.ReauthPeriod != nil {
			a.ReauthPeriod = *c.ReauthPeriod
		}
		if c.UnauthVLAN != nil {
			a.UnauthVLAN = *c.UnauthVLAN
		}
		if len(c.Roles) > 0 {
			// a copy: the current setup must stay as it is
			roles := map[string]string{}
			for k, v := range a.Roles {
				roles[k] = v
			}
			for k, v := range c.Roles {
				if v == "" {
					delete(roles, k)
				} else {
					roles[k] = v
				}
			}
			a.Roles = nil
			if len(roles) > 0 {
				a.Roles = roles
			}
		}
		out = append(out, a)
	}
	return out
}

// BuildPortAccessCommands returns the exact CLI lines that will be sent.
// They contain the shared secret in clear; show them through MaskSecret.
func BuildPortAccessCommands(f Family, c PortAccessChange, cur PortAccess) ([]string, error) {
	if err := c.validate(f, cur); err != nil {
		return nil, err
	}
	cmds := []string{"configure terminal"}
	if f == FamilyAOSCX {
		cmds = append(cmds, cxPortAccess(c, cur)...)
	} else {
		cmds = append(cmds, aossPortAccess(c, cur)...)
	}
	if len(cmds) == 1 {
		return nil, fmt.Errorf("nothing changed")
	}
	cmds = append(cmds, "end")
	if c.Save {
		cmds = append(cmds, "write memory")
	}
	return cmds, nil
}

func aossPortAccess(c PortAccessChange, cur PortAccess) []string {
	var cmds []string
	if s := c.AddServer; s != nil {
		cmds = append(cmds, fmt.Sprintf("radius-server host %s key \"%s\"", s.Host, s.Key))
	}
	if c.RemoveServer != "" {
		cmds = append(cmds, "no radius-server host "+c.RemoveServer)
	}
	on := globalWant(c.GlobalDot1x, c.Dot1x, cur.Dot1xOn)
	if on && !cur.Dot1xOn {
		cmds = append(cmds, "aaa authentication port-access eap-radius")
	}
	if c.portFields() {
		cmds = append(cmds, aossPorts(c, cur)...)
	}
	switch {
	case on && !cur.Dot1xOn:
		cmds = append(cmds, "aaa port-access authenticator active")
	case !on && cur.Dot1xOn:
		cmds = append(cmds, "no aaa port-access authenticator active")
	}
	return cmds
}

func aossPorts(c PortAccessChange, cur PortAccess) []string {
	var cmds []string
	ports := CompressPorts(c.Ports)
	var dot1xPorts, macPorts []string
	for _, a := range c.after(cur) {
		if a.Dot1x {
			dot1xPorts = append(dot1xPorts, a.Port)
		}
		if a.MacAuth {
			macPorts = append(macPorts, a.Port)
		}
	}
	if c.Dot1x != nil {
		if *c.Dot1x {
			cmds = append(cmds, "aaa port-access authenticator "+ports)
		} else {
			cmds = append(cmds, "no aaa port-access authenticator "+ports)
		}
	}
	if c.MacAuth != nil {
		if *c.MacAuth {
			cmds = append(cmds, "aaa port-access mac-based "+ports)
		} else {
			cmds = append(cmds, "no aaa port-access mac-based "+ports)
		}
	}
	// options go to each method that stays on for the port
	opts := func(method, limitKw string, list []string) {
		if len(list) == 0 {
			return
		}
		pre := "aaa port-access " + method + " " + CompressPorts(list)
		if v := c.ClientLimit; v != nil {
			if *v == 0 {
				cmds = append(cmds, "no "+pre+" "+limitKw)
			} else {
				cmds = append(cmds, fmt.Sprintf("%s %s %d", pre, limitKw, *v))
			}
		}
		if v := c.ReauthPeriod; v != nil {
			cmds = append(cmds, fmt.Sprintf("%s reauth-period %d", pre, *v))
		}
		if v := c.UnauthVLAN; v != nil {
			if *v == 0 {
				cmds = append(cmds, "no "+pre+" unauth-vid")
			} else {
				cmds = append(cmds, fmt.Sprintf("%s unauth-vid %d", pre, *v))
			}
		}
	}
	opts("authenticator", "client-limit", dot1xPorts)
	opts("mac-based", "addr-limit", macPorts)
	return cmds
}

const cxDot1x, cxMac = "aaa authentication port-access dot1x authenticator", "aaa authentication port-access mac-auth"

// cxGlobal uses the context form, the way the running-config shows it.
func cxGlobal(kw string, want, cur bool) []string {
	switch {
	case want && !cur:
		return []string{kw, "enable", "exit"}
	case !want && cur:
		return []string{kw, "no enable", "exit"}
	}
	return nil
}

func cxPortAccess(c PortAccessChange, cur PortAccess) []string {
	var cmds []string
	if s := c.AddServer; s != nil {
		l := fmt.Sprintf("radius-server host %s key plaintext %s", s.Host, s.Key)
		if s.VRF != "" {
			l += " vrf " + s.VRF
		}
		cmds = append(cmds, l)
	}
	if c.RemoveServer != "" {
		s, _ := findServer(cur, c.RemoveServer)
		l := "no radius-server host " + s.Host
		if s.VRF != "" {
			l += " vrf " + s.VRF
		}
		cmds = append(cmds, l)
	}
	// a role is defined before ports use it, and removed only once none do
	if r := c.SetRole; r != nil {
		old, exists := findRole(cur, r.Name)
		var lines []string
		switch {
		case r.Description == old.Description:
		case r.Description != "":
			lines = append(lines, "description "+r.Description)
		default:
			lines = append(lines, "no description")
		}
		lines = append(lines, roleVLANLines(old, *r)...)
		// an existing role that stays the same isn't touched; a new one is created even when empty
		if len(lines) > 0 || !exists {
			cmds = append(cmds, "port-access role "+r.Name)
			cmds = append(cmds, lines...)
			cmds = append(cmds, "exit")
		}
	}
	cmds = append(cmds, cxGlobal(cxDot1x, globalWant(c.GlobalDot1x, c.Dot1x, cur.Dot1xOn), cur.Dot1xOn)...)
	cmds = append(cmds, cxGlobal(cxMac, globalWant(c.GlobalMacAuth, c.MacAuth, cur.MacAuthOn), cur.MacAuthOn)...)
	if c.portFields() {
		cmds = append(cmds, cxPorts(c, cur)...)
	}
	if c.DeleteRole != "" {
		cmds = append(cmds, "no port-access role "+c.DeleteRole)
	}
	return cmds
}

func cxPorts(c PortAccessChange, cur PortAccess) []string {
	var cmds []string
	const dot1x, mac = cxDot1x, cxMac
	for _, a := range c.after(cur) {
		cmds = append(cmds, "interface "+a.Port)
		switch {
		case c.Dot1x != nil && !*c.Dot1x:
			cmds = append(cmds, "no "+dot1x)
		case c.Dot1x != nil || (c.ReauthPeriod != nil && a.Dot1x):
			cmds = append(cmds, dot1x)
			if c.Dot1x != nil {
				cmds = append(cmds, "enable")
			}
			if v := c.ReauthPeriod; v != nil && *v > 0 {
				cmds = append(cmds, "reauth", fmt.Sprintf("reauth-period %d", *v))
			} else if v != nil {
				cmds = append(cmds, "no reauth")
			}
			cmds = append(cmds, "exit")
		}
		if c.MacAuth != nil {
			if *c.MacAuth {
				cmds = append(cmds, mac, "enable", "exit")
			} else {
				cmds = append(cmds, "no "+mac)
			}
		}
		if v := c.ClientLimit; v != nil {
			if *v == 0 {
				cmds = append(cmds, "no aaa authentication port-access client-limit")
			} else {
				cmds = append(cmds, fmt.Sprintf("aaa authentication port-access client-limit %d", *v))
			}
		}
		for _, k := range RoleKinds {
			if name, ok := c.Roles[k]; ok && name != "" {
				cmds = append(cmds, "aaa authentication port-access "+k+" "+name)
			} else if ok {
				cmds = append(cmds, "no aaa authentication port-access "+k)
			}
		}
		// A port that no longer asks for any login drops its roles and client
		// limit too: they'd do nothing, and nothing on screen would show them.
		// validate() makes sure this change doesn't set them at the same time.
		if !a.Dot1x && !a.MacAuth {
			for _, k := range RoleKinds {
				if a.Roles[k] != "" {
					cmds = append(cmds, "no aaa authentication port-access "+k)
				}
			}
			if a.ClientLimit != 0 {
				cmds = append(cmds, "no aaa authentication port-access client-limit")
			}
		}
		cmds = append(cmds, "exit")
	}
	return cmds
}

// MaskSecret hides the shared secret of c in lines meant for the screen.
func MaskSecret(lines []string, c PortAccessChange) []string {
	if c.AddServer == nil || c.AddServer.Key == "" {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.ReplaceAll(l, c.AddServer.Key, "********")
	}
	return out
}

// AfterPortAccess returns the port-access setup as it should be once c has
// been accepted, so the UI shows the change before the switch is read again.
func AfterPortAccess(cur PortAccess, c PortAccessChange) PortAccess {
	next := PortAccess{Dot1xOn: cur.Dot1xOn, MacAuthOn: cur.MacAuthOn, Servers: []RadiusServer{}, Roles: []PortRole{},
		Clients: cur.Clients, ClientsRead: cur.ClientsRead, ClientsAt: cur.ClientsAt}
	added := false
	for _, r := range cur.Roles {
		switch {
		case r.Name == c.DeleteRole:
			continue
		case c.SetRole != nil && r.Name == c.SetRole.Name:
			r, added = *c.SetRole, true
		}
		next.Roles = append(next.Roles, r)
	}
	if c.SetRole != nil && !added {
		next.Roles = append(next.Roles, *c.SetRole)
	}
	for _, s := range cur.Servers {
		if c.RemoveServer == "" || !strings.EqualFold(s.Host, c.RemoveServer) {
			next.Servers = append(next.Servers, s)
		}
	}
	if s := c.AddServer; s != nil {
		next.Servers = append(next.Servers, RadiusServer{Host: s.Host, VRF: s.VRF, HasKey: true})
	}
	next.Dot1xOn = globalWant(c.GlobalDot1x, c.Dot1x, cur.Dot1xOn)
	next.MacAuthOn = globalWant(c.GlobalMacAuth, c.MacAuth, cur.MacAuthOn)
	var set portAuthSet
	for _, a := range cur.Ports {
		*set.get(a.Port) = a
	}
	if c.portFields() {
		for _, a := range c.after(cur) {
			*set.get(a.Port) = a
		}
	}
	next.Ports = set.list()
	return next
}
