package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

func TestStartupSwitchesPersistIndependently(t *testing.T) {
	a := &App{data: t.TempDir()}
	tester := ui.NewTester(a.startupSettings, 700, 260)
	for _, label := range []string{"启动时自动启动内核", "启动时不显示窗口"} {
		tester.Key(0, ui.KeyTab)
		if !tester.Focused(label) {
			t.Fatalf("%q is not keyboard reachable", label)
		}
		tester.Key(0, ui.KeySpace)
	}
	reloaded := &App{data: a.data}
	reloaded.loadStartup()
	if !reloaded.startup.AutoStartCore || !reloaded.startup.HideWindow {
		t.Fatalf("reloaded preferences = %+v, save error = %q", reloaded.startup, a.startupError)
	}
	// The last switch is still focused. Turning it off must leave auto start on.
	tester.Key(0, ui.KeySpace)
	reloaded.loadStartup()
	if !reloaded.startup.AutoStartCore || reloaded.startup.HideWindow {
		t.Fatalf("changing window visibility affected auto start: %+v", reloaded.startup)
	}
}

func TestAutoStartUsesOnlyAppliedAndAuthorizedConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, selected, configuredKernel, problem string
		running, development, tun                 bool
	}{
		{name: "no selection", problem: "请先选择内核"},
		{name: "no applied configuration", selected: "kernel", problem: "请先应用一份配置"},
		{name: "changed kernel", selected: "kernel", configuredKernel: "previous", problem: "请重新应用配置"},
		{name: "unapproved kernel", selected: "kernel", configuredKernel: "kernel", problem: "授权所选内核"},
		{name: "TUN needs system service", selected: "kernel", configuredKernel: "kernel", development: true, tun: true, problem: "安装并授权后台服务"},
		{name: "already running", running: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{autoStartPending: true, client: &ipc.Client{Development: tc.development}}
			a.snapshot.State.SelectedKernel = tc.selected
			if tc.configuredKernel != "" {
				a.snapshot.State.ActiveRevision = "applied"
				a.snapshot.State.Revisions = []model.Revision{{ID: "applied", KernelID: tc.configuredKernel}}
			}
			a.snapshot.Status.Running = tc.running
			a.snapshot.Status.ActiveTUN = tc.tun
			a.maybeAutoStartCore()
			if !a.autoStartPending || a.problem != "" {
				t.Fatal("automatic startup must wait for the helper")
			}
			a.connected = true
			a.maybeAutoStartCore()
			if a.autoStartPending || a.busy || !strings.Contains(a.problem, tc.problem) || (tc.problem == "" && a.problem != "") {
				t.Fatalf("unexpected startup result: pending=%v busy=%v problem=%q", a.autoStartPending, a.busy, a.problem)
			}
			a.problem = ""
			a.maybeAutoStartCore()
			if a.problem != "" || a.busy {
				t.Fatal("a later snapshot repeated the startup action")
			}
		})
	}
}
