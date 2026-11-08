package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nativejet/internal/ipc"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

func TestDevelopmentNamedPipe(t *testing.T) {
	dir := t.TempDir()
	sid, err := platform.CurrentSID()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	socket := `\\.\pipe\nativejet-test-` + storage.ID()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := &Server{Development: true, OwnerSID: sid, Socket: socket, Ready: make(chan struct{}),
		Data: filepath.Join(dir, "data"), Inbox: filepath.Join(dir, "kernels")}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	select {
	case <-server.Ready:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	client := &ipc.Client{Socket: socket, Development: true, Requirement: exe}
	var profile model.Profile
	draft := model.Profile{Name: "Fixture", Kind: "local", Content: "rules: [MATCH,DIRECT]\n"}
	if err := client.Call(ctx, "profile.save", map[string]model.Profile{"profile": draft}, &profile); err != nil {
		t.Fatal(err)
	}
	var snapshot model.Snapshot
	if err := client.Call(ctx, "snapshot", nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Status.Privileged || len(snapshot.State.Profiles) != 1 || snapshot.State.Profiles[0].ID != profile.ID {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	untrusted := &ipc.Client{Socket: socket, Development: true, Requirement: filepath.Join(dir, "unexpected.exe")}
	if err := untrusted.Call(ctx, "snapshot", nil, nil); err == nil {
		t.Fatal("client accepted a server with the wrong executable path")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
