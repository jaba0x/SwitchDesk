package store

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jaba0x/switchdesk/internal/aruba"
)

// rewriteInventory copies a .sdbak, replacing text inside its inventory.json,
// as someone crafting an export could.
func rewriteInventory(t *testing.T, src, dst, old, new string) {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out, _ := os.Create(dst)
	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		if f.Name == "inventory.json" {
			b = []byte(strings.ReplaceAll(string(b), old, new))
		}
		w, _ := zw.Create(f.Name)
		w.Write(b)
	}
	zw.Close()
	out.Close()
}

// Switch IDs from an export are used in file paths and the UI. A crafted ID
// must never survive an import, in either mode.
func TestImportReplacesUnsafeIDs(t *testing.T) {
	root := t.TempDir()
	a, swA := seed(t, filepath.Join(root, "a"), "10.0.0.1")
	good := filepath.Join(root, "x.sdbak")
	if err := a.Export(good, "correct horse", true); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "bad.sdbak")
	evil := `../../victim\"><img src=x onerror=alert(1)>`
	rewriteInventory(t, good, bad, swA.ID, evil)
	if info, err := InspectBundle(bad, "correct horse"); err != nil || info.Switches != 1 {
		t.Fatal("crafted bundle unreadable", info, err)
	}
	zr, _ := zip.OpenReader(bad)
	planted := false
	for _, f := range zr.File {
		if f.Name == "inventory.json" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			planted = strings.Contains(string(b), "../../victim")
		}
	}
	zr.Close()
	if !planted {
		t.Fatal("test setup: crafted id not in the bundle")
	}

	victim := filepath.Join(root, "victim")
	os.MkdirAll(victim, 0o700)
	os.WriteFile(filepath.Join(victim, "keep.cfg"), []byte("x"), 0o600)

	hex := regexp.MustCompile(`^[0-9a-f]{12}$`)
	for _, mode := range []string{"merge", "replace"} {
		b, err := Open(filepath.Join(root, "b-"+mode, "data"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Import(bad, "correct horse", mode); err != nil {
			t.Fatal(mode, err)
		}
		for _, sw := range b.Switches() {
			if !hex.MatchString(sw.ID) {
				t.Fatalf("%s: unsafe id kept: %q", mode, sw.ID)
			}
			b.DeleteSwitch(sw.ID)
		}
	}
	if _, err := os.Stat(filepath.Join(victim, "keep.cfg")); err != nil {
		t.Fatal("a file outside the data folder was touched:", err)
	}
}

func TestOpenBundleIgnoresNonCanonicalArchivePaths(t *testing.T) {
	root := t.TempDir()
	s, sw := seed(t, filepath.Join(root, "source"), "10.0.0.1")
	good := filepath.Join(root, "good.sdbak")
	if err := s.Export(good, "correct horse", true); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(good)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	bad := filepath.Join(root, "bad.sdbak")
	out, err := os.Create(bad)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		if f.Name == "snapshots/"+sw.ID+".json" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			r.Close()
			t.Fatal(err)
		}
		if _, err := io.Copy(w, r); err != nil {
			r.Close()
			t.Fatal(err)
		}
		r.Close()
	}
	if _, err := zw.Create("snapshots/../snapshots/" + sw.ID + ".json"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := openBundle(bad, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	defer b.zr.Close()
	if _, ok := b.files["snapshots/"+sw.ID+".json"]; ok {
		t.Fatal("non-canonical archive path was accepted")
	}
}

// Even with a bad ID already on disk, the store never builds a path from it,
// and Open gives the switch a fresh ID.
func TestBadIDOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, sw := seed(t, dir, "10.0.0.9")
	if _, err := s.ReadBackup("../..", "x.cfg"); err == nil {
		t.Fatal("ReadBackup accepted a traversal id")
	}
	if err := s.SaveSnapshot("../x", aruba.Snapshot{}); err == nil {
		t.Fatal("SaveSnapshot accepted a traversal id")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "inventory.json"))
	os.WriteFile(filepath.Join(dir, "inventory.json"), []byte(strings.ReplaceAll(string(b), sw.ID, "../evil")), 0o600)
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if id := s2.Switches()[0].ID; !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("bad id kept on open: %q", id)
	}
}
