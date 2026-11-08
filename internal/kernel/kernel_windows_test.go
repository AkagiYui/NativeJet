package kernel

import (
	"archive/zip"
	"bytes"
	"runtime"
	"testing"
)

func TestWindowsReleaseAsset(t *testing.T) {
	name := "mihomo-windows-amd64-v1-v1.19.33.zip"
	if runtime.GOARCH == "arm64" {
		name = "mihomo-windows-arm64-v1.19.33.zip"
	}
	if !releaseAsset(name) {
		t.Fatalf("rejected %s", name)
	}
	for _, name := range []string{"v1.19.33.zip", "mihomo-windows-amd64-v3-v1.19.33.zip",
		"mihomo-windows-amd64-v1-foo-v1.19.33.zip", "mihomo-darwin-arm64-v1.19.33.gz"} {
		if releaseAsset(name) {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestWindowsArchiveRejectsUnexpectedEntries(t *testing.T) {
	for _, tc := range []struct {
		files []string
		valid bool
	}{
		{[]string{"mihomo-windows-amd64-v1.exe"}, true},
		{[]string{"mihomo-windows-../evil.exe"}, false},
		{[]string{"mihomo-windows-amd64-v1.exe", "wintun.dll"}, false},
		{[]string{"mihomo-windows-evil.exe:stream"}, false},
	} {
		var archive bytes.Buffer
		w := zip.NewWriter(&archive)
		for _, name := range tc.files {
			entry, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte("fixture")); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		got, _, err := unpack(archive.Bytes(), "mihomo-windows-amd64-v1-v1.19.33.zip")
		if (err == nil) != tc.valid || tc.valid && string(got) != "fixture" {
			t.Errorf("files %v: got %q, err %v", tc.files, got, err)
		}
	}
}
