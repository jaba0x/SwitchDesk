package core

import (
	"sort"
	"strings"

	"github.com/jaba0x/switchdesk/internal/aruba"
)

// Finding where a client is plugged in. Every switch on the path learns the
// client's MAC, so a plain lookup lists them all; the one that matters is the
// switch whose port has no other switch behind it (asked for on Reddit).

type MACHit struct {
	SwitchID   string `json:"switchId"`
	SwitchName string `json:"switchName"`
	Port       string `json:"port"`
	PortName   string `json:"portName,omitempty"`
	VLAN       string `json:"vlan"`
	MAC        string `json:"mac"`
	// Uplink names the switch seen through this port over LLDP/CDP, or the
	// trunk it belongs to; "" means this is the edge port.
	Uplink string `json:"uplink,omitempty"`
	Edge   bool   `json:"edge"`
	Others int    `json:"others"` // other addresses learned on the same port
}

// FindMAC looks a MAC address, or the start or end of one, up in every
// switch's address table. Edge ports come first.
func (m *Manager) FindMAC(query string) []MACHit {
	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.NewReplacer("-", "", ":", "", ".", "", " ", "").Replace(q)
	if len(q) < 4 {
		return nil
	}
	return m.macHits(func(bare string) bool {
		return strings.HasPrefix(bare, q) || strings.HasSuffix(bare, q) || (len(q) >= 6 && strings.Contains(bare, q))
	}, 200)
}

// ClientList is every address in the switches' tables, once, at the port
// the client is plugged into. Seen counts the switches that learned it.
type ClientEntry struct {
	MACHit
	Seen int `json:"seen"`
}

// Clients lists every learned MAC address once, with the edge port where
// there is one. Addresses only seen on uplinks (a client behind a switch
// SwitchDesk doesn't manage) keep their best uplink row.
func (m *Manager) Clients() []ClientEntry {
	hits := m.macHits(func(string) bool { return true }, 200000)
	var out []ClientEntry
	for i := 0; i < len(hits); {
		j := i
		for j < len(hits) && hits[j].MAC == hits[i].MAC {
			j++
		}
		out = append(out, ClientEntry{MACHit: hits[i], Seen: j - i})
		i = j
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].SwitchName != out[b].SwitchName {
			return out[a].SwitchName < out[b].SwitchName
		}
		return portLess(out[a].Port, out[b].Port)
	})
	return out
}

// portLess orders 1/1/2 before 1/1/10.
func portLess(a, b string) bool { return aruba.PortLess(a, b) }

func (m *Manager) macHits(match func(bare string) bool, limit int) []MACHit {
	_, infos, _, _ := m.views()
	topo := buildTopology(infos, nil, false)
	// switch id + local port -> the switch at the other end
	linked := map[string]string{}
	names := map[string]string{}
	for _, in := range infos {
		names[in.sw.ID] = displayName(in.sw, in.snap)
	}
	for _, e := range topo.Edges {
		if e.FromPort != "" {
			linked[e.From+"\x00"+e.FromPort] = names[e.To]
		}
		if e.ToPort != "" {
			linked[e.To+"\x00"+e.ToPort] = names[e.From]
		}
	}
	var hits []MACHit
	for _, in := range infos {
		if in.snap == nil || len(in.snap.MACs) == 0 {
			continue
		}
		perPort := map[string]int{}
		for _, e := range in.snap.MACs {
			perPort[e.Port]++
		}
		portName := map[string]string{}
		trunkOf := map[string]string{}
		for _, p := range in.snap.Ports {
			portName[p.Name] = p.Description
			if p.Trunk != "" {
				trunkOf[p.Name] = p.Trunk
			}
		}
		// a neighbor that is a switch, by LLDP capability, even if SwitchDesk doesn't manage it
		neighborSwitch := map[string]string{}
		for _, n := range in.snap.Neighbors {
			c := strings.ToLower(n.Capabilities)
			if strings.Contains(c, "bridge") || strings.Contains(c, "switch") || strings.Contains(c, "router") {
				if n.SysName != "" {
					neighborSwitch[n.LocalPort] = n.SysName
				} else if neighborSwitch[n.LocalPort] == "" {
					neighborSwitch[n.LocalPort] = n.ChassisID
				}
			}
		}
		for _, e := range in.snap.MACs {
			if !match(strings.ReplaceAll(e.MAC, ":", "")) {
				continue
			}
			h := MACHit{SwitchID: in.sw.ID, SwitchName: names[in.sw.ID], Port: e.Port, PortName: portName[e.Port], VLAN: e.VLAN, MAC: e.MAC, Others: perPort[e.Port] - 1}
			if up := linked[in.sw.ID+"\x00"+e.Port]; up != "" {
				h.Uplink = up
			} else if up := neighborSwitch[e.Port]; up != "" {
				h.Uplink = up
			} else if strings.HasPrefix(strings.ToLower(e.Port), "trk") || strings.HasPrefix(strings.ToLower(e.Port), "lag") {
				h.Uplink = e.Port
			} else if t := trunkOf[e.Port]; t != "" {
				h.Uplink = t
			}
			h.Edge = h.Uplink == ""
			hits = append(hits, h)
			if len(hits) >= limit {
				break
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].MAC != hits[j].MAC {
			return hits[i].MAC < hits[j].MAC
		}
		if hits[i].Edge != hits[j].Edge {
			return hits[i].Edge
		}
		if hits[i].Others != hits[j].Others {
			return hits[i].Others < hits[j].Others
		}
		return hits[i].SwitchName < hits[j].SwitchName
	})
	return hits
}
