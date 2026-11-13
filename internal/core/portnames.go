package core

import (
	"context"
	"errors"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/sshx"
	"github.com/jaba0x/switchdesk/internal/store"
)

// Port names from the Ports tab, and from LLDP neighbors (issue #32).

// PortNameSuggestions lists the names a switch's ports would get from their
// LLDP neighbors. Ports named by hand are not in it.
func (m *Manager) PortNameSuggestions(id string) []aruba.PortNameSuggestion {
	sw, ok := m.st.Switch(id)
	if !ok {
		return nil
	}
	m.mu.Lock()
	snap := m.snaps[id]
	m.mu.Unlock()
	return aruba.SuggestPortNames(snap, sw.LLDPNames)
}

func (m *Manager) portNameCommands(id string, ch aruba.PortNameChange) ([]string, error) {
	sw, ok := m.st.Switch(id)
	if !ok {
		return nil, errors.New("switch not found")
	}
	fam := m.family(id, sw)
	m.mu.Lock()
	var ports []aruba.Port
	if s := m.snaps[id]; s != nil {
		ports = s.Ports
	}
	m.mu.Unlock()
	if fam == "" || len(ports) == 0 {
		return nil, errors.New("collect data from this switch first")
	}
	return aruba.BuildPortNameCommands(fam, ch, ports)
}

// PreviewPortNames returns the CLI lines a port name change would send.
func (m *Manager) PreviewPortNames(id string, ch aruba.PortNameChange) ([]string, error) {
	return m.portNameCommands(id, ch)
}

// ApplyPortNames sends the change, then re-collects the switch.
func (m *Manager) ApplyPortNames(id string, ch aruba.PortNameChange) (aruba.ApplyResult, error) {
	cmds, err := m.portNameCommands(id, ch)
	if err != nil {
		return aruba.ApplyResult{}, err
	}
	var res aruba.ApplyResult
	err = m.withSession(id, 3*time.Minute, func(ctx context.Context, s *sshx.Session, _ aruba.Family) error {
		res = aruba.ApplyCommands(ctx, s, cmds)
		return nil
	})
	if err != nil {
		return res, err
	}
	if res.OK {
		m.namesApplied(id, ch)
	}
	go m.Enqueue(id)
	return res, nil
}

// namesApplied shows the new names right away and remembers which came from LLDP.
func (m *Manager) namesApplied(id string, ch aruba.PortNameChange) {
	m.mu.Lock()
	if s := m.snaps[id]; s != nil {
		next := *s
		next.Ports = aruba.AfterPortNames(s.Ports, ch)
		m.snaps[id] = &next
	}
	m.mu.Unlock()
	fromLLDP := map[string]bool{}
	for _, p := range ch.LLDP {
		fromLLDP[p] = true
	}
	set := map[string]string{}
	var drop []string
	for p, n := range ch.Names {
		if fromLLDP[p] {
			set[p] = n
		} else {
			drop = append(drop, p) // typed by hand: SwitchDesk keeps its hands off it
		}
	}
	m.st.SetLLDPNames(id, set, drop)
	m.changed()
}

// autoPortNames runs after a switch was read, while its session is still
// open, when Settings → Name ports after their LLDP neighbors is on.
// It reports whether names were sent.
func (m *Manager) autoPortNames(ctx context.Context, sess *sshx.Session, sw store.Switch, snap *aruba.Snapshot) bool {
	if !m.st.Settings().AutoPortNames || snap.Family == aruba.FamilyInstantOn {
		return false
	}
	sugg := aruba.SuggestPortNames(snap, sw.LLDPNames)
	if len(sugg) == 0 {
		return false
	}
	ch := aruba.PortNameChange{Names: map[string]string{}, Save: true}
	for _, s := range sugg {
		ch.Names[s.Port] = s.Name
		ch.LLDP = append(ch.LLDP, s.Port)
	}
	cmds, err := aruba.BuildPortNameCommands(snap.Family, ch, snap.Ports)
	if err != nil {
		return false
	}
	res := aruba.ApplyCommands(ctx, sess, cmds)
	if res.OK {
		m.namesApplied(sw.ID, ch)
	}
	return res.OK
}
