package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"nativejet/internal/config"
	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func ValidateState(state model.State) error {
	if state.Version != 1 {
		return errors.New("配置数据版本不受支持")
	}
	seen := map[string]bool{}
	for _, p := range state.Profiles {
		if !validID(p.ID) || seen[p.ID] || (p.Kind != "local" && p.Kind != "subscription") {
			return errors.New("配置标识无效或重复")
		}
		seen[p.ID] = true
		if _, _, err := config.Parse(p.Content); err != nil {
			return fmt.Errorf("配置 %s 无法迁移: %w", p.Name, err)
		}
		if strings.TrimSpace(p.Override) != "" {
			if _, _, err := config.Parse(p.Override); err != nil {
				return err
			}
		}
	}
	if state.GlobalOverrideByID(state.SelectedGlobalOverride) == nil {
		return errors.New("找不到选用的全局覆盖")
	}
	seen = map[string]bool{}
	for _, preset := range state.GlobalOverrides {
		if !validID(preset.ID) || seen[preset.ID] || strings.TrimSpace(preset.Name) == "" {
			return errors.New("全局覆盖标识或名称无效")
		}
		seen[preset.ID] = true
		if _, _, err := config.Parse(preset.Content); err != nil {
			return fmt.Errorf("全局覆盖 %s 无法迁移: %w", preset.Name, err)
		}
	}
	for _, k := range state.Installed {
		if !digestPattern.MatchString(k.ID) {
			return errors.New("内核标识无效")
		}
	}
	for _, r := range state.Revisions {
		if !validID(r.ID) || !validID(r.ProfileID) || !digestPattern.MatchString(r.KernelID) {
			return errors.New("历史版本标识无效")
		}
	}
	return nil
}
func validID(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// migrate keeps the source untouched and commits metadata only after every
// copied file has been verified. Existing destination profiles/history survive.
func migrate(payload Payload, source *os.Root, target string) (model.State, error) {
	store, err := storage.New(target)
	if err != nil {
		return model.State{}, err
	}
	state, err := store.Load()
	if err != nil {
		return state, err
	}
	if payload.State == nil {
		return state, nil
	}
	incoming := *payload.State
	if err = ValidateState(incoming); err != nil {
		return state, err
	}
	runtimeDir := storage.RuntimeDirectory(target)
	if _, err = storage.New(runtimeDir); err != nil {
		return state, err
	}
	for _, file := range payload.Files {
		data, err := readRegular(source, file.Path, kernel.MaxBinary)
		if err != nil {
			return state, err
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != file.Size || hex.EncodeToString(sum[:]) != file.SHA256 {
			return state, fmt.Errorf("迁移文件已改变: %s", file.Path)
		}
		dest := filepath.Join(target, filepath.FromSlash(file.Path))
		mode := os.FileMode(0600)
		if strings.HasPrefix(file.Path, "cores/") {
			pieces := strings.Split(file.Path, "/")
			if len(pieces) != 3 || !digestPattern.MatchString(pieces[1]) || pieces[2] != "mihomo" || file.SHA256 != pieces[1] {
				return state, errors.New("内核迁移路径无效")
			}
			mode = 0555
		} else {
			relative := strings.TrimPrefix(file.Path, "runtime/")
			dest = filepath.Join(runtimeDir, filepath.FromSlash(relative))
		}
		if info, e := os.Lstat(dest); e == nil {
			if !info.Mode().IsRegular() {
				return state, errors.New("目标迁移路径不是普通文件")
			}
			// Existing runtime databases stay authoritative. Core files and immutable
			// revisions must have identical bytes when their identities collide.
			if strings.HasPrefix(file.Path, "cores/") || strings.HasPrefix(file.Path, "runtime/revisions/") {
				hash, e := kernel.Hash(dest)
				if e != nil || hash != file.SHA256 {
					return state, errors.New("已保存文件与迁移标识冲突，原文件已保留")
				}
			}
			continue
		} else if !errors.Is(e, os.ErrNotExist) {
			return state, e
		}
		if err = storage.Atomic(dest, data, mode); err != nil {
			return state, err
		}
	}
	for _, p := range incoming.Profiles {
		index := slices.IndexFunc(state.Profiles, func(v model.Profile) bool { return v.ID == p.ID })
		if index < 0 {
			state.Profiles = append(state.Profiles, p)
		} else {
			state.Profiles[index] = p
		}
	}
	for _, k := range incoming.Installed {
		k.Path = filepath.Join(target, "cores", k.ID, "mihomo")
		k.Installed = true
		hash, e := kernel.Hash(k.Path)
		if e != nil || hash != k.ID {
			return state, errors.New("迁移内核校验失败")
		}
		if !slices.ContainsFunc(state.Installed, func(v model.Kernel) bool { return v.ID == k.ID }) {
			state.Installed = append(state.Installed, k)
		}
	}
	for _, r := range incoming.Revisions {
		if !slices.ContainsFunc(state.Revisions, func(v model.Revision) bool { return v.ID == r.ID }) {
			state.Revisions = append(state.Revisions, r)
		}
	}
	state.GlobalOverrides = incoming.GlobalOverrides
	state.SelectedGlobalOverride = incoming.SelectedGlobalOverride
	state.SelectedKernel = incoming.SelectedKernel
	state.Process = nil
	if incoming.ActiveRevision != "" {
		index := slices.IndexFunc(incoming.Revisions, func(r model.Revision) bool { return r.ID == incoming.ActiveRevision })
		if index < 0 {
			return state, errors.New("原配置的已应用版本丢失")
		}
		original := incoming.Revisions[index]
		preview, _, err := config.Compile(config.Layer{Name: "原文", Content: original.Source},
			config.Layer{Name: "全局覆盖", Content: original.GlobalOverride}, config.Layer{Name: "当前配置覆盖", Content: original.ProfileOverride})
		if err != nil {
			return state, err
		}
		revision := original
		revision.ID = storage.ID()
		revision.Hash = preview.Hash
		revision.CreatedAt = time.Now()
		if err = storage.Atomic(filepath.Join(runtimeDir, "revisions", revision.ID+".yaml"), []byte(preview.YAML), 0600); err != nil {
			return state, err
		}
		state.Revisions = append(state.Revisions, revision)
		state.ActiveProfile = revision.ProfileID
		state.ActiveRevision = revision.ID
	}
	if err = store.Save(state); err != nil {
		return state, err
	}
	return state, nil
}
