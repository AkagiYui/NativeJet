package app

import "github.com/egoist/mygo/ui"

// Match the width of MyGo's Windows caption buttons.
const windowButtonWidth = 46

var (
	windowMinimizeIcon = outlineIcon(`<path d="M2 12h20"/>`)
	windowMaximizeIcon = outlineIcon(`<rect x="2" y="2" width="20" height="20"/>`)
	windowRestoreIcon  = outlineIcon(`<path d="M7 7V2h15v15h-5"/><rect x="2" y="7" width="15" height="15"/>`)
	windowCloseIcon    = outlineIcon(`<path d="m2 2 20 20m0-20L2 22"/>`)
)

func (a *App) drawsWindowButtons() bool {
	return platformName == "Linux" && a.win != nil && !a.win.IsFullScreen()
}

func (a *App) titleBar(c *ui.Context) ui.TitleBar {
	bar := c.TitleBar()
	if a.drawsWindowButtons() {
		bar.Right = 3 * windowButtonWidth
	}
	return bar
}

func (a *App) windowButtons(c *ui.Context) {
	w := a.win
	ui.Overlay(c, func() {
		ui.Row(c.Key("window-buttons")).Absolute().Top(0).Right(0).Height(toolbarHeight).Children(func() {
			if windowButton(c.Key("minimize"), "最小化", windowMinimizeIcon, false) {
				w.Minimize()
			}
			label, icon, toggle := "最大化", windowMaximizeIcon, w.Maximize
			if w.IsMaximized() {
				label, icon, toggle = "还原窗口", windowRestoreIcon, w.Unmaximize
			}
			if windowButton(c.Key("maximize"), label, icon, false) {
				toggle()
			}
			if windowButton(c.Key("close"), "关闭窗口", windowCloseIcon, true) {
				// Close on the UI thread after the current frame returns.
				go w.Close()
			}
		})
	})
}

func windowButton(c *ui.Context, label string, icon *ui.SVG, closeButton bool) bool {
	p := palette(c)
	b := ui.ButtonBase(c).Size(windowButtonWidth, toolbarHeight).Role(ui.RoleButton).Label(label).Tooltip(label)
	ink := p.Text
	if b.Hovered() || b.Pressed() {
		back := p.SurfaceHover
		if b.Pressed() {
			back = p.SurfacePressed
		}
		if closeButton {
			ink, back = ui.Hex("#ffffff"), ui.Hex("#c42b1c")
			if b.Pressed() {
				back = ui.Hex("#a52619")
			}
		}
		b.Background(back)
	}
	b.Children(func() { ui.Icon(c, icon).Size(12, 12).TextColor(ink) })
	return b.Clicked()
}
