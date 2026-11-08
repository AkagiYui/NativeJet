package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Payload struct {
	Version           int          `json:"version"`
	OwnerUID          int          `json:"ownerUid,omitempty"`
	OwnerSID          string       `json:"ownerSid,omitempty"`
	HelperRequirement string       `json:"helperRequirement"`
	HelperSHA256      string       `json:"helperSha256"`
	State             *model.State `json:"state,omitempty"`
	Files             []File       `json:"files"`
	ApproveKernel     string       `json:"approveKernel,omitempty"`
}
type Prepared struct {
	Directory, Digest, Helper string
}

func (p *Prepared) Close() {
	if p != nil {
		_ = os.RemoveAll(p.Directory)
	}
}

// Prepare runs with ordinary user rights. Only immutable, hashed snapshots are
// passed to the privileged installer; no credentials appear in its arguments.
func Prepare(helperPath string, snapshot *model.Snapshot) (_ *Prepared, err error) {
	hash, err := kernel.Hash(helperPath)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "nativejet-install-")
	if err != nil {
		return nil, err
	}
	p := &Prepared{Directory: dir, Helper: helperPath}
	defer func() {
		if err != nil {
			p.Close()
		}
	}()
	if err = secureStaging(dir); err != nil {
		return nil, err
	}
	payload := Payload{Version: 1, HelperSHA256: hash}
	if err = prepareIdentity(helperPath, &payload); err != nil {
		return nil, err
	}
	if snapshot != nil {
		data, _ := json.Marshal(snapshot.State)
		var state model.State
		if err = json.Unmarshal(data, &state); err != nil {
			return nil, err
		}
		payload.State = &state
		payload.ApproveKernel = state.SelectedKernel
		for _, k := range state.Installed {
			if !digestPattern.MatchString(k.ID) {
				return nil, errors.New("无效内核标识")
			}
			entry, copyErr := stageFile(dir, filepath.Join("cores", k.ID, kernel.Executable), k.Path, kernel.MaxBinary)
			if copyErr != nil {
				return nil, copyErr
			}
			if entry.SHA256 != k.ID {
				return nil, errors.New("内核文件已改变，请重新选择后再安装服务")
			}
			payload.Files = append(payload.Files, entry)
		}
		runtimeDir := snapshot.Status.RuntimeDirectory
		if runtimeDir != "" {
			var total int64
			err = filepath.WalkDir(runtimeDir, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("内核运行目录包含符号链接，已停止迁移")
				}
				if entry.IsDir() {
					return nil
				}
				info, e := entry.Info()
				if e != nil {
					return e
				}
				if !info.Mode().IsRegular() {
					return nil
				} // sockets are never copied
				relative, e := filepath.Rel(runtimeDir, path)
				if e != nil {
					return e
				}
				if strings.HasPrefix(relative, ".") {
					return nil
				}
				if len(payload.Files) >= 10000 || total+info.Size() > 2<<30 {
					return errors.New("内核缓存过大，无法自动迁移")
				}
				file, e := stageFile(dir, filepath.Join("runtime", relative), path, kernel.MaxBinary)
				if e == nil {
					payload.Files = append(payload.Files, file)
					total += file.Size
				}
				return e
			})
			if err != nil {
				return nil, err
			}
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(data) > 24<<20 {
		return nil, errors.New("迁移清单过大")
	}
	if err = storage.Atomic(filepath.Join(dir, "payload.json"), data, 0600); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	p.Digest = hex.EncodeToString(sum[:])
	return p, nil
}

func stageFile(root, relative, source string, limit int64) (File, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return File{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return File{}, errors.New("迁移文件必须为大小受限的普通文件")
	}
	f, err := os.Open(source)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	target := filepath.Join(root, relative)
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return File{}, err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return File{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(f, limit+1))
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return File{}, err
	}
	if n > limit {
		return File{}, errors.New("迁移文件过大")
	}
	return File{Path: filepath.ToSlash(relative), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func readPayload(directory, digest string) (Payload, *os.Root, error) {
	var payload Payload
	if !digestPattern.MatchString(digest) {
		return payload, nil, errors.New("无效的迁移摘要")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return payload, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !privateStaging(directory, info) {
		return payload, nil, errors.New("迁移目录权限不正确")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return payload, nil, err
	}
	fail := func(e error) (Payload, *os.Root, error) { root.Close(); return Payload{}, nil, e }
	if opened, err := root.Stat("."); err != nil || !os.SameFile(info, opened) {
		return fail(errors.New("迁移目录已改变"))
	}
	data, err := readRegular(root, "payload.json", 24<<20)
	if err != nil {
		return fail(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return fail(errors.New("授权后迁移清单已改变，拒绝安装"))
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		return fail(err)
	}
	if payload.Version != 1 || !validPayloadOwner(directory, info, payload) || !digestPattern.MatchString(payload.HelperSHA256) {
		return fail(errors.New("迁移清单身份无效"))
	}
	if payload.ApproveKernel != "" && !digestPattern.MatchString(payload.ApproveKernel) {
		return fail(errors.New("内核授权标识无效"))
	}
	if len(payload.Files) > 10000 {
		return fail(errors.New("迁移文件过多"))
	}
	seen := map[string]bool{}
	var total int64
	for _, file := range payload.Files {
		if !fs.ValidPath(file.Path) || !filepath.IsLocal(filepath.FromSlash(file.Path)) || !(strings.HasPrefix(file.Path, "cores/") || strings.HasPrefix(file.Path, "runtime/")) ||
			strings.ContainsAny(file.Path, "\\:") || seen[manifestFileKey(file.Path)] || !digestPattern.MatchString(file.SHA256) ||
			file.Size < 0 || file.Size > kernel.MaxBinary {
			return fail(fmt.Errorf("无效迁移文件 %q", file.Path))
		}
		seen[manifestFileKey(file.Path)] = true
		total += file.Size
		if total > 2<<30 {
			return fail(errors.New("迁移数据过大"))
		}
	}
	return payload, root, nil
}

func readRegular(root *os.Root, path string, limit int64) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("拒绝非普通文件或超限文件")
	}
	f, err := root.OpenFile(path, payloadOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) || opened.Size() > limit {
		return nil, errors.New("迁移文件已改变或超限")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, errors.New("文件过大")
	}
	return data, err
}
