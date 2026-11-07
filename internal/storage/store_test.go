package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestAtomicHonorsExplicitModeWithPrivateUmask(t *testing.T) {
	old := syscall.Umask(0077)
	defer syscall.Umask(old)
	for _, mode := range []os.FileMode{0600, 0644, 0755} {
		path := filepath.Join(t.TempDir(), "file")
		if err := Atomic(path, []byte("test"), mode); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("umask changed explicit mode: got %o want %o", info.Mode().Perm(), mode)
		}
	}
}

func TestStateRoundTripAndCorruption(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.GlobalOverride = "tun:\n  enable: false\n"
	if err = s.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil || loaded.GlobalOverride != state.GlobalOverride {
		t.Fatal("state did not round trip")
	}
	info, _ := os.Stat(s.Path("state.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("state contains credentials and must be private")
	}
	if err = os.WriteFile(s.Path("state.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(); err == nil {
		t.Fatal("corruption must not reset user data silently")
	}
	b, _ := os.ReadFile(s.Path("state.json"))
	if string(b) != "broken" {
		t.Fatal("corrupt data overwritten")
	}
}
func TestStoreRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "actual")
	_ = os.Mkdir(target, 0700)
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link); err == nil {
		t.Fatal("accepted symlink store")
	}
}
