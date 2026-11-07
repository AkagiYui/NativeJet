package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"nativejet/internal/logging"
	"nativejet/internal/model"
)

func (a *App) configView(c *ui.Context) {
	workspaceHeader(c, configIcon, "配置", func() {
		if action(c, plusIcon, "添加配置", false).Disabled(a.busy || !a.connected).Clicked() {
			a.addConfig()
		}
		if action(c, nil, "历史版本", false).Disabled(a.busy).Clicked() {
			a.history()
		}
		disabled := a.busy || !a.connected || a.current() == nil
		if action(c, nil, "应用配置", true).Disabled(disabled).Clicked() {
			a.apply()
		}
	})
	if len(a.snapshot.State.Profiles) > 0 {
		a.profileRow(c)
	}
	commandRow(c, func() {
		if ui.Tabs(c, &a.tab, "原始配置", "覆盖配置", "最终配置").Changed() && a.tab == 2 {
			a.loadPreview()
		}
		ui.Box(c).Grow(1)
		caption := "尚未应用配置"
		if id := a.snapshot.State.ActiveRevision; id != "" {
			caption = "已应用版本 " + short(id)
			for _, r := range a.snapshot.State.Revisions {
				if r.ID == id && r.GlobalOverrideName != "" {
					caption += " · " + r.GlobalOverrideName
					break
				}
			}
		}
		if a.dirty() {
			caption += " · 有未保存的修改"
		}
		ui.Text(c, caption).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine().Tooltip(caption)
	})
	switch a.tab {
	case 0:
		a.sourceView(c)
	case 1:
		a.overrideView(c)
	case 2:
		a.finalView(c)
	}
}

func (a *App) profileRow(c *ui.Context) {
	commandRow(c, func() {
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
		if ui.Select(c, &current, labels).Label("正在查看的配置").Grow(1).Disabled(a.busy).Changed() && !a.busy {
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
				stateLabel(c, "已应用", c.Theme().Success)
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
}

func (a *App) sourceView(c *ui.Context) {
	p := a.current()
	if p == nil {
		emptyState(c, configIcon, "添加第一份配置", "导入订阅地址、YAML 文件，或直接粘贴内容。", func() {
			if action(c, plusIcon, "添加配置", true).Disabled(a.busy || !a.connected).Clicked() {
				a.addConfig()
			}
		})
		return
	}
	a.source.ReadOnly = p.Kind == "subscription" || a.busy
	commandRow(c, func() {
		if p.Kind == "subscription" {
			muted(c, "订阅原文 · 只读 · "+p.UpdatedAt.Local().Format("2006-01-02 15:04")+" 更新")
		} else {
			muted(c, "本地配置 · 可编辑")
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
	workspaceEditor(c.Key("source-editor"), a.source).Grow(1)
}
func (a *App) finalView(c *ui.Context) {
	commandRow(c, func() {
		ui.Segmented(c, &a.format, "字段与来源", "YAML").Label("最终配置视图")
		muted(c, "已隐藏常见敏感值")
		ui.Box(c).Grow(1)
		if a.preview.Hash != "" {
			if action(c.Key("copy-hash"), nil, "复制 SHA-256", false).Tooltip(a.preview.Hash).Clicked() {
				mygo.Clipboard.WriteText(a.preview.Hash)
			}
		}
		if action(c, scanIcon, "刷新预览", false).Disabled(a.busy || !a.connected || a.current() == nil).Clicked() {
			a.loadPreview()
		}
		if action(c, nil, "与已应用版本比较", false).Disabled(a.busy || a.snapshot.State.ActiveRevision == "" || a.preview.YAML == "").Clicked() {
			a.showDiff()
		}
	})
	if a.preview.YAML == "" {
		emptyState(c, configIcon, "还没有最终配置预览", "点击“刷新预览”合并原文和覆盖配置。", nil)
		return
	}
	if a.format == 1 {
		workspaceEditor(c.Key("final-yaml"), a.final).Grow(1)
	} else {
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			columns := []ui.TableColumn{{Title: "字段", MinWidth: 115}, {Title: "提交值", MinWidth: 86}, {Title: "来源", MinWidth: 88}}
			workspaceTable(c, &a.fields, columns, len(a.preview.Fields), "最终配置字段", func(row, col int) {
				f := a.preview.Fields[row]
				text := f.Path
				switch col {
				case 1:
					text = value(f.Value)
				case 2:
					text = f.Source
				}
				ui.Text(c, text).FontSize(12).SingleLine().Tooltip(text)
			})
			inspector(c, "字段来源", nil, func() {
				detailScroll(c, func() {
					if a.field < 0 || a.field >= len(a.preview.Fields) {
						muted(c, "选择一个字段查看其来源。")
						return
					}
					f := a.preview.Fields[a.field]
					ui.Text(c, f.Path).FontSize(14).FontWeight(600).Selectable()
					for _, trace := range f.Trace {
						label := trace.Layer
						if trace.Line > 0 {
							label += fmt.Sprintf(" · 第 %d 行", trace.Line)
						}
						text := value(trace.Value)
						if trace.Deleted {
							text = "删除字段"
						}
						property(c, label, text)
					}
					ui.Divider(c)
					property(c, "最终提交值", value(f.Value))
				})
			})
		})
	}
}

var kernelVersionPattern = regexp.MustCompile(`(?i)^(?:mihomo(?: meta)?|clash\.meta)\s+((?:v?\d+\.\d+|alpha-)\S*)`)

func kernelVersionLabel(version string) string {
	if match := kernelVersionPattern.FindStringSubmatch(version); match != nil {
		return match[1]
	}
	return version
}

func kernelStateLabel(k model.Kernel, selected string) string {
	if k.Error != "" {
		return "无法使用"
	}
	if k.ID == selected {
		return "当前选用"
	}
	return "可用"
}

func (a *App) kernelView(c *ui.Context) {
	workspaceHeader(c, kernelIcon, "内核", func() {
		if action(c, folderIcon, "打开目录", false).Disabled(!a.connected).Clicked() {
			a.openDirectory()
		}
		if action(c, scanIcon, "重新扫描", false).Disabled(a.busy || !a.connected).Clicked() {
			a.request("kernel.scan", nil, "内核目录扫描完成", nil)
		}
		if action(c, downloadIcon, "下载内核", true).Disabled(a.busy || !a.connected).Clicked() {
			a.downloadDialog()
		}
	})
	status := a.snapshot.Status
	commandRow(c, func() {
		ui.Icon(c, folderIcon).Size(15, 15).TextColor(c.Theme().TextMuted)
		ui.Text(c, "内核目录").FontWeight(600)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			path := status.KernelDirectory
			if path == "" {
				path = "等待后台服务"
			}
			ui.Text(c, path).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine().Tooltip(path)
		})
		muted(c, fmt.Sprintf("%d 个内核", len(a.snapshot.Kernels)))
	})
	if len(a.snapshot.Kernels) == 0 {
		emptyState(c, kernelIcon, "还没有发现内核", "把 macOS 内核放入目录后重新扫描，或点击“下载内核”。", nil)
		return
	}

	ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		columns := []ui.TableColumn{
			{Title: "文件名"},
			{Title: "版本号", Width: 132},
			{Title: "架构", Width: 75},
			{Title: "状态", Width: 100},
		}
		selectedKernel := a.snapshot.State.SelectedKernel
		kernels := sortableTableRows(a.snapshot.Kernels, columns, a.kernelSort, func(k model.Kernel, column int) tableSortKey {
			return kernelSortKey(k, column, selectedKernel)
		})
		followTableSelection(&a.kernelRows, kernels, func(k model.Kernel) string {
			if k.Path != "" {
				return k.Path
			}
			return k.ID + "\x00" + k.Name
		})
		workspaceTable(c, &a.kernelRows, columns, len(kernels), "识别到的内核", func(row, col int) {
			k := kernels[row]
			text := k.Name
			switch col {
			case 1:
				text = kernelVersionLabel(k.Version)
				if text == "" {
					text = "—"
				}
				ui.Text(c, text).FontSize(12).SingleLine().Tooltip(k.Version)
				return
			case 2:
				text = k.Arch
			case 3:
				text = kernelStateLabel(k, selectedKernel)
			}
			ui.Text(c, text).FontSize(12).SingleLine().Tooltip(text)
		})
		selection := a.kernelSelection
		if selection < 0 || selection >= len(kernels) {
			selection = 0
		}
		k := kernels[selection]
		inspector(c, "内核详情", func() {
			disabled := a.busy || k.Error != "" || status.Running || k.ID == a.snapshot.State.SelectedKernel
			if action(c, nil, "使用所选内核", true).Disabled(disabled).Clicked() {
				a.request("kernel.select", map[string]string{"id": k.ID}, "内核已选用", nil)
			}
		}, func() {
			detailScroll(c, func() {
				if status.Running {
					muted(c, "切换版本前，请先停止内核。")
				}
				property(c, "版本", k.Version)
				property(c, "文件", k.Path)
				if k.Error != "" {
					ui.Text(c, k.Error).FontSize(12).TextColor(c.Theme().Danger)
				} else {
					property(c, "SHA-256", k.ID)
				}
				if status.KernelPath != "" {
					ui.Divider(c)
					property(c, "实际运行副本", status.KernelPath)
					if status.ConfigPath != "" {
						property(c, "配置文件", status.ConfigPath)
					}
				}
			})
		})
	})
}
func (a *App) connectionsView(c *ui.Context) {
	workspaceHeader(c, linkIcon, "连接", func() {
		if a.snapshot.Status.Running && a.connectionsError == nil {
			muted(c, fmt.Sprintf("%d 个活动连接", len(a.connections.Connections)))
		}
	})
	if !a.snapshot.Status.Running {
		emptyState(c, linkIcon, "内核未运行", "启动内核后，这里会显示实时连接。", nil)
		return
	}
	if problem := a.connectionsError; problem != nil {
		emptyState(c, linkIcon, "无法读取 Mihomo 控制接口", problem.Error(), nil)
		return
	}
	commandRow(c, func() {
		ui.TextInput(c, &a.search).Placeholder("搜索目标、进程或策略").Grow(1).MinWidth(0).Label("搜索连接")
		if action(c, nil, "关闭全部", false).Disabled(a.busy).Clicked() {
			a.request("connections.closeAll", nil, "连接已关闭", nil)
		}
	})
	rows := []model.Connection{}
	query := strings.ToLower(a.search)
	for _, conn := range a.connections.Connections {
		if strings.Contains(strings.ToLower(conn.Metadata.Host+" "+conn.Metadata.Process+" "+conn.Metadata.DestinationIP+" "+strings.Join(conn.Chains, " ")), query) {
			rows = append(rows, conn)
		}
	}
	if len(rows) == 0 {
		if query == "" {
			emptyState(c, linkIcon, "暂无活动连接", "", nil)
		} else {
			emptyState(c, linkIcon, "没有匹配的连接", "试试其他目标、进程或策略名称。", nil)
		}
		return
	}
	ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		columns := []ui.TableColumn{{Title: "目标", MinWidth: 105}, {Title: "进程", MinWidth: 68}, {Title: "策略", MinWidth: 80}, {Title: "下载", Width: 86, Align: ui.End}}
		rows := sortableTableRows(rows, columns, a.connectionSort, connectionSortKey)
		followTableSelection(&a.connectionRows, rows, func(conn model.Connection) string { return conn.ID })
		workspaceTable(c, &a.connectionRows, columns, len(rows), "活动连接列表", func(row, col int) {
			conn := rows[row]
			text := connectionHost(conn)
			switch col {
			case 1:
				text = connectionProcess(conn)
			case 2:
				text = strings.Join(conn.Chains, " → ")
			case 3:
				text = bytesString(conn.Download)
			}
			ui.Text(c, text).FontSize(12).SingleLine().Tooltip(text)
		})
		var selected *model.Connection
		if a.connectionIndex >= 0 && a.connectionIndex < len(rows) {
			selected = &rows[a.connectionIndex]
		}
		inspector(c, "连接详情", func() {
			if selected != nil && action(c, nil, "关闭此连接", false).Disabled(a.busy).Clicked() {
				a.request("connections.close", map[string]string{"id": selected.ID}, "连接已关闭", nil)
			}
		}, func() {
			if selected == nil {
				detailScroll(c, func() { muted(c, "选择一条连接查看规则和流量。") })
				return
			}
			conn := *selected
			detailScroll(c, func() {
				ui.Text(c, connectionHost(conn)).FontSize(14).FontWeight(600).Selectable()
				property(c, "目标地址", conn.Metadata.DestinationIP+":"+conn.Metadata.DestinationPort)
				property(c, "协议 / 类型", conn.Metadata.Network+" · "+conn.Metadata.Type)
				property(c, "匹配规则", strings.TrimSuffix(conn.Rule+" · "+conn.RulePayload, " · "))
				property(c, "转发链", strings.Join(conn.Chains, " → "))
				ui.Divider(c)
				property(c, "上传 / 下载", bytesString(conn.Upload)+" / "+bytesString(conn.Download))
			})
		})
	})
}

func connectionHost(conn model.Connection) string {
	if conn.Metadata.Host != "" {
		return conn.Metadata.Host
	}
	return conn.Metadata.DestinationIP
}

func connectionProcess(conn model.Connection) string {
	if conn.Metadata.Process != "" {
		return conn.Metadata.Process
	}
	if conn.Metadata.ProcessPath != "" {
		return filepath.Base(conn.Metadata.ProcessPath)
	}
	return ""
}

func (a *App) logsView(c *ui.Context) {
	workspaceHeader(c, logIcon, "日志", func() {
		label := "暂停显示"
		if a.paused {
			label = "继续显示"
			stateLabel(c.Key("paused"), "已暂停", c.Theme().Warning)
		}
		if action(c.Key("pause"), nil, label, false).Clicked() {
			a.paused = !a.paused
		}
		if action(c.Key("export"), downloadIcon, "导出日志", false).Clicked() {
			a.exportLogs()
		}
	})
	var rows []model.Log
	commandRow(c, func() {
		ui.TextInput(c, &a.logSearch).Placeholder("搜索日志内容").Grow(1).MinWidth(0).Label("搜索日志")
		ui.Select(c, &a.logSource, []string{"全部来源", "内核日志", "启动输出", "后台服务"}).Width(125).Label("日志来源")
		ui.Select(c, &a.logLevel, []string{"全部", "info", "warning", "error", "debug"}).Width(100).Label("日志级别")
		rows = a.filteredLogs()
		ui.Text(c, fmt.Sprintf("显示 %d 条 · 缓存 %d 条", len(rows), len(a.logs))).FontSize(11).
			TextColor(c.Theme().TextMuted).SingleLine().Shrink(0).
			Tooltip("最多保留 2000 条，暂停只停止拉取。")
	})
	if len(rows) == 0 {
		if len(a.logs) == 0 {
			emptyState(c, logIcon, "暂无日志", "内核日志和启动输出会显示在这里。", nil)
		} else {
			emptyState(c, logIcon, "没有匹配的日志", "调整搜索词、日志来源或级别。", nil)
		}
	} else {
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			columns := []ui.TableColumn{{Title: "时间", Width: 82}, {Title: "级别", Width: 75}, {Title: "来源", Width: 84}, {Title: "内容", MinWidth: 105}}
			rows := sortableTableRows(rows, columns, a.logSort, logSortKey)
			if a.logSelected != 0 {
				a.logSelection = slices.IndexFunc(rows, func(l model.Log) bool { return l.Sequence == a.logSelected })
			}
			workspaceTable(c, &a.logRows, columns, len(rows), "日志列表", func(row, col int) {
				l := rows[row]
				_, message := logging.Parse(l.Message)
				text := message
				switch col {
				case 0:
					text = l.Time.Local().Format("15:04:05")
				case 1:
					text = l.Level
				case 2:
					text = logSourceLabel(l.Source)
				}
				el := ui.Text(c, text).FontSize(12).SingleLine()
				if col == 3 {
					el.Tooltip(text)
				} else if col == 1 {
					switch l.Level {
					case "error":
						el.TextColor(c.Theme().Danger).FontWeight(600)
					case "warning":
						el.TextColor(c.Theme().Warning).FontWeight(600)
					}
				}
			})
			if a.logSelection < 0 || a.logSelection >= len(rows) {
				inspector(c, "完整日志", nil, func() {
					detailScroll(c, func() { muted(c, "选择一条日志查看完整内容。") })
				})
				return
			}
			l := rows[a.logSelection]
			a.logSelected = l.Sequence
			if a.logDetailSequence != l.Sequence {
				a.logDetail.Buffer.Reset(l.Message)
				a.logDetailSequence = l.Sequence
			}
			inspector(c, "完整日志", func() {
				if action(c, nil, "复制这条日志", false).Clicked() {
					mygo.Clipboard.WriteText(l.Time.Local().Format(time.RFC3339) + " [" + l.Level + "] [" + logSourceLabel(l.Source) + "]\n" + l.Message)
				}
			}, func() {
				ui.Column(c).FillWidth().Shrink(0).Padding(10, 12).Gap(5).
					BorderWidth(0, 0, 1, 0).BorderColor(palette(c).line).Children(func() {
					muted(c, l.Time.Local().Format(time.DateTime))
					muted(c, l.Level+" · "+logSourceLabel(l.Source))
				})
				ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(8, 12).Children(func() {
					workspaceEditor(c, a.logDetail).Grow(1)
				})
			})
		})
	}
}

const settingsPage = "设置"

func (a *App) settingsView(c *ui.Context) {
	workspaceHeader(c, settingsIcon, settingsPage, func() {
		if action(c, scanIcon, "刷新并重新连接", false).Disabled(a.busy).Clicked() {
			a.refreshService()
			if a.registration == nil {
				a.connect(mygoContext())
			}
		}
	})
	p := palette(c)
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Padding(16, 20).ScrollbarInsets(0, 4, 0, 0).Children(func() {
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			a.appearanceSettings(c)
			ui.Column(c).FillWidth().Padding(16).Gap(12).Radius(8).Border(1, p.line).Background(p.Surface).
				Role(ui.RoleGroup).Label("后台服务").Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, serviceIcon).Size(17, 17).TextColor(p.TextMuted)
					ui.Text(c, "NativeJet Helper").FontSize(15).FontWeight(600)
					ui.Box(c).Grow(1)
					switch {
					case a.connected && a.snapshot.Status.Privileged:
						stateLabel(c, "系统服务已连接", p.Success)
					case a.registration != nil:
						stateLabel(c, "已安装 · 未连接", p.Warning)
					case a.connected:
						stateLabel(c, "普通用户服务", p.Warning)
					default:
						stateLabel(c, "未安装系统服务", p.TextMuted)
					}
				})
				muted(c, "安装并授权系统服务后即可使用 TUN。安装需要 macOS 管理员密码。")
				if a.registration != nil && !a.connected {
					muted(c, "系统服务尚未连接，可更新服务或重新连接。")
				} else {
					muted(c, fmt.Sprintf("现有 %d 份配置将自动保留，原文、覆盖和历史一起迁移。", len(a.snapshot.State.Profiles)))
				}
				if a.snapshot.State.SelectedKernel != "" {
					property(c, "所选内核", a.snapshot.Status.KernelVersion)
					if a.registration != nil && !a.snapshot.Status.KernelAuthorized {
						stateLabel(c, "所选内核尚未获得管理员授权", p.Warning)
					}
				}
				muted(c, "关窗后，后台服务仍可管理内核。卸载会保留系统中的配置和历史，并复制回普通用户服务。")
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					label := "安装并授权后台服务"
					if a.registration != nil {
						label = "更新并授权后台服务"
						if a.connected && !a.snapshot.Status.KernelAuthorized && a.snapshot.State.SelectedKernel != "" {
							label = "授权所选内核"
						}
					}
					if action(c, nil, label, true).Disabled(a.busy).Clicked() {
						a.installService(false, a.snapshot.State.SelectedKernel, nil)
					}
					if action(c, nil, "卸载后台服务，保留配置", false).
						Disabled(a.busy || a.registration == nil || !a.connected || a.snapshot.Status.Running).Clicked() {
						a.installService(true, "", nil)
					}
				})
				if a.helperError != "" {
					ui.Text(c, a.helperError).TextColor(p.Danger).Selectable()
				}
			})
			ui.Row(c).FillWidth().Padding(16).Gap(16).AlignItems(ui.Center).Radius(8).Border(1, p.line).Background(p.Surface).
				Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
						ui.Text(c, "Clash 订阅链接").FontSize(15).FontWeight(600)
						muted(c, "将网页上的 Clash 一键导入链接交给 NativeJet 打开。")
					})
					if action(c, nil, "将 NativeJet 设为 Clash 链接默认应用", false).Shrink(0).Disabled(a.busy).Clicked() {
						if err := mygo.App.RegisterURLScheme("clash"); err != nil {
							a.problem = "无法设置 Clash 链接的默认应用，请从打包的 NativeJet.app 重试"
						} else {
							a.problem = ""
							a.message = "NativeJet 已设为 Clash 链接的默认应用"
						}
					}
				})
		})
	})
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
		useWorkspaceTheme(c)
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
								if r.GlobalOverrideName != "" {
									muted(c, "全局覆盖 · "+r.GlobalOverrideName)
								}
							})
							if r.ID == a.snapshot.State.ActiveRevision {
								stateLabel(c, "当前版本", c.Theme().Success)
							} else if ui.Button(c, "恢复并应用").Disabled(a.busy || a.dirty()).Clicked() {
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
			useWorkspaceTheme(c)
			ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
				ui.Text(c, "已应用版本 → 当前预览（敏感值已隐藏）").Bold()
				editor.DiffView(c, diff).Grow(1).FillWidth()
			})
		})})
	})
}
