package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"nativejet/internal/kernel"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

func TestStartRejectsChangedManagedCopy(t *testing.T) {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("approved executable")))
	path := filepath.Join(t.TempDir(), "cores", digest, kernel.Executable)
	if err := storage.Atomic(path, []byte("replaced executable"), 0555); err != nil {
		t.Fatal(err)
	}
	var supervisor Supervisor
	err := supervisor.Start(context.Background(), path, digest, t.TempDir(), "unused.yaml", "revision", Controller{})
	if err == nil || !strings.Contains(err.Error(), "身份发生变化") || supervisor.Record() != nil {
		t.Fatalf("startup failed to check the actual executable before process creation: %v", err)
	}
}

func TestLogRingKeepsLatestEntries(t *testing.T) {
	var ring Ring
	for range model.LogCacheLimit + 1 {
		ring.Add("info", "core", "connection matched")
	}
	items := ring.Since(0)
	if len(items) != model.LogCacheLimit || items[0].Sequence != 2 || items[len(items)-1].Sequence != model.LogCacheLimit+1 {
		t.Fatalf("log buffer did not keep the latest %d entries, got %d entries", model.LogCacheLimit, len(items))
	}
}

func TestLogsBoundedAndCursors(t *testing.T) {
	var ring Ring
	message := strings.Repeat("x", 2048)
	for i := 0; i < 5000; i++ {
		ring.Add("info", "test", message)
	}
	items := ring.Since(0)
	if len(items) != logMessageBudget/len(message) || ring.bytes > logMessageBudget {
		t.Fatal("log buffer exceeded its budget")
	}
	cursor := items[len(items)-1].Sequence
	ring.Add("error", "test", "last")
	next := ring.Since(cursor)
	if len(next) != 1 || next[0].Message != "last" {
		t.Fatal("cursor did not return new log")
	}
}

func TestProcessOutputKeepsFragmentsAndBoundsLongLines(t *testing.T) {
	var ring Ring
	writer := processOutput{logs: &ring}
	after := ring.Cursor()
	_, _ = writer.Write([]byte(`level=error msg="Start TUN `))
	_, _ = writer.Write([]byte("listening error\"\nlevel=info msg=ready\nlast line without newline"))
	writer.flush()
	items := ring.Since(after)
	if len(items) != 3 || items[0].Level != "error" || !strings.Contains(items[0].Message, "Start TUN listening error") || items[2].Message != "last line without newline" {
		t.Fatalf("fragmented output lost: %#v", items)
	}
	_, _ = writer.Write([]byte(strings.Repeat("x", 1<<20)))
	writer.flush()
	for _, entry := range ring.Since(after) {
		if len(entry.Message) > 32<<10 {
			t.Fatal("unbounded output line")
		}
	}
}
