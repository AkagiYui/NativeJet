package app

import "github.com/egoist/mygo/ui"

type menuBarSpeedLayout string

const (
	menuBarSpeedStacked    menuBarSpeedLayout = "stacked"
	menuBarSpeedSideBySide menuBarSpeedLayout = "side-by-side"
)

func (a *App) menuBarSettings(c *ui.Context) {
	ui.Row(c.Key("menu-bar-settings")).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, "菜单栏速度").FontSize(15).FontWeight(600)
			muted(c, "选择上传和下载速度在 macOS 菜单栏中的排列方式。")
		})
		layouts := [...]menuBarSpeedLayout{menuBarSpeedStacked, menuBarSpeedSideBySide}
		selected := 1
		if a.appearance.MenuBarSpeedLayout == menuBarSpeedStacked {
			selected = 0
		}
		if ui.Segmented(c, &selected, "单列（上下排列）", "双列（左右并排）").Label("菜单栏速度布局").Changed() {
			prefs := a.appearance
			prefs.MenuBarSpeedLayout = layouts[selected]
			if a.saveAppearance(prefs) {
				a.updateTray()
			}
		}
	})
}
