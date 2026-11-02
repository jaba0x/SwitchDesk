package main

import (
	"errors"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jaba0x/switchdesk/internal/core"
	"github.com/jaba0x/switchdesk/internal/store"
)

// ---- data folder ----

type DataFolder struct {
	Path        string `json:"path"`
	DefaultPath string `json:"defaultPath"`
	Custom      bool   `json:"custom"`
}

func (a *App) DataFolder() DataFolder {
	def, _ := store.DefaultDir()
	return DataFolder{Path: a.st.Dir(), DefaultPath: def, Custom: !strings.EqualFold(a.st.Dir(), def)}
}

// StartupNotice returns (once) a warning raised while opening the data folder.
func (a *App) StartupNotice() string {
	n := a.notice
	a.notice = ""
	return n
}

// PickDataFolder opens a folder chooser and describes what's in the folder.
func (a *App) PickDataFolder() (*store.DirInfo, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose a data folder", CanCreateDirectories: true})
	if err != nil || dir == "" {
		return nil, err
	}
	info := store.InspectDir(dir, a.st.Dir())
	return &info, nil
}

func (a *App) DefaultFolderInfo() store.DirInfo {
	def, _ := store.DefaultDir()
	return store.InspectDir(def, a.st.Dir())
}

// ChangeDataFolder switches to dir.
//
//	move: copy current data there (overwriting), then remove it from the old folder
//	copy: copy current data there, keep the old folder as it is
//	use:  open whatever is already in dir
func (a *App) ChangeDataFolder(dir, mode string) error {
	if dir == "" {
		return errors.New("no folder chosen")
	}
	info := store.InspectDir(dir, a.st.Dir())
	if info.Same {
		return errors.New("that's already the data folder")
	}
	if !info.Writable {
		return errors.New("SwitchDesk can't write to that folder")
	}
	if mode == "use" && !info.HasData {
		return errors.New("that folder has no SwitchDesk data")
	}
	return a.swap(func() (*store.Store, error) {
		if mode == "move" || mode == "copy" {
			if err := a.st.CopyData(dir); err != nil {
				return nil, err
			}
		}
		next, err := store.Open(dir)
		if err != nil {
			return nil, err
		}
		if next.Locked() {
			if err := next.UnlockLike(a.st); err != nil {
				return nil, err
			}
		}
		if err := store.SetLocation(dir); err != nil {
			return nil, err
		}
		if mode == "move" {
			a.st.RemoveData()
		}
		return next, nil
	})
}

// swap stops polling, replaces the store and starts again.
func (a *App) swap(fn func() (*store.Store, error)) error {
	a.swapMu.Lock()
	defer a.swapMu.Unlock()
	if a.mgr == nil {
		return store.ErrLocked
	}
	a.mgr.Stop()
	next, err := fn()
	if err != nil {
		a.mgr = core.New(a.st, a.emit) // keep running on the old folder
		a.mgr.Start()
		return err
	}
	a.st = next
	a.mgr = core.New(a.st, a.emit)
	a.mgr.Start()
	a.emit("")
	return nil
}

func (a *App) OpenDataFolder() error { return openPath(a.st.Dir()) }

func openPath(dir string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// ---- export / import ----

func (a *App) ExportData(pass string, history bool) (string, error) {
	if len(pass) < 8 {
		return "", errors.New("use a passphrase of at least 8 characters")
	}
	name := "SwitchDesk-" + time.Now().Format("2006-01-02") + ".sdbak"
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Export SwitchDesk data", DefaultFilename: name,
		Filters: []runtime.FileFilter{{DisplayName: "SwitchDesk export (*.sdbak)", Pattern: "*.sdbak"}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("cancelled")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".sdbak") {
		path += ".sdbak"
	}
	return path, a.st.Export(path, pass, history)
}

func (a *App) PickImportFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import SwitchDesk data",
		Filters: []runtime.FileFilter{{DisplayName: "SwitchDesk export (*.sdbak)", Pattern: "*.sdbak"}},
	})
}

func (a *App) InspectImport(path, pass string) (store.BundleInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return store.BundleInfo{}, errors.New("file not found")
	}
	return store.InspectBundle(path, pass)
}

func (a *App) ImportData(path, pass, mode string) (store.ImportResult, error) {
	var res store.ImportResult
	err := a.swap(func() (*store.Store, error) {
		var err error
		res, err = a.st.Import(path, pass, mode)
		if err != nil {
			return nil, err
		}
		next, err := store.Open(a.st.Dir()) // reload snapshots and settings from disk
		if err == nil && next.Locked() {
			err = next.UnlockLike(a.st)
		}
		return next, err
	})
	return res, err
}
