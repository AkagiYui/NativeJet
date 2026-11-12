package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

func (a *App) overrideView(c *ui.Context) {
	state, saved, key := a.global, a.savedGlobal, "global-"+a.loadedGlobal
	canSave := a.snapshot.State.GlobalOverrideByID(a.loadedGlobal) != nil
	commandRow(c, func() {
		iconSegments(c, &a.scope, "覆盖范围", iconChoice{"全局覆盖", geoDataIcon}, iconChoice{"当前配置覆盖", configIcon})
		ui.Text(c, "合并规则").FontSize(12).TextColor(c.Theme().TextMuted).
			Tooltip("对象逐项合并，列表整体替换；显式 null 删除字段。false 与“未设置”不同。")
		ui.Box(c).Grow(1)
		if a.scope == 1 {
			state, saved, key = a.override, a.savedOverride, "profile-override"
			canSave = a.current() != nil
		}
		if action(c, undoIcon, "撤销修改", false).Disabled(a.busy || !state.Buffer.Modified()).Clicked() {
			state.Buffer.Reset(saved)
			state.Buffer.MarkSaved()
			a.accept(a.snapshot, false)
		}
		if action(c, saveIcon, "保存覆盖", true).Disabled(a.busy || !a.connected || !canSave || !state.Buffer.Modified()).Clicked() {
			a.saveOverride()
		}
	})
	if a.scope == 1 && a.current() == nil {
		emptyState(c, configIcon, "请先添加一份配置", "每份配置都可以保存自己的覆盖内容。", nil)
		return
	}
	if a.scope == 0 {
		if a.loadedGlobal == "" {
			emptyState(c, configIcon, "等待后台服务", "连接后可以管理全局覆盖。", nil)
			return
		}
		a.globalOverrideRow(c)
	}
	state.ReadOnly = a.busy || !a.connected
	workspaceEditor(c.Key(key), state).Grow(1)
}

func (a *App) globalOverrideRow(c *ui.Context) {
	s := &a.snapshot.State
	preset := s.GlobalOverrideByID(s.SelectedGlobalOverride)
	commandRow(c, func() {
		labels := make([]string, 0, len(s.GlobalOverrides))
		ids := make(map[string]string, len(s.GlobalOverrides))
		selected := ""
		for _, p := range s.GlobalOverrides {
			label := p.Name + " · " + short(p.ID)
			labels = append(labels, label)
			ids[label] = p.ID
			if p.ID == s.SelectedGlobalOverride {
				selected = label
			}
		}
		locked := !a.connected || a.busy || a.global.Buffer.Modified() || a.nameDialog.open || preset == nil
		if ui.Select(c.Key("selected-global-override"), &selected, labels).Label("选用的全局覆盖").Grow(1).MinWidth(0).
			Disabled(locked || len(labels) < 2).Changed() && !locked {
			if id := ids[selected]; id != "" && id != s.SelectedGlobalOverride {
				a.request("globalOverride.select", map[string]string{"id": id}, "已切换全局覆盖，点击“应用配置”后生效", nil)
			}
		}
		if action(c, copyIcon, "新建副本", false).Disabled(locked).Clicked() {
			a.nameGlobalOverride(*preset, true)
		}
		if action(c, editIcon, "重命名", false).Label("重命名全局覆盖").Disabled(locked).Clicked() {
			a.nameGlobalOverride(*preset, false)
		}
		if action(c, trashIcon, "删除", false).Label("删除全局覆盖").Disabled(locked || len(labels) < 2).Clicked() {
			a.deleteGlobalOverride(*preset)
		}
	})
	if a.loadedGlobal != s.SelectedGlobalOverride {
		detail := "选用的全局覆盖已在后台切换。请保存或撤销当前草稿。"
		if original := s.GlobalOverrideByID(a.loadedGlobal); original != nil {
			detail = "正在编辑「" + original.Name + "」的草稿，请先保存或撤销。"
		} else {
			detail = "正在编辑的全局覆盖已删除。可复制草稿内容后撤销修改。"
		}
		commandRow(c, func() { muted(c, detail) })
	}
}

type nameDialogState struct {
	open       bool
	title      string
	name       string
	copyPreset bool
	preset     model.GlobalOverride
}

func (a *App) nameGlobalOverride(preset model.GlobalOverride, copyPreset bool) {
	title, name := "重命名全局覆盖", preset.Name
	if copyPreset {
		title, name = "新建全局覆盖副本", preset.Name+" · 副本"
	}
	a.nameDialog = nameDialogState{open: true, title: title, name: name, copyPreset: copyPreset, preset: preset}
	a.win.Invalidate()
}

func (a *App) nameDialogView(c *ui.Context) {
	d := &a.nameDialog
	if !d.open {
		return
	}
	icon := editIcon
	if d.copyPreset {
		icon = copyIcon
	}
	modalLayer(c, "name", &d.open, palette(c).Scrim, func(_, panel ui.Element) {
		panel.Width(440).Height(210)
		overlayPanel(c, panel, overlayDialog)
		panel.Children(func() {
			ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
				heading(c, icon, d.title, "为这份全局覆盖设置一个名称。")
				ui.TextInput(c, &d.name).FillWidth().Label("全局覆盖名称")
				ui.Box(c).Grow(1)
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Box(c).Grow(1)
					if action(c, closeIcon, "取消", false).Clicked() {
						d.open = false
					}
					if action(c, saveIcon, "保存", true).Disabled(strings.TrimSpace(d.name) == "" || a.busy || !a.connected || a.global.Buffer.Modified()).Clicked() {
						params := map[string]string{"id": d.preset.ID, "name": d.name}
						message := "全局覆盖已重命名"
						if d.copyPreset {
							params = map[string]string{"name": d.name, "content": d.preset.Content}
							message = "已选用新建的全局覆盖，点击“应用配置”后生效"
						}
						a.request("globalOverride.save", params, message, nil)
						d.open = false
					}
				})
			})
		})
	})
}

func (a *App) deleteGlobalOverride(preset model.GlobalOverride) {
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageWarning, Title: "删除全局覆盖",
			Message: "删除「" + preset.Name + "」？", Detail: "已应用版本及其历史会保留。",
			Buttons: []string{"取消", "删除"}, CancelButton: 0, DestructiveButtons: []int{1}})
		if err == nil && result.Button == 1 {
			a.win.Update(func() {
				if a.connected && !a.global.Buffer.Modified() {
					a.request("globalOverride.delete", map[string]string{"id": preset.ID}, "全局覆盖已删除", nil)
				}
			})
		}
	}()
}
