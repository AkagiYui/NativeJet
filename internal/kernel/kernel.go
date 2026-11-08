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
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(m.Inbox, entry.Name())
		info, err := root.Lstat(entry.Name())
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > MaxBinary {
			continue
		}
		item := model.Kernel{Name: entry.Name(), Path: path, Arch: runtime.GOARCH}
		f, err := root.Open(entry.Name())
		if err != nil {
			continue
		}
		hash, err := hashBinary(f)
		if err != nil {
			_ = f.Close()
			item.Error = err.Error()
			out = append(out, item)
			continue
		}
		item.ID = hash
		m.probe(ctx, info, f, &item)
		_ = f.Close()
		_, err = os.Stat(filepath.Join(m.Installed, item.ID, Executable))
		item.Installed = err == nil
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return hashBinary(f)
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
	if len(id) != 64 {
		return model.Kernel{}, errors.New("无效内核标识")
	}
	items, err := m.Scan(ctx)
	if err != nil {
		return model.Kernel{}, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.Error != "" {
			return item, errors.New(item.Error)
		}
		// Hold the source descriptor and hash the bytes that are actually copied.
		root, err := os.OpenRoot(m.Inbox)
		if err != nil {
			return item, err
		}
		defer root.Close()
		f, err := root.Open(item.Name)
		if err != nil {
			return item, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return item, errors.New("内核必须是普通文件")
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxBinary+1))
		if err != nil {
			return item, err
		}
		h := sha256.Sum256(data)
		if len(data) > MaxBinary || hex.EncodeToString(h[:]) != id {
			return item, errors.New("扫描后内核文件已改变，请重新扫描")
		}
		dir := filepath.Join(m.Installed, id)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return item, err
		}
		target := filepath.Join(dir, Executable)
		if err = writeExecutable(target, data); err != nil {
			return item, err
		}
		item.Path = target
		item.Installed = true
		return item, nil
	}
	return model.Kernel{}, errors.New("找不到所选内核，请重新扫描目录")
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 64<<10 {
		left := (64 << 10) - b.Len()
		if len(p) > left {
			p = p[:left]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
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
	u, err := url.Parse(asset.URL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/MetaCubeX/mihomo/releases/download/") {
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
	if strings.HasPrefix(asset.Digest, "sha256:") {
		h := sha256.Sum256(compressed)
		if hex.EncodeToString(h[:]) != strings.TrimPrefix(asset.Digest, "sha256:") {
			return "", errors.New("官方 SHA-256 校验失败")
		}
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
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = root.Rename(tmp, name); err != nil {
		return "", err
	}
	if err = validateExecutable(filepath.Join(m.Inbox, name)); err != nil {
		_ = root.Remove(name)
		return "", err
	}
	receipt := map[string]string{"version": asset.Version, "assetId": strconv.FormatInt(asset.AssetID, 10), "digest": asset.Digest, "source": "MetaCubeX/mihomo"}
	meta, _ := json.Marshal(receipt)
	_ = storage.Atomic(filepath.Join(m.Installed, "receipts", storage.ID()+".json"), meta, 0600)
	return name, nil
}
