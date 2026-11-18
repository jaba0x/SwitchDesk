package aruba

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// VLANChange describes one VLAN edit coming from the UI.
type VLANChange struct {
	Action      string   `json:"action"` // create, update, delete
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	AddTagged   []string `json:"addTagged"`
	AddUntagged []string `json:"addUntagged"`
	RemovePorts []string `json:"removePorts"`
	// IDs deletes several VLANs in one go (issue #38); ID is ignored then.
	IDs  []int `json:"ids,omitempty"`
	Save bool  `json:"save"`
}

// deleting lists the VLANs a delete removes.
func (c VLANChange) deleting() []int {
	if c.Action != "delete" {
		return nil
	}
	if len(c.IDs) > 0 {
		return c.IDs
	}
	return []int{c.ID}
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9 _.\-]{1,32}$`)
var safePort = regexp.MustCompile(`^((\d+/){0,2}[A-Za-z]{0,3}\d+|lag\d+|Trk\d+)$`)

func (c VLANChange) validate(current []VLAN) error {
	if c.Action == "delete" {
		seen := map[int]bool{}
		for _, id := range c.deleting() {
			if id < 1 || id > 4094 {
				return fmt.Errorf("VLAN ID must be 1-4094")
			}
			if id == 1 {
				return fmt.Errorf("VLAN 1 can't be deleted")
			}
			if seen[id] {
				return fmt.Errorf("VLAN %d is listed twice", id)
			}
			seen[id] = true
			if !hasVLAN(current, id) && len(current) > 0 {
				return fmt.Errorf("VLAN %d doesn't exist on this switch", id)
			}
		}
		return nil
	}
	if c.ID < 1 || c.ID > 4094 {
		return fmt.Errorf("VLAN ID must be 1-4094")
	}
	switch c.Action {
	case "create", "update", "delete":
	default:
		return fmt.Errorf("unknown action %q", c.Action)
	}
	if c.Action == "delete" && c.ID == 1 {
		return fmt.Errorf("VLAN 1 can't be deleted")
	}
	if c.Name != "" && !safeName.MatchString(c.Name) {
		return fmt.Errorf("name may use letters, digits, space, _ . - (max 32)")
	}
	for _, list := range [][]string{c.AddTagged, c.AddUntagged, c.RemovePorts} {
		for _, p := range list {
			if !safePort.MatchString(p) {
				return fmt.Errorf("invalid port %q", p)
			}
		}
	}
	exists := false
	for _, v := range current {
		if v.ID == c.ID {
			exists = true
		}
	}
	if c.Action == "create" && exists {
		return fmt.Errorf("VLAN %d already exists", c.ID)
	}
	if (c.Action == "update" || c.Action == "delete") && !exists && len(current) > 0 {
		return fmt.Errorf("VLAN %d doesn't exist on this switch", c.ID)
	}
	return nil
}

func hasVLAN(vlans []VLAN, id int) bool {
	for _, v := range vlans {
		if v.ID == id {
			return true
		}
	}
	return false
}

// BuildVLANCommands returns the exact CLI lines that will be sent.
func BuildVLANCommands(f Family, c VLANChange, current []VLAN) ([]string, error) {
	if err := c.validate(current); err != nil {
		return nil, err
	}
	if f == FamilyInstantOn {
		return nil, fmt.Errorf("VLAN changes over SSH aren't supported on Instant On switches; use the Instant On portal or web UI")
	}
	cmds := []string{"configure terminal"}
	switch f {
	case FamilyAOSCX:
		if c.Action == "delete" {
			for _, id := range c.deleting() {
				cmds = append(cmds, fmt.Sprintf("no vlan %d", id))
			}
			break
		}
		cmds = append(cmds, fmt.Sprintf("vlan %d", c.ID))
		if c.Name != "" {
			cmds = append(cmds, "name "+c.Name)
		}
		cmds = append(cmds, "exit")
		for _, p := range c.AddUntagged {
			cmds = append(cmds, "interface "+p, "no routing", fmt.Sprintf("vlan access %d", c.ID), "exit")
		}
		for _, p := range c.AddTagged {
			cmds = append(cmds, "interface "+p, "no routing", fmt.Sprintf("vlan trunk allowed %d", c.ID), "exit")
		}
		for _, p := range c.RemovePorts {
			if memberTagged(current, c.ID, p) {
				cmds = append(cmds, "interface "+p, fmt.Sprintf("no vlan trunk allowed %d", c.ID), "exit")
			} else {
				cmds = append(cmds, "interface "+p, "no vlan access", "exit")
			}
		}
	default: // AOS-S
		if c.Action == "delete" {
			for _, id := range c.deleting() {
				cmds = append(cmds, fmt.Sprintf("no vlan %d", id))
			}
			break
		}
		cmds = append(cmds, fmt.Sprintf("vlan %d", c.ID))
		if c.Name != "" {
			cmds = append(cmds, fmt.Sprintf("name \"%s\"", c.Name))
		}
		if len(c.AddUntagged) > 0 {
			cmds = append(cmds, "untagged "+CompressPorts(c.AddUntagged))
		}
		if len(c.AddTagged) > 0 {
			cmds = append(cmds, "tagged "+CompressPorts(c.AddTagged))
		}
		var rmT, rmU []string
		for _, p := range c.RemovePorts {
			if memberTagged(current, c.ID, p) {
				rmT = append(rmT, p)
			} else {
				rmU = append(rmU, p)
			}
		}
		if len(rmT) > 0 {
			cmds = append(cmds, "no tagged "+CompressPorts(rmT))
		}
		if len(rmU) > 0 {
			cmds = append(cmds, "no untagged "+CompressPorts(rmU))
		}
		cmds = append(cmds, "exit")
	}
	cmds = append(cmds, "end")
	if c.Save {
		cmds = append(cmds, "write memory")
	}
	return cmds, nil
}

// AfterChange returns the VLAN table as it should be once c has been accepted,
// so the UI shows the change before the switch is read again. A port can only
// be untagged in one VLAN, so adding it untagged takes it out of the others.
func AfterChange(vlans []VLAN, c VLANChange) []VLAN {
	without := func(ports []string, drop map[string]bool) []string {
		var out []string
		for _, p := range ports {
			if !drop[p] {
				out = append(out, p)
			}
		}
		return out
	}
	set := func(ports []string) map[string]bool {
		m := map[string]bool{}
		for _, p := range ports {
			m[p] = true
		}
		return m
	}
	untagged, removed := set(c.AddUntagged), set(c.RemovePorts)
	gone := map[int]bool{}
	for _, id := range c.deleting() {
		gone[id] = true
	}
	var out []VLAN
	found := false
	for _, v := range vlans {
		if gone[v.ID] {
			continue
		}
		if v.ID == c.ID {
			found = true
			if c.Name != "" {
				v.Name = c.Name
			}
			tagged := set(c.AddTagged)
			// a port added again isn't listed twice
			v.Untagged = append(without(without(v.Untagged, removed), untagged), c.AddUntagged...)
			v.Tagged = append(without(without(v.Tagged, removed), tagged), c.AddTagged...)
		} else {
			v.Untagged = without(v.Untagged, untagged)
		}
		out = append(out, v)
	}
	if c.Action == "create" && !found {
		out = append(out, VLAN{ID: c.ID, Name: c.Name, Untagged: c.AddUntagged, Tagged: c.AddTagged})
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	return out
}

func memberTagged(vlans []VLAN, id int, port string) bool {
	for _, v := range vlans {
		if v.ID == id {
			for _, p := range v.Tagged {
				if p == port {
					return true
				}
			}
		}
	}
	return false
}

// ApplyResult is the transcript of a change.
type ApplyResult struct {
	OK         bool     `json:"ok"`
	Transcript []string `json:"transcript"`
	Error      string   `json:"error"`
}

// ApplyCommands sends config lines one by one and stops at the first rejection.
// "write memory" is only reached if every previous line was accepted.
func ApplyCommands(ctx context.Context, r Runner, cmds []string) ApplyResult {
	res := ApplyResult{}
	for _, c := range cmds {
		out, err := r.Run(ctx, c, 30*time.Second)
		res.Transcript = append(res.Transcript, "> "+c)
		if strings.TrimSpace(out) != "" {
			res.Transcript = append(res.Transcript, out)
		}
		if err != nil {
			res.Error = fmt.Sprintf("%s: %v", c, err)
			r.Run(ctx, "end", 5*time.Second)
			return res
		}
		if IsCLIError(out) || strings.Contains(strings.ToLower(out), "error") && !strings.Contains(strings.ToLower(out), "0 error") {
			res.Error = fmt.Sprintf("switch rejected %q", c)
			r.Run(ctx, "end", 5*time.Second)
			return res
		}
	}
	res.OK = true
	return res
}
