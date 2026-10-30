package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jaba0x/switchdesk/internal/aruba"
	"github.com/jaba0x/switchdesk/internal/core"
	"github.com/jaba0x/switchdesk/internal/store"
)

// App is the object bound to the frontend; every exported method is callable
// from JavaScript as window.go.main.App.<Method>.
type App struct {
	ctx context.Context
	st  *store.Store
	mgr *core.Manager

	emitMu   sync.Mutex
	emitWait bool

	swapMu sync.Mutex // held while the data folder is being swapped
	notice string     // shown once in the UI after startup
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dir, _, notice, err := store.ResolveDir()
	a.notice = notice
	if err == nil {
		a.st, err = store.Open(dir)
	}
	if err != nil {
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.ErrorDialog, Title: "SwitchDesk", Message: "Can't open data folder: " + err.Error()})
		os.Exit(1)
	}
	a.mgr = core.New(a.st, a.emit)
	a.mgr.Start()
	go a.updateLoop()
}

func (a *App) shutdown(ctx context.Context) {
	if a.mgr != nil {
		a.mgr.Stop()
	}
}

// emit coalesces bursts of change notifications into one UI refresh.
func (a *App) emit(string) {
	a.emitMu.Lock()
	if a.emitWait {
		a.emitMu.Unlock()
		return
	}
	a.emitWait = true
	a.emitMu.Unlock()
	time.AfterFunc(400*time.Millisecond, func() {
		a.emitMu.Lock()
		a.emitWait = false
		a.emitMu.Unlock()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "changed")
		}
	})
}

func (a *App) Version() string  { return Version }
func (a *App) Platform() string { return goruntime.GOOS }

// ---- views ----

func (a *App) GetDashboard() core.Dashboard { return a.mgr.Dashboard() }

func (a *App) GetSwitch(id string) (core.Detail, error) { return a.mgr.Detail(id) }

func (a *App) GetTopology(showEndpoints bool) core.Topology { return a.mgr.Topology(showEndpoints) }

func (a *App) SaveLayout(l map[string]store.Point) error { return a.st.SaveLayout(l) }

// ---- inventory ----

func (a *App) SaveSwitch(sw store.Switch) (store.Switch, error) {
	isNew := sw.ID == ""
	saved, err := a.st.SaveSwitch(sw)
	if err != nil {
		return saved, err
	}
	a.mgr.ClearAuthFailure(saved.ID)
	if isNew {
		go func() {
			a.mgr.Enqueue(saved.ID)
		}()
	}
	a.emit("")
	return saved, nil
}

type BulkResult struct {
	Added  int      `json:"added"`
	Errors []string `json:"errors"`
}

// BulkAdd takes one address per line (optionally "name,address" or "address name"),
// or an IP range like 10.0.0.10-10.0.0.40.
func (a *App) BulkAdd(text, credentialID, site string, family string) BulkResult {
	res := BulkResult{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, host := "", line
		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			name, host = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if net.ParseIP(name) != nil && net.ParseIP(host) == nil { // "ip,name"
				name, host = host, name
			}
		} else if f := strings.Fields(line); len(f) == 2 {
			host, name = f[0], f[1]
		}
		hosts := []string{host}
		if a1, b1, ok := strings.Cut(host, "-"); ok && net.ParseIP(a1) != nil {
			hosts = expandRange(a1, b1)
			name = ""
		}
		for _, h := range hosts {
			sw := store.Switch{Name: name, Host: h, Port: 22, CredentialID: credentialID, Site: site, Family: aruba.Family(family)}
			if sw.Family == aruba.FamilyInstantOn {
				// Instant On switches have no SSH CLI: read them through their web interface
				sw.Web, sw.Port = true, 443
			}
			_, err := a.st.SaveSwitch(sw)
			if err != nil {
				res.Errors = append(res.Errors, h+": "+err.Error())
				continue
			}
			res.Added++
		}
	}
	if res.Added > 0 {
		go a.mgr.RefreshAll()
		a.emit("")
	}
	return res
}

func expandRange(from, to string) []string {
	ip := net.ParseIP(from).To4()
	if ip == nil {
		return []string{from + "-" + to}
	}
	end := ip[3]
	if n, err := strconv.Atoi(to); err == nil && n < 256 {
		end = byte(n)
	} else if t := net.ParseIP(to).To4(); t != nil && t[0] == ip[0] && t[1] == ip[1] && t[2] == ip[2] {
		end = t[3]
	}
	var out []string
	for i := int(ip[3]); i <= int(end); i++ {
		out = append(out, fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], i))
	}
	return out
}

func (a *App) DeleteSwitch(id string) error {
	if err := a.st.DeleteSwitch(id); err != nil {
		return err
	}
	a.mgr.Forget(id)
	a.emit("")
	return nil
}

// DeleteSwitches removes several switches and their saved data.
func (a *App) DeleteSwitches(ids []string) error {
	for _, id := range ids {
		if err := a.st.DeleteSwitch(id); err != nil {
			return err
		}
		a.mgr.Forget(id)
	}
	a.emit("")
	return nil
}

// SetDisabled pauses (no polling, no SSH, no alerts) or resumes switches.
func (a *App) SetDisabled(ids []string, disabled bool) error {
	if err := a.st.SetDisabled(ids, disabled); err != nil {
		return err
	}
	if !disabled {
		go func() {
			for _, id := range ids {
				a.mgr.Resume(id)
			}
		}()
	}
	a.emit("")
	return nil
}

func (a *App) TestSwitch(sw store.Switch) (string, error) { return a.mgr.TestConnection(sw) }

// UseWebInterface moves Instant On switches added for SSH over to their web
// interface, and reads them again. ids empty means every such switch.
func (a *App) UseWebInterface(ids []string) (int, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	n := 0
	for _, sw := range a.st.Switches() {
		if sw.Web || sw.Cloud != nil || sw.Family != aruba.FamilyInstantOn || (len(ids) > 0 && !want[sw.ID]) {
			continue
		}
		sw.Web, sw.Port = true, 443
		if _, err := a.st.SaveSwitch(sw); err != nil {
			return n, err
		}
		a.mgr.ClearAuthFailure(sw.ID)
		go a.mgr.Enqueue(sw.ID)
		n++
	}
	a.emit("")
	return n, nil
}

// DiscoverCloud lists the switches on an Instant On account's sites.
func (a *App) DiscoverCloud(credentialID string) ([]core.CloudSite, error) {
	return a.mgr.DiscoverCloud(credentialID)
}

// AddCloud adds Instant On switches picked from DiscoverCloud.
func (a *App) AddCloud(credentialID string, picks []core.CloudPick) (int, error) {
	n, err := a.mgr.AddCloud(credentialID, picks)
	a.emit("")
	return n, err
}

func (a *App) Refresh(id string) { a.mgr.Enqueue(id) }

func (a *App) RefreshAll() { a.mgr.RefreshAll() }

func (a *App) TrustHostKey(id string) { a.mgr.TrustNewHostKey(id) }

// ---- credentials & settings ----

func (a *App) Credentials() []store.Credential { return a.st.Credentials() }

func (a *App) SaveCredential(c store.Credential) (store.Credential, error) {
	out, err := a.st.SaveCredential(c)
	if err == nil {
		for _, sw := range a.st.Switches() {
			if sw.CredentialID == out.ID {
				a.mgr.ClearAuthFailure(sw.ID)
			}
		}
	}
	return out, err
}

func (a *App) DeleteCredential(id string) error { return a.st.DeleteCredential(id) }

func (a *App) GetSettings() store.Settings { return a.st.Settings() }

func (a *App) SaveSettings(s store.Settings) error {
	err := a.st.SaveSettings(s)
	a.emit("")
	return err
}

// ---- actions on a switch ----

func (a *App) RunCommand(id, cmd string) (string, error) { return a.mgr.RunCommand(id, cmd) }

func (a *App) PreviewVLAN(id string, ch aruba.VLANChange) ([]string, error) {
	return a.mgr.PreviewVLAN(id, ch)
}

func (a *App) ApplyVLAN(id string, ch aruba.VLANChange) (aruba.ApplyResult, error) {
	return a.mgr.ApplyVLAN(id, ch)
}

func (a *App) PreviewPortAccess(id string, ch aruba.PortAccessChange) ([]string, error) {
	return a.mgr.PreviewPortAccess(id, ch)
}

func (a *App) ApplyPortAccess(id string, ch aruba.PortAccessChange) (aruba.ApplyResult, error) {
	return a.mgr.ApplyPortAccess(id, ch)
}

func (a *App) RefreshPortClients(id string) ([]aruba.PortClient, error) {
	return a.mgr.RefreshPortClients(id)
}

func (a *App) GetBackup(id, name string) (string, error) { return a.st.ReadBackup(id, name) }

// ExportText asks where to save and writes the content (configs, raw output).
func (a *App) ExportText(defaultName, content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: defaultName, Title: "Save"})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("cancelled")
	}
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func (a *App) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		runtime.BrowserOpenURL(a.ctx, u)
	}
}

func (a *App) VLANOverview() []core.SwitchVLANs { return a.mgr.VLANOverview() }
