package app

import (
	"context"
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed assets/MenuBarTemplate.png
var menuBarIcon []byte

//go:embed assets/MenuBarOutlineTemplate.png
var menuBarOutlineIcon []byte

// Start creates the menu bar item and main window on MyGo's ready callback.
func (a *App) Start() {
	a.trayCoreItem = &mygo.MenuItem{
		Label:    "启用内核",
		Disabled: true,
		Click: func(*mygo.MenuItem, *mygo.Window) {
			a.toggleCore()
			a.win.Invalidate()
		},
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "dev.nativejet.app.menu-bar",
		Icon:           menuBarOutlineIcon,
		IconIsTemplate: true,
		ToolTip:        "NativeJet",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开 NativeJet", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			a.trayCoreItem,
			mygo.Separator(),
			{Label: "将 NativeJet 设为 Clash 链接默认应用", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.showWindow()
				if err := mygo.App.RegisterURLScheme("clash"); err != nil {
					a.problem = "无法设置 Clash 链接的默认应用，请从打包的 NativeJet.app 重试"
				} else {
					a.problem = ""
					a.message = "NativeJet 已设为 Clash 链接的默认应用"
				}
				a.win.Invalidate()
			}},
			{Label: "退出 NativeJet", Role: mygo.RoleQuit},
		}),
	})
	if err != nil {
		log.Printf("Could not create the NativeJet menu bar item: %v", err)
	}
	a.tray = tray
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.showWindow()
		}
	})

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:                "NativeJet",
		TitleBarStyle:        mygo.TitleBarHiddenInset,
		TrafficLightPosition: &mygo.Point{X: 14, Y: 16},
		Width:                1160,
		Height:               800,
		MinWidth:             920,
		MinHeight:            640,
		Page:                 mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff},
		Content:              ui.View(a.view),
	})
	ctx, cancel := context.WithCancel(context.Background())
	a.win.OnClosed(cancel)
	a.win.OnClose(func(e *mygo.CloseEvent) {
		// Keep the existing window and its drafts when there is a tray to reopen it.
		if a.tray != nil && !a.quitting {
			e.PreventDefault()
			a.win.Hide()
			mygo.App.Dock.Hide()
		}
	})
	a.message = "正在连接后台服务…"
	a.connect(ctx)
	go a.poll(ctx)
}

func (a *App) toggleCore() {
	if a.quitting || !a.connected || a.busy {
		return
	}
	method := "core.start"
	if a.snapshot.Status.Running {
		method = "core.stop"
	}
	a.request(method, nil, "内核状态已更新", nil)
}

func (a *App) coreRunning() bool {
	return a.connected && a.snapshot.Status.Running
}

func (a *App) updateTray() {
	if a.tray == nil || a.quitting {
		return
	}
	enabled := a.connected && !a.busy
	if a.trayCoreItem.IsEnabled() != enabled {
		a.trayCoreItem.SetEnabled(enabled)
	}
	running := a.coreRunning()
	if a.trayRunning == running {
		return
	}
	icon, label := menuBarOutlineIcon, "启用内核"
	if running {
		icon, label = menuBarIcon, "停用内核"
	}
	if err := a.tray.SetIcon(icon, true); err != nil {
		log.Printf("Could not update the NativeJet menu bar icon: %v", err)
	}
	a.trayCoreItem.SetLabel(label)
	a.trayRunning = running
}

func (a *App) showWindow() {
	if a.win == nil || a.quitting {
		return
	}
	if mygo.App.ActivationPolicy() == mygo.ActivationPolicyAccessory {
		mygo.App.Dock.Show()
	}
	a.win.Restore()
	a.win.Show()
	a.win.Invalidate()
}
