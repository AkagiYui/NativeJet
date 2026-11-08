package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"nativejet/internal/model"
)

func ID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func Atomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+ID())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	// The privileged installer uses umask 077. Honor explicit modes for public
	// service identity files as well as private configuration files.
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = replaceFile(tmp, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err == nil {
		defer d.Close()
		_ = d.Sync()
	}
	return nil
}

type Store struct{ Directory string }

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("数据目录必须为真实目录")
	}
	if err := checkDirectory(dir, info); err != nil {
		return nil, err
	}

	return &Store{Directory: dir}, nil
}
func (s *Store) Path(parts ...string) string {
	return filepath.Join(append([]string{s.Directory}, parts...)...)
}
func (s *Store) Load() (model.State, error) {
	var state model.State
	b, err := os.ReadFile(s.Path("state.json"))
	if errors.Is(err, os.ErrNotExist) {
		preset := model.DefaultGlobalOverride(DefaultOverride)
		state = model.State{Version: 1, GlobalOverrides: []model.GlobalOverride{preset}, SelectedGlobalOverride: preset.ID,
			Profiles: []model.Profile{}, Installed: []model.Kernel{}, Revisions: []model.Revision{}}
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(b, &state); err != nil {
		return state, fmt.Errorf("状态文件损坏，已保留原文件: %w", err)
	}
	if state.Version != 1 {
		return state, fmt.Errorf("不支持的数据版本 %d", state.Version)
	}
	if state.GlobalOverrideByID(state.SelectedGlobalOverride) == nil {
		return state, errors.New("找不到选用的全局覆盖，已保留原文件")
	}
	return state, nil
}
func (s *Store) Save(state model.State) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return Atomic(s.Path("state.json"), b, 0600)
}
