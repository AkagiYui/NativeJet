package kernel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"nativejet/internal/model"
	"nativejet/internal/storage"
)

const MaxBinary = 256 << 20

type Manager struct {
	Inbox, Installed string
	UID, GID         int
	HTTP             *http.Client
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: 90 * time.Second}
}
func (m *Manager) Scan(ctx context.Context) ([]model.Kernel, error) {
	root, err := os.OpenRoot(m.Inbox)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	out := []model.Kernel{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		item, _, err := m.inspect(ctx, root, entry.Name())
		if err != nil {
			continue
		}
		info, err := os.Lstat(filepath.Join(m.Installed, item.ID, Executable))
		item.Installed = err == nil && info.Mode().IsRegular()
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func digestBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (m *Manager) inspect(ctx context.Context, root *os.Root, name string) (model.Kernel, []byte, error) {
	item := model.Kernel{Name: name, Path: filepath.Join(m.Inbox, name), Arch: runtime.GOARCH, Source: model.KernelSourceLocal}
	data, info, err := readRegular(root, name, MaxBinary)
	if err != nil {
		return item, nil, err
	}
	// Format checks, fingerprints, and the eventual managed copy all use this
	// immutable snapshot from one descriptor, never a reopened pathname.
	item.ID = digestBytes(data)
	m.probe(ctx, info, bytes.NewReader(data), &item)
	m.applyReceipt(&item)
	return item, data, nil
}

func Hash(path string) (string, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	defer root.Close()
	f, info, err := openRegular(root, filepath.Base(path))
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash, err := hashBinary(f)
	if err == nil {
		err = unchanged(root, filepath.Base(path), f, info)
	}
	return hash, err
}

func hashBinary(f io.Reader) (string, error) {
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, MaxBinary+1))
	if err != nil {
		return "", err
	}
	if n > MaxBinary {
		return "", errors.New("内核文件过大")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (m *Manager) Install(ctx context.Context, id string) (model.Kernel, error) {
	if !validID(id) {
		return model.Kernel{}, errors.New("无效内核标识")
	}
	root, err := os.OpenRoot(m.Inbox)
	if err != nil {
		return model.Kernel{}, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return model.Kernel{}, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return model.Kernel{}, err
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		item, data, err := m.inspect(ctx, root, entry.Name())
		if err != nil {
			continue
		}
		if item.ID != id {
			continue
		}
		if item.Error != "" {
			return item, errors.New(item.Error)
		}
		if err := ctx.Err(); err != nil {
			return item, err
		}
		return m.installSnapshot(item, data)
	}
	return model.Kernel{}, errors.New("所选内核已改变或不存在，请重新扫描目录")
}

func (m *Manager) Releases(ctx context.Context) ([]model.Release, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/MetaCubeX/mihomo/releases?per_page=12", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NativeJet")
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub 返回 %s（可能触发请求限额）", resp.Status)
	}
	var releases []struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Size   int64  `json:"size"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return nil, err
	}
	items := []model.Release{}
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		for _, a := range r.Assets {
			if !releaseAsset(a.Name) {
				continue
			}
			items = append(items, model.Release{Name: a.Name, Version: r.Tag, AssetID: a.ID, Size: a.Size, Digest: a.Digest, URL: a.URL})
		}
	}
	return items, nil
}
func (m *Manager) Download(ctx context.Context, assetID int64) (string, error) {
	releases, err := m.Releases(ctx)
	if err != nil {
		return "", err
	}
	var asset *model.Release
	for i := range releases {
		if releases[i].AssetID == assetID {
			asset = &releases[i]
			break
		}
	}
	if asset == nil {
		return "", errors.New("找不到所选发布资源，请刷新版本列表")
	}
	if !officialDownloadURL(asset.URL) {
		return "", errors.New("拒绝非官方内核下载地址")
	}
	client := *m.client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 8 {
			return errors.New("重定向过多")
		}
		host := req.URL.Hostname()
		if req.URL.Scheme != "https" || (host != "github.com" && !strings.HasSuffix(host, ".githubusercontent.com")) {
			return errors.New("拒绝非 GitHub HTTPS 下载重定向")
		}
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载失败: %s", resp.Status)
	}
	compressed, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20+1))
	if err != nil {
		return "", err
	}
	if len(compressed) > 128<<20 {
		return "", errors.New("下载文件过大")
	}
	archiveHash := digestBytes(compressed)
	verified := false
	if asset.Digest != "" {
		if !strings.HasPrefix(asset.Digest, "sha256:") || !validID(strings.TrimPrefix(asset.Digest, "sha256:")) ||
			strings.TrimPrefix(asset.Digest, "sha256:") != archiveHash {
			return "", errors.New("官方 SHA-256 校验失败或摘要格式不受支持")
		}
		verified = true
	}
	binary, name, err := unpack(compressed, asset.Name)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(m.Inbox)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if filepath.Base(name) != name {
		return "", errors.New("无效资源名称")
	}
	tmp := ".download-" + storage.ID()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		return "", err
	}
	defer root.Remove(tmp)
	if _, err = f.Write(binary); err == nil {
		err = m.setDownloadOwner(f)
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		var info os.FileInfo
		info, err = f.Stat()
		if err == nil {
			item := model.Kernel{Name: name, Arch: runtime.GOARCH}
			m.probe(ctx, info, bytes.NewReader(binary), &item)
			if item.Error != "" {
				err = errors.New(item.Error)
			}
		}
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	receipt := downloadReceipt{SHA256: digestBytes(binary), Version: asset.Version, AssetID: asset.AssetID,
		URL: asset.URL, AssetDigest: asset.Digest, ArchiveSHA256: archiveHash, DigestVerified: verified}
	if err = m.saveReceipt(receipt); err != nil {
		return "", err
	}
	if err = root.Rename(tmp, name); err != nil {
		return "", err
	}
	return name, nil
}
