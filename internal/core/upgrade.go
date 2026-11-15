package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/cxrest"
	"github.com/jaba0x/switchdesk/internal/firmware"
	"github.com/jaba0x/switchdesk/internal/sshx"
	"github.com/jaba0x/switchdesk/internal/store"
	"github.com/jaba0x/switchdesk/internal/tlspin"
)

// Job is one firmware update of one switch.
type Job struct {
	ID         string    `json:"id"`
	SwitchID   string    `json:"switchId"`
	SwitchName string    `json:"switchName"`
	Image      string    `json:"image"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Reboot     bool      `json:"reboot"`
	Auto       bool      `json:"auto"`
	State      string    `json:"state"` // queued, running, staged, done, failed, cancelled
	Step       string    `json:"step"`
	Progress   float64   `json:"progress"` // 0..1 while copying
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"createdAt"`
	EndedAt    time.Time `json:"endedAt"`
	Log        []string  `json:"log"`
	// Batch ties the updates started together; when one of them fails after
	// its reboot, the batch's other reboots don't start (issue #34).
	Batch string `json:"batch,omitempty"`
	Order int    `json:"order,omitempty"` // 1-based place in the batch, downstream switches first

	rebooted bool // the reboot command was sent
	halted   bool // stopped by haltBatch
}

func (j *Job) active() bool { return j.State == "queued" || j.State == "running" }

type upgrader struct {
	mu    sync.Mutex
	jobs  []*Job
	tftp  *firmware.TFTP
	users int
	queue chan *Job
	seen  map[string]time.Time // job id -> last time its switch talked to our TFTP server
}

// FirmwareInfo is what the UI shows per switch.
type FirmwareInfo struct {
	Running   string      `json:"running"`
	Banks     aruba.Banks `json:"banks"`
	Available string      `json:"available"`
	Image     string      `json:"image"`
	Staged    bool        `json:"staged"` // newest image already sits in the spare bank as boot default
	Supported bool        `json:"supported"`
	Job       *Job        `json:"job,omitempty"`
}

// ---------- availability ----------

func (m *Manager) firmwareFor(sw store.Switch, snap *aruba.Snapshot) FirmwareInfo {
	fi := FirmwareInfo{}
	if snap == nil || snap.CollectedAt.IsZero() {
		return fi
	}
	fi.Running = snap.Facts.Software
	fi.Banks = snap.Banks
	fi.Supported = snap.Family == aruba.FamilyAOSS || snap.Family == aruba.FamilyAOSCX
	cur, ok := firmware.ParseVersion(snap.Facts.Software)
	if !ok || m.lib == nil {
		return fi
	}
	fi.Running = cur.Text
	if img, ok := m.lib.Latest(cur.Branch); ok && img.Version.Newer(cur) {
		fi.Available = img.Version.Text
		fi.Image = img.File
		spare := snap.Banks.Spare()
		fi.Staged = snap.Banks.Version(spare) == img.Version.Text && snap.Banks.Default == spare
	}
	return fi
}

// ---------- jobs ----------

func (m *Manager) Jobs() []Job {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	out := make([]Job, 0, len(m.up.jobs))
	for i := len(m.up.jobs) - 1; i >= 0; i-- {
		j := *m.up.jobs[i]
		j.Log = append([]string(nil), j.Log...)
		out = append(out, j)
	}
	return out
}

func (m *Manager) jobFor(switchID string) *Job {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	for i := len(m.up.jobs) - 1; i >= 0; i-- {
		if m.up.jobs[i].SwitchID == switchID {
			j := *m.up.jobs[i]
			return &j
		}
	}
	return nil
}

func (m *Manager) update(j *Job, fn func(j *Job)) {
	m.up.mu.Lock()
	fn(j)
	m.up.mu.Unlock()
	m.changed()
}

func (m *Manager) logf(j *Job, format string, a ...any) {
	m.update(j, func(j *Job) {
		j.Log = append(j.Log, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(format, a...))
		if len(j.Log) > 200 {
			j.Log = j.Log[len(j.Log)-200:]
		}
	})
}

// StartUpgrade queues updates. image "" means the newest matching image in the library.
//
// Several switches are queued bottom-up (issue #34): the ones furthest from
// the spanning-tree root (or the busiest switch) first, so a switch never
// reboots while a switch behind it still waits for its turn.
func (m *Manager) StartUpgrade(ids []string, image string, reboot, auto bool) ([]Job, error) {
	var out []Job
	var errs []string
	batch := ""
	if len(ids) > 1 {
		batch = store.NewID()
		ids = m.upgradeOrder(ids)
	}
	for i, id := range ids {
		j, err := m.newJob(id, image, reboot, auto, batch, i+1)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, *j)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

func (m *Manager) newJob(id, image string, reboot, auto bool, batch string, order int) (*Job, error) {
	if m.lib == nil {
		return nil, errors.New("the firmware library folder couldn't be opened")
	}
	sw, ok := m.st.Switch(id)
	if !ok {
		return nil, errors.New("switch not found")
	}
	name := sw.Host
	m.mu.Lock()
	snap := m.snaps[id]
	m.mu.Unlock()
	if snap != nil {
		name = displayName(sw, snap)
	}
	if sw.Disabled {
		return nil, fmt.Errorf("%s is disabled", name)
	}
	if sw.FirmwareHold {
		return nil, fmt.Errorf("%s is held on its current firmware; release it on the Firmware page first", name)
	}
	if snap == nil || snap.CollectedAt.IsZero() {
		return nil, fmt.Errorf("%s hasn't been read yet", name)
	}
	if snap.Family != aruba.FamilyAOSS && snap.Family != aruba.FamilyAOSCX {
		return nil, fmt.Errorf("%s: %s firmware is updated from the Instant On portal", name, snap.Family.Label())
	}
	cur, ok := firmware.ParseVersion(snap.Facts.Software)
	if !ok {
		return nil, fmt.Errorf("%s: can't read the running version", name)
	}
	var img firmware.Image
	if image == "" {
		img, ok = m.lib.Latest(cur.Branch)
		if !ok || !img.Version.Newer(cur) {
			return nil, fmt.Errorf("%s: no newer %s image in the library", name, cur.Branch)
		}
	} else {
		img, ok = m.lib.Get(image)
		if !ok {
			return nil, fmt.Errorf("image %s isn't in the library", image)
		}
		if img.Version.Branch != cur.Branch {
			return nil, fmt.Errorf("%s runs %s; %s is for %s switches", name, cur.Text, img.File, img.Version.Branch)
		}
	}
	m.up.mu.Lock()
	for _, j := range m.up.jobs {
		if j.SwitchID == id && j.active() {
			m.up.mu.Unlock()
			return nil, fmt.Errorf("%s already has an update in progress", name)
		}
	}
	j := &Job{ID: store.NewID(), SwitchID: id, SwitchName: name, Image: img.File, From: cur.Text, To: img.Version.Text,
		Reboot: reboot, Auto: auto, State: "queued", Step: "Waiting to start", CreatedAt: time.Now()}
	if batch != "" {
		j.Batch, j.Order = batch, order
	}
	m.up.jobs = append(m.up.jobs, j)
	if len(m.up.jobs) > 200 {
		m.up.jobs = m.up.jobs[len(m.up.jobs)-200:]
	}
	m.up.mu.Unlock()
	m.changed()
	select {
	case m.up.queue <- j:
	default:
		m.update(j, func(j *Job) { j.State, j.Error = "failed", "too many updates queued" })
	}
	return j, nil
}

func (m *Manager) CancelJob(id string) error {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	for _, j := range m.up.jobs {
		if j.ID == id {
			if j.State != "queued" {
				return errors.New("only updates that haven't started can be cancelled")
			}
			j.State, j.Step, j.EndedAt = "cancelled", "Cancelled", time.Now()
			go m.changed()
			return nil
		}
	}
	return errors.New("update not found")
}

func (m *Manager) ClearFinishedJobs() {
	m.up.mu.Lock()
	keep := m.up.jobs[:0]
	for _, j := range m.up.jobs {
		if j.active() {
			keep = append(keep, j)
		}
	}
	m.up.jobs = keep
	m.up.mu.Unlock()
	m.changed()
}

// jobWorker runs updates. Two at a time at most, and only one that reboots,
// so the network is never missing more than one switch.
func (m *Manager) jobWorker(rebootLock *sync.Mutex, slot chan struct{}) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case j := <-m.up.queue:
			m.up.mu.Lock()
			cancelled := j.State == "cancelled"
			m.up.mu.Unlock()
			if cancelled {
				continue
			}
			slot <- struct{}{}
			if j.Reboot {
				rebootLock.Lock()
			}
			m.runJob(j)
			if j.Reboot {
				m.pauseAfterReboot(j)
				rebootLock.Unlock()
			}
			<-slot
		}
	}
}

func (m *Manager) fail(j *Job, err error) {
	m.logf(j, "Failed: %v", err)
	m.update(j, func(j *Job) { j.State, j.Error, j.Step, j.EndedAt = "failed", err.Error(), "Failed", time.Now() })
	m.up.mu.Lock()
	halt := j.Batch != "" && j.rebooted
	m.up.mu.Unlock()
	if halt {
		m.haltBatch(j)
	}
}

// haltBatch is the circuit breaker of issue #34: a switch that didn't come
// back properly stops the reboots still waiting in its batch. Copies that
// don't reboot carry on. Start the stopped ones again once the cause is known.
func (m *Manager) haltBatch(failed *Job) {
	m.up.mu.Lock()
	var stopped []*Job
	for _, j := range m.up.jobs {
		if j != failed && j.Batch == failed.Batch && j.State == "queued" && j.Reboot {
			j.State, j.EndedAt, j.halted = "cancelled", time.Now(), true
			j.Step = fmt.Sprintf("Stopped: %s didn't come back properly. Check it, then start this update again", failed.SwitchName)
			stopped = append(stopped, j)
		}
	}
	m.up.mu.Unlock()
	for _, j := range stopped {
		m.logf(j, "%s", j.Step)
	}
	if len(stopped) > 0 {
		m.logf(failed, "Stopped %d more reboot(s) of this batch", len(stopped))
	}
}

// pauseAfterReboot waits Settings → Wait between reboots before the next
// queued reboot may start, so spanning tree settles (issue #34).
func (m *Manager) pauseAfterReboot(j *Job) {
	wait := time.Duration(m.st.Settings().RebootPause) * time.Second
	if wait <= 0 {
		return
	}
	m.up.mu.Lock()
	ok := j.State == "done"
	next := false
	for _, o := range m.up.jobs {
		if o.State == "queued" && o.Reboot {
			next = true
		}
	}
	m.up.mu.Unlock()
	if !ok || !next {
		return
	}
	m.logf(j, "Waiting %s before the next reboot", wait)
	m.sleep(wait)
}

// upgradeOrder sorts switches downstream first: by hop distance from the
// spanning-tree root over the LLDP/CDP map, furthest first. Switches the map
// can't place keep their order and go last. Third-party switches seen over
// LLDP are part of the map, so they bridge the gaps between managed ones.
func (m *Manager) upgradeOrder(ids []string) []string {
	_, infos, _, _ := m.views()
	return orderByDepth(ids, topoDepth(buildTopology(infos, nil, false)))
}

func orderByDepth(ids []string, depth map[string]int) []string {
	out := append([]string(nil), ids...)
	sort.SliceStable(out, func(a, b int) bool {
		da, oka := depth[out[a]]
		db, okb := depth[out[b]]
		if oka != okb {
			return oka
		}
		return da > db
	})
	return out
}

// topoDepth is each node's hop distance from the spanning-tree root(s), or
// from the managed switch with the most links when no root is known.
func topoDepth(t Topology) map[string]int {
	adj := map[string][]string{}
	for _, e := range t.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	var roots []string
	for _, n := range t.Nodes {
		if n.StpRoot {
			roots = append(roots, n.ID)
		}
	}
	if len(roots) == 0 {
		best, most := "", -1
		for _, n := range t.Nodes {
			if n.Managed && len(adj[n.ID]) > most {
				best, most = n.ID, len(adj[n.ID])
			}
		}
		if best != "" {
			roots = []string{best}
		}
	}
	d := map[string]int{}
	q := []string{}
	for _, r := range roots {
		d[r] = 0
		q = append(q, r)
	}
	for len(q) > 0 {
		id := q[0]
		q = q[1:]
		for _, nb := range adj[id] {
			if _, seen := d[nb]; !seen {
				d[nb] = d[id] + 1
				q = append(q, nb)
			}
		}
	}
	return d
}

func (m *Manager) step(j *Job, s string) {
	m.logf(j, "%s", s)
	m.update(j, func(j *Job) { j.Step = s })
}

// Timings for the reboot wait; tests shorten them.
var (
	tftpAddr    = ":69"
	pollDown    = 5 * time.Second
	pollUp      = 10 * time.Second
	settleAfter = 45 * time.Second
	bankSettle  = 3 * time.Minute // how long a switch may take to show an uploaded image in its bank

	tftpNoRequest = 2 * time.Minute // a switch that hasn't asked for the image by now won't
	tftpStalled   = 2 * time.Minute
	tftpWatchTick = 5 * time.Second
)

// tftpAcquire starts the shared TFTP server if needed.
func (m *Manager) tftpAcquire() error {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	if m.up.tftp == nil {
		srv, err := firmware.StartTFTP(tftpAddr, m.offer)
		if err != nil {
			return err
		}
		srv.Progress = m.tftpProgress
		m.up.tftp = srv
	}
	m.up.users++
	return nil
}

func (m *Manager) tftpRelease() {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	m.up.users--
	if m.up.users <= 0 && m.up.tftp != nil {
		m.up.tftp.Close()
		m.up.tftp, m.up.users = nil, 0
	}
}

// offer only serves images that a running job is copying.
func (m *Manager) offer(name string) (string, bool) {
	m.up.mu.Lock()
	wanted := false
	for _, j := range m.up.jobs {
		if j.State == "running" && j.Image == name {
			wanted = true
			m.markSeenLocked(j.ID)
		}
	}
	m.up.mu.Unlock()
	if !wanted {
		return "", false
	}
	p, err := m.lib.Path(name)
	return p, err == nil
}

func (m *Manager) tftpProgress(remote, name string, sent, total int64) {
	if total <= 0 {
		return
	}
	m.up.mu.Lock()
	var match []*Job
	for _, j := range m.up.jobs {
		if j.State == "running" && j.Image == name {
			match = append(match, j)
		}
	}
	m.up.mu.Unlock()
	if len(match) == 0 {
		return
	}
	target := match[0]
	if len(match) > 1 {
		for _, j := range match {
			if sw, ok := m.st.Switch(j.SwitchID); ok && sw.Host == remote {
				target = j
			}
		}
	}
	m.up.mu.Lock()
	m.markSeenLocked(target.ID)
	m.up.mu.Unlock()
	m.update(target, func(j *Job) { j.Progress = float64(sent) / float64(total) })
}

func (m *Manager) markSeenLocked(jobID string) {
	if m.up.seen == nil {
		m.up.seen = map[string]time.Time{}
	}
	m.up.seen[jobID] = time.Now()
}

func (m *Manager) lastSeen(jobID string) time.Time {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	return m.up.seen[jobID]
}

// tftpWatch cancels a TFTP copy when the switch never asks for the image, or
// the transfer stops moving, instead of waiting for the switch's own timeout
// (30 minutes on AOS-CX). It stops watching once the whole file has been sent,
// because the switch then spends a few minutes writing it to flash.
func (m *Manager) tftpWatch(ctx context.Context, j *Job, cancel context.CancelCauseFunc, local string) {
	start := time.Now()
	t := time.NewTicker(tftpWatchTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		m.up.mu.Lock()
		done := j.Progress >= 1
		m.up.mu.Unlock()
		if done {
			return
		}
		last := m.lastSeen(j.ID)
		switch {
		case last.IsZero() && time.Since(start) > tftpNoRequest:
			cancel(fmt.Errorf("the switch never asked this computer (%s) for the image over TFTP in %s. Check that it can reach %s on UDP port 69, and that this computer's firewall lets SwitchDesk accept it", local, tftpNoRequest.Round(time.Second), local))
			return
		case !last.IsZero() && time.Since(last) > tftpStalled:
			cancel(fmt.Errorf("the TFTP transfer stopped moving for %s", tftpStalled.Round(time.Second)))
			return
		}
	}
}

func (m *Manager) runJob(j *Job) {
	m.update(j, func(j *Job) { j.State = "running" })
	if j.Auto && j.Reboot {
		set := m.st.Settings()
		if !inWindow(time.Now(), set.WindowStart, set.WindowEnd) {
			// queued too late: stage now, the next window does the reboot
			m.update(j, func(j *Job) { j.Reboot = false })
			m.logf(j, "Maintenance window has ended; copying only, the reboot waits for the next window")
		}
	}
	sw, ok := m.st.Switch(j.SwitchID)
	if !ok {
		m.fail(j, errors.New("switch was removed"))
		return
	}
	var spare string
	var fam aruba.Family
	err := m.withSession(j.SwitchID, 50*time.Minute, func(ctx context.Context, s *sshx.Session, f aruba.Family) error {
		fam = f
		m.step(j, "Reading firmware banks")
		banks, err := readBanks(ctx, s, f)
		if err != nil {
			return err
		}
		spare = banks.Spare()
		m.logf(j, "Primary %s, secondary %s, running from %s", orDash(banks.Primary), orDash(banks.Secondary), orDash(banks.Running))
		if banks.Version(spare) == j.To {
			m.step(j, fmt.Sprintf("%s is already in the %s bank, skipping the copy", j.To, spare))
		} else {
			if err := m.copyImage(ctx, j, sw, s, f, spare); err != nil {
				return err
			}
			m.update(j, func(j *Job) { j.Progress = 1 })
			banks, err = waitBank(ctx, s, f, spare, j.To, bankSettle)
			if err != nil {
				return err
			}
			if banks.Version(spare) != j.To {
				return fmt.Errorf("after the copy the %s bank holds %q, not %s", spare, banks.Version(spare), j.To)
			}
			m.step(j, fmt.Sprintf("Verified %s in the %s bank", j.To, spare))
		}
		m.step(j, "Setting "+spare+" as the boot bank")
		if err := aruba.SetDefaultBank(ctx, s, f, spare); err != nil {
			return err
		}
		if j.Reboot {
			m.step(j, "Saving config and rebooting from "+spare)
			m.update(j, func(j *Job) { j.rebooted = true })
			return aruba.Reboot(ctx, s, f, spare)
		}
		return nil
	})
	if err != nil {
		m.fail(j, err)
		return
	}
	go m.Enqueue(j.SwitchID) // refresh banks shown in the UI
	if !j.Reboot {
		m.update(j, func(j *Job) {
			j.State, j.Step, j.EndedAt = "staged", fmt.Sprintf("Ready: %s boots on the next restart", j.To), time.Now()
		})
		return
	}

	m.step(j, "Waiting for the switch to go down and come back")
	if err := m.waitBack(sw, fam); err != nil {
		m.fail(j, err)
		return
	}
	m.step(j, "Back online, checking the version")
	m.collect(j.SwitchID)
	m.mu.Lock()
	snap := m.snaps[j.SwitchID]
	m.mu.Unlock()
	got := ""
	if snap != nil {
		if v, ok := firmware.ParseVersion(snap.Facts.Software); ok {
			got = v.Text
		}
	}
	if got != j.To {
		m.fail(j, fmt.Errorf("the switch came back running %q instead of %s", got, j.To))
		return
	}
	m.logf(j, "Running %s", got)
	m.update(j, func(j *Job) { j.State, j.Step, j.EndedAt = "done", "Updated to "+got, time.Now() })
}

func readBanks(ctx context.Context, s *sshx.Session, f aruba.Family) (aruba.Banks, error) {
	images, err := s.Run(ctx, aruba.ImagesCommand(f), 60*time.Second)
	if err != nil {
		return aruba.Banks{}, err
	}
	ver, _ := s.Run(ctx, "show version", 30*time.Second)
	b := aruba.ParseBanks(f, images, ver)
	if !b.Known {
		return b, errors.New("couldn't read the firmware banks from the switch")
	}
	return b, nil
}

// vrfFor picks the AOS-CX VRF that reaches this computer: mgmt when the
// address we manage the switch on belongs to the mgmt port.
func (m *Manager) vrfFor(sw store.Switch, f aruba.Family) string {
	if f != aruba.FamilyAOSCX {
		return ""
	}
	m.mu.Lock()
	snap := m.snaps[sw.ID]
	m.mu.Unlock()
	if snap == nil {
		return "default"
	}
	in := false
	for _, l := range strings.Split(snap.RunningConfig, "\n") {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") {
			in = t == "interface mgmt"
			continue
		}
		if in && strings.HasPrefix(t, "ip static "+sw.Host+"/") {
			return "mgmt"
		}
		if in && t == "ip dhcp" {
			return "mgmt"
		}
	}
	return "default"
}

// waitBack waits for SSH to stop answering, then to answer again.
func (m *Manager) waitBack(sw store.Switch, fam aruba.Family) error {
	addr := net.JoinHostPort(sw.Host, strconv.Itoa(sw.Port))
	up := func() bool {
		c, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err == nil {
			c.Close()
		}
		return err == nil
	}
	down := time.Now().Add(5 * time.Minute)
	for up() && time.Now().Before(down) {
		if m.sleep(pollDown) {
			return errors.New("SwitchDesk was closed during the update")
		}
	}
	back := time.Now().Add(25 * time.Minute)
	for !up() {
		if time.Now().After(back) {
			return errors.New("the switch hasn't come back after 25 minutes; check it on the console")
		}
		if m.sleep(pollUp) {
			return errors.New("SwitchDesk was closed during the update")
		}
	}
	// SSH answers before the CLI is fully ready
	if m.sleep(settleAfter) {
		return errors.New("SwitchDesk was closed during the update")
	}
	return nil
}

func (m *Manager) sleep(d time.Duration) (stopped bool) {
	select {
	case <-m.ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
}

func lastLines(s string, n int) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			keep = append(keep, t)
		}
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, " / ")
}

// ---------- automatic mode ----------

func inWindow(now time.Time, start, end string) bool {
	s, err1 := time.Parse("15:04", start)
	e, err2 := time.Parse("15:04", end)
	if err1 != nil || err2 != nil {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	a, b := s.Hour()*60+s.Minute(), e.Hour()*60+e.Minute()
	if a <= b {
		return cur >= a && cur < b
	}
	return cur >= a || cur < b // window over midnight
}

func (m *Manager) autoLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
		m.autoTick(time.Now())
	}
}

func (m *Manager) autoTick(now time.Time) {
	set := m.st.Settings()
	if set.FirmwareAuto != "stage" && set.FirmwareAuto != "install" {
		return
	}
	install := set.FirmwareAuto == "install" && inWindow(now, set.WindowStart, set.WindowEnd)
	sws := m.st.Switches()
	sort.Slice(sws, func(i, k int) bool { return sws[i].ID < sws[k].ID })
	var reboots []string
	images := map[string]string{}
	for _, sw := range sws {
		if sw.Disabled {
			continue
		}
		m.mu.Lock()
		snap := m.snaps[sw.ID]
		online := m.status[sw.ID] != nil && m.status[sw.ID].Online
		m.mu.Unlock()
		if !online || snap == nil {
			continue
		}
		if sw.FirmwareHold {
			continue
		}
		fi := m.firmwareFor(sw, snap)
		if !fi.Supported || fi.Available == "" {
			continue
		}
		if last := m.jobFor(sw.ID); last != nil {
			if last.active() {
				continue
			}
			// don't retry a failed version for a day, nor a reboot a failure stopped
			if (last.State == "failed" || last.halted) && last.To == fi.Available && now.Sub(last.EndedAt) < 24*time.Hour {
				continue
			}
		}
		switch {
		case install:
			reboots = append(reboots, sw.ID)
			images[sw.ID] = fi.Image
		case !fi.Staged:
			m.newJob(sw.ID, fi.Image, false, true, "", 0) // copy to the spare bank only
		}
	}
	// copies if needed, then reboots: downstream switches first, as one batch (#34)
	batch := ""
	if len(reboots) > 1 {
		batch = store.NewID()
		reboots = m.upgradeOrder(reboots)
	}
	for i, id := range reboots {
		m.newJob(id, images[id], true, true, batch, i+1)
	}
}

// Library gives the app access to the firmware library (nil if it couldn't be opened).
func (m *Manager) Library() *firmware.Library { return m.lib }

// copyImage gets the image into the spare bank. The image is sent from this
// computer to the switch: over SSH (SFTP or SCP) on AOS-S, and over HTTPS to
// the REST API on AOS-CX. That works over VPNs and zero trust access where the
// switch can't reach this computer. TFTP, where the switch pulls the file
// from us, is only the fallback.
func (m *Manager) copyImage(ctx context.Context, j *Job, sw store.Switch, s *sshx.Session, f aruba.Family, spare string) error {
	var pushErr error
	via, tried := "SSH", false
	if target, ok := aruba.PushTarget(f, spare); ok {
		tried = true
		pushErr = m.pushImage(ctx, j, sw, s, target, spare)
	} else if bank, ok := aruba.CXBank(spare); ok && f == aruba.FamilyAOSCX {
		via, tried = "HTTPS", true
		pushErr = m.pushImageCX(ctx, j, sw, s, bank)
	}
	if tried && pushErr == nil {
		return nil
	}
	if pushErr != nil {
		m.logf(j, "Upload over %s failed: %v", via, pushErr)
		m.logf(j, "Trying TFTP instead, where the switch downloads the image from this computer")
	}
	err := m.tftpCopy(ctx, j, sw, s, f, spare)
	if err != nil && pushErr != nil {
		return fmt.Errorf("upload over %s failed (%v), and TFTP failed too (%v)", via, pushErr, err)
	}
	return err
}

func (m *Manager) pushImage(ctx context.Context, j *Job, sw store.Switch, s *sshx.Session, target, spare string) error {
	p, err := m.lib.Path(j.Image)
	if err != nil {
		return err
	}
	file, err := os.Open(p)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		return err
	}

	m.step(j, "Turning on file transfer over SSH (ip ssh filetransfer)")
	st, err := aruba.EnableFileTransfer(ctx, s)
	if err != nil {
		return err
	}
	defer func() {
		if cmds := aruba.RestoreCommands(st); len(cmds) > 0 {
			// the session may be near its deadline after a slow upload
			rctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := aruba.RestoreFileTransfer(rctx, s, st); err != nil {
				m.logf(j, "Couldn't restore the file transfer settings: %v. Run these on the switch: %s", err, strings.Join(cmds, "; "))
			} else {
				m.logf(j, "Restored the previous settings: %s", strings.Join(cmds, "; "))
			}
		}
	}()

	m.step(j, fmt.Sprintf("Uploading %s (%s) to the %s bank over SSH", j.Image, humanBytes(info.Size()), spare))
	o := sshx.Options{Host: sw.Host, Port: sw.Port, Username: cred.Username, Password: cred.Password, KnownKey: sw.HostKey, Timeout: 15 * time.Second}
	start := time.Now()
	method, err := upload(ctx, o, target, file, info.Size(), func(sent, total int64) {
		if total > 0 {
			m.update(j, func(j *Job) { j.Progress = float64(sent) / float64(total) })
		}
	})
	if err != nil {
		return err
	}
	m.logf(j, "Uploaded with %s in %s", strings.ToUpper(method), time.Since(start).Round(time.Second))
	return nil
}

// pushImageCX uploads the image through the AOS-CX REST API. AOS-CX has no
// SFTP/SCP server, and HTTPS goes from this computer to the switch like SSH
// does, so it works where the switch can't reach this computer.
func (m *Manager) pushImageCX(ctx context.Context, j *Job, sw store.Switch, s *sshx.Session, bank string) error {
	p, err := m.lib.Path(j.Image)
	if err != nil {
		return err
	}
	file, err := os.Open(p)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	cred, err := m.st.Credential(sw.CredentialID)
	if err != nil {
		return err
	}
	vrf := m.vrfFor(sw, aruba.FamilyAOSCX)
	st, err := aruba.EnableHTTPS(ctx, s, vrf)
	if err != nil {
		return fmt.Errorf("turning on the web server (https-server vrf %s): %w", vrf, err)
	}
	if ch := aruba.HTTPSChanges(st); len(ch) > 0 {
		m.step(j, "Changed for the upload: "+strings.Join(ch, "; "))
	}
	defer func() {
		if cmds := aruba.HTTPSRestoreCommands(st); len(cmds) > 0 {
			rctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := aruba.RestoreHTTPS(rctx, s, st); err != nil {
				m.logf(j, "Couldn't restore the web server settings: %v. Run these on the switch: %s", err, strings.Join(cmds, "; "))
			} else {
				m.logf(j, "Restored the previous settings: %s", strings.Join(cmds, "; "))
			}
		}
	}()

	m.step(j, fmt.Sprintf("Uploading %s (%s) to the %s bank over HTTPS", j.Image, humanBytes(info.Size()), bank))
	start := time.Now()
	base := "https://" + net.JoinHostPort(sw.Host, cxHTTPSPort)
	pin := &tlspin.Pin{Want: sw.CertFP}
	err = cxPush(ctx, base, pin, cred.Username, cred.Password, bank, j.Image, file, info.Size(), func(sent, total int64) {
		if total > 0 {
			m.update(j, func(j *Job) { j.Progress = float64(sent) / float64(total) })
		}
	})
	if sw.CertFP == "" && pin.Seen() != "" && !errors.Is(err, tlspin.ErrChanged) {
		m.st.SetCertFP(sw.ID, pin.Seen())
	}
	if errors.Is(err, tlspin.ErrChanged) {
		return fmt.Errorf("%w; not sending the admin password. If the switch was replaced or its certificate renewed, use Trust new key on its page", err)
	}
	if err != nil {
		if errors.Is(err, cxrest.ErrNoREST) {
			err = fmt.Errorf("%w. Check that this computer can reach %s on TCP 443", err, sw.Host)
		}
		return err
	}
	m.logf(j, "Uploaded over HTTPS in %s; the switch checked the image", time.Since(start).Round(time.Second))
	return nil
}

// swapped out in tests
var (
	cxPush      = cxrest.Push
	cxHTTPSPort = "443"
)

// upload is swapped out in tests.
var upload = sshx.Upload

func (m *Manager) tftpCopy(ctx context.Context, j *Job, sw store.Switch, s *sshx.Session, f aruba.Family, spare string) error {
	if err := m.tftpAcquire(); err != nil {
		return err
	}
	defer m.tftpRelease()
	local, err := firmware.LocalIPFor(sw.Host)
	if err != nil {
		return fmt.Errorf("no route to %s: %v", sw.Host, err)
	}
	vrf := m.vrfFor(sw, f)
	m.step(j, fmt.Sprintf("Copying %s to the %s bank from %s over TFTP", j.Image, spare, local))
	m.up.mu.Lock()
	delete(m.up.seen, j.ID)
	m.up.mu.Unlock()
	wctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go m.tftpWatch(wctx, j, cancel, local)
	out, err := aruba.CopyImage(wctx, s, f, local, j.Image, spare, vrf)
	if out != "" {
		m.logf(j, "%s", lastLines(out, 4))
	}
	if cause := context.Cause(wctx); err != nil && cause != nil && !errors.Is(cause, context.Canceled) {
		// stop the copy on the switch so the next command doesn't land in it
		s.Interrupt()
		return cause
	}
	if err != nil && strings.Contains(strings.ToLower(out), "peer unreachable") {
		err = fmt.Errorf("%w. The switch couldn't reach %s on UDP 69; this happens over VPN or zero trust access", err, local)
	}
	return err
}

// waitBank re-reads the banks until want shows up in bank or wait runs out.
// AOS-S keeps writing an uploaded image to flash for a little while after the
// transfer ends.
func waitBank(ctx context.Context, s *sshx.Session, f aruba.Family, bank, want string, wait time.Duration) (aruba.Banks, error) {
	deadline := time.Now().Add(wait)
	for {
		b, err := readBanks(ctx, s, f)
		if err == nil && b.Version(bank) == want {
			return b, nil
		}
		if time.Now().After(deadline) {
			return b, err
		}
		select {
		case <-ctx.Done():
			return b, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
