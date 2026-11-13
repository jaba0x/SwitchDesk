package aruba

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Port names (issue #32): the "name" of an AOS-S port, the "description" of
// an AOS-CX interface.

// PortNameChange sets or clears the names of some ports.
type PortNameChange struct {
	Names map[string]string `json:"names"` // port -> new name; "" clears it
	// LLDP lists the ports whose new name was taken from their LLDP neighbor.
	// SwitchDesk remembers those, so it may update them when the neighbor
	// changes; any other name is the user's and is never replaced.
	LLDP []string `json:"lldp,omitempty"`
	Save bool     `json:"save"`
}

// MaxPortName is the longest name both families accept.
const MaxPortName = 64

// CheckPortName says why a name can't be sent to the switch, or nil.
func CheckPortName(name string) error {
	if len(name) > MaxPortName {
		return fmt.Errorf("%q is longer than %d characters", name, MaxPortName)
	}
	for _, r := range name {
		switch {
		case r < 0x20 || r > 0x7e:
			return fmt.Errorf("%q: use plain letters, digits and punctuation (no accents or emoji)", name)
		case r == '"' || r == '?' || r == '\\':
			// the switch CLI ends a quoted name at ", and answers ? with help
			return fmt.Errorf("%q: the characters \" ? and \\ can't be used in a port name", name)
		}
	}
	return nil
}

// CleanPortName turns a neighbor's system name into one CheckPortName accepts,
// or "" when nothing usable is left.
func CleanPortName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '?' && r != '\\' {
			b.WriteRune(r)
		} else if r == '\t' {
			b.WriteRune(' ')
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len(out) > MaxPortName {
		out = strings.TrimSpace(out[:MaxPortName])
	}
	return out
}

func portNameOrder(names map[string]string) []string {
	out := make([]string, 0, len(names))
	for p := range names {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return PortLess(out[i], out[j]) })
	return out
}

// BuildPortNameCommands returns the CLI lines that set the names in c. Ports
// whose name doesn't change are left out.
func BuildPortNameCommands(f Family, c PortNameChange, ports []Port) ([]string, error) {
	if f == FamilyInstantOn {
		return nil, errors.New("port names over SSH aren't supported on Instant On switches; use the Instant On app or the switch's web page")
	}
	cur := map[string]string{}
	for _, p := range ports {
		cur[p.Name] = p.Description
	}
	cmds := []string{"configure terminal"}
	n := 0
	for _, p := range portNameOrder(c.Names) {
		name := strings.TrimSpace(c.Names[p])
		old, ok := cur[p]
		if !ok || !safePort.MatchString(p) {
			return nil, fmt.Errorf("port %s isn't on this switch", p)
		}
		if err := CheckPortName(name); err != nil {
			return nil, err
		}
		if name == old {
			continue
		}
		n++
		cmds = append(cmds, "interface "+p)
		switch {
		case f == FamilyAOSCX && name == "":
			cmds = append(cmds, "no description")
		case f == FamilyAOSCX:
			cmds = append(cmds, "description "+name)
		case name == "":
			cmds = append(cmds, "no name")
		default:
			cmds = append(cmds, fmt.Sprintf("name \"%s\"", name))
		}
		cmds = append(cmds, "exit")
	}
	if n == 0 {
		return nil, errors.New("nothing changed")
	}
	cmds = append(cmds, "end")
	if c.Save {
		cmds = append(cmds, "write memory")
	}
	return cmds, nil
}

// AfterPortNames returns the ports with the new names, so the UI shows them
// before the switch is read again.
func AfterPortNames(ports []Port, c PortNameChange) []Port {
	out := make([]Port, len(ports))
	copy(out, ports)
	for i := range out {
		if n, ok := c.Names[out[i].Name]; ok {
			out[i].Description = strings.TrimSpace(n)
		}
	}
	return out
}

// PortNameSuggestion is a name SwitchDesk would give a port from its LLDP neighbor.
type PortNameSuggestion struct {
	Port     string `json:"port"`
	Current  string `json:"current"`
	Name     string `json:"name"`
	Neighbor string `json:"neighbor"` // the neighbor's system name as it reported it
	// Replaces is set when Current was itself taken from LLDP earlier and the
	// neighbor has changed since.
	Replaces bool `json:"replaces,omitempty"`
}

// SuggestPortNames names ports after the device LLDP sees on them. A port
// only gets a suggestion when it has no name, or when its name is the one
// SwitchDesk took from LLDP before (fromLLDP); a name someone typed is left alone.
func SuggestPortNames(s *Snapshot, fromLLDP map[string]string) []PortNameSuggestion {
	if s == nil || s.Family == FamilyInstantOn {
		return nil
	}
	cur := map[string]string{}
	for _, p := range s.Ports {
		cur[p.Name] = p.Description
	}
	type pick struct {
		name, raw string
		bridge    bool
	}
	best := map[string]pick{}
	for _, n := range s.Neighbors {
		if n.Protocol == "cloud" {
			continue
		}
		name := CleanPortName(n.SysName)
		if name == "" {
			continue
		}
		c := strings.ToLower(n.Capabilities)
		bridge := strings.Contains(c, "bridge") || strings.Contains(c, "switch") || strings.Contains(c, "router")
		// a port with several neighbors (a phone with a PC behind it, an
		// unmanaged switch): prefer the switch, then the first one seen
		if b, ok := best[n.LocalPort]; ok && (b.bridge || !bridge) {
			continue
		}
		best[n.LocalPort] = pick{name, n.SysName, bridge}
	}
	var out []PortNameSuggestion
	for port, b := range best {
		old, ok := cur[port]
		if !ok || old == b.name {
			continue
		}
		prev, ours := fromLLDP[port]
		if old != "" && !(ours && prev == old) {
			continue
		}
		out = append(out, PortNameSuggestion{Port: port, Current: old, Name: b.name, Neighbor: b.raw, Replaces: old != ""})
	}
	sort.Slice(out, func(i, j int) bool { return PortLess(out[i].Port, out[j].Port) })
	return out
}

// PortLess orders 1/1/2 before 1/1/10 and 2 before 10.
func PortLess(a, b string) bool {
	pa, pb := strings.FieldsFunc(a, func(r rune) bool { return r == '/' }), strings.FieldsFunc(b, func(r rune) bool { return r == '/' })
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		if ea == nil && eb == nil {
			return na < nb
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}
