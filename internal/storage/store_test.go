package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nativejet/internal/model"
)

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
	preset := model.GlobalOverride{ID: ID(), Name: "本地测试", Content: "tun:\n  enable: false\n"}
	state.GlobalOverrides = append(state.GlobalOverrides, preset)
	state.SelectedGlobalOverride = preset.ID
	if err = s.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil || len(loaded.GlobalOverrides) != 2 || loaded.SelectedGlobalOverride != preset.ID || loaded.GlobalOverrides[1] != preset {
		t.Fatal("state did not round trip")
	}
	info, _ := os.Stat(s.Path("state.json"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
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

func TestLoadMigratesSingleGlobalOverride(t *testing.T) {
	for _, content := range []string{"# 自定义\nmixed-port: 9000\n", ""} {
		store, err := New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		legacy, _ := json.Marshal(map[string]any{"version": 1, "globalOverride": content})
		if err = Atomic(store.Path("state.json"), legacy, 0600); err != nil {
			t.Fatal(err)
		}
		state, err := store.Load()
		if err != nil || len(state.GlobalOverrides) != 1 || state.GlobalOverrides[0] != model.DefaultGlobalOverride(content) ||
			state.SelectedGlobalOverride != state.GlobalOverrides[0].ID {
			t.Fatalf("legacy override was not preserved: %+v, %v", state.GlobalOverrides, err)
		}
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
