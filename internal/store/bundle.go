package store

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// A bundle is a zip with a manifest, the inventory (passwords re-encrypted
// with a key derived from the passphrase) and optionally snapshots and config
// copies. It carries no machine key, so it opens on any computer.

const bundleFormat = 1

type manifest struct {
	App       string    `json:"app"`
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Salt      string    `json:"salt"`
	Check     string    `json:"check"` // encrypts a known value so a wrong passphrase is caught early
	Switches  int       `json:"switches"`
	History   bool      `json:"history"`
}

type BundleInfo struct {
	CreatedAt time.Time `json:"createdAt"`
	Switches  int       `json:"switches"`
	History   bool      `json:"history"`
}

func deriveKey(pass string, salt []byte) []byte {
	return argon2.IDKey([]byte(pass), salt, 3, 64*1024, 2, 32)
}

func sealWith(key []byte, plain string) string {
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))
}

func openWith(key []byte, v string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("short ciphertext")
	}
	p, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(p), err
}

var ErrBadPassphrase = errors.New("wrong passphrase")

// Export writes a bundle to path.
func (s *Store) Export(dst, pass string, history bool) error {
	if len(pass) < 8 {
		return errors.New("use a passphrase of at least 8 characters")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	key := deriveKey(pass, salt)

	s.mu.Lock()
	d := s.d
	d.Credentials = make([]Credential, len(s.d.Credentials))
	for i, c := range s.d.Credentials {
		p, err := s.decrypt(c.Password)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("can't read password for %q: %w", c.Name, err)
		}
		c.Password = "bundle:" + sealWith(key, p)
		d.Credentials[i] = c
	}
	inv, err := json.MarshalIndent(d, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}

	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	write := func(name string, b []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	m := manifest{App: "SwitchDesk", Format: bundleFormat, CreatedAt: time.Now(), Salt: base64.StdEncoding.EncodeToString(salt), Check: sealWith(key, "switchdesk"), Switches: len(d.Switches), History: history}
	mb, _ := json.MarshalIndent(m, "", "  ")
	err = write("manifest.json", mb)
	if err == nil {
		err = write("inventory.json", inv)
	}
	if err == nil && history {
		for _, sub := range []string{"snapshots", "backups"} {
			root := filepath.Join(s.dir, sub)
			err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(s.dir, p)
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				return write(filepath.ToSlash(rel), b)
			})
			if err != nil {
				break
			}
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

type bundle struct {
	m     manifest
	key   []byte
	d     data
	files map[string]*zip.File // snapshots/... and backups/...
	zr    *zip.ReadCloser
}

func openBundle(src, pass string) (*bundle, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, errors.New("this isn't a SwitchDesk export file")
	}
	b := &bundle{zr: zr, files: map[string]*zip.File{}}
	read := func(f *zip.File) ([]byte, error) {
		if f.UncompressedSize64 > 200<<20 {
			return nil, errors.New("file inside the export is too large")
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	var invFile *zip.File
	for _, f := range zr.File {
		if !filepath.IsLocal(filepath.FromSlash(f.Name)) {
			continue
		}
		name := path.Clean(f.Name)
		if name != f.Name {
			continue
		}
		switch {
		case name == "manifest.json":
			raw, err := read(f)
			if err != nil || json.Unmarshal(raw, &b.m) != nil {
				zr.Close()
				return nil, errors.New("the export's manifest is damaged")
			}
		case name == "inventory.json":
			invFile = f
		case (strings.HasPrefix(name, "snapshots/") || strings.HasPrefix(name, "backups/")) && !strings.Contains(name, "..") && !path.IsAbs(name):
			b.files[name] = f
		}
	}
	if b.m.App != "SwitchDesk" || invFile == nil {
		zr.Close()
		return nil, errors.New("this isn't a SwitchDesk export file")
	}
	if b.m.Format > bundleFormat {
		zr.Close()
		return nil, errors.New("this export was made by a newer SwitchDesk; update the app first")
	}
	salt, _ := base64.StdEncoding.DecodeString(b.m.Salt)
	b.key = deriveKey(pass, salt)
	if v, err := openWith(b.key, b.m.Check); err != nil || v != "switchdesk" {
		zr.Close()
		return nil, ErrBadPassphrase
	}
	raw, err := read(invFile)
	if err == nil {
		err = json.Unmarshal(raw, &b.d)
	}
	if err != nil {
		zr.Close()
		return nil, errors.New("the export's inventory is damaged")
	}
	for i, c := range b.d.Credentials {
		if strings.HasPrefix(c.Password, "bundle:") {
			p, err := openWith(b.key, strings.TrimPrefix(c.Password, "bundle:"))
			if err != nil {
				zr.Close()
				return nil, ErrBadPassphrase
			}
			b.d.Credentials[i].Password = p
		}
	}
	return b, nil
}

// InspectBundle checks the passphrase and returns what's inside.
func InspectBundle(src, pass string) (BundleInfo, error) {
	b, err := openBundle(src, pass)
	if err != nil {
		return BundleInfo{}, err
	}
	defer b.zr.Close()
	return BundleInfo{CreatedAt: b.m.CreatedAt, Switches: len(b.d.Switches), History: b.m.History}, nil
}

type ImportResult struct {
	Added       int      `json:"added"`
	Skipped     []string `json:"skipped"`
	Credentials int      `json:"credentials"`
}

// Import loads a bundle. "replace" swaps the whole inventory; "merge" adds
// switches and credentials that aren't here yet (matched by address and port).
func (s *Store) Import(src, pass, mode string) (ImportResult, error) {
	res := ImportResult{}
	b, err := openBundle(src, pass)
	if err != nil {
		return res, err
	}
	defer b.zr.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	extract := func(oldID, newID string) {
		for name, f := range b.files {
			var target string
			switch {
			case name == "snapshots/"+oldID+".json":
				target = filepath.Join(s.dir, "snapshots", newID+".json")
			case strings.HasPrefix(name, "backups/"+oldID+"/") && validBackupName(path.Base(name)):
				target = filepath.Join(s.dir, "backups", newID, path.Base(name))
			default:
				continue
			}
			os.MkdirAll(filepath.Dir(target), 0o700)
			r, err := f.Open()
			if err != nil {
				continue
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err == nil {
				io.Copy(out, io.LimitReader(r, 200<<20))
				out.Close()
			}
			r.Close()
		}
	}
	// IDs in the file are never trusted as-is: they are used in file paths
	// and in the UI.
	// keepID: an ID from the file is kept only if it has the NewID form and
	// isn't in use here; otherwise the record gets a fresh one.
	used := func(id string) bool {
		for _, o := range s.d.Switches {
			if o.ID == id {
				return true
			}
		}
		for _, o := range s.d.Credentials {
			if o.ID == id {
				return true
			}
		}
		return false
	}
	keepID := func(id string) string {
		if validID(id) && !used(id) {
			return id
		}
		return NewID()
	}
	addCred := func(c Credential) (string, error) {
		enc, err := s.encrypt(c.Password)
		if err != nil {
			return "", err
		}
		c.ID, c.Password = keepID(c.ID), enc
		s.d.Credentials = append(s.d.Credentials, c)
		res.Credentials++
		return c.ID, nil
	}
	addSwitch := func(sw Switch, credMap map[string]string, layout map[string]Point) {
		oldID := sw.ID
		sw.ID = keepID(sw.ID)
		sw.CredentialID = credMap[sw.CredentialID]
		s.d.Switches = append(s.d.Switches, sw)
		if p, ok := b.d.Layout[oldID]; ok {
			layout[sw.ID] = p
		}
		if validID(oldID) {
			extract(oldID, sw.ID)
		}
		res.Added++
	}

	switch mode {
	case "replace":
		os.RemoveAll(filepath.Join(s.dir, "snapshots"))
		os.RemoveAll(filepath.Join(s.dir, "backups"))
		os.MkdirAll(filepath.Join(s.dir, "snapshots"), 0o700)
		os.MkdirAll(filepath.Join(s.dir, "backups"), 0o700)
		s.d.Switches, s.d.Credentials, s.d.Layout = nil, nil, map[string]Point{}
		s.d.Settings = normSettings(b.d.Settings)
		credMap := map[string]string{}
		for _, c := range b.d.Credentials {
			id, err := addCred(c)
			if err != nil {
				return res, err
			}
			credMap[c.ID] = id
		}
		for _, sw := range b.d.Switches {
			addSwitch(sw, credMap, s.d.Layout)
		}
	case "merge":
		credMap := map[string]string{}
		for _, c := range b.d.Credentials {
			// reuse an existing credential only when it is the same login, so
			// an imported switch can't be paired with a password it didn't bring
			found := ""
			for _, o := range s.d.Credentials {
				if o.Kind == c.Kind && o.Username == c.Username {
					if pw, err := s.decrypt(o.Password); err == nil && pw == c.Password {
						found = o.ID
						break
					}
				}
			}
			if found != "" {
				credMap[c.ID] = found
				continue
			}
			id, err := addCred(c)
			if err != nil {
				return res, err
			}
			credMap[c.ID] = id
		}
		for _, sw := range b.d.Switches {
			dup := false
			for _, o := range s.d.Switches {
				switch {
				case sw.Cloud != nil && o.Cloud != nil:
					dup = dup || strings.EqualFold(o.Cloud.DeviceID, sw.Cloud.DeviceID)
				case sw.Cloud == nil && o.Cloud == nil:
					dup = dup || (strings.EqualFold(o.Host, sw.Host) && o.Port == sw.Port)
				}
			}
			if dup {
				res.Skipped = append(res.Skipped, firstNonEmpty(sw.Name, sw.Host)+" is already here")
				continue
			}
			addSwitch(sw, credMap, s.d.Layout)
		}
	default:
		return res, fmt.Errorf("unknown import mode %q", mode)
	}
	return res, s.saveLocked()
}
