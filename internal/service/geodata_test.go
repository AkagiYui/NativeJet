package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

func TestGeoDataThroughHelper(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	if err := m.SaveOverride("", "external-controller-unix: fixture-api.sock\ntun:\n  enable: false\n"); err != nil {
		t.Fatal(err)
	}
	profile, err := m.SaveProfile(model.Profile{Name: "geodata", Kind: "local", Content: "test-geodata: true\n"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err = m.Apply(ctx, profile.ID); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(m.runtime, "GEOIP.DB"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	server := &Server{}
	status := func() model.GeoData {
		t.Helper()
		result, err := server.dispatch(ctx, os.Getuid(), m, ipc.Request{Method: "geo.status"})
		if err != nil {
			t.Fatal(err)
		}
		return result.(model.GeoData)
	}
	update := func(pid int) error {
		t.Helper()
		params, err := json.Marshal(map[string]int{"pid": pid})
		if err != nil {
			t.Fatal(err)
		}
		_, err = server.dispatch(ctx, os.Getuid(), m, ipc.Request{Method: "geo.update", Params: params})
		return err
	}
	stopped := status()
	if stopped.PID != 0 || stopped.SourceError == "" || stopped.Files[0].Name != "GEOIP.DB" || stopped.Files[0].URL != "" {
		t.Fatalf("unexpected stopped status: %+v", stopped)
	}
	if err := update(0); err == nil {
		t.Fatal("stopped core accepted an update")
	}

	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	before := status()
	if before.PID == 0 || before.SourceError != "" || before.DatMode == nil || !*before.DatMode {
		t.Fatalf("expected settings from the running core: %+v", before)
	}
	if before.Files[1].URL != "https://example.com/ip" || before.Files[2].URL != "https://example.com/site" || before.Files[2].Name != "" {
		t.Fatalf("expected separate URL and file status: %+v", before.Files)
	}
	if err := update(before.PID + 1); err == nil || status().Files[2].Name != "" {
		t.Fatal("update for another PID reached the controller")
	}
	if err := update(before.PID); err != nil {
		t.Fatal(err)
	}
	after := status()
	file := after.Files[2]
	if after.PID != before.PID || file.Name != "GeoSite.dat" || file.Size != int64(len("fixture data")) || file.ModifiedAt.IsZero() {
		t.Fatalf("expected the same core and new file metadata: %+v", after)
	}
}
