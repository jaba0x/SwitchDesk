package aruba

import (
	"regexp"
	"strings"
)

// PortClient is one device logged in (or trying to) on an AOS-CX port, from
// "show port-access clients [detail]".
type PortClient struct {
	Port       string `json:"port"`
	MAC        string `json:"mac"`
	User       string `json:"user,omitempty"`
	IP         string `json:"ip,omitempty"`
	Method     string `json:"method,omitempty"` // dot1x, mac-auth, device-profile
	Status     string `json:"status,omitempty"`
	Role       string `json:"role,omitempty"`
	RoleStatus string `json:"roleStatus,omitempty"` // Applied, Failed, ...
	VLAN       string `json:"vlan,omitempty"`       // the untagged VLAN: access or native
	Tagged     string `json:"tagged,omitempty"`
	Session    string `json:"session,omitempty"`
}

// PortClientsCmd lists the clients with their VLAN; the plain table has no VLAN.
const PortClientsCmd = "show port-access clients detail"

var clientHead = regexp.MustCompile(`^Client\s+([0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}|[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4})\s*,?\s*(.*)$`)
var macField = regexp.MustCompile(`^(?:[0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}|[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}|[0-9A-Fa-f]{6}-[0-9A-Fa-f]{6})$`)

// ParsePortClients reads "show port-access clients detail", or the plain
// table when the detail form isn't there. ok is false when the switch
// rejected the command, so "no clients" is only claimed when it's known.
func ParsePortClients(out string) (clients []PortClient, ok bool) {
	if strings.TrimSpace(out) == "" || IsCLIError(out) {
		return nil, false
	}
	if cs := parseClientsDetail(out); len(cs) > 0 {
		return cs, true
	}
	return parseClientsTable(out), true
}

func parseClientsDetail(out string) []PortClient {
	var cs []PortClient
	var c *PortClient
	section := ""
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if m := clientHead.FindStringSubmatch(l); m != nil {
			cs = append(cs, PortClient{MAC: NormMAC(m[1]), User: strings.TrimSpace(m[2])})
			c, section = &cs[len(cs)-1], ""
			continue
		}
		if c == nil || l == "" || strings.Trim(l, "-=") == "" {
			continue
		}
		k, v, found := strings.Cut(l, ":")
		if !found {
			// "Session Details", "VLAN Details", "Authentication Details", ...
			if strings.HasSuffix(strings.ToLower(l), "details") {
				section = strings.ToLower(l)
			}
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch {
		case k == "port":
			c.Port = v
		case k == "ipv4 address" || (k == "ipv6 address" && c.IP == ""):
			c.IP = v
		case k == "session time":
			c.Session = v
		case k == "access" || k == "native untagged":
			c.VLAN = v
		case k == "vlans assigned" && c.VLAN == "":
			c.VLAN = v
		case k == "allowed trunk":
			c.Tagged = v
		case k == "role":
			c.Role = v
		case k == "status" && strings.HasPrefix(section, "authorization"):
			c.RoleStatus = v
		case k == "status":
			// "dot1x Authenticated", "mac-auth In-Progress"
			if f := strings.Fields(v); len(f) > 1 && isAuthMethod(f[0]) {
				c.Method, c.Status = f[0], strings.Join(f[1:], " ")
			} else {
				c.Status = v
			}
		}
	}
	return cs
}

func isAuthMethod(s string) bool {
	switch strings.ToLower(s) {
	case "dot1x", "mac-auth", "device-profile", "port-security":
		return true
	}
	return false
}

// parseClientsTable reads the summary table. Columns are cut at the header's
// positions, so a value with a space in it (Status "In Progress") stays whole.
func parseClientsTable(out string) []PortClient {
	var cs []PortClient
	var cols []string
	var at []int
	for _, raw := range strings.Split(out, "\n") {
		l := strings.TrimRight(raw, "\r")
		if cols == nil {
			if strings.Contains(l, "Port") && strings.Contains(l, "MAC") {
				for _, h := range []string{"Port", "MAC", "Onboarding", "Status", "Role", "Device", "VLAN"} {
					if i := strings.Index(l, h); i >= 0 {
						cols, at = append(cols, h), append(at, i)
					}
				}
			}
			continue
		}
		f := strings.Fields(l)
		if len(f) < 2 || !macField.MatchString(f[1]) {
			continue
		}
		c := PortClient{Port: f[0], MAC: NormMAC(f[1])}
		cell := func(name string) string {
			for i, h := range cols {
				if h != name || at[i] >= len(l) {
					continue
				}
				end := len(l)
				for j := range at {
					if at[j] > at[i] && at[j] < end {
						end = at[j]
					}
				}
				return strings.TrimSpace(l[at[i]:end])
			}
			return ""
		}
		c.Method, c.Status, c.Role, c.VLAN = cell("Onboarding"), cell("Status"), cell("Role"), cell("VLAN")
		cs = append(cs, c)
	}
	return cs
}
