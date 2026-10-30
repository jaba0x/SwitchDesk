package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/store"
)

type Severity string

const (
	Critical Severity = "critical"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case Critical:
		return 0
	case Warning:
		return 1
	}
	return 2
}

type Issue struct {
	SwitchID   string   `json:"switchId"`
	SwitchName string   `json:"switchName"`
	Severity   Severity `json:"severity"`
	Category   string   `json:"category"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
}

// evaluate turns one switch's status and last snapshot into issues.
func evaluate(sw store.Switch, st *Status, snap *aruba.Snapshot, set store.Settings) []Issue {
	name := displayName(sw, snap)
	var out []Issue
	add := func(sev Severity, cat, title, detail string) {
		out = append(out, Issue{SwitchID: sw.ID, SwitchName: name, Severity: sev, Category: cat, Title: title, Detail: detail})
	}

	if st.Checked && !st.Online && sw.Cloud != nil {
		d := st.PingError
		if !st.Since.IsZero() {
			d += " since " + st.Since.Format("Jan 2 15:04")
		}
		add(Critical, "Reachability", "Switch is offline", d)
	} else if st.Checked && !st.Online {
		d := "No answer on TCP " + fmt.Sprint(sw.Port)
		if !st.Since.IsZero() {
			d += " since " + st.Since.Format("Jan 2 15:04")
		}
		if st.PingError != "" {
			d += " (" + st.PingError + ")"
		}
		add(Critical, "Reachability", "Switch is offline", d)
	}
	if st.HostKeyChanged {
		if sw.Web {
			add(Critical, "Security", "Switch identity changed", "The switch presented a different HTTPS certificate than before, or stopped offering encrypted login. SwitchDesk stopped signing in to protect the password. If the switch was replaced or reset, trust it again from the switch page.")
		} else {
			add(Critical, "SSH", "SSH host key changed", "The switch presented a different host key than before. If the switch was replaced or reset, trust the new key from the switch page.")
		}
	}
	if st.AuthFailed && sw.Cloud != nil {
		add(Critical, "Instant On", "Instant On login failed", st.CollectError)
	} else if st.CollectError != "" && sw.Cloud != nil {
		add(Warning, "Instant On", "Couldn't read the Instant On portal", st.CollectError)
	} else if st.AuthFailed {
		add(Critical, "SSH", "SSH login failed", st.CollectError)
	} else if st.CollectError != "" && !st.HostKeyChanged && st.Online {
		add(Warning, "SSH", "Data collection failed", st.CollectError)
	}
	if snap == nil || snap.CollectedAt.IsZero() {
		return out
	}

	opticIssues(snap, add)
	for _, n := range snap.Notes {
		add(Severity(n.Severity), n.Category, n.Title, n.Detail)
	}
	if c := snap.Cloud; c != nil {
		for _, a := range c.Alerts {
			sev := Info
			switch a.Severity {
			case "major":
				sev = Critical
			case "minor":
				sev = Warning
			}
			d := "Reported by the Instant On portal"
			if !a.Raised.IsZero() {
				d += " since " + a.Raised.Format("Jan 2 15:04")
			}
			add(sev, "Instant On", a.Text, d)
		}
		if c.UpToDate != nil && !*c.UpToDate {
			add(Info, "Firmware", "Firmware update available", "Running "+orDash(snap.Facts.Software)+". Instant On updates are installed from the Instant On app or portal.")
		}
		if c.PoEBudgetW > 0 && c.PoEUsedW >= c.PoEBudgetW*0.9 {
			add(Warning, "Health", fmt.Sprintf("PoE budget nearly used: %.0f of %.0f W", c.PoEUsedW, c.PoEBudgetW), "New PoE devices may not get power.")
		}
	}
	var loops []string
	for _, p := range snap.Ports {
		if p.Loop {
			loops = append(loops, p.Name)
		}
	}
	if len(loops) > 0 {
		add(Critical, "Spanning tree", "Loop detected", "Ports: "+strings.Join(loops, ", ")+". Find the looped cable or device on these ports.")
	}

	// spanning tree
	if snap.STP.Known && !snap.STP.Enabled {
		add(Critical, "Spanning tree", "Spanning tree is disabled", "A single looped cable can take this segment down.")
	}
	if snap.STP.Enabled {
		if st.TopologyDelta > 0 {
			add(Warning, "Spanning tree", fmt.Sprintf("%d topology change(s) since last check", st.TopologyDelta), "Last change: "+orDash(snap.STP.LastChange)+". Frequent changes usually mean a flapping link or an edge port without admin-edge.")
		}
		if snap.STP.IsRoot && (snap.STP.BridgePriority == "32768" || snap.STP.BridgePriority == "8") {
			add(Warning, "Spanning tree", "Root bridge elected by default priority", "This switch is root only because of its MAC address. Set an explicit priority on the intended core.")
		}
		var blocking []string
		for _, p := range snap.STP.Ports {
			if p.State == "Blocking" || p.State == "Discarding" {
				blocking = append(blocking, p.Port)
			}
		}
		if len(blocking) > 0 {
			add(Info, "Spanning tree", fmt.Sprintf("%d port(s) blocking", len(blocking)), "Ports: "+strings.Join(blocking, ", ")+". Expected on redundant links.")
		}
	}

	// management plane security
	sec := snap.Security
	if sec.TelnetEnabled {
		fix := "Disable with: no telnet-server"
		if snap.Family == aruba.FamilyAOSCX {
			fix = "Disable with: no telnet-server vrf <vrf>"
		}
		seen := "The switch accepts connections on TCP port 23. "
		if sec.TelnetPort == "" {
			seen = ""
		}
		add(Critical, "Security", "Telnet is enabled", seen+"Credentials travel in clear text. "+fix)
	}
	if snap.Family == aruba.FamilyAOSS && sec.HTTPPlain {
		seen := "The switch answers on TCP port 80. "
		if sec.HTTPPort == "" {
			seen = ""
		}
		add(Warning, "Security", "Plain HTTP web management is enabled", seen+"Use web-management ssl and disable plaintext.")
	}
	if sec.SNMPDefault && !set.IgnoreSNMP {
		add(Critical, "Security", "Default SNMP community in use", "Community \"public\" or \"private\" is configured.")
	}
	if snap.RunningConfig != "" {
		if len(sec.NTPServers) == 0 {
			add(Warning, "Management", "No NTP/SNTP server", "Log timestamps and certificates depend on correct time.")
		}
		if len(sec.LogServers) == 0 {
			add(Info, "Management", "No syslog server", "Events are only kept in the switch's local buffer.")
		}
		if len(sec.AAA) == 0 {
			add(Info, "Management", "Local accounts only", "No RADIUS/TACACS+ authentication configured.")
		}
		if snap.Family == aruba.FamilyAOSS && !sec.ManagerPassword {
			add(Critical, "Security", "No manager password set", "Anyone with console or network access gets full rights.")
		}
	}
	if pa := snap.PortAccess; pa != nil && len(pa.Ports) > 0 {
		var ports []string
		dot1x := false
		for _, a := range pa.Ports {
			ports = append(ports, a.Port)
			dot1x = dot1x || a.Dot1x
		}
		list := aruba.CompressPorts(ports)
		if len(pa.Servers) == 0 {
			add(Warning, "Port access", "Port authentication without a RADIUS server", "Ports "+list+" ask clients to log in, but no RADIUS server is set, so nobody can.")
		}
		if dot1x && !pa.Dot1xOn {
			add(Info, "Port access", "802.1X is set on ports but turned off", "The ports are configured for 802.1X, but it isn't turned on for the switch, so they don't ask for a login.")
		}
	}
	if snap.Facts.ConfigSaved != nil && !*snap.Facts.ConfigSaved {
		add(Warning, "Config", "Unsaved configuration changes", "Running config differs from startup. Changes will be lost on reboot.")
	}

	// hardware
	for _, e := range snap.Env {
		if !e.OK {
			kind := map[string]string{"fan": "Fan", "psu": "Power supply", "temp": "Temperature"}[e.Kind]
			add(Critical, "Hardware", kind+" problem", e.Status)
		}
	}
	if snap.Facts.CPU >= 0 && set.CPUWarn > 0 && snap.Facts.CPU >= set.CPUWarn {
		add(Warning, "Health", fmt.Sprintf("High CPU: %d%%", snap.Facts.CPU), "")
	}
	if snap.Facts.Memory >= 0 && set.MemWarn > 0 && snap.Facts.Memory >= set.MemWarn {
		add(Warning, "Health", fmt.Sprintf("High memory: %d%%", snap.Facts.Memory), "")
	}

	// logs
	major := 0
	var sample string
	for _, l := range snap.Logs {
		if l.Severity == "M" || l.Severity == "E" {
			major++
			if sample == "" {
				sample = l.Text
			}
		}
	}
	if major > 0 {
		add(Warning, "Logs", fmt.Sprintf("%d major/error event(s) in recent log", major), sample)
	}
	if snap.CollectedAt.Before(time.Now().Add(-6 * time.Hour)) {
		add(Info, "SSH", "Data is more than 6 hours old", "Last collected "+snap.CollectedAt.Format("Jan 2 15:04"))
	}
	return out
}

// networkIssues looks across switches (e.g. more than one STP root). Roots
// are only compared between switches joined by layer 2 links on the map:
// branches connected through routers each have their own spanning tree, and
// a root per branch is expected.
func networkIssues(snaps map[string]*aruba.Snapshot, names map[string]string, topo Topology) []Issue {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if p, ok := parent[x]; ok && p != x {
			r := find(p)
			parent[x] = r
			return r
		}
		parent[x] = x
		return x
	}
	for _, e := range topo.Edges {
		parent[find(e.From)] = find(e.To)
	}
	type domain struct{ roots map[string][]string }
	domains := map[string]*domain{}
	for id, s := range snaps {
		if s == nil || !s.STP.Enabled || s.STP.RootMAC == "" {
			continue
		}
		d := domains[find(id)]
		if d == nil {
			d = &domain{roots: map[string][]string{}}
			domains[find(id)] = d
		}
		d.roots[s.STP.RootMAC] = append(d.roots[s.STP.RootMAC], names[id])
	}
	var out []Issue
	for _, d := range domains {
		if len(d.roots) <= 1 {
			continue
		}
		var parts []string
		for mac, sws := range d.roots {
			sort.Strings(sws)
			parts = append(parts, fmt.Sprintf("%s (%s)", mac, strings.Join(sws, ", ")))
		}
		sort.Strings(parts)
		out = append(out, Issue{
			Severity: Warning, Category: "Spanning tree", SwitchName: "Network",
			Title:  fmt.Sprintf("%d different spanning-tree roots in one layer 2 network", len(d.roots)),
			Detail: "These switches are linked at layer 2 but don't agree on the root. Roots seen: " + strings.Join(parts, "; ") + ". Mixed MST regions or a link where spanning tree is off can cause this.",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Detail < out[j].Detail })
	return out
}

func sortIssues(is []Issue) {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].Severity.rank() != is[j].Severity.rank() {
			return is[i].Severity.rank() < is[j].Severity.rank()
		}
		return is[i].SwitchName < is[j].SwitchName
	})
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func displayName(sw store.Switch, snap *aruba.Snapshot) string {
	if sw.Name != "" {
		return sw.Name
	}
	if snap != nil && snap.Facts.Hostname != "" {
		return snap.Facts.Hostname
	}
	return sw.Host
}

// opticIssues flags transceivers whose optical levels are out of range. Ports
// that are down with no light are skipped: an SFP in an empty port is normal.
func opticIssues(snap *aruba.Snapshot, add func(sev Severity, cat, title, detail string)) {
	up := map[string]bool{}
	known := map[string]bool{}
	for _, p := range snap.Ports {
		up[p.Name], known[p.Name] = p.Up, true
	}
	var crit, warn []string
	for _, o := range snap.Optics {
		level, reason := o.Judge()
		if level == "" {
			continue
		}
		rx, hasRx := o.WorstRx()
		if known[o.Port] && !up[o.Port] && (!hasRx || rx <= aruba.NoLight) {
			continue
		}
		d := "port " + o.Port
		if o.Type != "" {
			d += " (" + o.Type + ")"
		}
		if hasRx {
			d += fmt.Sprintf(": Rx %.1f dBm", rx)
			if o.RxLowWarn != nil && strings.Contains(reason, "below") {
				d += fmt.Sprintf(", warning at %.1f", *o.RxLowWarn)
			}
		}
		d += ", " + reason
		if level == "alarm" {
			crit = append(crit, d)
		} else {
			warn = append(warn, d)
		}
	}
	tip := ". Clean the connectors, check the patch cords and the optic on the far end."
	if len(crit) > 0 {
		add(Critical, "Optics", fmt.Sprintf("Optical power alarm on %d port(s)", len(crit)), strings.Join(crit, "; ")+tip)
	}
	if len(warn) > 0 {
		add(Warning, "Optics", fmt.Sprintf("Optical power warning on %d port(s)", len(warn)), strings.Join(warn, "; ")+tip)
	}
}
