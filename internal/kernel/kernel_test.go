package kernel

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"nativejet/internal/model"
)

func buildFixture(t *testing.T, arch, module string) []byte {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module " + module + "\n\ngo 1.27\n",
		"main.go": `package main
import ("fmt"; "os"; "path/filepath")
func main() {
    exe, _ := os.Executable()
    _ = os.WriteFile(filepath.Join(filepath.Dir(exe), ".executed"), []byte("ran"), 0600)
    fmt.Println("Mihomo v9.9.9")
}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, Executable)
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-trimpath", "-o", path, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+arch)
	if module == "" {
		cmd.Env = append(cmd.Env, "GO111MODULE=off")
	} else {
		cmd.Env = append(cmd.Env, "GO111MODULE=on")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build controlled fixture: %s: %v", out, err)
	}
	if arch == runtime.GOARCH {
		// Verify that our execution canary would actually detect the old -v probe.
		if out, err := exec.Command(path, "-v").CombinedOutput(); err != nil {
			t.Fatalf("fixture canary: %s: %v", out, err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".executed")); err != nil {
			t.Fatal("fixture did not produce its execution canary")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	m := &Manager{Inbox: filepath.Join(base, "inbox"), Installed: filepath.Join(base, "cores"), UID: os.Getuid(), GID: os.Getgid()}
	if err := os.MkdirAll(m.Inbox, 0700); err != nil {
		t.Fatal(err)
	}
	return m
}

func putCandidate(t *testing.T, m *Manager, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(m.Inbox, name), data, 0755); err != nil {
		t.Fatal(err)
	}
}

func assertNeverExecuted(t *testing.T, m *Manager) {
	t.Helper()
	err := filepath.WalkDir(filepath.Dir(m.Inbox), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".executed" {
			t.Errorf("inspection executed a candidate: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKernelInspectionAndSelectionNeverExecute(t *testing.T) {
	data := buildFixture(t, runtime.GOARCH, "github.com/MetaCubeX/mihomo")
	ctx := context.Background()
	t.Run("unknown version and selection", func(t *testing.T) {
		m := testManager(t)
		putCandidate(t, m, Executable, data)
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 1 || items[0].Error != "" || items[0].Version != UnknownVersion || items[0].Source != model.KernelSourceLocal {
			t.Fatalf("static metadata: %+v, %v", items, err)
		}
		item, err := m.Install(ctx, items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.VerifyInstalled(item); err != nil {
			t.Fatal(err)
		}
		assertNeverExecuted(t, m)
	})
	t.Run("filename version stays labeled", func(t *testing.T) {
		m := testManager(t)
		name := "mihomo-v1.2.3"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		putCandidate(t, m, name, data)
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 1 || items[0].Version != "Mihomo v1.2.3（文件名）" {
			t.Fatalf("filename version: %+v, %v", items, err)
		}
		assertNeverExecuted(t, m)
	})
	t.Run("missing metadata never falls back to execution", func(t *testing.T) {
		m := testManager(t)
		stripped := bytes.ReplaceAll(data, []byte("\xff Go buildinf:"), []byte("\x00 Go buildinf:"))
		if bytes.Equal(stripped, data) {
			t.Fatal("fixture has no Go build info to remove")
		}
		putCandidate(t, m, Executable, stripped)
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 1 || items[0].Version != UnknownVersion || items[0].Error == "" {
			t.Fatalf("missing metadata: %+v, %v", items, err)
		}
		if _, err := m.Install(ctx, items[0].ID); err == nil {
			t.Fatal("unidentified program was selected")
		}
		assertNeverExecuted(t, m)
	})
	t.Run("unidentified executable stays inert", func(t *testing.T) {
		m := testManager(t)
		// Unlike a patched binary, this has a valid platform signature and can
		// run, but contains no Mihomo module metadata.
		unidentified := buildFixture(t, runtime.GOARCH, "")
		putCandidate(t, m, Executable, unidentified)
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 1 || items[0].Error == "" || items[0].Version != UnknownVersion {
			t.Fatalf("unidentified executable: %+v, %v", items, err)
		}
		assertNeverExecuted(t, m)
	})
	t.Run("replaced source cannot reuse a scanned fingerprint", func(t *testing.T) {
		m := testManager(t)
		putCandidate(t, m, Executable, data)
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 1 {
			t.Fatal(err)
		}
		putCandidate(t, m, Executable, []byte("replaced"))
		if _, err := m.Install(ctx, items[0].ID); err == nil {
			t.Fatal("replaced source retained its old identity")
		}
		assertNeverExecuted(t, m)
	})
	t.Run("copy uses the inspected snapshot", func(t *testing.T) {
		m := testManager(t)
		putCandidate(t, m, Executable, data)
		root, err := os.OpenRoot(m.Inbox)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		item, snapshot, err := m.inspect(ctx, root, Executable)
		if err != nil || item.Error != "" {
			t.Fatalf("inspect: %+v, %v", item, err)
		}
		putCandidate(t, m, Executable, []byte("replaced"))
		installed, err := m.installSnapshot(item, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(installed.Path)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatal("copy reopened the changed pathname")
		}
		assertNeverExecuted(t, m)
	})
	t.Run("symlink candidates are rejected", func(t *testing.T) {
		m := testManager(t)
		target := filepath.Join(t.TempDir(), Executable)
		if err := os.WriteFile(target, data, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(m.Inbox, Executable)); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			t.Fatal(err)
		}
		items, err := m.Scan(ctx)
		if err != nil || len(items) != 0 {
			t.Fatalf("symlink accepted: %+v, %v", items, err)
		}
		if _, err := m.Install(ctx, digestBytes(data)); err == nil {
			t.Fatal("symlink selected")
		}
		if _, err := Hash(filepath.Join(m.Inbox, Executable)); err == nil {
			t.Fatal("symlink hashed")
		}
	})
	t.Run("managed copies reject tampering and unsafe permissions", func(t *testing.T) {
		m := testManager(t)
		putCandidate(t, m, Executable, data)
		item, err := m.Install(ctx, digestBytes(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(item.Path, 0755); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && m.VerifyInstalled(item) == nil {
			t.Fatal("writable managed executable was accepted")
		}
		if err := os.WriteFile(item.Path, append(bytes.Clone(data), 0), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(item.Path, executableMode); err != nil {
			t.Fatal(err)
		}
		if err := m.VerifyInstalled(item); err == nil {
			t.Fatal("tampered managed executable was accepted")
		}
	})
	t.Run("managed directories reject symlinks", func(t *testing.T) {
		m := testManager(t)
		if err := os.MkdirAll(m.Installed, 0700); err != nil {
			t.Fatal(err)
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(m.Installed, digestBytes(data))); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			t.Fatal(err)
		}
		putCandidate(t, m, Executable, data)
		if _, err := m.Install(ctx, digestBytes(data)); err == nil {
			t.Fatal("installation followed a managed directory symlink")
		}
		if _, err := os.Stat(filepath.Join(outside, Executable)); !os.IsNotExist(err) {
			t.Fatalf("installation wrote outside its managed directory: %v", err)
		}
		if _, err := OpenVerified(filepath.Join(m.Installed, digestBytes(data), Executable), digestBytes(data)); err == nil {
			t.Fatal("startup followed a managed directory symlink")
		}
	})
	t.Run("managed executable rejects symlinks with identical bytes", func(t *testing.T) {
		m := testManager(t)
		putCandidate(t, m, Executable, data)
		item, err := m.Install(ctx, digestBytes(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(item.Path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(m.Inbox, Executable), item.Path); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink creation unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if err := m.VerifyInstalled(item); err == nil {
			t.Fatal("startup accepted a symlink with matching bytes")
		}
	})
	t.Run("downloads and their automatic scans stay static", func(t *testing.T) {
		testDownloads(t, data)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testDownloads(t *testing.T, data []byte) {
	t.Helper()
	name := "mihomo-" + runtime.GOOS + "-" + runtime.GOARCH + "-"
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		name += "compatible-"
	} else if runtime.GOOS != "darwin" && runtime.GOARCH == "amd64" {
		name += "v1-"
	}
	name += "v1.2.3"
	var archive bytes.Buffer
	if runtime.GOOS == "windows" {
		name += ".zip"
		w := zip.NewWriter(&archive)
		f, err := w.Create(strings.TrimSuffix(name, ".zip") + ".exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		name += ".gz"
		w := gzip.NewWriter(&archive)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, digest        string
		wantError, verified bool
	}{
		{name: "verified", digest: "sha256:" + digestBytes(archive.Bytes()), verified: true},
		{name: "without published digest"},
		{name: "digest mismatch", digest: "sha256:" + strings.Repeat("0", 64), wantError: true},
		{name: "invalid digest", digest: "sha256:invalid", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			// A pre-existing candidate must remain inert during a download's scan.
			putCandidate(t, m, Executable, data)
			m.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body []byte
				if r.URL.Host == "api.github.com" {
					body, _ = json.Marshal([]any{map[string]any{"tag_name": "v1.2.3", "assets": []any{map[string]any{
						"id": 42, "name": name, "size": archive.Len(), "digest": tc.digest,
						"browser_download_url": "https://github.com/MetaCubeX/mihomo/releases/download/v1.2.3/" + name,
					}}}})
				} else if r.URL.Host == "github.com" {
					body = archive.Bytes()
				} else {
					return nil, fmt.Errorf("unexpected request %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
			})}
			downloaded, err := m.Download(context.Background(), 42)
			if (err != nil) != tc.wantError {
				t.Fatalf("download error: %v", err)
			}
			if tc.wantError {
				assertNeverExecuted(t, m)
				return
			}
			items, err := m.Scan(context.Background())
			if err != nil || len(items) != 2 {
				t.Fatalf("post-download scan: %+v, %v", items, err)
			}
			for _, item := range items {
				if item.ID != digestBytes(data) || item.Source != model.KernelSourceGitHub || item.DigestVerified != tc.verified || item.SourceURL == "" {
					t.Fatalf("receipt not bound to executable bytes: %+v", item)
				}
			}
			item, err := m.Install(context.Background(), digestBytes(data))
			if err != nil || item.DigestVerified != tc.verified || item.Source != model.KernelSourceGitHub {
				t.Fatalf("selected provenance: %+v, %v", item, err)
			}
			putCandidate(t, m, downloaded, append(bytes.Clone(data), 0))
			items, err = m.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items {
				if item.Name == downloaded && (item.Source != model.KernelSourceLocal || item.DigestVerified || item.AssetDigest != "") {
					t.Fatalf("replaced file inherited download provenance: %+v", item)
				}
			}
			assertNeverExecuted(t, m)
		})
	}
}
