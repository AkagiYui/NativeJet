package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"nativejet/internal/config"
	"nativejet/internal/ipc"
	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

// The test executable only implements the control API. It has no TUN, proxy
// listener, DNS changes or external requests.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-t":
			fmt.Fprintln(os.Stderr, "fixture: unexpected config validation")
			os.Exit(1)
		case "-d":
			fixtureCore()
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}
func arg(key string) string {
	for i, v := range os.Args {
		if v == key && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}
func fixtureCore() {
	data, err := os.ReadFile(arg("-f"))
	if err != nil {
		os.Exit(2)
	}
	var cfg map[string]any
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		os.Exit(2)
	}
	if cfg["test-startup-fatal"] == true {
		fmt.Fprint(os.Stderr, `time="2026-11-06T11:05:59-08:00" level=fatal msg="listen tcp 127.0.0.1:7897: bind: address already in use"`)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stdout, `level=info msg="fixture startup output"`)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	socket, _ := cfg["external-controller-unix"].(string)
	address, _ := cfg["external-controller"].(string)
	secret, _ := cfg["secret"].(string)
	network := "tcp"
	if socket != "" {
		network, address, secret = "unix", socket, ""
		_ = os.Remove(socket)
	}
	if address == "" {
		<-ctx.Done()
		return
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	server := &http.Server{}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret != "" && r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/version", "/configs":
			if cfg["test-geodata"] == true && r.URL.Path == "/configs" && r.Method == http.MethodGet {
				fmt.Fprintln(w, `{"geodata-mode":true,"geox-url":{"mmdb":"https://example.com/mmdb","geo-ip":"https://example.com/ip","geo-site":"https://example.com/site","asn":"https://example.com/asn"}}`)
				return
			}
			fmt.Fprintln(os.Stderr, "fixture: unexpected config or readiness request")
			http.NotFound(w, r)
		case "/configs/geo":
			if cfg["test-geodata"] != true || r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			if err := os.WriteFile(filepath.Join(arg("-d"), "GeoSite.dat"), []byte("fixture data"), 0600); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/logs":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_ = json.NewEncoder(w).Encode(map[string]string{"type": "info", "payload": "fixture log stream"})
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/connections":
			_ = json.NewEncoder(w).Encode(model.Connections{Connections: []model.Connection{}})
		case "/memory":
			fmt.Fprintln(w, `{"inuse":0,"oslimit":0}`)
			w.(http.Flusher).Flush()
			fmt.Fprintln(w, `{"inuse":22020096,"oslimit":0}`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/proxies":
			fmt.Fprintln(w, `{"proxies":{
				"configured":{"type":"Shadowsocks","history":[{"delay":81}]},
				"route":{"type":"Selector","all":["configured"]}
			}}`)
		case "/providers/proxies":
			fmt.Fprintln(w, `{"providers":{
				"default":{"proxies":[{"name":"configured","type":"Shadowsocks"}]},
				"nested":{"vehicleType":"Compatible","proxies":[{"name":"configured","type":"Shadowsocks"},{"name":"route","type":"Selector","all":["configured"]}]},
				"subscription":{"vehicleType":"HTTP","testUrl":"https://example.com","proxies":[{"name":"provided","history":[{"delay":72}],"extra":{"https://example.com":{"alive":false,"history":[{"delay":0}]}}}],"subscriptionInfo":{"Upload":100,"Download":200,"Total":800}}
			}}`)
		case "/rules":
			fmt.Fprintln(w, `{"rules":[{"type":"DOMAIN","extra":{"hitCount":8,"missCount":2}},{"type":"MATCH"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	go func() { <-ctx.Done(); _ = server.Close() }()
	_ = server.Serve(listener)
	_ = os.Remove(socket)
}
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	m, err := New(Options{Data: filepath.Join(base, "data"), Inbox: filepath.Join(base, "kernels"), UID: os.Getuid(), GID: os.Getgid()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Shutdown(); _ = os.RemoveAll(m.runtime) })
	return m
}
func installFixture(t *testing.T, m *Manager) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	// Lifecycle fixtures are deliberately not Mihomo binaries. Seed their
	// managed copy explicitly; inbox identification has separate static tests.
	id, err := kernel.Hash(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.kernel.Installed, id, kernel.Executable)
	if err = storage.Atomic(path, data, 0555); err != nil {
		t.Fatal(err)
	}
	state := m.clone()
	state.Installed = append(state.Installed, model.Kernel{ID: id, Name: "fixture-mihomo", Path: path, Version: "TestFixture", Installed: true})
	if err = m.commit(state); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	kernels, err := m.Scan(ctx)
	if err != nil || len(kernels) != 1 {
		t.Fatalf("scan: %v %#v", err, kernels)
	}
	if kernels[0].Error != "" {
		t.Fatal(kernels[0].Error)
	}
	if err = m.SelectKernel(ctx, kernels[0].ID); err != nil {
		t.Fatal(err)
	}
}

func saveGlobalOverride(m *Manager, content string) error {
	return m.SaveGlobalOverride(m.clone().SelectedGlobalOverride, nil, &content)
}

func TestPrivilegedKernelRequiresExplicitHashApproval(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	m.approvedKernels = map[string]bool{}
	p, err := m.SaveProfile(model.Profile{Name: "authorization", Kind: "local", Content: "mode: rule\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err == nil || !strings.Contains(err.Error(), "管理员授权") {
		t.Fatalf("unapproved core was accepted: %v", err)
	}
	snap := m.Snapshot()
	if snap.Status.KernelAuthorized || snap.State.ActiveRevision != "" {
		t.Fatal("unapproved core was applied")
	}
	m.approvedKernels = map[string]bool{snap.State.SelectedKernel: true}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if !m.Snapshot().Status.KernelAuthorized {
		t.Fatal("approval was not exposed to GUI")
	}
}

func TestManagedKernelStartsOnlyOnExplicitStart(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := saveGlobalOverride(m, "tun:\n  enable: false\n"); err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveProfile(model.Profile{Name: "explicit start", Kind: "local", Content: "mode: rule\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if m.core.Running() {
		t.Fatal("applying a stopped configuration executed a core")
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("explicit start failed: %v; logs: %+v", err, m.Logs(0))
	}
	if !m.core.Running() {
		t.Fatal("explicit start did not execute the managed copy")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	k := m.clone().Installed[0]
	if err := storage.Atomic(k.Path, []byte("tampered"), 0555); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err == nil || m.core.Running() {
		t.Fatal("tampered managed copy was executed")
	}
}

func TestStartupFailureIncludesFinalStderrAndIgnoresPreviousAttempt(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := saveGlobalOverride(m, "external-controller-unix: fixture-api.sock\nmixed-port: 7897\ntun:\n  enable: false\n"); err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveProfile(model.Profile{Name: "startup", Kind: "local", Content: "mode: rule\ntest-startup-fatal: true\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	m.core.Logs.Add("error", "process", `level=error msg="Start TUN listening error: configure tun interface: add route: 1.0.0.0/8: file exists"`)
	err = m.Start(ctx)
	if err == nil || !strings.Contains(err.Error(), "端口已被占用") || !strings.Contains(err.Error(), "127.0.0.1:7897") || strings.Contains(err.Error(), "TUN 路由冲突") {
		t.Fatalf("incorrect startup diagnostic: %v", err)
	}
	if m.core.Running() || m.Snapshot().Status.Error != err.Error() {
		t.Fatal("startup failure was not retained for GUI")
	}
	found := false
	for _, entry := range m.Logs(0) {
		if entry.Source == "process" && entry.Level == "error" && strings.Contains(entry.Message, "level=fatal") {
			found = true
		}
	}
	if !found {
		t.Fatal("final stderr without newline was discarded")
	}
	p.Content = "mode: rule\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Start(ctx); err != nil {
		t.Fatalf("old error prevented retry: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stream, output := false, false
		for _, entry := range m.Logs(0) {
			if entry.Source == "core" && entry.Message == "fixture log stream" {
				stream = true
			}
			if entry.Source == "process" && strings.Contains(entry.Message, "fixture startup output") {
				output = true
			}
		}
		if stream && output {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("startup output or live core log stream was missing")
}

func TestApplyRestartsCoreAndRollsBackOnProcessExit(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := saveGlobalOverride(m, "external-controller-unix: fixture-api.sock\ntun:\n  enable: false\n"); err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveProfile(model.Profile{Name: "local", Kind: "local", Content: "mode: rule\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if m.core.Running() {
		t.Fatal("applying while stopped started the core")
	}
	if err = m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	first := m.Snapshot().Status
	p.Content = "mode: global\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	applied := m.Snapshot().Status
	if !applied.Running || applied.PID == first.PID || applied.ActiveRevision == first.ActiveRevision {
		t.Fatal("applying with the same controller did not restart the core")
	}
	if values, err := m.revisionValues(applied.ActiveRevision); err != nil || values["mode"] != "global" {
		t.Fatalf("the new revision was not saved: %v", err)
	}
	if _, err = m.Preview(p.ID, "", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, entry := range m.Logs(0) {
		if strings.Contains(entry.Message, "fixture: unexpected") {
			t.Fatal(entry.Message)
		}
	}

	p.Content = "mode: rule\ntest-startup-fatal: true\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, p.ID); err == nil || !strings.Contains(err.Error(), "端口已被占用") {
		t.Fatalf("restart did not report the child process failure: %v", err)
	}
	restored := m.Snapshot()
	if !restored.Status.Running || restored.Status.PID == applied.PID || restored.State.ActiveRevision != applied.ActiveRevision ||
		restored.State.Process == nil || restored.State.Process.RevisionID != applied.ActiveRevision || len(restored.State.Revisions) != 2 {
		t.Fatal("the failed restart did not restore the previous revision")
	}
}

func TestSwitchGlobalOverrideKeepsAppliedRevisionAndCanRestoreDeletedPreset(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := saveGlobalOverride(m, "tun:\n  enable: false\nmixed-port: 7000\nallow-lan: false\n"); err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveProfile(model.Profile{Name: "local", Kind: "local", Content: "mode: rule\n", Override: "mixed-port: 7890\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	first := m.Snapshot().State
	name, content := "临时", "tun:\n  enable: false\nlog-level: debug\nmixed-port: 8000\n"
	if err = m.SaveGlobalOverride("", &name, &content); err != nil {
		t.Fatal(err)
	}
	second := m.Snapshot().State
	if second.ActiveRevision != first.ActiveRevision || second.SelectedGlobalOverride == first.SelectedGlobalOverride {
		t.Fatal("creating a selected preset changed the applied revision or failed to select it")
	}
	preview, err := m.Preview(p.ID, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.YAML, "log-level: debug") || !strings.Contains(preview.YAML, "mixed-port: 7890") || strings.Contains(preview.YAML, "allow-lan") {
		t.Fatalf("preview merged an unselected override or changed precedence: %s", preview.YAML)
	}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteGlobalOverride(first.SelectedGlobalOverride); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(ctx, first.ActiveRevision); err != nil {
		t.Fatal(err)
	}
	state, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.GlobalOverrides) != 2 || state.GlobalOverrideByID(second.SelectedGlobalOverride).Content != content ||
		state.SelectedGlobalOverride == second.SelectedGlobalOverride || state.Revisions[len(state.Revisions)-1].Hash != first.Revisions[0].Hash {
		t.Fatal("restoring the deleted preset lost the current preset or failed to restore the applied YAML")
	}
}

func TestUserControllerAndRunningWithoutIt(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	if err = saveGlobalOverride(m, "tun:\n  enable: false\nsecret: user-key\n"); err != nil {
		t.Fatal(err)
	}
	source := "mode: rule\nexternal-ui: dashboard\nexternal-controller: " + address + "\n"
	p, err := m.SaveProfile(model.Profile{Name: "controller", Kind: "local", Content: source})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	applied := m.clone().ActiveRevision
	values, err := m.revisionValues(applied)
	if err != nil || values["external-controller"] != address || values["external-ui"] != "dashboard" ||
		values["secret"] != "user-key" || values["external-controller-unix"] != nil {
		t.Fatalf("user controller configuration was changed: %v", err)
	}
	if err = m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	before := m.Snapshot().Status
	if !before.Running {
		t.Fatal("core was not running")
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		if _, err = m.Core(ctx, http.MethodGet, "/connections", nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("could not reach the configured authenticated controller: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	p.Content = "mode: global\ntest-startup-fatal: true\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, p.ID); err == nil {
		t.Fatal("failed restart was accepted")
	}
	rolledBack := m.Snapshot().Status
	if !rolledBack.Running || rolledBack.ActiveRevision != applied {
		t.Fatal("the previous controller was not restored")
	}
	p.Content = "mode: global\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	after := m.Snapshot().Status
	_, err = m.Core(ctx, http.MethodGet, "/connections", nil)
	if !after.Running || after.PID == rolledBack.PID || err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatal("core did not continue with the controller disabled")
	}
}

func TestStartUsesAppliedRevisionWithoutApplyingPendingEdits(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := saveGlobalOverride(m, "tun:\n  enable: false\n"); err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveProfile(model.Profile{Name: "applied revision", Kind: "local", Content: "mode: rule\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	state := m.clone()
	applied := state.ActiveRevision
	p.Content = "mode: global\n"
	if _, err = m.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err = m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	state = m.clone()
	values, err := m.revisionValues(state.ActiveRevision)
	if err != nil || state.ActiveRevision != applied || values["mode"] != "rule" || len(state.Revisions) != 1 {
		t.Fatalf("startup changed the applied revision or applied pending edits: %v", err)
	}
	if state.Profiles[0].Content != p.Content || state.Revisions[0].ID != applied {
		t.Fatal("pending edits or history were lost")
	}
}

func TestSubscriptionReadOnlyAndUpdatesRemainPending(t *testing.T) {
	m := newTestManager(t)
	var content atomic.Value
	content.Store("mixed-port: 7890\nrules: ['MATCH,DIRECT']\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "clash.meta" {
			t.Error("subscription user agent")
		}
		fmt.Fprint(w, content.Load().(string))
	}))
	defer server.Close()
	p, err := m.Subscribe(context.Background(), "sub", server.URL, 24)
	if err != nil {
		t.Fatal(err)
	}
	p.Content = "mixed-port: 1234\n"
	if _, err = m.SaveProfile(p); err == nil {
		t.Fatal("subscription raw content was editable")
	}
	content.Store("mixed-port: 7891\n")
	if err = m.UpdateSubscription(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	s := m.clone()
	if s.ActiveRevision != "" || s.ActiveProfile != "" {
		t.Fatal("subscription update applied automatically")
	}
	if s.Profiles[0].Content != content.Load().(string) {
		t.Fatal("updated subscription not saved")
	}
	content.Store("broken: [")
	if err = m.UpdateSubscription(context.Background(), p.ID); err == nil {
		t.Fatal("broken download accepted")
	}
	if m.clone().Profiles[0].Content != "mixed-port: 7891\n" {
		t.Fatal("failed subscription update replaced old source")
	}
}

func TestNodeSubscriptionFollowsProfileLifecycle(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	ctx := context.Background()
	original := base64.RawURLEncoding.EncodeToString([]byte("trojan://fixture-secret@old.example:443#node"))
	var content atomic.Value
	content.Store(original)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := content.Load().(string)
		if body == original {
			w.Header().Set("ETag", `"first"`)
		} else {
			w.Header().Set("ETag", `"next"`)
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	p, err := m.Subscribe(ctx, "nodes", server.URL, 24)
	if err != nil {
		t.Fatal(err)
	}
	source, _, err := config.Parse(p.Content)
	if err != nil || p.SourceText() != original || len(source) != 1 || source["proxies"] == nil {
		t.Fatalf("source body or fragment lost: %v", err)
	}
	forged := p
	forged.SubscriptionRaw = "changed"
	if _, err = m.SaveProfile(forged); err == nil {
		t.Fatal("subscription response was editable")
	}
	override := "proxy-groups: [{name: Route, type: select, proxies: [node]}]\nrules: ['MATCH,Route']\n"
	if err = m.SaveOverride(p.ID, override); err != nil {
		t.Fatal(err)
	}
	preview, err := m.Preview(p.ID, "", &original, nil, nil)
	if err != nil || !strings.Contains(preview.YAML, "MATCH,Route") || !strings.Contains(preview.YAML, "old.example") ||
		strings.Contains(preview.YAML, "fixture-secret") {
		t.Fatalf("invalid preview: %v", err)
	}
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	before := m.clone()
	current := before.Revisions[len(before.Revisions)-1]
	if current.SubscriptionRaw != original || current.Source != p.Content {
		t.Fatal("revision lost source or converted fragment")
	}
	next := "trojan://fixture-secret@new.example:443#node"
	content.Store(next)
	if err = m.UpdateSubscription(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	after := m.clone()
	if after.ActiveRevision != before.ActiveRevision || after.Profiles[0].SourceText() != next || after.Profiles[0].Override != override {
		t.Fatal("refresh changed applied configuration or lost the user's overlay")
	}
	content.Store(next + "\nunsupported://example.org")
	if err = m.UpdateSubscription(ctx, p.ID); err == nil || m.clone().Profiles[0] != after.Profiles[0] {
		t.Fatal("failed refresh replaced last usable subscription")
	}
	if err = m.Restore(ctx, current.ID); err != nil {
		t.Fatal(err)
	}
	restored := m.clone().Profiles[0]
	if restored.SourceText() != original || restored.Content != p.Content || restored.ETag != "" {
		t.Fatal("restore failed to recover original body, converted fragment and cache state")
	}
}

func TestIPCProtocolAndPersistence(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "helper.sock")
	if len(socket) > 100 {
		short, err := os.MkdirTemp("/private/tmp", "nj-ipc-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(short)
		socket = filepath.Join(short, "s")
	}
	ctx, cancel := context.WithCancel(context.Background())
	exe, _ := os.Executable()
	requirement, err := platform.SigningRequirement(exe)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Development: true, Socket: socket, Data: filepath.Join(dir, "data"), Inbox: filepath.Join(dir, "kernels"), PeerRequirement: requirement}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	client := &ipc.Client{Socket: socket, Development: true, Requirement: requirement}
	var snap model.Snapshot
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := client.Call(context.Background(), "snapshot", nil, &snap)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snap.Status.Protocol != 1 || snap.Status.Privileged {
		t.Fatal("incorrect helper identity")
	}
	// A different, legitimately signed OS executable must not gain access.
	payload, _ := json.Marshal(ipc.Request{Version: 1, Method: "snapshot"})
	command := exec.Command("/usr/bin/curl", "--silent", "--show-error", "--max-time", "2", "--unix-socket", socket, "--data", string(payload), "http://helper/v1")
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("untrusted executable reached signed helper: %s", output)
	}
	var p model.Profile
	if err := client.Call(context.Background(), "profile.save", map[string]any{"profile": model.Profile{Name: "pasted", Kind: "local", Content: "mode: rule\n"}}, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("missing created profile")
	}
	if err := client.Call(context.Background(), "shell.exec", map[string]string{"text": "unexpected"}, nil); err == nil {
		t.Fatal("helper accepted an arbitrary command")
	}
	if err := client.Call(context.Background(), "config.apply", map[string]string{"id": p.ID}, nil); err == nil {
		t.Fatal("apply succeeded without a selected core")
	}
	if err := client.Call(context.Background(), "snapshot", nil, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.State.Profiles) != 1 || snap.State.Profiles[0].Content != "mode: rule\n" {
		t.Fatal("saved local profile missing")
	}
	first := snap.State.SelectedGlobalOverride
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.Call(context.Background(), method, params, out); err != nil {
			t.Fatal(err)
		}
	}
	call("globalOverride.save", map[string]string{"name": "test", "content": "log-level: debug\n"}, nil)
	call("snapshot", nil, &snap)
	if len(snap.State.GlobalOverrides) != 2 || snap.State.SelectedGlobalOverride == first {
		t.Fatal("new override was not added and selected through IPC")
	}
	second := snap.State.SelectedGlobalOverride
	call("globalOverride.save", map[string]string{"id": second, "name": "renamed"}, nil)
	call("globalOverride.select", map[string]string{"id": first}, nil)
	var preview model.Preview
	call("config.preview", map[string]string{"id": p.ID, "globalOverrideId": second}, &preview)
	if !slices.ContainsFunc(preview.Fields, func(f model.Field) bool {
		return f.Path == "log-level" && f.Value == "debug" && f.Source == "全局覆盖 · renamed"
	}) {
		t.Fatal("preview did not identify the requested override")
	}
	call("globalOverride.select", map[string]string{"id": second}, nil)
	call("globalOverride.delete", map[string]string{"id": second}, nil)
	call("snapshot", nil, &snap)
	if len(snap.State.GlobalOverrides) != 1 || snap.State.SelectedGlobalOverride != first {
		t.Fatal("deleting the selected override did not select the remaining override")
	}
}
