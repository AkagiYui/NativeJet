package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/storage"
)

type menuBarSpeedLayout string

const (
	menuBarSpeedStacked    menuBarSpeedLayout = "stacked"
	menuBarSpeedSideBySide menuBarSpeedLayout = "side-by-side"
)

type appearancePreferences struct {
	MenuBarSpeedLayout menuBarSpeedLayout `json:"menuBarSpeedLayout"`
}

func (a *App) appearancePath() string { return filepath.Join(a.data, "appearance.json") }

func (a *App) loadAppearance() {
	a.menuBarSpeedLayout = menuBarSpeedSideBySide
	data, err := os.ReadFile(a.appearancePath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var prefs appearancePreferences
	if err == nil {
		err = json.Unmarshal(data, &prefs)
	}
	if err != nil {
		a.menuBarSettingsError = "无法读取菜单栏设置：" + err.Error()
		return
	}
	if prefs.MenuBarSpeedLayout == menuBarSpeedStacked {
		a.menuBarSpeedLayout = prefs.MenuBarSpeedLayout
	}
}

func (a *App) setMenuBarSpeedLayout(layout menuBarSpeedLayout) {
	data, err := json.Marshal(appearancePreferences{MenuBarSpeedLayout: layout})
	if err == nil {
		err = storage.Atomic(a.appearancePath(), data, 0600)
	}
	if err != nil {
		a.menuBarSettingsError = "无法保存菜单栏设置：" + err.Error()
		return
	}
	a.menuBarSettingsError = ""
	a.menuBarSpeedLayout = layout
	a.updateTray()
}

func (a *App) menuBarSettings(c *ui.Context) {
	p := palette(c)
	ui.Column(c.Key("menu-bar-settings")).FillWidth().Padding(16).Gap(8).
		Radius(8).Border(1, p.line).Background(p.Surface).
		Role(ui.RoleGroup).Label("菜单栏").Children(func() {
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, "菜单栏速度").FontSize(15).FontWeight(600)
				muted(c, "选择上传和下载速度在 macOS 菜单栏中的排列方式。")
			})
			layouts := [...]menuBarSpeedLayout{menuBarSpeedStacked, menuBarSpeedSideBySide}
			selected := 1
			if a.menuBarSpeedLayout == menuBarSpeedStacked {
				selected = 0
			}
			if ui.Segmented(c, &selected, "单列（上下排列）", "双列（左右并排）").Label("菜单栏速度布局").Changed() {
				a.setMenuBarSpeedLayout(layouts[selected])
			}
		})
		if a.menuBarSettingsError != "" {
			ui.Text(c, a.menuBarSettingsError).TextColor(p.Danger).Selectable()
		}
	})
}
