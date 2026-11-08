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

func (a *App) Run() error {
	mygo.App.SetName("NativeJet")
	if platformName == "Windows" || platformName == "Linux" {
		if !mygo.App.RequestSingleInstanceLock() {
			return nil
		}
		mygo.App.OnSecondInstance(func([]string, string) { a.showWindow() })
	}
	if platformName == "macOS" && a.startup.HideWindow {
		// Apply the policy before AppKit launches so it won't activate the app.
		mygo.App.Dock.Hide()
	}
	mygo.App.WhenReady(a.start)
	mygo.App.OnOpenURL(func(raw string) {
		// macOS can deliver the link before the main window is created.
		mygo.App.WhenReady(func() { a.OpenURL(raw) })
	})
	return mygo.App.Run()
}

func (a *App) start() {
	mygo.Theme.SetSource(a.appearance.Theme)
	a.trayCoreItem = &mygo.MenuItem{
		Label:    "启用内核",
		Disabled: true,
		Click: func(*mygo.MenuItem, *mygo.Window) {
			a.toggleCore()
			a.win.Invalidate()
		},
	}
	if desktopTrayAvailable() {
		tray, err := mygo.NewTray(mygo.TrayOptions{
			ID:             "dev.nativejet.app.menu-bar",
			Icon:           menuBarOutlineIcon,
			IconIsTemplate: true,
			Menu: mygo.NewMenu([]*mygo.MenuItem{
				{Label: "打开 NativeJet", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
				a.trayCoreItem,
				mygo.Separator(),
				{Label: "退出 NativeJet", Role: mygo.RoleQuit},
			}),
		})
		if err != nil {
			log.Printf("Could not create the NativeJet tray item: %v", err)
		}
		a.tray = tray
	}
	a.updateTray()
	hidden := a.startup.HideWindow && a.tray != nil
	if platformName == "macOS" && a.startup.HideWindow && !hidden {
		// Keep the app reachable if the menu bar item couldn't be created.
		mygo.App.Dock.Show()
	}
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.flushConnectionTablePreferences()
		a.quitting = true
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.showWindow()
		}
	})
	mygo.App.OnDidBecomeActive(func() {
		if a.page == settingsPage && a.win != nil && !a.quitting {
			a.refreshLoginItem()
			a.win.Invalidate()
		}
	})

	options := mygo.WindowOptions{
		Title:     "NativeJet",
		Width:     1160,
		Height:    800,
		MinWidth:  920,
		MinHeight: 640,
		Hidden:    hidden,
		Page:      mygo.PageOptions{BackgroundThrottling: mygo.ThrottlingOff},
		Content:   ui.View(a.view),
	}
	switch platformName {
	case "macOS":
		options.TitleBarStyle = mygo.TitleBarHiddenInset
		options.TrafficLightPosition = &mygo.Point{X: 14, Y: 16}
	case "Windows":
		options.TitleBarStyle = mygo.TitleBarHidden
		options.TitleBarHeight = toolbarHeight
	}
	a.spotlight.showOnStartup()
	a.win = mygo.NewWindow(options)
	if a.page == settingsPage {
		a.refreshLoginItem()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.windowContext = ctx
	a.win.OnClosed(cancel)
	refresh := func() {
		a.refreshWindowData(true)
		a.pollSnapshot()
	}
	a.win.OnShow(refresh)
	a.win.OnRestore(refresh)
	a.win.OnClose(func(e *mygo.CloseEvent) {
		a.flushConnectionTablePreferences()
		// Keep the existing window and its drafts when there is a tray to reopen it.
		if a.tray != nil && !a.quitting {
			e.PreventDefault()
			a.win.Hide()
			if platformName == "macOS" {
				mygo.App.Dock.Hide()
			}
		}
	})
	a.message = "正在连接后台服务…"
	a.autoStartPending = a.startup.AutoStartCore
	a.connect(ctx)
	go a.poll(ctx)
	go a.pollTraffic(ctx)
}

func (a *App) windowVisible() bool {
	return a.win != nil && a.win.IsVisible() && !a.win.IsMinimized()
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
	if running != a.trayDisplay.running {
		label := "启用内核"
		if running {
			label = "停用内核"
		}
		a.trayCoreItem.SetLabel(label)
	}
	display := menuBarDisplay{
		running: running,
		layout:  a.appearance.MenuBarSpeedLayout,
	}
	if display.layout != menuBarSpeedOff {
		display.labels = a.trafficLabels()
	}
	a.updateMenuBarDisplay(display)
}

func (a *App) showWindow() {
	if a.win == nil || a.quitting {
		return
	}
	if platformName == "macOS" && mygo.App.ActivationPolicy() == mygo.ActivationPolicyAccessory {
		mygo.App.Dock.Show()
	}
	a.win.Restore()
	a.win.Show()
	a.win.Invalidate()
}
