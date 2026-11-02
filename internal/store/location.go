package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The default folder always holds location.json, which may point the app at a
// different data folder (another disk, a synced drive, a shared folder).

type location struct {
	DataDir string `json:"dataDir"`
}

func pointerPath() (string, error) {
	d, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "location.json"), nil
}

// ResolveDir returns the folder to open. notice is non-empty when a custom
// folder is configured but can't be used, so the app fell back to the default.
func ResolveDir() (dir string, custom bool, notice string, err error) {
	def, err := DefaultDir()
	if err != nil {
		return "", false, "", err
	}
	p, _ := pointerPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return def, false, "", nil
	}
	var loc location
	if json.Unmarshal(b, &loc) != nil || loc.DataDir == "" {
		return def, false, "", nil
	}
	if st, err := os.Stat(loc.DataDir); err != nil || !st.IsDir() {
		return def, false, fmt.Sprintf("The data folder %s isn't available, so SwitchDesk opened the default folder. Reconnect the drive and restart, or pick another folder in Settings.", loc.DataDir), nil
	}
	return loc.DataDir, true, "", nil
}

// SetLocation stores the custom folder; an empty dir resets to the default.
func SetLocation(dir string) error {
	def, err := DefaultDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(def, 0o700); err != nil {
		return err
	}
	p := filepath.Join(def, "location.json")
	if dir == "" || samePath(dir, def) {
		err := os.Remove(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, _ := json.MarshalIndent(location{DataDir: dir}, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

func samePath(a, b string) bool {
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(ca), filepath.Clean(cb))
}

// DirInfo describes a candidate data folder.
type DirInfo struct {
	Path     string `json:"path"`
	HasData  bool   `json:"hasData"`
	Switches int    `json:"switches"`
	Writable bool   `json:"writable"`
	Same     bool   `json:"same"`
}

func InspectDir(dir, current string) DirInfo {
	info := DirInfo{Path: dir, Same: samePath(dir, current)}
	if b, err := os.ReadFile(filepath.Join(dir, "inventory.json")); err == nil {
		var d data
		if json.Unmarshal(b, &d) == nil {
			info.HasData = true
			info.Switches = len(d.Switches)
		}
	}
	probe := filepath.Join(dir, ".switchdesk-write-test")
	if os.WriteFile(probe, []byte("ok"), 0o600) == nil {
		info.Writable = true
		os.Remove(probe)
	}
	return info
}

// dataEntries are the files and folders that make up a data folder.
var dataEntries = []string{"inventory.json", "secret.key", "secret.lock", "snapshots", "backups", "firmware"}

// CopyData copies this store's files into dst (overwriting what's there).
func (s *Store) CopyData(dst string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, e := range dataEntries {
		src := filepath.Join(s.dir, e)
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			if e == "secret.key" || e == "secret.lock" {
				os.Remove(filepath.Join(dst, e)) // the key goes over exactly as it is here
			}
			continue
		}
		os.RemoveAll(filepath.Join(dst, e))
		if err := copyTree(src, filepath.Join(dst, e)); err != nil {
			return fmt.Errorf("copying %s: %w", e, err)
		}
	}
	return nil
}

// RemoveData deletes the data files from this store's folder (after a move).
// location.json and anything else in the folder is left alone.
func (s *Store) RemoveData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range dataEntries {
		os.RemoveAll(filepath.Join(s.dir, e))
	}
}

func copyTree(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst, st.Mode().Perm())
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
