package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

func TestConnectionColumnSettingsSurviveRestart(t *testing.T) {
	opts := Options{Data: t.TempDir(), Helper: "/unused", Development: true}
	openApp := func() *App {
		t.Helper()
		a, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		a.page, a.connectionIndex = "连接", -1
		return a
	}
	a := openApp()
	tester := ui.NewTester(a.view, 1160, 640)
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	bounds := func(label string) ui.Rect {
		t.Helper()
		rect, ok := tester.Find(label)
		if !ok || rect.W <= 0 || rect.H <= 0 {
			t.Fatalf("missing %q", label)
		}
		return rect
	}
	showConnections := func(a *App) {
		a.connected, a.snapshot.Status.Running = true, true
		a.connections = &model.Connections{Connections: []model.Connection{
			{ID: "small", Download: 90},
			{ID: "large", Download: 9000},
		}}
		a.connections.Connections[0].Metadata.Host = "small.example"
		a.connections.Connections[1].Metadata.Host = "large.example"
	}

	// Column visibility is configurable before the core starts.
	click("列设置")
	click("显示类型")
	tester.Key(0, ui.KeyEscape)
	showConnections(a)
	tester.Frame()

	table := bounds("活动连接列表")
	process := bounds("进程")
	x, y := table.X+164+88-2, process.Y+process.H/2
	tester.Press(x, y)
	tester.Move(x+30, y)
	tester.Release(x+30, y)
	if width := a.connectionRows.Columns.Widths["process"]; width != 118 {
		t.Fatalf("dragged process width = %v, want 118", width)
	}

	click("列设置")
	click("显示进程")
	tester.Key(0, ui.KeyEscape)
	if _, ok := tester.Find("进程"); ok {
		t.Fatal("hidden process column is still shown")
	}

	host := bounds("主机")
	x, y = host.X+host.W/2, host.Y+host.H/2
	tester.Press(x, y)
	for _, dx := range []float32{5, 30, 60, 90} {
		tester.Move(x+dx, y)
	}
	tester.Release(x+90, y)
	click("累计下载")
	click("累计下载")
	tester.Advance(connectionTableSaveDelay + time.Millisecond)

	reloaded := openApp()
	if reloaded.connectionTableError != "" {
		t.Fatal(reloaded.connectionTableError)
	}
	if got := reloaded.connectionTable.Order[:3]; !slices.Equal(got, []string{"chain", "process", "host"}) {
		t.Errorf("reloaded order = %v", got)
	}
	if width := reloaded.connectionRows.Columns.Widths["process"]; width != 118 {
		t.Errorf("reloaded hidden process width = %v, want 118", width)
	}
	if !reloaded.connectionTable.Hidden["process"] || !reloaded.connectionTable.Hidden["type"] {
		t.Errorf("reloaded hidden columns = %v", reloaded.connectionTable.Hidden)
	}
	if got := reloaded.connectionSort; got != (ui.SortOrder{Column: "download", Descending: true}) {
		t.Errorf("reloaded sort = %+v", got)
	}
	showConnections(reloaded)
	tester = ui.NewTester(reloaded.view, 1160, 640)
	if bounds("策略").X >= bounds("主机").X || bounds("large.example").Y >= bounds("small.example").Y {
		t.Error("reloaded order or sort is not applied to the table")
	}

	click("列设置")
	click("显示进程")
	click("显示累计下载")
	tester.Key(0, ui.KeyEscape)
	if strategy, process, host := bounds("策略"), bounds("进程"), bounds("主机"); !(strategy.X < process.X && process.X < host.X) {
		t.Errorf("restored process is not between strategy and host: %v, %v, %v", strategy, process, host)
	}
	if bounds("large.example").Y >= bounds("small.example").Y {
		t.Error("hiding the sorted column changed the sort")
	}
	click("列设置")
	click("恢复默认列设置")
	// Closing the window flushes pending changes without waiting for the timer.
	reloaded.flushConnectionTablePreferences()
	reset := openApp()
	if len(reset.connectionTable.Hidden) != 0 || len(reset.connectionRows.Columns.Widths) != 0 ||
		reset.connectionSort != (ui.SortOrder{}) || reset.connectionTable.Order[0] != "host" {
		t.Errorf("reloaded defaults = %+v", reset.connectionTable)
	}
}
