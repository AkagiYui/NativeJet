package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/buildinfo"
	"nativejet/internal/model"
)

func TestStatusBarDoesNotMoveWorkspace(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.connected = true
	a.message = "后台服务已连接"
	a.appendLogs([]model.Log{{Sequence: 1, Source: "process", Level: "error", Message: "内核启动失败"}})
	tester := ui.NewTester(a.view, 920, 640)
	version := platformName + " · " + buildinfo.Version
	start, ok := tester.Find(version)
	if !ok {
		t.Fatal("status bar does not show the version")
	}
	kernel, ok := tester.Find("启动内核")
	if !ok {
		t.Fatal("sidebar does not show the kernel control")
	}
	assertPosition := func(state string) {
		t.Helper()
		for _, control := range []struct {
			label string
			want  ui.Rect
		}{{version, start}, {"启动内核", kernel}} {
			if got, ok := tester.Find(control.label); !ok || got != control.want {
				t.Errorf("%s: %s moved from %v to %v (visible: %v)", state, control.label, control.want, got, ok)
			}
		}
	}

	canceled := 0
	a.busy = true
	a.operationCancel = func() { canceled++ }
	tester.Frame()
	assertPosition("cancellable operation")
	if err := tester.Click("取消操作"); err != nil || canceled != 1 || a.operationCancel != nil {
		t.Fatalf("cancel did not run once: count=%d, err=%v", canceled, err)
	}
	tester.Frame()
	assertPosition("cancellation pending")

	a.busy = false
	a.problem = "操作失败"
	tester.Frame()
	assertPosition("short error")
	a.problem = "内核启动失败\n" + strings.Repeat("等待控制接口返回就绪结果时出错 ", 120)
	tester.Frame()
	assertPosition("long multiline error")
	if err := tester.Click("查看内核日志"); err != nil || a.page != "日志" || a.logSelected != 1 {
		t.Fatalf("error action did not open the relevant log: page=%q selected=%d err=%v", a.page, a.logSelected, err)
	}
	tester.Frame()
	assertPosition("log page after error action")

	a.problem = ""
	a.connected = false
	tester.Frame()
	assertPosition("disconnected")
	a.connected, a.snapshot.Status.Privileged = true, true
	tester.Frame()
	assertPosition("privileged service connected")
}

func TestServiceAddressRequiresCommandClick(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page = serviceTestsPage
	tester := ui.NewTester(a.view, 920, 640)
	const site = "https://www.google.com/"
	if err := tester.Click(site); err != nil {
		t.Fatal(err)
	}
	if len(tester.OpenedURLs()) != 0 {
		t.Fatal("ordinary click opened the service address")
	}
	if err := tester.ClickWith(ui.Cmd, site); err != nil {
		t.Fatal(err)
	}
	if got := tester.OpenedURLs(); len(got) != 1 || got[0] != site {
		t.Fatalf("command-click opened %v", got)
	}
}

func TestPagesShareOneBottomEdge(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.connected = true
	a.accept(model.Snapshot{
		Status:  model.Status{KernelDirectory: "/kernels", Running: true, ServiceTests: true, ServiceCatalog: true},
		State:   model.State{Profiles: []model.Profile{{ID: "local", Name: "测试配置", Kind: "local", Content: "mode: rule\n"}}},
		Kernels: []model.Kernel{{ID: "sample", Name: "mihomo", Path: "/kernels/mihomo", Arch: "arm64"}},
	}, true)
	a.connections = &model.Connections{Connections: []model.Connection{{ID: "sample-connection"}}}
	a.connections.Connections[0].Metadata.Host = "example.test"
	a.appendLogs([]model.Log{{Sequence: 1, Source: "core", Level: "info", Message: "测试日志"}})
	a.preview = model.Preview{YAML: "mode: rule\n", Hash: strings.Repeat("a", 64), Fields: []model.Field{{Path: "mode", Value: "rule"}}}
	a.previewInput = a.currentPreviewInput()

	tester := ui.NewTester(a.view, 920, 640)
	bounds := func(label string) ui.Rect {
		t.Helper()
		r, ok := tester.Find(label)
		if !ok || r.W <= 0 || r.H <= 0 {
			t.Fatalf("%q has no visible bounds", label)
		}
		return r
	}
	for _, window := range []struct {
		w, h int
		dark bool
	}{{920, 640, false}, {1160, 800, true}} {
		tester.SetSize(window.w, window.h)
		tester.SetDark(window.dark)
		bar := bounds("应用状态栏")
		if bar.X != 0 || bar.W != float32(window.w) || bar.Y+bar.H != float32(window.h) {
			t.Fatalf("status bar does not span the window bottom: %v", bar)
		}
		for _, page := range []struct {
			name, content string
		}{{"配置", "最终配置字段"}, {"内核", "识别到的内核"}, {proxiesPage, ""}, {"连接", "活动连接列表"}, {serviceTestsPage, "服务测试结果"}, {"日志", "日志列表"}, {settingsPage, ""}} {
			a.page, a.tab, a.format = page.name, 2, 0
			tester.Frame()
			if got := bounds("应用状态栏"); got != bar {
				t.Errorf("%s: status bar moved from %v to %v", page.name, bar, got)
			}
			if page.content != "" {
				if r := bounds(page.content); r.Y+r.H != bar.Y {
					t.Errorf("%s: content ends at %v, status bar starts at %v", page.name, r.Y+r.H, bar.Y)
				}
			}
		}
		card := bounds("后台服务")
		install := bounds("安装并授权后台服务")
		uninstall := bounds("卸载后台服务，保留配置")
		if install.Y != uninstall.Y || install.X+install.W > uninstall.X ||
			install.X < card.X || uninstall.X+uninstall.W > card.X+card.W ||
			install.Y < card.Y || uninstall.Y+uninstall.H > card.Y+card.H {
			t.Errorf("service buttons %v and %v are not side by side inside %v", install, uninstall, card)
		}
		const clashAction = "将 NativeJet 设为 Clash 链接默认应用"
		if r, ok := tester.Find(clashAction); !ok || r.W <= 0 || r.H <= 0 {
			tester.Scroll(card.X+card.W/2, card.Y+card.H/2, 0, 500)
			card = bounds("后台服务")
		}
		clash := bounds(clashAction)
		if clash.Y < card.Y+card.H || clash.Y+clash.H > bar.Y {
			t.Errorf("Clash action %v is not visible below the service card %v", clash, card)
		}
	}

	a.page, a.connectionIndex = "连接", -1
	tester.Frame()
	connectionHeader := bounds("连接详情")
	a.connectionIndex = 0
	tester.Frame()
	if got := bounds("连接详情"); got != connectionHeader {
		t.Errorf("connection inspector title moved from %v to %v when the close button appeared", connectionHeader, got)
	}
	bounds("关闭此连接")

	a.page = "日志"
	a.logSelected, a.logSelection = 0, -1
	tester.Frame()
	header := bounds("完整日志")
	a.logSelected = 1
	tester.Frame()
	if got := bounds("完整日志"); got != header {
		t.Errorf("log inspector title moved from %v to %v when the copy button appeared", header, got)
	}
	bounds("复制这条日志")
	if err := tester.Click("搜索日志"); err != nil {
		t.Fatal(err)
	}
	tester.Type("no match")
	bounds("显示 0 条 · 缓存 1 条")
}
