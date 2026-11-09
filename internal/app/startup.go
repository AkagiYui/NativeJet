package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"nativejet/internal/storage"
)

type startupPreferences struct {
	AutoStartCore bool `json:"autoStartCore"`
	HideWindow    bool `json:"hideWindow"`
}

func (a *App) startupPath() string { return filepath.Join(a.data, "startup.json") }

func (a *App) loadStartup() {
	data, err := os.ReadFile(a.startupPath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var prefs startupPreferences
	if err == nil {
		err = json.Unmarshal(data, &prefs)
	}
	if err != nil {
		a.startupError = "无法读取启动设置：" + err.Error()
		return
	}
	a.startup = prefs
}

func (a *App) saveStartup(prefs startupPreferences) {
	data, err := json.Marshal(prefs)
	if err == nil {
		err = storage.Atomic(a.startupPath(), data, 0600)
	}
	if err != nil {
		a.startupError = "无法保存启动设置：" + err.Error()
		return
	}
	a.startup = prefs
	a.startupError = ""
}

// A successful connection consumes the startup action, even if the core is
// already running or needs manual setup. Reconnects never repeat the action.
func (a *App) maybeAutoStartCore() {
	if !a.autoStartPending || a.quitting || !a.connected || a.busy {
		return
	}
	a.autoStartPending = false
	if a.snapshot.Status.Running {
		return
	}
	if reason := a.autoStartProblem(); reason != "" {
		a.problem = "自动启动内核失败：" + reason
		return
	}
	a.requestHelper("core.start", nil, "已自动启动内核", nil)
}

func (a *App) autoStartProblem() string {
	s := a.snapshot.State
	switch {
	case s.SelectedKernel == "":
		return "请先选择内核"
	case s.ActiveRevision == "":
		return "请先应用一份配置"
	case !slices.ContainsFunc(s.Revisions, func(r model.Revision) bool {
		return r.ID == s.ActiveRevision && r.KernelID == s.SelectedKernel
	}):
		// Manual core.start may apply saved edits when the selected kernel changes.
		return "所选内核已变更，请重新应用配置"
	case a.needsServiceAuthorization("core.start", nil):
		if a.client.Development {
			return "当前配置启用了 TUN，请先在「设置」中安装并授权后台服务"
		}
		return "请先在「设置」中授权所选内核"
	}
	return ""
}

type loginItemSettings struct {
	available bool
	status    platform.LoginItemStatus
	problem   string
}

func (a *App) refreshLoginItem() {
	if !a.loginItem.available {
		return
	}
	status, err := loginItem("status")
	a.loginItem.status = status
	a.loginItem.problem = ""
	if err != nil {
		a.loginItem.problem = "无法读取登录项：" + err.Error()
	}
}

func (a *App) setLoginItem(enabled bool) {
	op := "unregister"
	if enabled {
		op = "register"
	}
	status, err := loginItem(op)
	if err != nil {
		a.refreshLoginItem()
		a.loginItem.problem = "无法更改登录项：" + err.Error()
		return
	}
	a.loginItem.status = status
	a.loginItem.problem = ""
}

func (a *App) startupSettings(c *ui.Context) {
	p := palette(c)
	ui.Column(c.Key("startup-settings")).FillWidth().Padding(16).Gap(12).
		Radius(8).Border(1, p.line).Background(p.Surface).
		Role(ui.RoleGroup).Label("启动").Children(func() {
		iconHeading(c, playIcon, "启动", 15, 2)
		login := a.loginItem.status == platform.LoginItemOn || a.loginItem.status == platform.LoginItemRequiresApproval
		description := "登录 " + platformName + " 后自动打开 NativeJet。"
		switch {
		case !a.loginItem.available:
			description = "请从发布包正常启动 NativeJet 后设置登录项。"
		case a.loginItem.status == platform.LoginItemRequiresApproval:
			description = "登录项尚未获准，请前往系统设置允许 NativeJet 在登录时打开。"
		case a.loginItem.status == platform.LoginItemUnavailable:
			description = "系统无法读取 NativeJet 的登录项。"
		}
		if startupSwitch(c, playIcon, "开机启动", description, &login, !a.loginItem.available || a.loginItem.status == platform.LoginItemUnavailable) {
			a.setLoginItem(login)
		}
		if a.loginItem.status == platform.LoginItemRequiresApproval || a.loginItem.problem != "" {
			if action(c, settingsIcon, "打开系统登录项设置", false).AlignSelf(ui.Start).Clicked() {
				if _, err := loginItem("settings"); err != nil {
					a.loginItem.problem = err.Error()
				}
			}
		}
		if a.loginItem.problem != "" {
			ui.Text(c, a.loginItem.problem).TextColor(p.Danger).Selectable()
		}
		prefs := a.startup
		if startupSwitch(c, kernelIcon, "启动时自动启动内核", "下次启动 NativeJet 时，连接后台服务后启动内核。需要已应用的配置及所需授权。", &prefs.AutoStartCore, false) {
			a.saveStartup(prefs)
		}
		hideDescription := "下次启动时只显示" + trayName + "图标，可从" + trayName + "打开主窗口。"
		if platformName == "Linux" && a.tray == nil {
			hideDescription = "仅在桌面提供系统托盘时生效。当前桌面启动时会显示窗口。"
		}
		if startupSwitch(c, monitorIcon, "启动时不显示窗口", hideDescription, &prefs.HideWindow, false) {
			a.saveStartup(prefs)
		}
		if a.startupError != "" {
			ui.Text(c, a.startupError).TextColor(p.Danger).Selectable()
		}
	})
}

func startupSwitch(c *ui.Context, icon *ui.SVG, title, description string, checked *bool, disabled bool) (changed bool) {
	ui.Row(c.Key(title)).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			iconHeading(c, icon, title, 13, 3)
			muted(c, description)
		})
		changed = ui.Switch(c, checked).Label(title).Description(description).Shrink(0).Disabled(disabled).Changed()
	})
	return changed
}
