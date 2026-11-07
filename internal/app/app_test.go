package app

import (
	"nativejet/internal/model"
	"testing"
)

func TestSnapshotRefreshPreservesDrafts(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	s := model.Snapshot{State: model.State{Profiles: []model.Profile{{ID: "local", Name: "local", Kind: "local", Content: "mode: rule\n"}}, GlobalOverride: "mixed-port: 7897\n"}}
	a.accept(s, true)
	a.source.Buffer.Set("mode: global\n")
	a.global.Buffer.Set("mixed-port: 1234\n")
	a.override.Buffer.Set("ipv6: false\n")
	a.accept(s, false)
	if a.source.Buffer.String() != "mode: global\n" || a.global.Buffer.String() != "mixed-port: 1234\n" || a.override.Buffer.String() != "ipv6: false\n" {
		t.Fatal("background refresh discarded an edit")
	}
	a.source.Buffer.MarkSaved()
	a.savedSource = "mode: global\n"
	s.State.Profiles[0].Content = "mode: global\n"
	a.accept(s, false)
	if a.global.Buffer.String() != "mixed-port: 1234\n" || a.override.Buffer.String() != "ipv6: false\n" {
		t.Fatal("saving one document discarded another document's draft")
	}
}

func TestLogsMergeSourcesAndOpenFailureDetails(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.appendLogs([]model.Log{{Sequence: 1, Source: "core", Level: "info", Message: "runtime stream"}, {Sequence: 2, Source: "process", Level: "error", Message: "TUN route failed"}})
	a.appendLogs([]model.Log{{Sequence: 2, Source: "process", Level: "error", Message: "TUN route failed"}, {Sequence: 3, Source: "helper", Level: "error", Message: "startup failed"}})
	if len(a.logs) != 3 || a.logAfter != 3 {
		t.Fatal("overlapping poll and request duplicated logs")
	}
	a.logSource = "内核日志"
	rows := a.filteredLogs()
	if len(rows) != 1 || rows[0].Source != "core" {
		t.Fatal("source filter hid the core log stream")
	}
	a.paused = true
	a.logSearch = "old search"
	a.logLevel = "debug"
	a.openLogs()
	if a.page != "日志" || a.paused || a.logSearch != "" || a.logSelected != 2 || len(a.filteredLogs()) != 3 {
		t.Fatal("failure shortcut did not reveal the actual core error")
	}
}
