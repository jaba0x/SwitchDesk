package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppPassword(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	c, err := s.SaveCredential(Credential{Name: "core", Username: "admin", Password: "sw1tch-pass"})
	if err != nil {
		t.Fatal(err)
	}
	if s.HasPassword() || s.Locked() {
		t.Fatal("new folder should have no password")
	}
	if err := s.SetPassword("", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := s.SetPassword("", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); !os.IsNotExist(err) {
		t.Fatal("secret.key still on disk next to secret.lock")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "inventory.json")); strings.Contains(string(b), "sw1tch-pass") {
		t.Fatal("password in clear")
	}

	// a fresh start is locked and can't read the passwords
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Locked() {
		t.Fatal("not locked after restart")
	}
	if _, err := s2.Credential(c.ID); err == nil {
		t.Fatal("password readable while locked")
	}
	if _, err := s2.SaveCredential(Credential{Name: "x", Username: "a", Password: "b"}); err == nil {
		t.Fatal("saved a password while locked")
	}
	if err := s2.Unlock("wrong horse"); err != ErrWrongPassword {
		t.Fatalf("wrong password: %v", err)
	}
	if err := s2.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if got, err := s2.Credential(c.ID); err != nil || got.Password != "sw1tch-pass" {
		t.Fatalf("after unlock: %q %v", got.Password, err)
	}
	if !s2.CheckPassword("correct horse") || s2.CheckPassword("nope") {
		t.Fatal("CheckPassword")
	}

	// a copy of the folder opens with the same password, a moved key too
	dst := t.TempDir()
	if err := s2.CopyData(dst); err != nil {
		t.Fatal(err)
	}
	s3, _ := Open(dst)
	if !s3.Locked() {
		t.Fatal("copy not locked")
	}
	if err := s3.UnlockLike(s2); err != nil {
		t.Fatal(err)
	}

	// change, then remove
	if err := s2.SetPassword("wrong", "another one"); err == nil {
		t.Fatal("changed without the current password")
	}
	if err := s2.SetPassword("correct horse", "another one"); err != nil {
		t.Fatal(err)
	}
	if err := s2.SetPassword("another one", ""); err != nil {
		t.Fatal(err)
	}
	s4, _ := Open(dir)
	if s4.Locked() || s4.HasPassword() {
		t.Fatal("still protected after removing the password")
	}
	if got, _ := s4.Credential(c.ID); got.Password != "sw1tch-pass" {
		t.Fatal("password lost when removing the app password")
	}
}

func TestForgottenAppPassword(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	sw, _ := s.SaveSwitch(Switch{Host: "10.0.0.1"})
	c, _ := s.SaveCredential(Credential{Name: "core", Username: "admin", Password: "secret"})
	s.SetPassword("", "forgotten pass")

	s2, _ := Open(dir)
	if err := s2.ForgetPassword(); err != nil {
		t.Fatal(err)
	}
	if s2.Locked() || s2.HasPassword() {
		t.Fatal("still locked")
	}
	if _, ok := s2.Switch(sw.ID); !ok {
		t.Fatal("switch lost")
	}
	got, err := s2.Credential(c.ID)
	if err != nil || got.Password != "" || got.Username != "admin" {
		t.Fatalf("credential after reset: %+v %v", got, err)
	}
	if s3, _ := Open(dir); s3.Locked() {
		t.Fatal("locked after reset and restart")
	}
}
