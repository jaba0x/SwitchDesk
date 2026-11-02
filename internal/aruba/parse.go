package aruba

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------- helpers ----------

var nonAlnum = regexp.MustCompile(`[^a-z0-9%]+`)

func normKey(k string) string { return nonAlnum.ReplaceAllString(strings.ToLower(k), "") }

var colonSpaces = regexp.MustCompile(`\s+:\s?`)
var bigGap = regexp.MustCompile(`\s{3,}`)

// parseKV reads "Key : Value" pairs, including the two-column layout AOS-S uses
// ("Software revision  : WC.16.10   Base MAC Addr  : 001122-334455").
// The first occurrence of a key wins.
func parseKV(text string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		line = colonSpaces.ReplaceAllString(strings.TrimSpace(line), ": ")
		for _, tok := range bigGap.Split(line, -1) {
			i := strings.Index(tok, ": ")
			if i <= 0 {
				if strings.HasSuffix(tok, ":") && len(tok) > 1 {
					k := normKey(tok[:len(tok)-1])
					if _, ok := m[k]; !ok {
						m[k] = ""
					}
				}
				continue
			}
			k := normKey(tok[:i])
			v := strings.TrimSpace(tok[i+2:])
			if k == "" {
				continue
			}
			if _, ok := m[k]; !ok {
				m[k] = v
			}
		}
	}
	return m
}

func first(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[normKey(k)]; ok && v != "" {
			return v
		}
	}
	return ""
}

func atoiLoose(s string) int {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	f := strings.Fields(s)
	if len(f) == 0 {
		return -1
	}
	n, err := strconv.Atoi(strings.ReplaceAll(f[0], ",", ""))
	if err != nil {
		return -1
	}
	return n
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// IsCLIError reports whether output looks like the switch rejected the command.
func IsCLIError(out string) bool {
	l := strings.ToLower(out)
	return strings.Contains(l, "invalid input") || strings.Contains(l, "unknown command") ||
		strings.Contains(l, "% invalid") || strings.Contains(l, "command not found") ||
		strings.Contains(l, "incomplete input") || strings.Contains(l, "ambiguous input")
}

// NormMAC turns any MAC notation into aa:bb:cc:dd:ee:ff. Returns "" if not a MAC.
func NormMAC(s string) string {
	h := strings.ToLower(regexp.MustCompile(`[^0-9a-fA-F]`).ReplaceAllString(s, ""))
	if len(h) != 12 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(h[i : i+2])
	}
	return b.String()
}

// ---------- family detection ----------

var cxVersionRe = regexp.MustCompile(`\b[A-Z]{2}\.10\.\d+`)

func DetectFamily(showVersion, banner string) Family {
	t := showVersion + "\n" + banner
	l := strings.ToLower(t)
	switch {
	case strings.Contains(l, "arubaos-cx") || strings.Contains(l, "aos-cx") || cxVersionRe.MatchString(t):
		return FamilyAOSCX
	case strings.Contains(l, "instant on") || strings.Contains(l, "instanton") ||
		regexp.MustCompile(`\b19[3-6]0\b|\b1830\b`).MatchString(t) && !strings.Contains(l, "image stamp"):
		return FamilyInstantOn
	case strings.Contains(l, "image stamp") || strings.Contains(l, "procurve") ||
		regexp.MustCompile(`\b(WC|KB|YA|YB|WB|RA|KA|K|WA|YC|KB)\.1[56]\.\d+`).MatchString(t):
		return FamilyAOSS
	}
	return FamilyAOSS
}

// ---------- system / version ----------

func ParseSystem(text string) Facts {
	m := parseKV(text)
	f := Facts{CPU: -1, Memory: -1}
	f.Hostname = first(m, "System Name", "Hostname")
	f.Model = first(m, "Product Name", "Platform Name")
	f.Serial = first(m, "Serial Number", "Chassis Serial Nbr", "Chassis Serial Number")
	f.BaseMAC = first(m, "Base MAC Addr", "Base MAC Address")
	f.Software = first(m, "Software revision", "ArubaOS-CX Version", "AOS-CX Version", "Software Version", "Description")
	f.Uptime = first(m, "Up Time", "Uptime")
	f.Location = first(m, "System Location", "Location")
	f.Contact = first(m, "System Contact", "Contact")
	if v := first(m, "CPU Util (%)", "CPU Util", "CPU Utilization (%)"); v != "" {
		f.CPU = atoiLoose(v)
	}
	if v := first(m, "Memory Usage (%)", "Memory Utilization (%)"); v != "" {
		f.Memory = atoiLoose(v)
	} else {
		// AOS-S prints "Memory - Total : n" and "Free : n"
		total := atoiLoose(first(m, "Memory - Total", "Memory Total", "Total"))
		free := atoiLoose(first(m, "Free"))
		if total > 0 && free >= 0 && free <= total {
			f.Memory = int(float64(total-free) / float64(total) * 100)
		}
	}
	return f
}

var modelRe = regexp.MustCompile(`\b(J[A-Z0-9]{4,5}A?)\s+([^\n]*?(?:Switch|Swch|Swtch|Sw)\b[^\n]*)`)

// ModelFromText pulls something like "JL256A 2930F-48G-PoE+-4SFP+ Switch" out of
// any output (show version, show modules, LLDP descriptions).
func ModelFromText(s string) string {
	if m := modelRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1] + " " + m[2])
	}
	return ""
}

// ---------- spanning tree ----------

var stpRoleRe = regexp.MustCompile(`(?i)^\s*(\S+)\s+.*?\b(Root|Designated|Alternate|Backup|Disabled|Master|Disabled)\b\s+(Forwarding|Blocking|Discarding|Learning|Listening|Disabled|Down)\b(.*)$`)

func ParseSTP(text string) STP {
	s := STP{}
	if strings.TrimSpace(text) == "" || IsCLIError(text) {
		return s
	}
	s.Known = true
	m := parseKV(text)
	en := strings.ToLower(first(m, "STP Enabled", "Spanning tree status", "Spanning Tree Status"))
	s.Enabled = strings.HasPrefix(en, "yes") || strings.HasPrefix(en, "enabled") || strings.HasPrefix(en, "on")
	s.Mode = first(m, "Force Version", "Protocol", "Mode", "STP Mode")
	if s.Mode == "" {
		if i := strings.Index(strings.ToLower(text), "protocol:"); i >= 0 {
			s.Mode = strings.Fields(text[i+len("protocol:"):] + " ")[0]
		}
	}
	if s.Mode == "" {
		switch {
		case strings.Contains(text, "MST"):
			s.Mode = "MSTP"
		case strings.Contains(strings.ToUpper(text), "RPVST"):
			s.Mode = "RPVST+"
		}
	}
	s.BridgePriority = first(m, "Switch Priority", "Bridge Priority")
	s.BridgeMAC = NormMAC(first(m, "Switch MAC Address", "Bridge MAC Address"))
	s.RootPriority = first(m, "CST Root Priority", "IST Regional Root Priority", "Root Priority")
	s.RootMAC = NormMAC(first(m, "CST Root MAC Address", "Root MAC Address", "IST Regional Root MAC Address"))
	s.RootPort = first(m, "CST Root Port", "Root Port")
	s.TopologyChanges = atoiLoose(first(m, "Topology Change Count", "Number of topology changes", "Topology Changes"))
	if s.TopologyChanges < 0 {
		s.TopologyChanges = 0
	}
	s.LastChange = first(m, "Time Since Last Change", "Last topology change occurred")

	// AOS-CX layout: "Root ID Priority : 4096" / "MAC-Address: ..." then "Bridge ID ..."
	lines := strings.Split(text, "\n")
	section := ""
	for _, l := range lines {
		t := strings.TrimSpace(l)
		lt := strings.ToLower(t)
		switch {
		case strings.HasPrefix(lt, "root id"):
			section = "root"
		case strings.HasPrefix(lt, "bridge id"):
			section = "bridge"
		case t == "" || strings.HasPrefix(lt, "port "):
			if strings.HasPrefix(lt, "port ") {
				section = ""
			}
		}
		if section == "" {
			continue
		}
		if i := strings.Index(lt, "priority"); i >= 0 {
			if j := strings.Index(t[i:], ":"); j >= 0 {
				v := strings.Fields(t[i+j+1:] + " ")
				if len(v) > 0 {
					if section == "root" && s.RootPriority == "" {
						s.RootPriority = v[0]
					} else if section == "bridge" && s.BridgePriority == "" {
						s.BridgePriority = v[0]
					}
				}
			}
		}
		if i := strings.Index(lt, "mac-address"); i >= 0 {
			if j := strings.Index(t[i:], ":"); j >= 0 {
				mac := NormMAC(strings.Fields(t[i+j+1:] + " ")[0])
				if section == "root" && s.RootMAC == "" {
					s.RootMAC = mac
				} else if section == "bridge" && s.BridgeMAC == "" {
					s.BridgeMAC = mac
				}
			}
		}
	}

	seen := map[string]bool{}
	for _, l := range lines {
		mm := stpRoleRe.FindStringSubmatch(l)
		if mm == nil {
			continue
		}
		port := mm[1]
		if strings.EqualFold(port, "port") || strings.Contains(port, "-") && !strings.Contains(port, "/") || seen[port] {
			continue
		}
		seen[port] = true
		p := STPPort{Port: port, Role: strings.Title(strings.ToLower(mm[2])), State: strings.Title(strings.ToLower(mm[3]))}
		fs := strings.Fields(l)
		for _, f := range fs[1:] {
			if n := atoiLoose(f); n > 0 && p.Cost == "" && f != "128" {
				p.Cost = f
				break
			}
		}
		s.Ports = append(s.Ports, p)
		if p.Role == "Root" && s.RootPort == "" {
			s.RootPort = port
		}
	}
	if strings.Contains(strings.ToLower(text), "this switch is root") || strings.Contains(strings.ToLower(text), "this bridge is the root") {
		s.IsRoot = true
	} else if s.BridgeMAC != "" && s.RootMAC != "" {
		s.IsRoot = s.BridgeMAC == s.RootMAC
	} else if s.Enabled && s.RootPort == "" && len(s.Ports) > 0 {
		s.IsRoot = true
	}
	if s.RootPort == "This switch is root" {
		s.RootPort = ""
		s.IsRoot = true
	}
	return s
}

// ---------- interfaces ----------

// 24, A1, 1/1/49 (AOS-CX), and on stacked AOS-S 1/24 and 1/A1 (issue #14)
var portNameRe = regexp.MustCompile(`^((\d+/){0,2}[A-Z]?\d+|lag\d+|Trk\d+)$`)

func ParseInterfacesBrief(text string) []Port {
	var out []Port
	for _, line := range strings.Split(text, "\n") {
		line = strings.ReplaceAll(line, "|", " ")
		fs := strings.Fields(line)
		if len(fs) < 3 {
			continue
		}
		// AOS-S lists a trunk member as "13-Trk3" or "1/A1-Trk1"
		name, trunk := fs[0], ""
		if i := strings.Index(name, "-Trk"); i > 0 {
			name, trunk = name[:i], name[i+1:]
		}
		if !portNameRe.MatchString(name) {
			continue
		}
		p := Port{Name: name, Trunk: trunk}
		idx := -1
		for i := 1; i < len(fs)-1; i++ {
			a, b := strings.ToLower(fs[i]), strings.ToLower(fs[i+1])
			if (a == "yes" || a == "no") && (b == "up" || b == "down") {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		p.Enabled = strings.EqualFold(fs[idx], "yes")
		p.Up = strings.EqualFold(fs[idx+1], "up")
		rest := fs[idx+2:]
		if idx >= 1 && !strings.EqualFold(fs[1], "yes") && !strings.EqualFold(fs[1], "no") {
			p.Type = fs[1]
		}
		// AOS-CX: Port NativeVLAN Mode Type Enabled Status Reason... Speed Description
		if idx >= 3 && atoiLoose(fs[1]) >= 0 && (fs[2] == "access" || fs[2] == "trunk" || fs[2] == "routed" || fs[2] == "--") {
			p.VLAN = fs[1]
			p.Mode = fs[2]
			p.Type = fs[3]
			// speed is the last numeric (or "--") token before description
			for j := len(rest) - 1; j >= 0; j-- {
				if atoiLoose(rest[j]) > 0 {
					p.Speed = rest[j]
					if j+1 < len(rest) {
						d := strings.Join(rest[j+1:], " ")
						if d != "--" {
							p.Description = d
						}
					}
					break
				}
			}
		} else if len(rest) > 0 {
			// AOS-S: ... Enabled Status Mode(1000FDx) MDI Flow Bcast
			p.Speed = rest[0]
		}
		out = append(out, p)
	}
	return out
}

// ---------- neighbors (LLDP / CDP) ----------

func ParseNeighbors(text, proto string) []Neighbor {
	if IsCLIError(text) {
		return nil
	}
	var out []Neighbor
	var cur *Neighbor
	var lastKey string
	flush := func() {
		if cur != nil && cur.LocalPort != "" && (cur.SysName != "" || cur.ChassisID != "" || cur.MgmtIP != "") {
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		i := strings.Index(t, ":")
		if i <= 0 {
			continue
		}
		key := normKey(t[:i])
		val := strings.TrimSpace(t[i+1:])
		switch key {
		case "localport", "port":
			if cur != nil && cur.LocalPort != "" && cur.LocalPort != val {
				flush()
			}
			if cur == nil {
				cur = &Neighbor{Protocol: proto}
			}
			cur.LocalPort = strings.TrimPrefix(val, "Port ")
		}
		if cur == nil {
			continue
		}
		switch key {
		case "sysname", "systemname", "neighborchassisname", "deviceid", "neighbordeviceid", "chassisname":
			if cur.SysName == "" {
				cur.SysName = val
			}
		case "chassisid", "neighborchassisid":
			if cur.ChassisID == "" {
				cur.ChassisID = val
			}
		case "portid", "neighborportid", "deviceport", "neighborport", "portidoutgoingport":
			if cur.PortID == "" {
				cur.PortID = strings.TrimPrefix(val, "Port ")
			}
		case "portdescr", "neighborportdesc", "portdescription":
			cur.PortDescr = val
		case "systemdescr", "neighborchassisdescription", "systemdescription", "platform", "version":
			if cur.Description == "" {
				cur.Description = val
			}
		case "systemcapabilitiesenabled", "chassiscapabilitiesenabled", "capability", "capabilities":
			cur.Capabilities = val
		case "address", "neighbormanagementaddress", "managementaddress", "ipaddress", "mgmtaddress":
			if cur.MgmtIP == "" && regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(val) {
				cur.MgmtIP = val
			}
		}
		lastKey = key
	}
	_ = lastKey
	flush()
	// The device ID on CDP is often a MAC when the peer is a phone.
	for i := range out {
		if out[i].SysName == "" && out[i].ChassisID != "" {
			out[i].SysName = out[i].ChassisID
		}
	}
	return out
}

// ---------- environment ----------

var fanFailRe = regexp.MustCompile(`(\d+)\s*/\s*\d+\s+Fans? in Failure State`)
var envBadRe = regexp.MustCompile(`(?i)\b(fault|faulty|failed|failure|not powered|aux not powered|critical|emergency|over ?temp(erature)?\s*:\s*yes|alarm|not ok)\b`)

func ParseEnv(kind, text string) []EnvItem {
	if strings.TrimSpace(text) == "" || IsCLIError(text) {
		return nil
	}
	var items []EnvItem
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "---") {
			continue
		}
		lt := strings.ToLower(t)
		if strings.Contains(lt, "have been in failure") || strings.Contains(lt, "has been in failure") {
			continue // historical counter, not current state
		}
		if m := fanFailRe.FindStringSubmatch(t); m != nil {
			n := atoiLoose(m[1])
			items = append(items, EnvItem{Kind: kind, Name: "Fan summary", Status: t, OK: n == 0})
			continue
		}
		// skip header lines
		if strings.Contains(lt, "failures") && strings.Contains(lt, "state") ||
			strings.HasPrefix(lt, "ps#") || strings.HasPrefix(lt, "num") || strings.HasPrefix(lt, "mbr") ||
			strings.HasPrefix(lt, "name ") || strings.HasPrefix(lt, "fan information") || strings.HasPrefix(lt, "power supply status") {
			continue
		}
		fs := strings.Fields(strings.ReplaceAll(t, "|", " "))
		if len(fs) < 2 {
			continue
		}
		bad := envBadRe.MatchString(t)
		ok := !bad && (strings.Contains(lt, " ok") || strings.Contains(lt, "powered") || strings.Contains(lt, "normal") ||
			strings.Contains(lt, "good") || strings.Contains(lt, "ready"))
		if !bad && !ok {
			continue // not a status row
		}
		if strings.Contains(lt, "not present") || strings.Contains(lt, "absent") || strings.Contains(lt, "empty") {
			continue
		}
		items = append(items, EnvItem{Kind: kind, Name: fs[0], Status: t, OK: !bad})
	}
	return items
}

// ---------- logs ----------

var aosLogRe = regexp.MustCompile(`^([IWMEDC])\s+\d{2}/\d{2}/\d{2}\s`)

func ParseLogs(text string, max int) []LogLine {
	var out []LogLine
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "----") || strings.HasPrefix(strings.ToLower(t), "keys:") ||
			strings.HasPrefix(t, "Event Log") || strings.HasPrefix(t, "----  ") {
			continue
		}
		sev := "I"
		if m := aosLogRe.FindStringSubmatch(t); m != nil {
			sev = m[1]
			if sev == "C" {
				sev = "E"
			}
		} else {
			lt := strings.ToLower(t)
			switch {
			case strings.Contains(lt, "|crit") || strings.Contains(lt, "|emer") || strings.Contains(lt, "|alert") || strings.Contains(lt, "|err"):
				sev = "E"
			case strings.Contains(lt, "|warn"):
				sev = "W"
			case !regexp.MustCompile(`\d{4}-\d{2}-\d{2}|\d{2}:\d{2}:\d{2}`).MatchString(t):
				continue // not a log line
			}
		}
		out = append(out, LogLine{Severity: sev, Text: t})
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// ---------- port lists ----------

var rangePart = regexp.MustCompile(`^([A-Za-z]*)(\d+)-([A-Za-z]*)(\d+)$`)

// 1/1/1-1/1/24 (AOS-CX), 1/1-1/24 and 1/A1-1/A4 (stacked AOS-S)
var cxRange = regexp.MustCompile(`^((?:\d+/)+[A-Za-z]*)(\d+)-((?:\d+/)+[A-Za-z]*)?(\d+)$`)

// ExpandPorts turns "1-4,A1-A2,49" or "1/1/1-1/1/3" into individual ports.
func ExpandPorts(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if m := cxRange.FindStringSubmatch(part); m != nil && (m[3] == "" || m[3] == m[1]) {
			a, _ := strconv.Atoi(m[2])
			b, _ := strconv.Atoi(m[4])
			for i := a; i <= b && i-a < 1024; i++ {
				out = append(out, m[1]+strconv.Itoa(i))
			}
			continue
		}
		if m := rangePart.FindStringSubmatch(part); m != nil && m[1] == m[3] {
			a, _ := strconv.Atoi(m[2])
			b, _ := strconv.Atoi(m[4])
			for i := a; i <= b && i-a < 1024; i++ {
				out = append(out, m[1]+strconv.Itoa(i))
			}
			continue
		}
		out = append(out, part)
	}
	return out
}

// CompressPorts is the inverse of ExpandPorts, for display and CLI generation.
func CompressPorts(ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	type key struct{ prefix string }
	groups := map[string][]int{}
	var order []string
	var others []string
	re := regexp.MustCompile(`^(.*?)(\d+)$`)
	for _, p := range ports {
		m := re.FindStringSubmatch(p)
		if m == nil {
			others = append(others, p)
			continue
		}
		n, _ := strconv.Atoi(m[2])
		if _, ok := groups[m[1]]; !ok {
			order = append(order, m[1])
		}
		groups[m[1]] = append(groups[m[1]], n)
	}
	var parts []string
	for _, pre := range order {
		ns := groups[pre]
		sort.Ints(ns)
		start, prev := ns[0], ns[0]
		emit := func() {
			if start == prev {
				parts = append(parts, pre+strconv.Itoa(start))
			} else {
				parts = append(parts, pre+strconv.Itoa(start)+"-"+pre+strconv.Itoa(prev))
			}
		}
		for _, n := range ns[1:] {
			if n == prev {
				continue
			}
			if n == prev+1 {
				prev = n
				continue
			}
			emit()
			start, prev = n, n
		}
		emit()
	}
	parts = append(parts, others...)
	return strings.Join(parts, ",")
}

// ExpandInts parses VLAN id lists like "1,20,30-35".
func ExpandInts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if a, b, ok := strings.Cut(part, "-"); ok {
			x, e1 := strconv.Atoi(strings.TrimSpace(a))
			y, e2 := strconv.Atoi(strings.TrimSpace(b))
			if e1 == nil && e2 == nil && y >= x && y-x < 4095 {
				for i := x; i <= y; i++ {
					out = append(out, i)
				}
			}
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		}
	}
	return out
}
