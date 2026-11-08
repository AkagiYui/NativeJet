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
