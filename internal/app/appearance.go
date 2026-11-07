package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"nativejet/internal/storage"
)

type appearancePreferences struct {
	Theme              mygo.ThemeSource   `json:"theme"`
	MenuBarSpeedLayout menuBarSpeedLayout `json:"menuBarSpeedLayout"`
}

func (a *App) appearancePath() string { return filepath.Join(a.data, "appearance.json") }

func (a *App) loadAppearance() {
	a.appearance = appearancePreferences{Theme: mygo.ThemeSystem, MenuBarSpeedLayout: menuBarSpeedSideBySide}
	data, err := os.ReadFile(a.appearancePath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var prefs appearancePreferences
	if err == nil {
		err = json.Unmarshal(data, &prefs)
	}
	if err != nil {
		a.appearanceError = "无法读取外观设置：" + err.Error()
		return
	}
	if prefs.Theme == mygo.ThemeLight || prefs.Theme == mygo.ThemeDark {
		a.appearance.Theme = prefs.Theme
	}
	if prefs.MenuBarSpeedLayout == menuBarSpeedStacked {
		a.appearance.MenuBarSpeedLayout = prefs.MenuBarSpeedLayout
	}
}

func (a *App) saveAppearance(prefs appearancePreferences) bool {
	data, err := json.Marshal(prefs)
	if err == nil {
		err = storage.Atomic(a.appearancePath(), data, 0600)
	}
	if err != nil {
		a.appearanceError = "无法保存外观设置：" + err.Error()
		return false
	}
	a.appearance = prefs
	a.appearanceError = ""
	return true
}

func (a *App) appearanceSettings(c *ui.Context) {
	p := palette(c)
	ui.Column(c.Key("appearance-settings")).FillWidth().Padding(16).Gap(12).
		Radius(8).Border(1, p.line).Background(p.Surface).
		Role(ui.RoleGroup).Label("外观").Children(func() {
		ui.Row(c.Key("theme")).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, "主题").FontSize(15).FontWeight(600)
				muted(c, "选择 NativeJet 的浅色或深色外观。")
			})
			themes := []mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark}
			selected := max(0, slices.Index(themes, a.appearance.Theme))
			if ui.Segmented(c, &selected, "跟随系统", "浅色", "深色").Label("主题").Changed() {
				prefs := a.appearance
				prefs.Theme = themes[selected]
				if a.saveAppearance(prefs) {
					mygo.Theme.SetSource(prefs.Theme)
				}
			}
		})
		a.menuBarSettings(c)
		if a.appearanceError != "" {
			ui.Text(c, a.appearanceError).TextColor(p.Danger).Selectable()
		}
	})
}
