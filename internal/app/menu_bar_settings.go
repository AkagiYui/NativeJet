package app

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

type menuBarSpeedLayout string

const (
	menuBarSpeedStacked    menuBarSpeedLayout = "stacked"
	menuBarSpeedSideBySide menuBarSpeedLayout = "side-by-side"
	menuBarSpeedOff        menuBarSpeedLayout = "off"
)

func (a *App) menuBarSettings(c *ui.Context) {
	if platformName == "Windows" {
		show := a.appearance.MenuBarSpeedLayout != menuBarSpeedOff
		if startupSwitch(c, metricsIcon, "托盘速率提示", "悬停在托盘图标上时显示上传和下载速度。", &show, false) {
			prefs := a.appearance
			prefs.MenuBarSpeedLayout = menuBarSpeedOff
			if show {
				prefs.MenuBarSpeedLayout = menuBarSpeedSideBySide
			}
			if a.saveAppearance(prefs) {
				a.updateTray()
			}
		}
		return
	}
	ui.Row(c.Key("menu-bar-settings")).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			iconHeading(c, metricsIcon, "菜单栏速度", 15, 2)
			muted(c, "设置菜单栏中上传和下载速度的显示方式。")
		})
		layouts := []menuBarSpeedLayout{menuBarSpeedStacked, menuBarSpeedSideBySide, menuBarSpeedOff}
		selected := slices.Index(layouts, a.appearance.MenuBarSpeedLayout)
		if selected < 0 {
			selected = 1
		}
		if iconSegments(c, &selected, "菜单栏速度", iconChoice{"单列（上下排列）", listIcon}, iconChoice{"双列（左右并排）", panelIcon}, iconChoice{"关闭", closeIcon}).Changed() {
			prefs := a.appearance
			prefs.MenuBarSpeedLayout = layouts[selected]
			if a.saveAppearance(prefs) {
				a.updateTray()
			}
		}
	})
}
