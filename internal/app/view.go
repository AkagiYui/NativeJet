package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"nativejet/internal/buildinfo"
	"nativejet/internal/logging"
	"nativejet/internal/model"
)

func (a *App) view(c *ui.Context) {
	t := *c.Theme()
	t.FontSize = 13
	t.Spacing = 4
	if !t.Dark {
		t.Accent = ui.Hex("#3565d9")
		t.AccentText = ui.Hex("#ffffff")
	}
	c.SetTheme(&t)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			ui.Column(c.Key("page-" + a.page)).Grow(1).FillWidth().MinHeight(0).Padding(22).Gap(16).Children(func() {
				switch a.page {
				case "配置":
					a.configView(c)
				case "内核":
					a.kernelView(c)
				case "连接":
					a.connectionsView(c)
				case "日志":
					a.logsView(c)
				case "后台服务":
					a.serviceView(c)
				}
			})
			ui.Column(c).FillWidth().Padding(9, 18).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Gap(4).Children(func() {
				if a.busy {
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.busyText()).FontSize(12).TextColor(t.TextMuted).Grow(1).SingleLine().Label(a.busyText())
						if ui.Button(c, "取消操作").Disabled(a.operationCancel == nil).Clicked() {
							a.operationCancel()
							a.operationCancel = nil
							a.message = "正在取消操作，等待后台结束"
						}
					})
					return
				}
				if a.problem != "" {
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.problem).TextColor(t.Danger).Grow(1).Label("错误信息：" + a.problem)
						if ui.Button(c, "查看内核日志").Clicked() {
							a.openLogs()
						}
					})
				} else {
					ui.Text(c, a.message).FontSize(12).TextColor(t.TextMuted).Label("操作状态：" + a.message)
				}
			})
		})
	})
}

func (a *App) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(184).FillHeight().Shrink(0).Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		// Reserve the window controls and a drag area inside the sidebar.
		ui.Box(c).FillWidth().Height(float32(max(c.TitleBar().Height, 28))).Shrink(0).DragWindow()
		ui.Column(c).FillWidth().Grow(1).Padding(6, 12, 14).Gap(12).Children(func() {
			ui.Row(c).PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "N").Bold().FontSize(20)
				ui.Text(c, "NativeJet").Bold()
			})
			ui.Sidebar(c, &a.page, func() {
				for _, page := range []string{"内核", "配置", "连接", "日志"} {
					ui.SidebarItem(c, page, nil, page)
				}
			}).Grow(1)
			a.kernelStatus(c)
			if ui.Button(c, "后台服务").FillWidth().Clicked() {
				a.page = "后台服务"
				a.refreshService()
			}
			label := "未连接"
			if a.connected {
				label = "已连接"
				if !a.snapshot.Status.Privileged {
					label = "普通服务 · TUN 待授权"
				}
			}
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, "macOS · "+buildinfo.Version).FontSize(11).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *App) kernelStatus(c *ui.Context) {
	t := c.Theme()
	status := a.snapshot.Status
	version := status.KernelVersion
	if version == "" {
		version = "尚未选择内核"
	}
	state := "已停止"
	button, method := "启动内核", "core.start"
	if status.Running {
		state = "运行中"
		button, method = "停止内核", "core.stop"
	}
	ui.Column(c).FillWidth().Padding(8).Gap(8).Border(1, t.Border).Radius(8).Children(func() {
		ui.Text(c, version).FontSize(11).Bold().MaxLines(2).Tooltip(version)
		badge(c, state, status.Running)
		if ui.Button(c, button).FillWidth().Disabled(a.busy || !a.connected).Clicked() {
			a.request(method, nil, "内核状态已更新", nil)
		}
	})
}
func badge(c *ui.Context, text string, good bool) {
	t := c.Theme()
	color := t.TextMuted
	if good {
		color = t.Success
	}
	ui.Row(c).Padding(3, 7).Radius(5).Background(t.Surface).Children(func() { ui.Text(c, text).FontSize(11).TextColor(color) })
}
func heading(c *ui.Context, title, detail string) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(23).Bold()
		if detail != "" {
			ui.Text(c, detail).TextColor(c.Theme().TextMuted)
		}
	})
}
func muted(c *ui.Context, text string) { ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted) }
func (a *App) configView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		heading(c, "配置", "原文、覆盖与最终结果，在同一处查看。")
		ui.Box(c).Grow(1)
		if ui.Button(c, "添加配置").Disabled(a.busy || !a.connected).Clicked() {
			a.addConfig()
		}
	})
	if len(a.snapshot.State.Profiles) == 0 {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
			ui.Text(c, "添加第一份配置").FontSize(20).Bold()
			muted(c, "支持订阅地址、添加文件和粘贴 YAML 内容。")
			if ui.PrimaryButton(c, "添加配置").Disabled(!a.connected).Clicked() {
				a.addConfig()
			}
		})
		return
	}
	ui.Row(c).FillWidth().Padding(12).Border(1, t.Border).Radius(9).Gap(12).AlignItems(ui.Center).Children(func() {
		labels := []string{}
		current := ""
		lookup := map[string]string{}
		for _, p := range a.snapshot.State.Profiles {
			kind := "本地配置"
			if p.Kind == "subscription" {
				kind = "订阅"
			}
			text := p.Name + " · " + kind + " · " + short(p.ID)
			labels = append(labels, text)
			lookup[text] = p.ID
			if p.ID == a.selected {
				current = text
			}
		}
		if ui.Select(c, &current, labels).Label("正在查看的配置").Grow(1).Changed() {
			if a.dirty() {
				a.problem = "请先保存或撤销编辑，再切换配置"
			} else {
				a.selected = lookup[current]
				a.accept(a.snapshot, true)
				if a.tab == 2 {
					a.loadPreview()
				}
			}
		}
		p := a.current()
		if p != nil {
			if p.ID == a.snapshot.State.ActiveProfile {
				badge(c, "已应用", true)
			} else {
				badge(c, "尚未应用", false)
			}
			if p.Kind == "subscription" {
				if ui.Button(c, "更新订阅").Disabled(a.busy).Clicked() {
					a.request("profile.update", map[string]string{"id": p.ID}, "订阅已更新，运行配置保持不变", func(_ json.RawMessage) { a.accept(a.snapshot, false) })
				}
			} else if ui.Button(c, "导出原文").Disabled(a.busy).Clicked() {
				a.exportProfile()
			}
			if ui.Button(c, "删除").Disabled(a.busy || a.dirty() || p.ID == a.snapshot.State.ActiveProfile).Clicked() {
				a.deleteProfile(*p)
			}
		}
	})
	if ui.Tabs(c, &a.tab, "原始配置", "覆盖配置", "最终配置").Changed() {
		if a.tab == 2 {
			a.loadPreview()
		}
	}
	switch a.tab {
	case 0:
		p := a.current()
		if p == nil {
			return
		}
		a.source.ReadOnly = p.Kind == "subscription" || a.busy
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			if p.Kind == "subscription" {
				badge(c, "订阅原文 · 只读", false)
				muted(c, p.UpdatedAt.Local().Format("2006-01-02 15:04")+" 更新")
			} else {
				badge(c, "本地配置 · 可编辑", false)
				muted(c, "导入副本可直接编辑，保留原文件。")
			}
			ui.Box(c).Grow(1)
			if p.Kind == "local" {
				if ui.Button(c, "撤销修改").Disabled(a.busy || !a.source.Buffer.Modified()).Clicked() {
					a.source.Buffer.Reset(a.savedSource)
					a.source.Buffer.MarkSaved()
				}
				if ui.PrimaryButton(c, "保存配置").Disabled(a.busy || !a.source.Buffer.Modified()).Clicked() {
					a.saveSource()
				}
			} else if ui.Button(c, "创建本地副本").Disabled(a.busy || a.dirty()).Clicked() {
				copy := *p
				copy.ID = ""
				copy.Kind = "local"
				copy.Name += " · 副本"
				copy.URL = ""
				copy.ETag = ""
				a.request("profile.save", map[string]any{"profile": copy}, "本地副本已创建", func(raw json.RawMessage) {
					var p model.Profile
					_ = json.Unmarshal(raw, &p)
					a.selected = p.ID
					a.accept(a.snapshot, true)
				})
			}
		})
		editor.View(c.Key("source-editor"), a.source).Grow(1).FillWidth().MinHeight(180)
	case 1:
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Segmented(c, &a.scope, "全局覆盖", "当前配置覆盖")
			ui.Box(c).Grow(1)
			state := a.global
			saved := a.savedGlobal
			if a.scope == 1 {
				state = a.override
				saved = a.savedOverride
			}
			if ui.Button(c, "撤销修改").Disabled(a.busy || !state.Buffer.Modified()).Clicked() {
				state.Buffer.Reset(saved)
				state.Buffer.MarkSaved()
			}
			if ui.PrimaryButton(c, "保存覆盖").Disabled(a.busy || !state.Buffer.Modified()).Clicked() {
				a.saveOverride()
			}
		})
		muted(c, "对象逐项合并，列表整体替换；显式 null 删除字段。false 与“未设置”不同。")
		state := a.global
		if a.scope == 1 {
			state = a.override
		}
		state.ReadOnly = a.busy
		editor.View(c.Key(fmt.Sprint("override-", a.scope)), state).Grow(1).FillWidth().MinHeight(180)
	case 2:
		a.finalView(c)
	}
	ui.Row(c).FillWidth().PaddingY(10).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Gap(10).AlignItems(ui.Center).Children(func() {
		caption := "尚未应用配置"
		if a.snapshot.State.ActiveRevision != "" {
			caption = "已应用版本 " + short(a.snapshot.State.ActiveRevision)
		}
		if a.dirty() {
			caption += " · 有未保存的修改"
		}
		muted(c, caption)
		ui.Box(c).Grow(1)
		if ui.Button(c, "历史版本").Disabled(a.busy).Clicked() {
			a.history()
		}
		disabled := a.busy || !a.connected || a.current() == nil
		if ui.PrimaryButton(c, "应用配置").Disabled(disabled).Clicked() {
			a.apply()
		}
	})
}
func (a *App) finalView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c, &a.format, "字段与来源", "YAML")
		muted(c, "已隐藏常见敏感值")
		ui.Box(c).Grow(1)
		if ui.Button(c, "刷新预览").Disabled(a.busy).Clicked() {
			a.loadPreview()
		}
		if ui.Button(c, "与已应用版本比较").Disabled(a.busy || a.snapshot.State.ActiveRevision == "" || a.preview.YAML == "").Clicked() {
			a.showDiff()
		}
	})
	if a.preview.YAML == "" {
		ui.Column(c).Grow(1).Center().Children(func() { muted(c, "点击“刷新预览”生成最终配置。") })
		return
	}
	if a.format == 1 {
		editor.View(c, a.final).Grow(1).FillWidth()
		return
	}
	ui.Row(c).Grow(1).FillWidth().MinHeight(180).Gap(18).Children(func() {
		columns := []ui.TableColumn{{Title: "字段", Width: 180}, {Title: "提交值", Width: 110}, {Title: "来源"}}
		ui.Table(c, &a.fields, columns, len(a.preview.Fields), func(row, col int) {
			f := a.preview.Fields[row]
			text := f.Path
			switch col {
			case 1:
				text = value(f.Value)
			case 2:
				text = f.Source
			}
			ui.Text(c, text).FontSize(12).SingleLine()
		}).Grow(1).FillHeight().Label("最终配置字段")
		ui.Scroll(c).Width(220).FillHeight().BorderWidth(0, 0, 0, 1).BorderColor(t.Border).PaddingStart(16).Children(func() {
			ui.Column(c).Gap(14).FillWidth().Children(func() {
				ui.Text(c, "字段来源").Bold()
				if a.field < 0 || a.field >= len(a.preview.Fields) {
					return
				}
				f := a.preview.Fields[a.field]
				ui.Text(c, f.Path).TextColor(t.Accent)
				for _, trace := range f.Trace {
					ui.Column(c).Gap(3).Children(func() {
						muted(c, trace.Layer)
						text := value(trace.Value)
						if trace.Deleted {
							text = "删除字段"
						}
						ui.Text(c, text)
						if trace.Line > 0 {
							muted(c, fmt.Sprintf("第 %d 行", trace.Line))
						}
					})
				}
				ui.Text(c, "最终提交值").Bold()
				ui.Text(c, value(f.Value))
			})
		})
	})
	muted(c, "配置 SHA-256: "+a.preview.Hash)
}
func (a *App) kernelView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		heading(c, "内核", "从目录识别，或下载一个独立内核。")
		ui.Box(c).Grow(1)
		if ui.PrimaryButton(c, "下载内核").Disabled(a.busy || !a.connected).Clicked() {
			a.downloadDialog()
		}
	})
	ui.Column(c).FillWidth().Padding(16).Gap(10).Background(t.Surface).Radius(9).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "内核目录").Bold()
			ui.Box(c).Grow(1)
			if ui.Button(c, "打开目录").Disabled(!a.connected).Clicked() {
				a.openDirectory()
			}
			if ui.Button(c, "重新扫描").Disabled(a.busy || !a.connected).Clicked() {
				a.request("kernel.scan", nil, "内核目录扫描完成", nil)
			}
		})
		ui.Text(c, a.snapshot.Status.KernelDirectory).FontSize(12)
		muted(c, "把解压后的 macOS 内核放入目录，再扫描识别。目录中不附带任何内核。")
	})
	if a.snapshot.Status.KernelPath != "" {
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			ui.Text(c, "实际运行副本").Bold()
			muted(c, a.snapshot.Status.KernelPath)
			if a.snapshot.Status.ConfigPath != "" {
				muted(c, "配置文件："+a.snapshot.Status.ConfigPath)
			}
		})
	}
	if len(a.snapshot.Kernels) == 0 {
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Text(c, "还没有发现内核").FontSize(20).Bold()
			muted(c, "放入内核文件后点击“重新扫描”，或从内核管理下载。")
		})
		return
	}
	selection := a.kernelSelection
	columns := []ui.TableColumn{{Title: "文件 / 版本"}, {Title: "架构", Width: 65}, {Title: "状态", Width: 115}}
	ui.Table(c, &a.kernelRows, columns, len(a.snapshot.Kernels), func(row, col int) {
		k := a.snapshot.Kernels[row]
		text := k.Name
		if col == 1 {
			text = k.Arch
		}
		if col == 2 {
			text = "可用"
			if k.Error != "" {
				text = "无法使用"
			} else if k.ID == a.snapshot.State.SelectedKernel {
				text = "当前选用"
			}
		}
		ui.Text(c, text).FontSize(12).SingleLine()
	}).Grow(1).FillWidth().Label("识别到的内核")
	if a.kernelSelection >= 0 && a.kernelSelection < len(a.snapshot.Kernels) {
		selection = a.kernelSelection
	} else {
		selection = 0
	}
	k := a.snapshot.Kernels[selection]
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		ui.Text(c, k.Version).Bold()
		muted(c, k.Path)
		if k.Error != "" {
			ui.Text(c, k.Error).TextColor(t.Danger)
		} else {
			muted(c, "SHA-256: "+k.ID)
		}
	})
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		if a.snapshot.Status.Running {
			muted(c, "切换版本前，请先停止运行中的内核。")
		} else {
			muted(c, "选用时复制到受管理目录，再由后台服务启动。")
		}
		ui.Box(c).Grow(1)
		if ui.PrimaryButton(c, "使用所选内核").Disabled(a.busy || k.Error != "" || a.snapshot.Status.Running || k.ID == a.snapshot.State.SelectedKernel).Clicked() {
			a.request("kernel.select", map[string]string{"id": k.ID}, "内核已选用", nil)
		}
	})
}
func (a *App) connectionsView(c *ui.Context) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		heading(c, "连接", "查看流量去向、匹配规则和当前策略。")
		ui.Box(c).Grow(1)
		if a.snapshot.Status.Running && a.connectionsError == nil {
			badge(c, fmt.Sprintf("%d 个连接", len(a.connections.Connections)), false)
		}
	})
	ui.Tabs(c, &a.connectionTab, "活动连接", "策略组")
	if !a.snapshot.Status.Running {
		ui.Column(c).Grow(1).Center().Children(func() { muted(c, "内核未运行。启动后显示实时连接。") })
		return
	}
	problem := a.connectionsError
	if a.connectionTab == 1 {
		problem = a.proxiesError
	}
	if problem != nil {
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			muted(c, "连接和策略组需要 Mihomo 控制接口。")
			muted(c, problem.Error())
		})
		return
	}
	if a.connectionTab == 1 {
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Gap(15).FillWidth().Children(func() {
				for _, g := range a.groups {
					g := g
					ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).PaddingY(10).Children(func() {
						ui.Column(c).Grow(1).Children(func() { ui.Text(c, g.Name).Bold(); muted(c, g.Type) })
						selected := g.Now
						if ui.Select(c, &selected, g.All).Width(240).Disabled(a.busy || g.Type != "Selector").Changed() {
							a.request("proxies.select", map[string]string{"id": g.Name, "name": selected}, "策略组已更新", nil)
						}
					})
				}
				if len(a.groups) == 0 {
					muted(c, "等待从内核读取策略组…")
				}
			})
		})
		return
	}
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		ui.TextInput(c, &a.search).Placeholder("搜索目标、进程或策略").Grow(1).Label("搜索连接")
		if ui.Button(c, "关闭全部").Disabled(a.busy).Clicked() {
			a.request("connections.closeAll", nil, "连接已关闭", nil)
		}
	})
	rows := []model.Connection{}
	for _, conn := range a.connections.Connections {
		if strings.Contains(strings.ToLower(conn.Metadata.Host+" "+conn.Metadata.Process+" "+conn.Metadata.DestinationIP+" "+strings.Join(conn.Chains, " ")), strings.ToLower(a.search)) {
			rows = append(rows, conn)
		}
	}
	columns := []ui.TableColumn{{Title: "目标"}, {Title: "进程", Width: 130}, {Title: "策略", Width: 150}, {Title: "下载", Width: 100}}
	ui.Table(c, &a.connectionRows, columns, len(rows), func(row, col int) {
		conn := rows[row]
		text := conn.Metadata.Host
		if text == "" {
			text = conn.Metadata.DestinationIP
		}
		if col == 1 {
			text = conn.Metadata.Process
			if text == "" {
				text = filepath.Base(conn.Metadata.ProcessPath)
			}
		}
		if col == 2 {
			text = strings.Join(conn.Chains, " → ")
		}
		if col == 3 {
			text = bytesString(conn.Download)
		}
		ui.Text(c, text).FontSize(12).SingleLine()
	}).Grow(1).FillWidth().Label("活动连接")
	if a.connectionIndex >= 0 && a.connectionIndex < len(rows) {
		conn := rows[a.connectionIndex]
		ui.Column(c).FillWidth().Gap(7).Children(func() {
			ui.Row(c).FillWidth().Children(func() {
				ui.Text(c, conn.Metadata.Host).Bold()
				ui.Box(c).Grow(1)
				if ui.Button(c, "关闭此连接").Disabled(a.busy).Clicked() {
					a.request("connections.close", map[string]string{"id": conn.ID}, "连接已关闭", nil)
				}
			})
			muted(c, "目标 "+conn.Metadata.DestinationIP+":"+conn.Metadata.DestinationPort+" · "+conn.Metadata.Network+" · "+conn.Metadata.Type)
			muted(c, "规则 "+conn.Rule+" · "+conn.RulePayload)
			muted(c, "转发链 "+strings.Join(conn.Chains, " → ")+" · 上传 "+bytesString(conn.Upload)+" / 下载 "+bytesString(conn.Download))
		})
	}
}
func (a *App) logsView(c *ui.Context) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		heading(c, "日志", "实时内核日志、标准输出和错误输出；启动失败后仍可查看。")
		ui.Box(c).Grow(1)
		label := "暂停显示"
		if a.paused {
			label = "继续显示"
		}
		if ui.Button(c, label).Clicked() {
			a.paused = !a.paused
		}
		if ui.Button(c, "导出日志").Clicked() {
			a.exportLogs()
		}
	})
	ui.Row(c).FillWidth().Gap(10).Children(func() {
		ui.TextInput(c, &a.logSearch).Placeholder("搜索日志内容").Grow(1).Label("搜索日志")
		ui.Select(c, &a.logSource, []string{"全部来源", "内核日志", "启动输出", "后台服务"}).Width(125).Label("日志来源")
		ui.Select(c, &a.logLevel, []string{"全部", "info", "warning", "error", "debug"}).Width(100).Label("日志级别")
	})
	rows := a.filteredLogs()
	if a.logSelected != 0 {
		a.logSelection = slices.IndexFunc(rows, func(l model.Log) bool { return l.Sequence == a.logSelected })
	}
	columns := []ui.TableColumn{{Title: "时间", Width: 80}, {Title: "级别", Width: 65}, {Title: "来源", Width: 85}, {Title: "内容"}}
	ui.Table(c, &a.logRows, columns, len(rows), func(row, col int) {
		l := rows[row]
		_, text := logging.Parse(l.Message)
		switch col {
		case 0:
			text = l.Time.Local().Format("15:04:05")
		case 1:
			text = l.Level
		case 2:
			text = logSourceLabel(l.Source)
		}
		el := ui.Text(c, text).FontSize(12).SingleLine()
		if l.Level == "error" {
			el.TextColor(c.Theme().Danger)
		}
	}).Grow(1).FillWidth().MinHeight(100).Label("日志列表")
	if a.logSelection >= 0 && a.logSelection < len(rows) {
		l := rows[a.logSelection]
		a.logSelected = l.Sequence
		if a.logDetailSequence != l.Sequence {
			a.logDetail.Buffer.Reset(l.Time.Local().Format(time.RFC3339) + " [" + l.Level + "] [" + logSourceLabel(l.Source) + "]\n" + l.Message)
			a.logDetailSequence = l.Sequence
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "完整日志").Bold()
			ui.Box(c).Grow(1)
			if ui.Button(c, "复制这条日志").Clicked() {
				mygo.Clipboard.WriteText(a.logDetail.Buffer.String())
			}
		})
		editor.View(c, a.logDetail).Height(125).FillWidth()
	} else {
		muted(c, "选择一条日志可查看和复制完整内容。")
	}
	muted(c, fmt.Sprintf("显示 %d 条，共缓存 %d 条；最多保留 2000 条，暂停只停止拉取。", len(rows), len(a.logs)))
}
func (a *App) serviceView(c *ui.Context) {
	heading(c, "后台服务", "在这里安装和授权。完成后即可使用 TUN，本地构建同样支持。")
	ui.Column(c).Gap(12).Padding(18).Background(c.Theme().Surface).Radius(9).FillWidth().Children(func() {
		ui.Text(c, "NativeJet Helper").Bold()
		status := "尚未安装，当前使用普通用户服务"
		if a.registration != nil {
			status = "已安装系统后台服务"
		}
		muted(c, status)
		if a.connected && a.snapshot.Status.Privileged {
			muted(c, "已连接 · 可以启动 TUN")
		}
		if a.registration != nil && !a.connected {
			muted(c, "后台服务尚未连接，可在此更新服务或重新连接。")
		} else {
			muted(c, fmt.Sprintf("现有 %d 份配置将自动保留，原文、覆盖和历史一起迁移。", len(a.snapshot.State.Profiles)))
		}
		muted(c, "安装时会出现 macOS 管理员密码弹窗；取消后可继续使用现有配置。")
		if a.snapshot.State.SelectedKernel != "" {
			muted(c, "所选内核："+a.snapshot.Status.KernelVersion)
		}
		if a.registration != nil && !a.snapshot.Status.KernelAuthorized && a.snapshot.State.SelectedKernel != "" {
			ui.Text(c, "当前内核尚未获得管理员授权").TextColor(c.Theme().Warning)
		}
	})
	ui.Row(c).Gap(10).Children(func() {
		label := "安装并授权后台服务"
		if a.registration != nil {
			label = "更新并授权后台服务"
			if a.connected && !a.snapshot.Status.KernelAuthorized && a.snapshot.State.SelectedKernel != "" {
				label = "授权所选内核"
			}
		}
		if ui.PrimaryButton(c, label).Disabled(a.busy).Clicked() {
			a.installService(false, a.snapshot.State.SelectedKernel, nil)
		}
		if ui.Button(c, "刷新并重新连接").Disabled(a.busy).Clicked() {
			a.refreshService()
			if a.registration == nil {
				a.connect(mygoContext())
			}
		}
	})
	if a.helperError != "" {
		ui.Text(c, a.helperError).TextColor(c.Theme().Danger)
	}
	ui.Box(c).Grow(1)
	muted(c, "关闭窗口后，后台服务仍可管理内核。选择尚未授权的内核时，可在此授权。")
	muted(c, "卸载服务会保留系统中的配置和历史，并将配置复制回普通用户服务。")
	if ui.Button(c, "卸载后台服务，保留配置").Disabled(a.busy || a.registration == nil || !a.connected || a.snapshot.Status.Running).Clicked() {
		a.installService(true, "", nil)
	}
}
func bytesString(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
func (a *App) exportProfile() {
	p := a.current()
	if p == nil {
		return
	}
	text := a.source.Buffer.String()
	name := p.Name + ".yaml"
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "导出配置", DefaultPath: name})
		if err == nil && path != "" {
			err = writePrivate(path, []byte(text))
		}
		if err != nil {
			a.win.Update(func() { a.problem = err.Error() })
		}
	}()
}
func (a *App) deleteProfile(p model.Profile) {
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageWarning, Title: "删除配置", Message: "删除“" + p.Name + "”？", Detail: "已应用版本的历史记录会保留。", Buttons: []string{"取消", "删除"}, CancelButton: 0, DestructiveButtons: []int{1}})
		if err == nil && result.Button == 1 {
			a.win.Update(func() {
				a.request("profile.delete", map[string]string{"id": p.ID}, "配置已删除", func(_ json.RawMessage) { a.selected = ""; a.loaded = ""; a.accept(a.snapshot, true) })
			})
		}
	}()
}
func (a *App) history() {
	var window *mygo.Window
	window = mygo.NewWindow(mygo.WindowOptions{Title: "配置历史", Width: 720, Height: 520, Content: ui.View(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(22).Gap(16).Children(func() {
			heading(c, "配置历史", "恢复会应用当时的原文与覆盖。")
			ui.Scroll(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					for i := len(a.snapshot.State.Revisions) - 1; i >= 0; i-- {
						r := a.snapshot.State.Revisions[i]
						ui.Row(c.Key(r.ID)).FillWidth().Gap(12).PaddingY(8).Children(func() {
							ui.Column(c).Grow(1).Gap(3).Children(func() {
								ui.Text(c, short(r.ID)).Bold()
								muted(c, r.CreatedAt.Local().Format(time.DateTime))
								muted(c, "SHA-256 "+short(r.Hash))
							})
							if r.ID == a.snapshot.State.ActiveRevision {
								badge(c, "当前版本", true)
							} else if ui.Button(c, "恢复并应用").Disabled(a.busy).Clicked() {
								a.request("config.restore", map[string]string{"id": r.ID}, "历史版本已恢复", nil)
								window.Close()
							}
						})
					}
					if len(a.snapshot.State.Revisions) == 0 {
						muted(c, "尚无已应用版本。")
					}
				})
			})
		})
	})})
}
func (a *App) showDiff() {
	a.request("revision.yaml", map[string]string{"id": a.snapshot.State.ActiveRevision}, "已读取运行版本", func(raw json.RawMessage) {
		var before string
		_ = json.Unmarshal(raw, &before)
		old := editor.NewBuffer(before)
		old.SetLanguage(editor.LanguageFor("config.yaml"))
		next := editor.NewBuffer(a.preview.YAML)
		next.SetLanguage(editor.LanguageFor("config.yaml"))
		diff := &editor.DiffState{Old: old, New: next, Wrap: true}
		mygo.NewWindow(mygo.WindowOptions{Title: "已应用版本 → 当前预览", Width: 1000, Height: 700, Content: ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
				ui.Text(c, "已应用版本 → 当前预览（敏感值已隐藏）").Bold()
				editor.DiffView(c, diff).Grow(1).FillWidth()
			})
		})})
	})
}
