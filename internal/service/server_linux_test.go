package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

func TestLinuxUserHelperAuthenticatesUnixPeer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the ordinary-user helper must run outside root")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	socket := filepath.Join(dir, "helper.sock")
	server := &Server{Development: true, Socket: socket, Data: filepath.Join(dir, "data"),
		Inbox: filepath.Join(dir, "kernels"), OwnerUID: os.Getuid(), OwnerGID: os.Getgid(),
		Ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("helper did not shut down")
		}
	}()
	select {
	case <-server.Ready:
	case <-ctx.Done():
		t.Fatal("helper did not create its socket")
	}
	client := &ipc.Client{Socket: socket, Development: true}
	var snapshot model.Snapshot
	if err := client.Call(ctx, "snapshot", nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Status.Privileged || snapshot.Status.Protocol != model.ProtocolVersion {
		t.Fatalf("unexpected helper status: %+v", snapshot.Status)
	}
	installed := &ipc.Client{Socket: socket}
	if err := installed.Call(ctx, "snapshot", nil, &snapshot); err == nil {
		t.Fatal("user-owned helper was accepted as the system service")
	}

	duplicate := &Server{Development: true, Socket: socket, OwnerUID: os.Getuid()}
	if listener, cleanup, err := duplicate.listen(ctx); err == nil {
		defer cleanup()
		listener.Close()
		t.Fatal("a second helper replaced the running Unix listener")
	}
	if err := client.Call(ctx, "snapshot", nil, &snapshot); err != nil {
		t.Fatalf("original listener stopped accepting requests: %v", err)
	}
}
