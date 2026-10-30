// Package core ties the inventory, SSH collection, issue rules and topology together.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/firmware"
	"github.com/jaba0x/switchdesk/internal/ionlocal"
	"github.com/jaba0x/switchdesk/internal/sshx"
	"github.com/jaba0x/switchdesk/internal/store"
)

type Status struct {
	Online         bool      `json:"online"`
	Checked        bool      `json:"checked"`
	LatencyMs      int       `json:"latencyMs"`
	PingError      string    `json:"pingError"`
	LastCheck      time.Time `json:"lastCheck"`
	Since          time.Time `json:"since"` // when Online last flipped
	Collecting     bool      `json:"collecting"`
	LastCollect    time.Time `json:"lastCollect"`
	CollectError   string    `json:"collectError"`
	AuthFailed     bool      `json:"authFailed"`
	HostKeyChanged bool      `json:"hostKeyChanged"`
	TopologyDelta  int       `json:"topologyDelta"`
}

type Manager struct {
	st     *store.Store
	notify func(string)

	mu     sync.Mutex
	status map[string]*Status
	snaps  map[string]*aruba.Snapshot
	locks  map[string]*sync.Mutex
	active int

	lib   *firmware.Library
	up    upgrader
	cloud cloudState

	queue chan string
	ctx   context.Context
	stop  context.CancelFunc

	// saved snapshots are read in the background so the window can open at
	// once; loaded is closed when they're all in (issue #7)
	loaded      chan struct{}
	loadPending int
}

func New(st *store.Store, notify func(string)) *Manager {
	m := &Manager{st: st, notify: notify, status: map[string]*Status{}, snaps: map[string]*aruba.Snapshot{}, locks: map[string]*sync.Mutex{}, queue: make(chan string, 1024), loaded: make(chan struct{})}
	m.up.queue = make(chan *Job, 512)
	if lib, err := firmware.OpenLibrary(filepath.Join(st.Dir(), "firmware")); err == nil {
		m.lib = lib
	}
	sws := st.Switches()
	for _, sw := range sws {
		m.status[sw.ID] = &Status{}
	}
	m.loadPending = len(sws)
	go m.loadSaved(sws)
	return m
}

// loadSaved reads the snapshots saved by the last run. A switch that was
// read again in the meantime keeps its newer data.
func (m *Manager) loadSaved(sws []store.Switch) {
	defer close(m.loaded)
	last := time.Now()
	for _, sw := range sws {
		s, ok := m.st.LoadSnapshotLite(sw.ID)
		m.mu.Lock()
		if ok && m.snaps[sw.ID] == nil {
			snap := s
			m.snaps[sw.ID] = &snap
			if st := m.status[sw.ID]; st != nil && st.LastCollect.IsZero() {
				st.LastCollect = s.CollectedAt
			}
		}
		m.loadPending--
		m.mu.Unlock()
		if time.Since(last) > time.Second {
			m.changed() // show what's in so far
			last = time.Now()
		}
	}
	m.changed()
}

// WaitLoaded blocks until the saved snapshots are loaded.
func (m *Manager) WaitLoaded() { <-m.loaded }

func (m *Manager) changed() {
	if m.notify != nil {
		m.notify("changed")
	}
}

func (m *Manager) Start() {
	m.ctx, m.stop = context.WithCancel(context.Background())
	for i := 0; i < 16; i++ {
		go m.worker()
	}
	go m.pingLoop()
	go m.scheduleLoop()
	rebootLock, slot := &sync.Mutex{}, make(chan struct{}, 2)
	go m.jobWorker(rebootLock, slot)
	go m.jobWorker(rebootLock, slot)
	go m.autoLoop()
	if m.lib != nil {
		go m.lib.Rescan() // pick up firmware copied in by hand, without holding up startup
	}
}

// Stop cancels polling and waits (up to 30s) for running collections, so the
// data folder can be swapped safely afterwards.
func (m *Manager) Stop() {
	if m.stop != nil {
		m.stop()
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := m.active
		m.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) stat(id string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.status[id]
	if !ok {
		s = &Status{}
		m.status[id] = s
	}
	return s
}

func (m *Manager) lock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[id]
	if !ok {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	return l
}

// ---------- reachability ----------

func (m *Manager) pingLoop() {
	m.pingAll()
	for {
		iv := time.Duration(m.st.Settings().PingInterval) * time.Second
		if m.st.Settings().CheckMethod == store.CheckOff {
			iv = 30 * time.Second // only to notice when checks are turned back on
		}
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(iv):
			m.pingAll()
		}
	}
}

func (m *Manager) pingAll() {
	if m.st.Settings().CheckMethod == store.CheckOff {
		return
	}
	sws := m.st.Switches()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for _, sw := range sws {
		if sw.Disabled {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(sw store.Switch) {
			defer wg.Done()
			defer func() { <-sem }()
			m.ping(sw)
		}(sw)
	}
	wg.Wait()
	m.changed()
}

func (m *Manager) ping(sw store.Switch) {
	if sw.Cloud != nil {
		m.cloudRead(sw, false)
		return
	}
	var lat int
	var err error
	switch m.st.Settings().CheckMethod {
	case store.CheckOff:
		return
	case store.CheckPing:
		lat, err = icmpPing(m.ctx, sw.Host, 3*time.Second)
	default:
		start := time.Now()
		var conn net.Conn
		conn, err = net.DialTimeout("tcp", net.JoinHostPort(sw.Host, strconv.Itoa(sw.Port)), 3*time.Second)
		lat = int(time.Since(start).Milliseconds())
		if err == nil {
			conn.Close()
		}
	}
	// came back: refresh data right away
	if m.setReach(sw.ID, err == nil, lat, err) {
		m.Enqueue(sw.ID)
	}
}

// setReach records a switch as online or offline and reports whether it just
// came back.
func (m *Manager) setReach(id string, online bool, lat int, err error) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[id]
	if !ok {
		st = &Status{}
		m.status[id] = st
	}
	if !st.Checked || st.Online != online {
		st.Since = time.Now()
	}
	wasOffline := st.Checked && !st.Online
	st.Online, st.Checked, st.LastCheck = online, true, time.Now()
	if online {
		st.LatencyMs, st.PingError = lat, ""
	} else {
		st.LatencyMs, st.PingError = 0, shortErr(err)
	}
	return online && wasOffline
}

// readReach updates online/offline from a read when background checks are
// off, so the dashboard still shows something. A login that gets through
// means online; only a network error means offline, not a wrong password.
func (m *Manager) readReach(id string, err error) {
	if m.st.Settings().CheckMethod != store.CheckOff {
		return
	}
	var ne net.Error
	var oe *net.OpError
	switch {
	case err == nil, errors.Is(err, sshx.ErrHostKeyMismatch), errors.Is(err, ionlocal.ErrLogin),
		strings.Contains(err.Error(), "unable to authenticate"):
		// the switch answered, it just didn't let us in
		m.setReach(id, true, 0, nil)
	case errors.As(err, &oe), errors.As(err, &ne):
		m.setReach(id, false, 0, err)
	}
}

func shortErr(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "refused"):
		return "connection refused"
	case strings.Contains(s, "no route"):
		return "no route to host"
	case strings.Contains(s, "no such host"):
		return "unknown host"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// ---------- collection ----------

func (m *Manager) scheduleLoop() {
	// first pass shortly after start so the UI fills in, once the saved
	// data is in (otherwise every switch would look never read)
	select {
	case <-m.ctx.Done():
		return
	case <-m.loaded:
	}
	select {
	case <-m.ctx.Done():
		return
	case <-time.After(3 * time.Second):
	}
	for {
		set := m.st.Settings()
		if set.CollectInterval > 0 {
			due := time.Duration(set.CollectInterval) * time.Minute
			for _, sw := range m.st.Switches() {
				st := m.stat(sw.ID)
				m.mu.Lock()
				// with checks off nothing marks a switch online, so read it anyway
				up := st.Online || set.CheckMethod == store.CheckOff
				ready := !sw.Disabled && up && !st.Collecting && time.Since(st.LastCollect) > due && !st.HostKeyChanged && !st.AuthFailed
				m.mu.Unlock()
				if ready {
					m.Enqueue(sw.ID)
				}
			}
		}
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (m *Manager) Enqueue(id string) {
	if sw, ok := m.st.Switch(id); !ok || sw.Disabled {
		return
	}
	st := m.stat(id)
	m.mu.Lock()
	if st.Collecting {
		m.mu.Unlock()
		return
	}
	st.Collecting = true
	m.mu.Unlock()
	m.changed()
	select {
	case m.queue <- id:
	default:
		m.mu.Lock()
		st.Collecting = false
		m.mu.Unlock()
	}
}

func (m *Manager) worker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case id := <-m.queue:
			// respect the configured parallelism (it can change at runtime)
			for {
				m.mu.Lock()
				if m.active < m.st.Settings().Parallel {
					m.active++
					m.mu.Unlock()
					break
				}
				m.mu.Unlock()
				select {
				case <-m.ctx.Done():
					return
				case <-time.After(250 * time.Millisecond):
				}
			}
			m.collect(id)
			m.mu.Lock()
			m.active--
			m.mu.Unlock()
		}
	}
}

func (m *Manager) connect(ctx context.Context, sw store.Switch) (*sshx.Session, error) {
	if sw.Cloud != nil {
		return nil, ErrCloudOnly
	}
	if sw.Web {
		return nil, ErrWebOnly
	}
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		return nil, err
	}
	s, err := sshx.Dial(ctx, sshx.Options{Host: sw.Host, Port: sw.Port, Username: cred.Username, Password: cred.Password, KnownKey: sw.HostKey, Timeout: 10 * time.Second})
	if err != nil {
		return nil, err
	}
	if sw.HostKey == "" && s.Fingerprint != "" {
		m.st.SetHostKey(sw.ID, s.Fingerprint) // trust on first use
	}
	return s, nil
}

func (m *Manager) collect(id string) {
	st := m.stat(id)
	defer func() {
		m.mu.Lock()
		st.Collecting = false
		m.mu.Unlock()
		m.changed()
	}()
	sw, ok := m.st.Switch(id)
	if !ok {
		return
	}
	l := m.lock(id)
	l.Lock()
	defer l.Unlock()
	if sw.Cloud != nil {
		m.cloudRead(sw, true)
		return
	}
	if sw.Web {
		m.webRead(sw)
		return
	}

	ctx, cancel := context.WithTimeout(m.ctx, 4*time.Minute)
	defer cancel()
	sess, err := m.connect(ctx, sw)
	m.readReach(id, err)
	if err != nil {
		m.mu.Lock()
		st.CollectError = err.Error()
		st.HostKeyChanged = identityChanged(err)
		st.AuthFailed = strings.Contains(err.Error(), "unable to authenticate") || strings.Contains(err.Error(), "no credentials")
		st.LastCollect = time.Now()
		m.mu.Unlock()
		return
	}
	defer sess.Close()

	fam, ver := aruba.Detect(ctx, sess, sw.Family, sess.Banner)
	aruba.Prepare(ctx, sess, fam)
	snap := aruba.Collect(ctx, sess, fam, ver)
	applyPortProbes(&snap, sw.Host)

	m.mu.Lock()
	prev := m.snaps[id]
	st.TopologyDelta = 0
	if prev != nil && prev.STP.Enabled && snap.STP.TopologyChanges > prev.STP.TopologyChanges {
		st.TopologyDelta = snap.STP.TopologyChanges - prev.STP.TopologyChanges
	}
	m.snaps[id] = &snap
	st.LastCollect = time.Now()
	st.CollectError, st.AuthFailed, st.HostKeyChanged = "", false, false
	if len(snap.Errors) > 0 && snap.Facts.Hostname == "" {
		st.CollectError = strings.Join(snap.Errors, "; ")
	}
	m.mu.Unlock()

	if m.ctx.Err() != nil {
		return
	}
	m.st.SaveSnapshot(id, snap)
	m.st.SaveBackup(id, snap.RunningConfig, m.st.Settings().KeepBackups)
}

// Resume checks a re-enabled switch right away and reads it if it answers.
func (m *Manager) Resume(id string) {
	sw, ok := m.st.Switch(id)
	if !ok || sw.Disabled {
		return
	}
	if m.st.Settings().CheckMethod == store.CheckOff {
		m.Enqueue(id)
		return
	}
	m.ping(sw)
	m.changed()
	if m.stat(id).Online {
		m.Enqueue(id)
	}
}

// RefreshAll queues every online switch.
func (m *Manager) RefreshAll() {
	for _, sw := range m.st.Switches() {
		m.Enqueue(sw.ID)
	}
}

func (m *Manager) Forget(id string) {
	m.mu.Lock()
	delete(m.status, id)
	delete(m.snaps, id)
	m.mu.Unlock()
}

// TrustNewHostKey clears the pinned key so the next connect re-learns it.
func (m *Manager) TrustNewHostKey(id string) {
	m.st.SetHostKey(id, "")
	m.st.SetCertFP(id, "")
	m.st.SetWebEncrypted(id, false)
	st := m.stat(id)
	m.mu.Lock()
	st.HostKeyChanged = false
	st.CollectError = ""
	m.mu.Unlock()
	m.Enqueue(id)
}

func (m *Manager) ClearAuthFailure(id string) {
	st := m.stat(id)
	m.mu.Lock()
	st.AuthFailed = false
	m.mu.Unlock()
}

// ---------- views ----------

type Summary struct {
	Family      string    `json:"family"`
	Hostname    string    `json:"hostname"`
	Model       string    `json:"model"`
	Software    string    `json:"software"`
	Serial      string    `json:"serial"`
	Uptime      string    `json:"uptime"`
	CPU         int       `json:"cpu"`
	Memory      int       `json:"memory"`
	VLANs       int       `json:"vlans"`
	PortsUp     int       `json:"portsUp"`
	PortsTotal  int       `json:"portsTotal"`
	Neighbors   int       `json:"neighbors"`
	STPRole     string    `json:"stpRole"`
	Ports       []PortLED `json:"ports"`
	CollectedAt time.Time `json:"collectedAt"`
}

// PortLED is the minimum the faceplate needs per port.
type PortLED struct {
	Name        string `json:"name"`
	Up          bool   `json:"up"`
	Enabled     bool   `json:"enabled"`
	Blocking    bool   `json:"blocking"`
	Type        string `json:"type"`
	Speed       string `json:"speed"`
	Description string `json:"description,omitempty"` // the port's configured name
}

type SwitchView struct {
	store.Switch
	Status   Status       `json:"status"`
	Firmware FirmwareInfo `json:"firmware"`
	Summary  *Summary     `json:"summary"`
	Issues   []Issue      `json:"issues"`
	Worst    Severity     `json:"worst"`
}

type Dashboard struct {
	Total      int            `json:"total"`
	Online     int            `json:"online"`
	Offline    int            `json:"offline"`
	Unknown    int            `json:"unknown"`
	Disabled   int            `json:"disabled"`
	Critical   int            `json:"critical"`
	Warning    int            `json:"warning"`
	InfoCount  int            `json:"info"`
	Collecting int            `json:"collecting"`
	Issues     []Issue        `json:"issues"`
	Switches   []SwitchView   `json:"switches"`
	Families   map[string]int `json:"families"`
	DataDir    string         `json:"dataDir"`
	// Loading counts switches whose saved data is still being read at startup.
	Loading int `json:"loading,omitempty"`
}

func summarize(snap *aruba.Snapshot) *Summary {
	if snap == nil || snap.CollectedAt.IsZero() {
		return nil
	}
	s := &Summary{Family: snap.Family.Label(), Hostname: snap.Facts.Hostname, Model: snap.Facts.Model, Software: snap.Facts.Software,
		Serial: snap.Facts.Serial, Uptime: snap.Facts.Uptime, CPU: snap.Facts.CPU, Memory: snap.Facts.Memory,
		VLANs: len(snap.VLANs), Neighbors: len(snap.Neighbors), CollectedAt: snap.CollectedAt}
	blocking := map[string]bool{}
	for _, p := range snap.STP.Ports {
		if p.State == "Blocking" || p.State == "Discarding" {
			blocking[p.Port] = true
		}
	}
	for _, p := range snap.Ports {
		s.Ports = append(s.Ports, PortLED{Name: p.Name, Up: p.Up, Enabled: p.Enabled, Blocking: blocking[p.Name], Type: p.Type, Speed: p.Speed, Description: p.Description})
		s.PortsTotal++
		if p.Up {
			s.PortsUp++
		}
	}
	switch {
	case !snap.STP.Known:
		s.STPRole = "unknown"
	case !snap.STP.Enabled:
		s.STPRole = "disabled"
	case snap.STP.IsRoot:
		s.STPRole = "root"
	default:
		s.STPRole = "member"
	}
	return s
}

func (m *Manager) views() ([]SwitchView, []switchInfo, map[string]*aruba.Snapshot, map[string]string) {
	set := m.st.Settings()
	sws := m.st.Switches()
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []SwitchView
	var infos []switchInfo
	snaps := map[string]*aruba.Snapshot{}
	names := map[string]string{}
	for _, sw := range sws {
		st, ok := m.status[sw.ID]
		if !ok {
			st = &Status{}
			m.status[sw.ID] = st
		}
		snap := m.snaps[sw.ID]
		var issues []Issue
		if sw.Disabled {
			// a paused switch keeps its last data but raises nothing
			*st = Status{LastCollect: st.LastCollect}
		} else {
			issues = evaluate(sw, st, snap, set)
		}
		if issues == nil {
			issues = []Issue{} // the UI expects a list, not null
		}
		sortIssues(issues)
		v := SwitchView{Switch: sw, Status: *st, Summary: summarize(snap), Issues: issues, Firmware: m.firmwareFor(sw, snap)}
		v.Firmware.Job = m.jobFor(sw.ID)
		if len(issues) > 0 {
			v.Worst = issues[0].Severity
		}
		out = append(out, v)
		infos = append(infos, switchInfo{sw: sw, snap: snap, online: st.Online, issues: issues})
		snaps[sw.ID] = snap
		names[sw.ID] = displayName(sw, snap)
		if sw.Disabled {
			delete(snaps, sw.ID) // keep paused switches out of network-wide checks
		}
	}
	return out, infos, snaps, names
}

func (m *Manager) Dashboard() Dashboard {
	vs, infos, snaps, names := m.views()
	d := Dashboard{Switches: vs, Families: map[string]int{}, DataDir: m.st.Dir()}
	m.mu.Lock()
	d.Loading = m.loadPending
	m.mu.Unlock()
	for _, v := range vs {
		d.Total++
		switch {
		case v.Disabled:
			d.Disabled++
		case !v.Status.Checked:
			d.Unknown++
		case v.Status.Online:
			d.Online++
		default:
			d.Offline++
		}
		if v.Status.Collecting {
			d.Collecting++
		}
		fam := "Not collected"
		if v.Summary != nil {
			fam = v.Summary.Family
		}
		d.Families[fam]++
		d.Issues = append(d.Issues, v.Issues...)
	}
	d.Issues = append(d.Issues, networkIssues(snaps, names, buildTopology(infos, nil, false))...)
	if d.Issues == nil {
		d.Issues = []Issue{}
	}
	if d.Switches == nil {
		d.Switches = []SwitchView{}
	}
	sortIssues(d.Issues)
	for _, is := range d.Issues {
		switch is.Severity {
		case Critical:
			d.Critical++
		case Warning:
			d.Warning++
		default:
			d.InfoCount++
		}
	}
	return d
}

type Detail struct {
	SwitchView
	Snapshot *aruba.Snapshot `json:"snapshot"`
	Backups  []store.Backup  `json:"backups"`
}

func (m *Manager) Detail(id string) (Detail, error) {
	vs, _, _, _ := m.views()
	for _, v := range vs {
		if v.ID == id {
			m.mu.Lock()
			snap := m.snaps[id]
			m.mu.Unlock()
			if snap != nil && snap.Raw == nil {
				// loaded without raw output at startup; read it for this page
				full := *snap
				full.Raw = m.st.LoadRaw(id)
				snap = &full
			}
			backups := m.st.Backups(id)
			if backups == nil {
				backups = []store.Backup{}
			}
			return Detail{SwitchView: v, Snapshot: snap, Backups: backups}, nil
		}
	}
	return Detail{}, errors.New("switch not found")
}

func (m *Manager) Topology(showEndpoints bool) Topology {
	_, infos, _, _ := m.views()
	return buildTopology(infos, m.st.Layout(), showEndpoints)
}

// ---------- interactive actions ----------

func (m *Manager) family(id string, sw store.Switch) aruba.Family {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.snaps[id]; s != nil && s.Family != "" {
		return s.Family
	}
	if sw.Family != "" && sw.Family != aruba.FamilyAuto {
		return sw.Family
	}
	return ""
}

func (m *Manager) withSession(id string, timeout time.Duration, fn func(ctx context.Context, s *sshx.Session, fam aruba.Family) error) error {
	sw, ok := m.st.Switch(id)
	if !ok {
		return errors.New("switch not found")
	}
	l := m.lock(id)
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	sess, err := m.connect(ctx, sw)
	if err != nil {
		return err
	}
	defer sess.Close()
	fam := m.family(id, sw)
	if fam == "" {
		fam, _ = aruba.Detect(ctx, sess, sw.Family, sess.Banner)
	}
	aruba.Prepare(ctx, sess, fam)
	return fn(ctx, sess, fam)
}

// RunCommand executes one read-only command from the console.
func (m *Manager) RunCommand(id, cmd string) (string, error) {
	if !aruba.ReadOnlyAllowed(cmd) {
		return "", errors.New("only show, ping and traceroute commands are allowed in the console")
	}
	var out string
	err := m.withSession(id, 2*time.Minute, func(ctx context.Context, s *sshx.Session, _ aruba.Family) error {
		var err error
		out, err = s.Run(ctx, cmd, 90*time.Second)
		return err
	})
	return out, err
}

// PreviewVLAN returns the CLI lines a change would send.
func (m *Manager) PreviewVLAN(id string, ch aruba.VLANChange) ([]string, error) {
	sw, ok := m.st.Switch(id)
	if !ok {
		return nil, errors.New("switch not found")
	}
	fam := m.family(id, sw)
	if fam == "" {
		return nil, errors.New("collect data from this switch first")
	}
	m.mu.Lock()
	var cur []aruba.VLAN
	if s := m.snaps[id]; s != nil {
		cur = s.VLANs
	}
	m.mu.Unlock()
	return aruba.BuildVLANCommands(fam, ch, cur)
}

// ApplyVLAN sends the change, then re-collects the switch.
func (m *Manager) ApplyVLAN(id string, ch aruba.VLANChange) (aruba.ApplyResult, error) {
	cmds, err := m.PreviewVLAN(id, ch)
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
		// show the change now; the re-read below takes a while and confirms it
		m.mu.Lock()
		if s := m.snaps[id]; s != nil {
			next := *s
			next.VLANs = aruba.AfterChange(s.VLANs, ch)
			m.snaps[id] = &next
		}
		m.mu.Unlock()
		m.changed()
	}
	go m.Enqueue(id)
	return res, nil
}

func (m *Manager) portAccessCommands(id string, ch aruba.PortAccessChange) ([]string, error) {
	sw, ok := m.st.Switch(id)
	if !ok {
		return nil, errors.New("switch not found")
	}
	fam := m.family(id, sw)
	m.mu.Lock()
	var cur *aruba.PortAccess
	if s := m.snaps[id]; s != nil {
		cur = s.PortAccess
	}
	m.mu.Unlock()
	if fam == "" || cur == nil {
		return nil, errors.New("collect data from this switch first")
	}
	return aruba.BuildPortAccessCommands(fam, ch, *cur)
}

// PreviewPortAccess returns the CLI lines a port-access change would send, with the shared secret hidden.
func (m *Manager) PreviewPortAccess(id string, ch aruba.PortAccessChange) ([]string, error) {
	cmds, err := m.portAccessCommands(id, ch)
	return aruba.MaskSecret(cmds, ch), err
}

// ApplyPortAccess sends the change, then re-collects the switch.
func (m *Manager) ApplyPortAccess(id string, ch aruba.PortAccessChange) (aruba.ApplyResult, error) {
	cmds, err := m.portAccessCommands(id, ch)
	if err != nil {
		return aruba.ApplyResult{}, err
	}
	var res aruba.ApplyResult
	err = m.withSession(id, 3*time.Minute, func(ctx context.Context, s *sshx.Session, _ aruba.Family) error {
		res = aruba.ApplyCommands(ctx, s, cmds)
		return nil
	})
	// the switch echoes what it gets, so the secret is in the transcript too
	res.Transcript = aruba.MaskSecret(res.Transcript, ch)
	res.Error = strings.Join(aruba.MaskSecret([]string{res.Error}, ch), "")
	if err != nil {
		return res, err
	}
	if res.OK {
		m.mu.Lock()
		if s := m.snaps[id]; s != nil && s.PortAccess != nil {
			next := *s
			pa := aruba.AfterPortAccess(*s.PortAccess, ch)
			next.PortAccess = &pa
			m.snaps[id] = &next
		}
		m.mu.Unlock()
		m.changed()
	}
	go m.Enqueue(id)
	return res, nil
}

// RefreshPortClients reads who is on the ports now, without a full collection.
func (m *Manager) RefreshPortClients(id string) ([]aruba.PortClient, error) {
	var out string
	err := m.withSession(id, time.Minute, func(ctx context.Context, s *sshx.Session, fam aruba.Family) error {
		if fam != aruba.FamilyAOSCX {
			return errors.New("the client list is only available on AOS-CX switches")
		}
		var err error
		if out, err = s.Run(ctx, aruba.PortClientsCmd, 45*time.Second); err == nil && aruba.IsCLIError(out) {
			out, err = s.Run(ctx, "show port-access clients", 45*time.Second)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	clients, ok := aruba.ParsePortClients(out)
	if !ok {
		return nil, errors.New("the switch didn't accept show port-access clients")
	}
	m.mu.Lock()
	if s := m.snaps[id]; s != nil && s.PortAccess != nil {
		next, pa := *s, *s.PortAccess
		pa.Clients, pa.ClientsRead, pa.ClientsAt = clients, true, time.Now()
		next.PortAccess = &pa
		m.snaps[id] = &next
	}
	m.mu.Unlock()
	m.changed()
	return clients, nil
}

// TestConnection is used by the "Add switch" dialog.
func (m *Manager) TestConnection(sw store.Switch) (string, error) {
	if sw.Cloud != nil {
		return "", ErrCloudOnly
	}
	if sw.Web {
		return m.testWeb(sw)
	}
	if sw.Port == 0 {
		sw.Port = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		return "", err
	}
	// an existing switch keeps its pinned host key; take it from the store,
	// not from what the dialog sent
	known := ""
	if o, ok := m.st.Switch(sw.ID); ok && sw.ID != "" && strings.EqualFold(o.Host, sw.Host) && o.Port == sw.Port {
		known = o.HostKey
	}
	s, err := sshx.Dial(ctx, sshx.Options{Host: sw.Host, Port: sw.Port, Username: cred.Username, Password: cred.Password, KnownKey: known, Timeout: 10 * time.Second})
	if err != nil {
		return "", err
	}
	defer s.Close()
	fam, _ := aruba.Detect(ctx, s, sw.Family, s.Banner)
	return fmt.Sprintf("Connected to %s (%s). Host key %s", s.Hostname, fam.Label(), s.Fingerprint), nil
}

type SwitchVLANs struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Online bool         `json:"online"`
	VLANs  []aruba.VLAN `json:"vlans"`
	Family string       `json:"family"`
	// Editable is set when VLAN changes can be sent over SSH; ReadOnly says why not.
	Editable bool   `json:"editable"`
	ReadOnly string `json:"readOnly,omitempty"`
}

// vlanReadOnly says why a switch's VLANs can't be changed from SwitchDesk, or "" if they can.
func vlanReadOnly(sw store.Switch, snap *aruba.Snapshot) string {
	switch {
	case sw.Disabled:
		return "disabled"
	case sw.Cloud != nil:
		return "changed in the Instant On app or portal"
	case sw.Web || snap.Family == aruba.FamilyInstantOn:
		return "changed on the switch's web page"
	case snap.Family != aruba.FamilyAOSS && snap.Family != aruba.FamilyAOSCX:
		return "family not detected yet"
	}
	return ""
}

// VLANOverview returns every collected switch's VLAN table for the network-wide matrix.
func (m *Manager) VLANOverview() []SwitchVLANs {
	sws := m.st.Switches()
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []SwitchVLANs
	for _, sw := range sws {
		snap := m.snaps[sw.ID]
		if snap == nil {
			continue
		}
		online := false
		if st := m.status[sw.ID]; st != nil {
			online = st.Online
		}
		ro := vlanReadOnly(sw, snap)
		vlans := snap.VLANs
		if vlans == nil {
			vlans = []aruba.VLAN{} // a failed collect has none; send [] rather than null
		}
		out = append(out, SwitchVLANs{ID: sw.ID, Name: displayName(sw, snap), Online: online, VLANs: vlans,
			Family: snap.Family.Label(), Editable: ro == "", ReadOnly: ro})
	}
	return out
}
