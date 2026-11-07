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

// Start creates the menu bar item and main window on MyGo's ready callback.
func (a *App) Start() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "dev.nativejet.app.menu-bar",
		Icon:           menuBarIcon,
		IconIsTemplate: true,
		ToolTip:        "NativeJet",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开 NativeJet", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			mygo.Separator(),
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
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 14, Y: 12},
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

func (a *App) showWindow() {
	if a.win == nil || a.quitting {
		return
	}
	if mygo.App.ActivationPolicy() == mygo.ActivationPolicyAccessory {
		mygo.App.Dock.Show()
	}
	a.win.Restore()
	a.win.Show()
}
