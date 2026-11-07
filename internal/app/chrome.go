package app

import (
	_ "embed"
	"strings"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/buildinfo"
)

//go:embed assets/MenuBarTemplate.svg
var brandIconSVG []byte

//go:embed assets/MenuBarOutlineTemplate.svg
var brandOutlineSVG []byte

var (
	brandIcon    = ui.MustParseSVG(brandIconSVG)
	brandOutline = ui.MustParseSVG(brandOutlineSVG)
	kernelIcon   = outlineIcon(`<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3m6-3v3M9 19v3m6-3v3M2 9h3m-3 6h3m14-6h3m-3 6h3"/>`)
	configIcon   = outlineIcon(`<path d="M14 3H6a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8zM14 3v5h5M8 12h8m-8 4h6"/>`)
	proxiesIcon  = outlineIcon(`<rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/>`)
	linkIcon     = outlineIcon(`<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4m-6 4v-4h12v4"/>`)
	logIcon      = outlineIcon(`<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m7 9 3 3-3 3m6 0h4"/>`)
	serviceIcon  = outlineIcon(`<path d="M12 2.5 20 6v5c0 5-3 8-8 10.5C7 19 4 16 4 11V6zM8.5 12l2.5 2.5 4.5-5"/>`)
	settingsIcon = outlineIcon(`<path d="M4 6h16M4 12h16M4 18h16M8 4v4m8 2v4m-6 2v4"/>`)
	plusIcon     = outlineIcon(`<path d="M12 5v14M5 12h14"/>`)
	downloadIcon = outlineIcon(`<path d="M12 3v12m-4-4 4 4 4-4M4 16v4h16v-4"/>`)
	folderIcon   = outlineIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	scanIcon     = outlineIcon(`<path d="M19.5 8A8 8 0 1 0 20 15M20 3v5h-5"/>`)
	panelIcon    = outlineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M14 4v16"/>`)
)

func outlineIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round">` + body + `</g></svg>`))
}

func (a *App) view(c *ui.Context) {
	useWorkspaceTheme(c)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			ui.Column(c.Key("page-" + a.page)).Grow(1).FillHeight().MinWidth(0).Children(func() {
				switch a.page {
				case "配置":
					a.configView(c)
				case "内核":
					a.kernelView(c)
				case proxiesPage:
					a.proxiesView(c)
				case "连接":
					a.connectionsView(c)
				case "日志":
					a.logsView(c)
				case settingsPage:
					a.settingsView(c)
				}
			})
		})
		a.statusBar(c)
	})
}

func (a *App) sidebar(c *ui.Context) {
	p := palette(c)
	bar := c.TitleBar()
	icon := brandOutline
	if a.coreRunning() {
		icon = brandIcon
	}
	ui.Column(c).Width(sidebarWidth).FillHeight().Shrink(0).Background(p.rail).
		BorderWidth(0, 1, 0, 0).BorderColor(p.line).Children(func() {
		ui.Row(c).FillWidth().Height(max(toolbarHeight, bar.Height)).Shrink(0).
			Padding(0, 12, 0, max(12, bar.Left+8)).Gap(7).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Box(c).Size(22, 22).Center().Radius(controlRadius).Background(p.active).Children(func() {
				ui.Icon(c, icon).Size(15, 15).TextColor(p.Text)
			})
			ui.Text(c, "NativeJet").FontWeight(600).SingleLine()
		})
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(12, 8).Gap(3).
			Role(ui.RoleRadioGroup).Label("页面导航").FocusGroup(ui.Vertical).Children(func() {
			sectionLabel(c, "工作区")
			a.navItem(c, configIcon, "配置")
			a.navItem(c, kernelIcon, "内核")
			a.navItem(c, proxiesIcon, proxiesPage)
			a.navItem(c, linkIcon, "连接")
			a.navItem(c, logIcon, "日志")
			ui.Box(c).Grow(1)
			sectionLabel(c, "系统")
			if a.navItem(c, settingsIcon, settingsPage) {
				a.refreshService()
			}
		})
		a.kernelStatus(c)
	})
}

func (a *App) navItem(c *ui.Context, icon *ui.SVG, name string) bool {
	p := palette(c)
	item := ui.RadioBase(c.Key(name), &a.page, name).Label(name).Height(30).FillWidth().
		Radius(controlRadius).Padding(0, 10).Gap(8).AlignItems(ui.Center)
	active := a.page == name
	ink, weight := p.TextMuted, 400
	if active {
		item.Background(p.active)
		ink, weight = p.Accent, 600
	}
	item.Draw(func(draw *ui.Painter, r ui.Rect) {
		if active {
			draw.Fill(ui.Rect{X: r.X + 1, Y: r.Y + (r.H-16)/2, W: 2, H: 16}, p.Accent, 1)
		} else if item.Hovered() {
			draw.Fill(r, p.SurfaceHover, controlRadius)
		}
	})
	item.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(ink)
		ui.Text(c, name).FontWeight(weight).SingleLine()
	})
	return item.Changed()
}

func (a *App) kernelStatus(c *ui.Context) {
	t := c.Theme()
	status := a.snapshot.Status
	version := status.KernelVersion
	if version == "" {
		version = "尚未选择内核"
	}
	state, button := "已停止", "启动内核"
	ink := t.TextMuted
	if !a.connected {
		state = "未连接"
	} else if status.Running {
		state, button = "运行中", "停止内核"
		ink = t.Success
	}
	ui.Column(c).FillWidth().Padding(10, 12, 12).Gap(8).Shrink(0).
		BorderWidth(1, 0, 0, 0).BorderColor(palette(c).line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "内核状态").FontSize(12).FontWeight(600)
			ui.Box(c).Grow(1)
			stateLabel(c, state, ink)
		})
		ui.Text(c, version).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(version)
		if action(c, nil, button, false).FillWidth().Disabled(a.busy || !a.connected).Clicked() {
			a.toggleCore()
		}
	})
}

func (a *App) statusBar(c *ui.Context) {
	p := palette(c)
	label, ink := "未连接", p.TextMuted
	if a.connected && a.snapshot.Status.Privileged {
		label, ink = "系统服务已连接", p.Success
	} else if a.connected {
		label, ink = "普通服务 · TUN 待授权", p.Warning
	}
	ui.Row(c).FillWidth().Height(statusBarHeight).Shrink(0).PaddingX(12).Gap(12).AlignItems(ui.Center).
		Role(ui.RoleStatus).Label("应用状态栏").
		Background(p.rail).BorderWidth(1, 0, 0, 0).BorderColor(p.line).Children(func() {
		stateLabel(c, label, ink)
		ui.Box(c).Size(1, 12).Background(p.line).Shrink(0)
		message, color := a.message, p.TextMuted
		if a.busy {
			message = a.busyText()
		} else if a.problem != "" {
			message, color = a.problem, p.Danger
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			summary := strings.Join(strings.Fields(message), " ")
			text := ui.Text(c, summary).FontSize(11).TextColor(color).SingleLine().
				Tooltip(message).Label("操作状态：" + message)
			if !a.busy && a.problem != "" {
				text.Focusable()
			}
		})
		if a.busy {
			if action(c.Key("cancel"), nil, "取消操作", false).Disabled(a.operationCancel == nil).Clicked() {
				a.operationCancel()
				a.operationCancel = nil
				a.message = "正在取消操作，等待后台结束"
			}
		} else if a.problem != "" {
			if action(c.Key("show-logs"), nil, "查看内核日志", false).Clicked() {
				a.openLogs()
			}
		}
		ui.Text(c.Key("version"), "macOS · "+buildinfo.Version).FontSize(11).TextColor(p.TextMuted).SingleLine().Shrink(0)
	})
}

func stateLabel(c *ui.Context, text string, ink ui.Color) {
	ui.Row(c).AlignItems(ui.Center).Gap(5).Shrink(0).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(ink)
		ui.Text(c, text).FontSize(11).TextColor(ink).SingleLine()
	})
}

func workspaceHeader(c *ui.Context, icon *ui.SVG, title string, actions func()) {
	p := palette(c)
	bar := c.TitleBar()
	ui.Row(c).FillWidth().Height(max(toolbarHeight, bar.Height)).Shrink(0).
		Padding(0, max(12, bar.Right), 0, 16).AlignItems(ui.Center).Gap(8).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line).DragWindow().Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(p.TextMuted)
		ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine()
		ui.Box(c).Grow(1)
		if actions != nil {
			actions()
		}
	})
}

func commandRow(c *ui.Context, children func()) {
	ui.Row(c).FillWidth().Shrink(0).MinHeight(42).Padding(6, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(palette(c).line).Children(children)
}

func sectionLabel(c *ui.Context, title string) {
	ui.Text(c, title).Padding(4, 9).FontSize(11).FontWeight(600).TextColor(c.Theme().TextMuted).SingleLine()
}

func emptyState(c *ui.Context, icon *ui.SVG, title, detail string, buttons func()) {
	p := palette(c)
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Center().Padding(16).Gap(10).Children(func() {
		ui.Box(c).Size(40, 40).Radius(8).Center().Background(p.rail).Border(1, p.line).Children(func() {
			ui.Icon(c, icon).Size(21, 21).TextColor(p.TextMuted)
		})
		ui.Text(c, title).FontSize(15).FontWeight(600)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(p.TextMuted)
		}
		if buttons != nil {
			buttons()
		}
	})
}

func inspector(c *ui.Context, title string, tools, content func()) {
	p := palette(c)
	ui.Column(c).Width(inspectorWidth).FillHeight().Shrink(0).Background(p.Background).
		BorderWidth(0, 0, 0, 1).BorderColor(p.line).Children(func() {
		ui.Row(c).FillWidth().Height(inspectorHeaderHeight).Shrink(0).PaddingX(12).Gap(7).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.line).Children(func() {
			ui.Icon(c, panelIcon).Size(14, 14).TextColor(p.TextMuted)
			ui.Text(c, title).FontSize(12).FontWeight(600)
			if tools != nil {
				ui.Box(c).Grow(1)
				tools()
			}
		})
		content()
	})
}

func detailScroll(c *ui.Context, content func()) {
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Padding(14, 16, 14, 12).ScrollbarInsets(0, 4, 0, 0).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(content)
	})
}

func property(c *ui.Context, name, value string) {
	if value == "" {
		value = "—"
	}
	ui.Column(c).FillWidth().Gap(3).Children(func() {
		ui.Text(c, name).FontSize(11).FontWeight(500).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).FontSize(12).Selectable()
	})
}

func heading(c *ui.Context, title, detail string) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(600)
		if detail != "" {
			muted(c, detail)
		}
	})
}

func muted(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
}
