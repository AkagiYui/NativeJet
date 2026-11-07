package kernel

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"nativejet/internal/model"
	"nativejet/internal/platform"
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
	entries, err := os.ReadDir(m.Inbox)
	if err != nil {
		return nil, err
	}
	out := []model.Kernel{}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(m.Inbox, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > MaxBinary {
			continue
		}
		item := model.Kernel{Name: entry.Name(), Path: path, Arch: runtime.GOARCH}
		hash, err := Hash(path)
		if err != nil {
			item.Error = err.Error()
			out = append(out, item)
			continue
		}
		item.ID = hash
		if err = validateMachO(path); err != nil {
			item.Error = err.Error()
			out = append(out, item)
			continue
		}
		if info.Mode()&0111 == 0 {
			item.Error = "缺少执行权限：请在 Finder 或终端赋予文件执行权限"
			out = append(out, item)
			continue
		}
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(probe, path, "-v")
		cmd.SysProcAttr = platform.Credentials(m.UID, m.GID)
		cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + m.Inbox}
		var buf boundedBuffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err = cmd.Run()
		cancel()
		text := strings.TrimSpace(buf.String())
		if err != nil {
			item.Error = "无法读取版本: " + err.Error()
		} else if !strings.Contains(strings.ToLower(text), "mihomo") && !strings.Contains(text, "Clash.Meta") {
			item.Error = "首版支持 Mihomo / Clash.Meta 内核"
		} else {
			item.Version = strings.Split(text, "\n")[0]
		}
		if len(item.Version) > 180 {
			item.Version = item.Version[:180]
		}
		_, err = os.Stat(filepath.Join(m.Installed, item.ID, "mihomo"))
		item.Installed = err == nil
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func validateMachO(path string) error {
	valid := func(f *macho.File) bool {
		return runtime.GOARCH == "arm64" && f.Cpu == macho.CpuArm64 || runtime.GOARCH == "amd64" && f.Cpu == macho.CpuAmd64
	}
	f, err := macho.Open(path)
	if err == nil {
		defer f.Close()
		if valid(f) {
			return nil
		}
		return errors.New("内核架构与本机不匹配")
	}
	fat, err := macho.OpenFat(path)
	if err != nil {
		return errors.New("不是可识别的 macOS Mach-O 可执行文件")
	}
	defer fat.Close()
	for _, arch := range fat.Arches {
		if valid(arch.File) {
			return nil
		}
	}
	return errors.New("通用内核不包含本机架构")
}
func Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
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
		target := filepath.Join(dir, "mihomo")
		if err = storage.Atomic(target, data, 0555); err != nil {
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
			if !strings.HasPrefix(a.Name, "mihomo-darwin-"+runtime.GOARCH) || !strings.HasSuffix(a.Name, ".gz") || strings.Contains(a.Name, "alpha") {
				continue
			}
			if runtime.GOARCH == "amd64" && !strings.Contains(a.Name, "compatible") {
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
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	binary, err := io.ReadAll(io.LimitReader(reader, MaxBinary+1))
	if err != nil {
		return "", err
	}
	if len(binary) > MaxBinary {
		return "", errors.New("解压后的内核过大")
	}
	root, err := os.OpenRoot(m.Inbox)
	if err != nil {
		return "", err
	}
	defer root.Close()
	name := strings.TrimSuffix(asset.Name, ".gz")
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
		err = f.Chown(m.UID, m.GID)
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
	if err = validateMachO(filepath.Join(m.Inbox, name)); err != nil {
		_ = root.Remove(name)
		return "", err
	}
	receipt := map[string]string{"version": asset.Version, "assetId": strconv.FormatInt(asset.AssetID, 10), "digest": asset.Digest, "source": "MetaCubeX/mihomo"}
	meta, _ := json.Marshal(receipt)
	_ = storage.Atomic(filepath.Join(m.Installed, "receipts", storage.ID()+".json"), meta, 0600)
	return name, nil
}
