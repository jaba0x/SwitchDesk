package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// An app password is optional (issue #13). Without one, the key that
// encrypts the saved switch passwords sits in secret.key, protected by file
// permissions. With one, secret.key is replaced by secret.lock: the same key,
// encrypted with a key derived from the password (Argon2id + AES-GCM). The
// saved passwords can't be read, and SwitchDesk doesn't start polling, until
// the password is entered.

var (
	ErrLocked        = errors.New("SwitchDesk is locked; enter the app password first")
	ErrWrongPassword = errors.New("wrong password")
)

const lockFile = "secret.lock"

type lockData struct {
	V     int    `json:"v"`
	Salt  []byte `json:"salt"`
	Time  uint32 `json:"t"`
	Mem   uint32 `json:"m"` // KiB
	Par   uint8  `json:"p"`
	Nonce []byte `json:"nonce"`
	Key   []byte `json:"key"` // the data key, sealed
}

func (d lockData) kek(pass string) []byte {
	return argon2.IDKey([]byte(pass), d.Salt, d.Time, d.Mem, d.Par, 32)
}

func readLock(dir string) (lockData, error) {
	var d lockData
	b, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(b, &d); err != nil || d.V != 1 || len(d.Salt) < 16 || d.Time == 0 || d.Mem == 0 || d.Par == 0 {
		return d, errors.New(lockFile + " is damaged")
	}
	return d, nil
}

func openLock(d lockData, kek []byte) ([]byte, error) {
	blk, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	gcm, _ := cipher.NewGCM(blk)
	key, err := gcm.Open(nil, d.Nonce, d.Key, nil)
	if err != nil || len(key) != 32 {
		return nil, ErrWrongPassword
	}
	return key, nil
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// HasPassword reports whether the data folder is protected by an app password.
func (s *Store) HasPassword() bool {
	_, err := os.Stat(filepath.Join(s.dir, lockFile))
	return err == nil
}

// Locked reports whether the password still has to be entered.
func (s *Store) Locked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key == nil
}

// Unlock opens the data key with the app password.
func (s *Store) Unlock(pass string) error {
	d, err := readLock(s.dir)
	if err != nil {
		return err
	}
	kek := d.kek(pass)
	key, err := openLock(d, kek)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.key, s.kek = key, kek
	s.mu.Unlock()
	return nil
}

// UnlockLike opens another folder's lock with the key the password of this
// store derived, which works when it's a copy of the same data folder.
func (s *Store) UnlockLike(o *Store) error {
	o.mu.Lock()
	kek := o.kek
	o.mu.Unlock()
	d, err := readLock(s.dir)
	if err != nil {
		return err
	}
	if kek == nil {
		return ErrLocked
	}
	key, err := openLock(d, kek)
	if err != nil {
		return errors.New("that folder is protected by a different app password; restart SwitchDesk to open it")
	}
	s.mu.Lock()
	s.key, s.kek = key, kek
	s.mu.Unlock()
	return nil
}

// CheckPassword tells whether pass is the app password, for unlocking the
// screen after it was locked while running.
func (s *Store) CheckPassword(pass string) bool {
	d, err := readLock(s.dir)
	if err != nil {
		return false
	}
	key, err := openLock(d, d.kek(pass))
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key != nil && subtle.ConstantTimeCompare(key, s.key) == 1
}

// SetPassword sets, changes or (with next empty) removes the app password.
// old must be the current password when there is one.
func (s *Store) SetPassword(old, next string) error {
	if s.Locked() {
		return ErrLocked
	}
	if s.HasPassword() && !s.CheckPassword(old) {
		return errors.New("the current password is wrong")
	}
	if next != "" && len([]rune(next)) < 8 {
		return errors.New("use at least 8 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keyPath, lockPath := filepath.Join(s.dir, "secret.key"), filepath.Join(s.dir, lockFile)
	if next == "" {
		if err := writeAtomic(keyPath, s.key); err != nil {
			return err
		}
		s.kek = nil
		return os.Remove(lockPath)
	}
	d := lockData{V: 1, Salt: make([]byte, 16), Time: 3, Mem: 64 * 1024, Par: 2}
	rand.Read(d.Salt)
	kek := d.kek(next)
	blk, _ := aes.NewCipher(kek)
	gcm, _ := cipher.NewGCM(blk)
	d.Nonce = make([]byte, gcm.NonceSize())
	rand.Read(d.Nonce)
	d.Key = gcm.Seal(nil, d.Nonce, s.key, nil)
	b, _ := json.Marshal(d)
	if err := writeAtomic(lockPath, b); err != nil {
		return err
	}
	if err := os.Remove(keyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		os.Remove(lockPath) // don't claim a protection that isn't there
		return errors.New("couldn't remove secret.key, so the password would protect nothing: " + err.Error())
	}
	s.kek = kek
	return nil
}

// ForgetPassword is for a forgotten app password: it starts a new key and
// clears every saved password, keeping switches, settings and history. The
// passwords then have to be entered again under Settings → Credentials.
func (s *Store) ForgetPassword() error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.dir, "secret.key"), key); err != nil {
		return err
	}
	os.Remove(filepath.Join(s.dir, lockFile))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key, s.kek = key, nil
	for i := range s.d.Credentials {
		s.d.Credentials[i].Password = ""
	}
	return s.saveLocked()
}
