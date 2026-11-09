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
	brandIcon        = ui.MustParseSVG(brandIconSVG)
	brandOutline     = ui.MustParseSVG(brandOutlineSVG)
	kernelIcon       = outlineIcon(`<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3m6-3v3M9 19v3m6-3v3M2 9h3m-3 6h3m14-6h3m-3 6h3"/>`)
	geoDataIcon      = outlineIcon(`<circle cx="12" cy="12" r="9"/><ellipse cx="12" cy="12" rx="4" ry="9"/><path d="M3 12h18"/>`)
	configIcon       = outlineIcon(`<path d="M14 3H6a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8zM14 3v5h5M8 12h8m-8 4h6"/>`)
	proxiesIcon      = outlineIcon(`<rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/>`)
	linkIcon         = outlineIcon(`<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4m-6 4v-4h12v4"/>`)
	metricsIcon      = outlineIcon(`<path d="M3 12h4l3-7 4 14 3-7h4"/>`)
	serviceTestIcon  = outlineIcon(`<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>`)
	logIcon          = outlineIcon(`<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m7 9 3 3-3 3m6 0h4"/>`)
	serviceIcon      = outlineIcon(`<path d="M12 2.5 20 6v5c0 5-3 8-8 10.5C7 19 4 16 4 11V6zM8.5 12l2.5 2.5 4.5-5"/>`)
	settingsIcon     = outlineIcon(`<path d="M4 6h16M4 12h16M4 18h16M8 4v4m8 2v4m-6 2v4"/>`)
	guideIcon        = outlineIcon(`<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 1 1 4 2c-1.2.8-1.5 1.3-1.5 2.5M12 16.5h.01"/>`)
	plusIcon         = outlineIcon(`<path d="M12 5v14M5 12h14"/>`)
	downloadIcon     = outlineIcon(`<path d="M12 3v12m-4-4 4 4 4-4M4 16v4h16v-4"/>`)
	folderIcon       = outlineIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	scanIcon         = outlineIcon(`<path d="M19.5 8A8 8 0 1 0 20 15M20 3v5h-5"/>`)
	panelIcon        = outlineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M14 4v16"/>`)
	playIcon         = outlineIcon(`<path d="m8 5 12 7-12 7z"/>`)
	stopIcon         = outlineIcon(`<rect x="6" y="6" width="12" height="12" rx="1"/>`)
	pauseIcon        = outlineIcon(`<path d="M9 5v14m6-14v14"/>`)
	closeIcon        = outlineIcon(`<path d="m6 6 12 12m0-12L6 18"/>`)
	checkIcon        = outlineIcon(`<path d="m5 12 4 4L19 6"/>`)
	historyIcon      = outlineIcon(`<path d="M3 5v5h5m-5 0a9 9 0 1 1 2 8m7-11v5l3 2"/>`)
	saveIcon         = outlineIcon(`<path d="M4 4h13l3 3v13H4zM8 4v6h8V4M8 20v-6h8v6"/>`)
	copyIcon         = outlineIcon(`<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/>`)
	pasteIcon        = outlineIcon(`<rect x="8" y="3" width="8" height="4" rx="1"/><path d="M8 5H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M8 12h8m-8 4h5"/>`)
	editIcon         = outlineIcon(`<path d="m4 16 11-11a2.8 2.8 0 0 1 4 4L8 20l-5 1zM13 7l4 4"/>`)
	undoIcon         = outlineIcon(`<path d="M8 5l-5 5 5 5m-5-5h11a5 5 0 0 1 0 10h-3"/>`)
	trashIcon        = outlineIcon(`<path d="M3 6h18M9 6V3h6v3M6 6l1 14h10l1-14m-7 4v6m3-6v6"/>`)
	compareIcon      = outlineIcon(`<path d="M4 8h16m-4-4 4 4-4 4M20 16H4m4-4-4 4 4 4"/>`)
	layersIcon       = outlineIcon(`<path d="m12 3 9 5-9 5-9-5zm-9 9 9 5 9-5m-18 4 9 5 9-5"/>`)
	listIcon         = outlineIcon(`<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01"/>`)
	codeIcon         = outlineIcon(`<path d="m8 7-5 5 5 5m8-10 5 5-5 5m-3-13-2 16"/>`)
	subscriptionIcon = outlineIcon(`<path d="m9 15 6-6m-5-1 3-3a4.2 4.2 0 0 1 6 6l-3 3m-2 2-3 3a4.2 4.2 0 0 1-6-6l3-3"/>`)
	externalLinkIcon = outlineIcon(`<path d="M14 4h6v6m0-6-9 9m9 0v6a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h6"/>`)
	pinIcon          = outlineIcon(`<path d="m15 3 6 6-3 1-3 3v3l-2 2-7-7 2-2h3l3-3zM9 15l-5 5"/>`)
	unpinIcon        = outlineIcon(`<path d="m3 3 18 18M11 9l3-3 1-3 6 6-3 1-3 3m0 3-2 2-7-7 2-2m1 6-5 5"/>`)
	uploadIcon       = outlineIcon(`<path d="M12 16V3m-4 4 4-4 4 4M4 16v4h16v-4"/>`)
	monitorIcon      = outlineIcon(`<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M12 17v4m-4 0h8"/>`)
	sunIcon          = outlineIcon(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>`)
	moonIcon         = outlineIcon(`<path d="M20.5 15A9 9 0 0 1 9 3.5 9 9 0 1 0 20.5 15z"/>`)
	arrowLeftIcon    = outlineIcon(`<path d="M20 12H4m6-6-6 6 6 6"/>`)
	arrowRightIcon   = outlineIcon(`<path d="M4 12h16m-6-6 6 6-6 6"/>`)
	skipIcon         = outlineIcon(`<path d="m5 5 10 7-10 7zM19 5v14"/>`)
)

func outlineIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round">` + body + `</g></svg>`))
}

func (a *App) view(c *ui.Context) {
	useWorkspaceTheme(c)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			page := a.displayedPage()
			ui.Column(c.Key("page-" + page)).Grow(1).FillHeight().MinWidth(0).Children(func() {
				switch page {
				case "配置":
					a.configView(c)
				case "内核":
					a.kernelView(c)
				case geoDataPage:
					a.geoDataView(c)
				case proxiesPage:
					a.proxiesView(c)
				case "连接":
					a.connectionsView(c)
				case serviceTestsPage:
					a.serviceTestsView(c)
				case metricsPage:
					a.metricsView(c)
				case "日志":
					a.logsView(c)
				case settingsPage:
					a.settingsView(c)
				}
			})
		})
		a.statusBar(c)
	})
	a.spotlight.view(c)
	a.updateConnectionTablePreferences(c)
}

func (a *App) sidebar(c *ui.Context) {
	p := palette(c)
	bar := c.TitleBar()
	_, height := c.Size()
	navHeight, navPadding, navGap := float32(30), float32(12), float32(3)
	if height < 690 {
		navHeight, navPadding, navGap = 26, 6, 2
	}
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
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(navPadding, 8).Gap(navGap).
			Role(ui.RoleRadioGroup).Label("页面导航").FocusGroup(ui.Vertical).Children(func() {
			sectionLabel(c, panelIcon, "工作区")
			a.navItem(c, configIcon, "配置", navHeight)
			a.navItem(c, kernelIcon, "内核", navHeight)
			a.navItem(c, geoDataIcon, geoDataPage, navHeight)
			a.navItem(c, proxiesIcon, proxiesPage, navHeight)
			a.navItem(c, linkIcon, "连接", navHeight)
			a.navItem(c, serviceTestIcon, serviceTestsPage, navHeight)
			a.navItem(c, metricsIcon, metricsPage, navHeight)
			a.navItem(c, logIcon, "日志", navHeight)
			ui.Box(c).Grow(1)
			sectionLabel(c, settingsIcon, "系统")
			if a.navItem(c, settingsIcon, settingsPage, navHeight) {
				a.refreshLoginItem()
				a.refreshService()
			}
		})
		a.kernelStatus(c)
		a.trafficPanel(c)
	})
}

func (a *App) navItem(c *ui.Context, icon *ui.SVG, name string, height float32) bool {
	p := palette(c)
	item := ui.RadioBase(c.Key(name), &a.page, name).Label(name).Height(height).FillWidth().
		Radius(controlRadius).Padding(0, 10).Gap(8).AlignItems(ui.Center)
	active := a.displayedPage() == name
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
	icon := playIcon
	ink := t.TextMuted
	if !a.connected {
		state = "未连接"
	} else if status.Running {
		state, button = "运行中", "停止内核"
		icon, ink = stopIcon, t.Success
	}
	ui.Column(c).FillWidth().Padding(10, 12, 12).Gap(8).Shrink(0).
		BorderWidth(1, 0, 0, 0).BorderColor(palette(c).line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			iconHeading(c, kernelIcon, "内核状态", 12, 2)
			ui.Box(c).Grow(1)
			stateLabel(c, state, ink)
		})
		ui.Text(c, version).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(version)
		if action(c, icon, button, false).Bind(&a.spotlight.targets[spotlightStart]).
			FillWidth().Disabled(a.busy || !a.connected).Clicked() {
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
			if action(c.Key("cancel"), closeIcon, "取消操作", false).Disabled(a.operationCancel == nil).Clicked() {
				a.operationCancel()
				a.operationCancel = nil
				a.message = "正在取消操作，等待后台结束"
			}
		} else if a.problem != "" {
			if action(c.Key("show-logs"), logIcon, "查看内核日志", false).Clicked() {
				a.openLogs()
			}
		}
		guide := ui.ButtonBase(c.Key("usage-guide")).Label("使用引导").Role(ui.RoleButton).Cursor(ui.CursorPointer).Gap(5)
		guide.Children(func() {
			ui.Icon(c, guideIcon).Size(13, 13).TextColor(p.Accent).Shrink(0)
			ui.Text(c, "使用引导").FontSize(11).TextColor(p.Accent).Underline().SingleLine()
		})
		if guide.Clicked() {
			a.spotlight.show()
		}
		ui.Text(c.Key("version"), platformName+" · "+buildinfo.Version).FontSize(11).TextColor(p.TextMuted).SingleLine().Shrink(0)
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
		Padding(0, bar.Right+12, 0, 16).AlignItems(ui.Center).Gap(8).
		BorderWidth(0, 0, 1, 0).BorderColor(p.line).DragWindow().Children(func() {
		iconHeading(c, icon, title, 15, 1).Shrink(0)
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

func sectionLabel(c *ui.Context, icon *ui.SVG, title string) {
	iconHeading(c, icon, title, 11, 2).Padding(3, 9).TextColor(c.Theme().TextMuted)
}

func emptyState(c *ui.Context, icon *ui.SVG, title, detail string, buttons func()) {
	p := palette(c)
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Center().Padding(16).Gap(10).Children(func() {
		ui.Box(c).Size(40, 40).Radius(8).Center().Background(p.rail).Border(1, p.line).Children(func() {
			ui.Icon(c, icon).Size(21, 21).TextColor(p.TextMuted)
		})
		headingText(c, title, 15, 2)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(p.TextMuted)
		}
		if buttons != nil {
			buttons()
		}
	})
}

func inspector(c *ui.Context, icon *ui.SVG, title string, tools, content func()) {
	p := palette(c)
	ui.Column(c).Width(inspectorWidth).FillHeight().Shrink(0).Background(p.Background).
		BorderWidth(0, 0, 0, 1).BorderColor(p.line).Children(func() {
		ui.Row(c).FillWidth().Height(inspectorHeaderHeight).Shrink(0).PaddingX(12).Gap(7).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.line).Children(func() {
			iconHeading(c, icon, title, 12, 2).Shrink(0)
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

func heading(c *ui.Context, icon *ui.SVG, title, detail string) {
	ui.Column(c).Gap(4).Children(func() {
		iconHeading(c, icon, title, 17, 1)
		if detail != "" {
			muted(c, detail)
		}
	})
}

func iconHeading(c *ui.Context, icon *ui.SVG, title string, size float32, level int) ui.Element {
	return ui.Row(c).Gap(7).AlignItems(ui.Center).MinWidth(0).Children(func() {
		ui.Icon(c, icon).Size(size, size).TextColor(c.Theme().TextMuted).Shrink(0)
		headingText(c, title, size, level).SingleLine().MinWidth(0)
	})
}

func headingText(c *ui.Context, title string, size float32, level int) ui.Element {
	return ui.Text(c, title).FontSize(size).FontWeight(600).Role(ui.RoleHeading).Level(level)
}

func muted(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
}
