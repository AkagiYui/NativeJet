package app

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

func TestConnectionSpeedsAcrossSnapshots(t *testing.T) {
	a := &App{}
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	first := &model.Connections{Connections: []model.Connection{
		{ID: "active", Upload: 400, Download: 800},
		{ID: "idle", Upload: 1, Download: 2},
		{ID: "reset", Upload: 20, Download: 20},
		{ID: "closed", Upload: 10, Download: 10},
	}}
	a.acceptConnections(first, nil, at)
	if len(a.connectionSpeeds) != 0 {
		t.Fatal("first sample must not report a speed")
	}

	next := &model.Connections{Connections: []model.Connection{
		{ID: "idle", Upload: 1, Download: 2},
		{ID: "new", Upload: 50, Download: 100},
		{ID: "active", Upload: 500, Download: 1600},
		{ID: "reset", Upload: 1, Download: 1},
	}}
	at = at.Add(2500 * time.Millisecond)
	a.acceptConnections(next, nil, at)
	if got := a.connectionSpeeds["active"]; got != (connectionSpeed{upload: 40, download: 320}) {
		t.Errorf("speed should use the byte delta and elapsed time, got %+v", got)
	}
	if got, ok := a.connectionSpeeds["idle"]; !ok || got != (connectionSpeed{}) {
		t.Errorf("idle connection should report zero speed, got %+v, available %t", got, ok)
	}
	for _, id := range []string{"new", "reset", "closed"} {
		if _, ok := a.connectionSpeeds[id]; ok {
			t.Errorf("%s should have no speed", id)
		}
	}

	a.acceptConnections(nil, errors.New("controller unavailable"), at.Add(time.Second))
	a.acceptConnections(next, nil, at.Add(2*time.Second))
	if len(a.connectionSpeeds) != 0 {
		t.Fatal("first sample after a failed read must not report a speed")
	}
	a.acceptConnections(next, nil, at.Add(3*time.Second))
	a.resetWindowData()
	a.acceptConnections(next, nil, at.Add(4*time.Second))
	if len(a.connectionSpeeds) != 0 {
		t.Fatal("first sample after a core reset must not report a speed")
	}
}

func TestConnectionColumnsAtMinimumWindowWidth(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page, a.connected, a.snapshot.Status.Running, a.connectionIndex = "连接", true, true, -1

	const response = `{"connections":[{
		"id":"direct","metadata":{"host":"example.test","sourceIP":"2001:db8::1","sourcePort":"60123",
		"remoteDestination":"2001:db8::2","destinationPort":"443","type":"Tun","network":"tcp"},
		"upload":32,"download":4096,"rule":"Domain","rulePayload":"example.test","start":"2026-11-08T12:00:00.123Z"
	},{
		"id":"proxy","metadata":{"host":"other.test","destinationPort":"443"},"upload":128,"download":2048
	}]}`
	var first, next model.Connections
	for _, sample := range []*model.Connections{&first, &next} {
		if err := json.Unmarshal([]byte(response), sample); err != nil {
			t.Fatal(err)
		}
	}
	next.Connections[0].Download += 1024
	next.Connections[1].Download += 4096
	at := time.Date(2026, 11, 8, 12, 0, 1, 0, time.UTC)
	a.acceptConnections(&first, nil, at)
	a.acceptConnections(&next, nil, at.Add(time.Second))

	tester := ui.NewTester(a.view, 920, 640)
	table, ok := tester.Find("活动连接列表")
	if !ok {
		t.Fatal("active connections table is missing")
	}
	assertVisible := func(label string) ui.Rect {
		t.Helper()
		rect, found := tester.Find(label)
		if !found || rect.W <= 0 || rect.X < table.X || rect.X+rect.W > table.X+table.W {
			t.Fatalf("%q should be visible inside %v, got %v (found %t)", label, table, rect, found)
		}
		return rect
	}

	assertVisible("example.test:443")
	x, y := table.X+table.W/2, table.Y+table.H/2
	tester.Scroll(x, y, 430, 0)
	assertVisible("32.0 B")
	assertVisible("1.0 KB/s")
	assertVisible("0 B/s")
	if err := tester.Click("下载速度"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("下载速度"); err != nil {
		t.Fatal(err)
	}
	if first, second := assertVisible("4.0 KB/s"), assertVisible("1.0 KB/s"); first.Y >= second.Y {
		t.Errorf("descending speed sort placed %v after %v", first, second)
	}

	tester.Scroll(x, y, 300, 0)
	assertVisible("Domain · example.test")
	assertVisible(at.Add(-time.Second).Local().Format(time.DateTime))
	tester.Scroll(x, y, 2000, 0)
	assertVisible("[2001:db8::1]:60123")
	assertVisible("[2001:db8::2]:443")
	assertVisible("Tun (TCP)")
}
