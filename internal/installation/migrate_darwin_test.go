package installation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nativejet/internal/config"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func writePayload(t *testing.T, dir string, p Payload) string {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "payload.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func testPayload(t *testing.T) (Payload, string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "core")
	if err := os.WriteFile(source, []byte("test core bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte("test core bytes"))
	id := hex.EncodeToString(hash[:])
	file, err := stageFile(dir, "cores/"+id+"/mihomo", source, 1024)
	if err != nil {
		t.Fatal(err)
	}
	profileID, revisionID := storage.ID(), storage.ID()
	global := model.DefaultGlobalOverride("mixed-port: 7897\n")
	preset := model.GlobalOverride{ID: storage.ID(), Name: "pending", Content: "mixed-port: 9000\n"}
	state := model.State{Version: 1, SelectedKernel: id, ActiveProfile: profileID, ActiveRevision: revisionID,
		GlobalOverrides: []model.GlobalOverride{global, preset}, SelectedGlobalOverride: preset.ID,
		Profiles:  []model.Profile{{ID: profileID, Kind: "local", Name: "kept", Content: "mode: global\n", Override: "ipv6: false\n"}},
		Installed: []model.Kernel{{ID: id, Path: source, Version: "fixture"}},
		Revisions: []model.Revision{{ID: revisionID, ProfileID: profileID, KernelID: id, Source: "mode: rule\nexternal-controller: 127.0.0.1:9090\nsecret: fixture-key\n", GlobalOverride: global.Content, GlobalOverrideID: global.ID, GlobalOverrideName: global.Name, ProfileOverride: "tun:\n  enable: true\n"}}}
	p := Payload{Version: 1, OwnerUID: os.Getuid(), HelperRequirement: "test", HelperSHA256: strings.Repeat("a", 64), State: &state, Files: []File{file}}
	return p, dir, source
}
func TestMigrationPreservesPendingEditsAndAppliedRevision(t *testing.T) {
	p, dir, original := testPayload(t)
	digest := writePayload(t, dir, p)
	parsed, root, err := readPayload(dir, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	target := filepath.Join(t.TempDir(), "target")
	store, err := storage.New(target)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := store.Load()
	old.Profiles = append(old.Profiles, model.Profile{ID: storage.ID(), Kind: "local", Name: "destination only", Content: "mode: direct\n"})
	if err = store.Save(old); err != nil {
		t.Fatal(err)
	}
	state, err := migrate(parsed, root, target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(storage.RuntimeDirectory(target))
	if len(state.Profiles) != 2 || state.Profiles[1].Content != "mode: global\n" {
		t.Fatal("pending edits or existing destination profile lost")
	}
	if len(state.GlobalOverrides) != 2 || state.GlobalOverrides[0] != p.State.GlobalOverrides[0] ||
		state.GlobalOverrides[1] != p.State.GlobalOverrides[1] || state.SelectedGlobalOverride != p.State.SelectedGlobalOverride {
		t.Fatal("global override collection or selection lost")
	}
	if len(state.Revisions) != 2 || state.ActiveRevision == p.State.ActiveRevision {
		t.Fatal("applied history not preserved/recompiled")
	}
	yaml, err := os.ReadFile(filepath.Join(storage.RuntimeDirectory(target), "revisions", state.ActiveRevision+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	values, _, err := config.Parse(string(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if values["mode"] != "rule" || values["mixed-port"] != 7897 || !config.BoolAt(values, "tun.enable") {
		t.Fatal("migration applied pending edits instead of the applied revision")
	}
	if values["external-controller"] != "127.0.0.1:9090" || values["secret"] != "fixture-key" || values["external-controller-unix"] != nil {
		t.Fatal("migration changed the controller configuration")
	}
	if state.Installed[0].Path != filepath.Join(target, "cores", state.SelectedKernel, "mihomo") {
		t.Fatal("core path not normalized")
	}
	if bytes, err := os.ReadFile(original); err != nil || string(bytes) != "test core bytes" {
		t.Fatal("source core was changed")
	}
	if _, err = readRegular(root, "payload.json", 24<<20); err != nil {
		t.Fatal("source payload was removed")
	}
}
func TestMigrationRejectsTamperingWithoutCommittingState(t *testing.T) {
	p, dir, _ := testPayload(t)
	digest := writePayload(t, dir, p)
	_, root, err := readPayload(dir, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = os.WriteFile(filepath.Join(dir, p.Files[0].Path), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	store, _ := storage.New(target)
	before, _ := store.Load()
	_ = store.Save(before)
	bytes, _ := os.ReadFile(store.Path("state.json"))
	if _, err = migrate(p, root, target); err == nil {
		t.Fatal("accepted changed core")
	}
	after, _ := os.ReadFile(store.Path("state.json"))
	if string(after) != string(bytes) {
		t.Fatal("failed migration changed committed state")
	}
	p.Files[0].Path = "runtime/../../escape"
	digest = writePayload(t, dir, p)
	if _, root, err = readPayload(dir, digest); err == nil {
		root.Close()
		t.Fatal("accepted traversal")
	}
	if _, root, err = readPayload(dir, strings.Repeat("0", 64)); err == nil {
		root.Close()
		t.Fatal("accepted wrong manifest digest")
	}
}
func TestPayloadAndRegistrationRejectUnsafeFiles(t *testing.T) {
	p, dir, _ := testPayload(t)
	digest := writePayload(t, dir, p)
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, root, err := readPayload(dir, digest); err == nil {
		root.Close()
		t.Fatal("accepted public migration directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(dir, "manifest")
	if err := os.Rename(filepath.Join(dir, "payload.json"), original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, filepath.Join(dir, "payload.json")); err != nil {
		t.Fatal(err)
	}
	if _, root, err := readPayload(dir, digest); err == nil {
		root.Close()
		t.Fatal("accepted symlink payload")
	}
	r := Registration{Version: 1, OwnerUID: os.Getuid(), HelperRequirement: "test", HelperSHA256: strings.Repeat("a", 64)}
	data, _ := json.Marshal(r)
	path := filepath.Join(t.TempDir(), "registration")
	_ = os.WriteFile(path, data, 0666)
	_ = os.Chmod(path, 0666)
	if _, err := loadRegistration(path, uint32(os.Getuid())); err == nil {
		t.Fatal("accepted writable registration")
	}
}
func TestInstallerScriptQuotesLiteralPaths(t *testing.T) {
	p, dir, _ := testPayload(t)
	digest := writePayload(t, dir, p)
	prepared := Prepared{Directory: dir, Digest: digest, Helper: "/tmp/core '$(touch should-never-run)`echo nope`"}
	script, err := prepared.Script(false)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-n")
	command.Stdin = strings.NewReader(script)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("invalid shell syntax: %v %s", err, out)
	}
	literal := prepared.Helper
	command = exec.Command("/bin/sh", "-c", "printf '%s' "+shellQuote(literal))
	out, err := command.Output()
	if err != nil || string(out) != literal {
		t.Fatal("shell quoting did not preserve literal text")
	}
	if !strings.Contains(script, "${NJ_INSTALL_HASH%% *}") {
		t.Fatal("hash verification expression changed")
	}
}
