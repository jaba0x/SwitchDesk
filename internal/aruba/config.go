package aruba

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type configBlock struct {
	header string
	lines  []string
}

// splitConfig groups an Aruba running-config into top-level blocks.
// AOS-S closes blocks with "exit"; AOS-CX uses indentation only.
func splitConfig(cfg string) []configBlock {
	var blocks []configBlock
	var cur *configBlock
	for _, raw := range strings.Split(cfg, "\n") {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), ";") || strings.TrimSpace(raw) == "!" {
			continue
		}
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		t := strings.TrimSpace(raw)
		if !indented {
			blocks = append(blocks, configBlock{header: t})
			cur = &blocks[len(blocks)-1]
			continue
		}
		if t == "exit" {
			continue
		}
		if cur != nil {
			cur.lines = append(cur.lines, t)
		}
	}
	return blocks
}

var vlanHeaderRe = regexp.MustCompile(`^vlan (\d+)$`)
var quotedRe = regexp.MustCompile(`"([^"]*)"`)

// ParseConfigAOSS extracts VLANs and security posture from an AOS-S running-config.
func ParseConfigAOSS(cfg string) ([]VLAN, Security, bool) {
	// AOS-S has Telnet and plain HTTP on unless the config turns them off, but
	// only a config we actually read can say so.
	on := strings.TrimSpace(cfg) != ""
	sec := Security{SSHEnabled: true, TelnetEnabled: on, HTTPPlain: on}
	stp := false
	var vlans []VLAN
	for _, b := range splitConfig(cfg) {
		h := b.header
		switch {
		case vlanHeaderRe.MatchString(h):
			id, _ := strconv.Atoi(vlanHeaderRe.FindStringSubmatch(h)[1])
			v := VLAN{ID: id}
			for _, l := range b.lines {
				switch {
				case strings.HasPrefix(l, "name "):
					v.Name = unquote(strings.TrimPrefix(l, "name "))
				case strings.HasPrefix(l, "tagged "):
					v.Tagged = append(v.Tagged, ExpandPorts(strings.TrimPrefix(l, "tagged "))...)
				case strings.HasPrefix(l, "untagged "):
					v.Untagged = append(v.Untagged, ExpandPorts(strings.TrimPrefix(l, "untagged "))...)
				case strings.HasPrefix(l, "ip address "):
					v.IP = strings.TrimPrefix(l, "ip address ")
				}
			}
			vlans = append(vlans, v)
		case h == "no telnet-server":
			sec.TelnetEnabled = false
		case h == "no web-management":
			sec.HTTPPlain = false
		case strings.HasPrefix(h, "web-management ssl"):
			sec.HTTPSEnabled = true
		case h == "no web-management plaintext":
			sec.HTTPPlain = false
		case strings.HasPrefix(h, "time timezone "):
			sec.Timezone = strings.TrimPrefix(h, "time timezone ")
		case h == "no ip ssh":
			sec.SSHEnabled = false
		case strings.HasPrefix(h, "ip ssh port "):
			sec.SSHPort = strings.TrimPrefix(h, "ip ssh port ")
		case strings.HasPrefix(h, "snmp-server community "):
			c := strings.TrimPrefix(h, "snmp-server community ")
			if m := quotedRe.FindStringSubmatch(c); m != nil {
				c = m[1]
			} else {
				c = strings.Fields(c)[0]
			}
			sec.SNMPCommunities = append(sec.SNMPCommunities, c)
		case strings.HasPrefix(h, "sntp server ") || strings.HasPrefix(h, "ntp server ") || strings.HasPrefix(h, "ntp server-name "):
			if s := ntpServerAOSS(h); s != "" {
				sec.NTPServers = append(sec.NTPServers, s)
			}
		case strings.HasPrefix(h, "logging ") && !strings.HasPrefix(h, "logging severity") && !strings.HasPrefix(h, "logging facility") && !strings.HasPrefix(h, "logging filter"):
			fs := strings.Fields(h)
			if len(fs) >= 2 {
				sec.LogServers = append(sec.LogServers, fs[1])
			}
		case strings.HasPrefix(h, "aaa authentication") || strings.HasPrefix(h, "radius-server host") || strings.HasPrefix(h, "tacacs-server host"):
			sec.AAA = append(sec.AAA, h)
		case strings.HasPrefix(h, "password manager") || strings.HasPrefix(h, "password operator"):
			sec.ManagerPassword = true
		case h == "spanning-tree" || strings.HasPrefix(h, "spanning-tree mode") || strings.HasPrefix(h, "spanning-tree priority") || strings.HasPrefix(h, "spanning-tree config-name"):
			stp = true
		case h == "no spanning-tree":
			stp = false
		}
	}
	if sec.SSHPort == "" {
		sec.SSHPort = "22"
	}
	sec.SNMPDefault = hasDefaultCommunity(sec.SNMPCommunities)
	return vlans, sec, stp
}

// ParseConfigCX extracts VLANs (with access/trunk membership) and security posture
// from an AOS-CX running-config.
func ParseConfigCX(cfg string) ([]VLAN, Security, bool) {
	sec := Security{SSHEnabled: false, TelnetEnabled: false, HTTPPlain: false, SSHPort: "22"}
	stp := false
	byID := map[int]*VLAN{}
	get := func(id int) *VLAN {
		if v, ok := byID[id]; ok {
			return v
		}
		v := &VLAN{ID: id}
		byID[id] = v
		return v
	}
	for _, b := range splitConfig(cfg) {
		h := b.header
		switch {
		case strings.HasPrefix(h, "vlan ") && !strings.HasPrefix(h, "vlan trunk"):
			ids := ExpandInts(strings.TrimPrefix(h, "vlan "))
			for _, id := range ids {
				v := get(id)
				if len(ids) == 1 {
					for _, l := range b.lines {
						if strings.HasPrefix(l, "name ") {
							v.Name = unquote(strings.TrimPrefix(l, "name "))
						}
					}
				}
			}
		case strings.HasPrefix(h, "interface vlan"):
			id, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(h, "interface vlan"), " ")))
			if err == nil {
				for _, l := range b.lines {
					if strings.HasPrefix(l, "ip address ") {
						get(id).IP = strings.TrimPrefix(l, "ip address ")
					}
				}
			}
		case strings.HasPrefix(h, "interface "):
			port := strings.TrimSpace(strings.TrimPrefix(h, "interface "))
			if strings.HasPrefix(port, "mgmt") || strings.HasPrefix(port, "loopback") {
				continue
			}
			native := 0
			var allowed []int
			allowAll := false
			access := 0
			routed := false
			for _, l := range b.lines {
				switch {
				case strings.HasPrefix(l, "vlan access "):
					access, _ = strconv.Atoi(strings.TrimPrefix(l, "vlan access "))
				case strings.HasPrefix(l, "vlan trunk native "):
					f := strings.Fields(strings.TrimPrefix(l, "vlan trunk native "))
					if len(f) > 0 {
						native, _ = strconv.Atoi(f[0])
					}
				case strings.HasPrefix(l, "vlan trunk allowed "):
					a := strings.TrimPrefix(l, "vlan trunk allowed ")
					if a == "all" {
						allowAll = true
					} else {
						allowed = append(allowed, ExpandInts(a)...)
					}
				case l == "routing" || strings.HasPrefix(l, "ip address "):
					routed = true
				}
			}
			if routed && access == 0 && native == 0 && len(allowed) == 0 {
				continue
			}
			if access > 0 {
				v := get(access)
				v.Untagged = append(v.Untagged, port)
			}
			if native > 0 || len(allowed) > 0 || allowAll {
				if native == 0 {
					native = 1
				}
				get(native).Untagged = append(get(native).Untagged, port)
				if allowAll {
					for id, v := range byID {
						if id != native {
							v.Tagged = append(v.Tagged, port)
						}
					}
				}
				for _, id := range allowed {
					if id != native {
						get(id).Tagged = append(get(id).Tagged, port)
					}
				}
			}
		case strings.HasPrefix(h, "clock timezone "):
			sec.Timezone = strings.TrimPrefix(h, "clock timezone ")
		case strings.HasPrefix(h, "ssh server vrf"):
			sec.SSHEnabled = true
		case strings.HasPrefix(h, "ssh server port"):
			sec.SSHPort = strings.TrimPrefix(h, "ssh server port ")
		case strings.HasPrefix(h, "https-server vrf"):
			sec.HTTPSEnabled = true
		case strings.HasPrefix(h, "telnet server") || strings.HasPrefix(h, "telnet-server"):
			sec.TelnetEnabled = true
		case strings.HasPrefix(h, "snmp-server community "):
			f := strings.Fields(strings.TrimPrefix(h, "snmp-server community "))
			if len(f) > 0 {
				sec.SNMPCommunities = append(sec.SNMPCommunities, unquote(f[0]))
			}
		case strings.HasPrefix(h, "ntp server "):
			f := strings.Fields(h)
			if len(f) >= 3 {
				sec.NTPServers = append(sec.NTPServers, f[2])
			}
		case strings.HasPrefix(h, "logging ") && len(strings.Fields(h)) >= 2 && looksLikeHost(strings.Fields(h)[1]):
			sec.LogServers = append(sec.LogServers, strings.Fields(h)[1])
		case strings.HasPrefix(h, "aaa authentication") || strings.HasPrefix(h, "radius-server host") || strings.HasPrefix(h, "tacacs-server host"):
			sec.AAA = append(sec.AAA, h)
		case strings.HasPrefix(h, "user admin") && strings.Contains(h, "password"):
			sec.ManagerPassword = true
		case h == "spanning-tree" || strings.HasPrefix(h, "spanning-tree mode") || strings.HasPrefix(h, "spanning-tree priority") || strings.HasPrefix(h, "spanning-tree config-name"):
			stp = true
		}
	}
	// AOS-CX always runs SSH when we got in over SSH; the config line just names the VRF.
	if !sec.SSHEnabled {
		sec.SSHEnabled = true
	}
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var vlans []VLAN
	for _, id := range ids {
		vlans = append(vlans, *byID[id])
	}
	sec.SNMPDefault = hasDefaultCommunity(sec.SNMPCommunities)
	return vlans, sec, stp
}

// ParseVLANTable reads "show vlan(s)" tables from both families to fill names
// and status where the config didn't have them.
func ParseVLANTable(text string) []VLAN {
	var out []VLAN
	rowRe := regexp.MustCompile(`^\s*(\d{1,4})\s+(.+)$`)
	for _, line := range strings.Split(text, "\n") {
		m := rowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		if id < 1 || id > 4094 {
			continue
		}
		rest := m[2]
		v := VLAN{ID: id}
		if i := strings.Index(rest, "|"); i >= 0 { // AOS-S: "Name | Status Voice Jumbo"
			v.Name = strings.TrimSpace(rest[:i])
			f := strings.Fields(rest[i+1:])
			if len(f) > 0 {
				v.Status = f[0]
			}
		} else { // AOS-CX: Name Status Reason Type Interfaces
			f := strings.Fields(rest)
			if len(f) == 0 {
				continue
			}
			v.Name = f[0]
			if len(f) > 1 {
				v.Status = f[1]
			}
		}
		out = append(out, v)
	}
	return out
}

// MergeVLANs fills gaps in cfg VLANs from the table.
func MergeVLANs(cfg []VLAN, table []VLAN) []VLAN {
	byID := map[int]int{}
	for i, v := range cfg {
		byID[v.ID] = i
	}
	for _, t := range table {
		if i, ok := byID[t.ID]; ok {
			if cfg[i].Name == "" {
				cfg[i].Name = t.Name
			}
			if cfg[i].Status == "" {
				cfg[i].Status = t.Status
			}
		} else {
			cfg = append(cfg, t)
		}
	}
	sort.Slice(cfg, func(a, b int) bool { return cfg[a].ID < cfg[b].ID })
	return cfg
}

func hasDefaultCommunity(cs []string) bool {
	for _, c := range cs {
		l := strings.ToLower(c)
		if l == "public" || l == "private" {
			return true
		}
	}
	return false
}

func looksLikeHost(s string) bool {
	return regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$|^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`).MatchString(s)
}

// ntpServerAOSS returns the server in "sntp server priority 1 <addr>",
// "ntp server <addr> iburst" or `ntp server-name "<name>" iburst`.
func ntpServerAOSS(h string) string {
	fs := strings.Fields(h)
	i := 2
	if len(fs) > 4 && fs[2] == "priority" {
		i = 4
	}
	if i >= len(fs) {
		return ""
	}
	return unquote(fs[i])
}

var ifaceHeaderRe = regexp.MustCompile(`^interface (\S+)$`)

// PortNames reads the port names from a running config: `name "..."` in an
// AOS-S interface block, `description ...` in an AOS-CX one. A block for a
// range of ports ("interface 1-4") names each of them.
func PortNames(cfg string) map[string]string {
	out := map[string]string{}
	for _, b := range splitConfig(cfg) {
		m := ifaceHeaderRe.FindStringSubmatch(b.header)
		if m == nil || strings.HasPrefix(m[1], "vlan") || m[1] == "loopback" || m[1] == "mgmt" {
			continue
		}
		name := ""
		for _, l := range b.lines {
			switch {
			case strings.HasPrefix(l, "name "):
				name = unquote(strings.TrimSpace(strings.TrimPrefix(l, "name ")))
			case strings.HasPrefix(l, "description "):
				name = unquote(strings.TrimSpace(strings.TrimPrefix(l, "description ")))
			}
		}
		if name == "" {
			continue
		}
		for _, p := range ExpandPorts(m[1]) {
			out[p] = name
		}
	}
	return out
}
