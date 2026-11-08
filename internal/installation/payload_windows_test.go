package installation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPreparedPayload(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	payload, root, err := readPayload(prepared.Directory, prepared.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || !validOwner(payload.OwnerSID) {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if err := os.WriteFile(filepath.Join(prepared.Directory, "payload.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, root, err := readPayload(prepared.Directory, prepared.Digest); err == nil {
		_ = root.Close()
		t.Fatal("accepted a modified installation manifest")
	}
}
