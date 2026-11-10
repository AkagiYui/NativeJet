package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"nativejet/internal/model"
)

const configTemplate = "# 本地配置：不包含任何代理节点\nmixed-port: 7890\nmode: rule\nproxies: []\nproxy-groups: []\ndns:\n  enable: true\n  enhanced-mode: fake-ip\n  nameserver:\n    - https://1.1.1.1/dns-query\nrules:\n  - MATCH,DIRECT\n"

type importForm struct {
	app                                *App
	open                               bool
	tab                                int
	name, url, path, interval, problem string
	editor                             *editor.State
	fromLink                           bool
	busy                               bool
}

func (a *App) addConfig() {
	if a.importing != nil {
		return
	}
	if a.dirty() {
		a.problem = "请先保存或撤销当前编辑，再添加配置"
		return
	}
	a.openImportForm(nil)
}

func (a *App) openImportForm(link *subscriptionLink) {
	f := &importForm{app: a, open: true, name: "新的配置", interval: "24", editor: newEditor("配置内容", false)}
	if link != nil {
		f.fromLink = true
		f.name, f.url = link.name, link.address
	}
	a.importing = f
	a.win.Invalidate()
}
func (f *importForm) close() {
	f.open = false
}
func (f *importForm) content(c *ui.Context) {
	ui.Column(c).Fill().Padding(22).Gap(16).Children(func() {
		if f.fromLink {
			heading(c, subscriptionIcon, "导入订阅", "请核对 Clash 链接提供的订阅地址。确认后下载并添加，应用配置需另行操作。")
		} else {
			heading(c, plusIcon, "添加配置", "订阅原文只读；文件和粘贴内容保存为可编辑的本地配置。")
			iconSegments(c, &f.tab, "配置导入方式", iconChoice{"订阅地址", subscriptionIcon}, iconChoice{"添加文件", folderIcon}, iconChoice{"粘贴内容", pasteIcon})
		}
		ui.Column(c).FillWidth().Gap(5).Children(func() { muted(c, "配置名称"); ui.TextInput(c, &f.name).FillWidth().Label("配置名称") })
		switch f.tab {
		case 0:
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				muted(c, "订阅地址")
				ui.TextInput(c, &f.url).Placeholder("https://…/subscription").FillWidth().Label("订阅地址")
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "更新间隔（小时）")
				ui.TextInput(c, &f.interval).Width(90).Label("订阅更新间隔")
				muted(c, "0 为仅手动更新")
			})
			muted(c, "支持 Clash/Mihomo YAML、明文节点列表和 Base64 节点列表。用覆盖配置补充策略组和规则。")
			muted(c, "订阅更新后需手动应用。")
			ui.Box(c).Grow(1)
		case 1:
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				if action(c, folderIcon, "选择 YAML 文件", false).Disabled(f.busy).Clicked() {
					f.pick()
				}
				muted(c, f.path)
			})
			muted(c, "导入应用内副本，保留原文件；之后可直接编辑和保存。")
			f.editor.ReadOnly = true
			editor.View(c, f.editor).Grow(1).FillWidth()
		case 2:
			ui.Row(c).Gap(10).Children(func() {
				if action(c, pasteIcon, "从剪贴板粘贴", false).Disabled(f.busy).Clicked() {
					f.editor.Buffer.Reset(mygo.Clipboard.ReadText())
				}
				if action(c, configIcon, "填入空白配置模板", false).Disabled(f.busy).Clicked() {
					f.editor.Buffer.Reset(configTemplate)
				}
			})
			f.editor.ReadOnly = f.busy
			editor.View(c, f.editor).Grow(1).FillWidth()
		}
		if f.problem != "" {
			ui.Text(c, f.problem).TextColor(c.Theme().Danger)
		}
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			ui.Box(c).Grow(1)
			if action(c, closeIcon, "取消", false).Disabled(f.busy).Clicked() {
				f.close()
			}
			label, icon := "添加本地配置", plusIcon
			if f.tab == 0 {
				label, icon = "下载并添加订阅", downloadIcon
			}
			if f.busy {
				label, icon = "正在处理…", scanIcon
			}
			if action(c, icon, label, true).Disabled(f.busy).Clicked() {
				f.submit()
			}
		})
	})
}
func (f *importForm) pick() {
	go func() {
		// YAML is validated by the helper. Accept extensionless and .conf files as
		// well; macOS UTType associations for YAML vary between installed apps.
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: f.app.win, Title: "添加配置文件", Message: "选择包含 YAML 配置的文件；导入时会校验内容。"})
		if err != nil {
			f.app.win.Update(func() { f.problem = err.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		file, err := os.Open(paths[0])
		if err != nil {
			f.app.win.Update(func() { f.problem = err.Error() })
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		file.Close()
		if err == nil && len(data) > 4<<20 {
			err = errors.New("配置文件不能超过 4 MB")
		}
		f.app.win.Update(func() {
			if err != nil {
				f.problem = err.Error()
				return
			}
			f.path = paths[0]
			f.name = strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
			f.editor.Buffer.Reset(string(data))
			f.problem = ""
		})
	}()
}
func (f *importForm) submit() {
	f.problem = ""
	if f.app.busy || f.app.dirty() {
		f.problem = "请先完成主窗口中的操作并保存编辑"
		return
	}
	if !f.app.connected {
		f.problem = "后台服务尚未连接，请稍后重试"
		return
	}
	if strings.TrimSpace(f.name) == "" {
		f.problem = "请输入配置名称"
		return
	}
	method := "profile.save"
	params := map[string]any{"profile": model.Profile{Name: f.name, Kind: "local", Content: f.editor.Buffer.String()}}
	if f.tab == 0 {
		if f.fromLink {
			if _, err := parseWebSubscription(f.url); err != nil {
				f.problem = err.Error()
				return
			}
		}
		interval, err := strconv.Atoi(f.interval)
		if err != nil || interval < 0 {
			f.problem = "请输入有效的小时数，0 为手动更新"
			return
		}
		method = "profile.subscribe"
		params = map[string]any{"name": f.name, "url": f.url, "interval": interval}
	} else if strings.TrimSpace(f.editor.Buffer.String()) == "" {
		f.problem = "请选择文件或粘贴配置内容"
		return
	}
	f.busy = true
	f.app.busy = true
	f.app.updateTray()
	f.app.win.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var p model.Profile
		err := f.app.client.Call(ctx, method, params, &p)
		var snap model.Snapshot
		if err == nil {
			err = f.app.client.Call(ctx, "snapshot", nil, &snap)
		}
		f.app.win.Update(func() {
			f.busy = false
			f.app.busy = false
			defer f.app.updateTray()
			f.app.win.Invalidate()
			if err != nil {
				f.problem = err.Error()
				return
			}
			f.app.selected = p.ID
			f.app.page = "配置"
			f.app.tab = 0
			f.app.accept(snap, true)
			f.app.message = "配置已添加"
			f.app.problem = ""
			f.app.win.Invalidate()
			f.close()
		})
	}()
}

func (a *App) importDialog(c *ui.Context) {
	f := a.importing
	if f == nil {
		return
	}
	modalLayer(c, "import", &f.open, palette(c).Scrim, func(_, panel ui.Element) {
		winW, winH := c.Size()
		panel.Width(min(float32(720), winW-48)).Height(min(float32(650), winH-48))
		overlayPanel(c, panel, overlayDialog)
		panel.Children(func() { f.content(c) })
	})
	// A submission is in flight: the dialog stays until it settles, as the
	// old window's OnClose guard did.
	if f.busy && !f.open {
		f.open = true
	}
	if !f.open {
		a.importing = nil
		a.showPendingImport()
	}
}

type downloadDialogState struct {
	open     bool
	releases []model.Release
	selected string
	problem  string
	loading  bool
}

func (a *App) downloadDialog() {
	a.download = downloadDialogState{open: true, loading: true}
	a.win.Invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var items []model.Release
		err := a.client.Call(ctx, "kernel.releases", nil, &items)
		a.win.Update(func() {
			d := &a.download
			d.loading = false
			if err != nil {
				d.problem = err.Error()
			} else {
				d.releases = items
				if len(items) > 0 {
					d.selected = items[0].Version + " · " + items[0].Name
				}
			}
		})
	}()
}

func (a *App) downloadModal(c *ui.Context) {
	d := &a.download
	if !d.open {
		return
	}
	modalLayer(c, "download", &d.open, palette(c).Scrim, func(_, panel ui.Element) {
		winW, winH := c.Size()
		panel.Width(min(float32(700), winW-48)).Height(min(float32(440), winH-48))
		overlayPanel(c, panel, overlayDialog)
		panel.Children(func() {
			ui.Column(c).Fill().Padding(22).Gap(16).Children(func() {
				heading(c, downloadIcon, "下载 Mihomo", "来源：MetaCubeX/mihomo · GitHub Releases")
				if d.loading {
					muted(c, "正在获取适合本机架构的稳定版本…")
				} else if len(d.releases) == 0 {
					muted(c, "没有可用版本，可关闭后重试或手动放入内核目录。")
				} else {
					names := []string{}
					lookup := map[string]model.Release{}
					for _, r := range d.releases {
						text := r.Version + " · " + r.Name
						names = append(names, text)
						lookup[text] = r
					}
					ui.Select(c, &d.selected, names).FillWidth().Label("内核版本")
					r := lookup[d.selected]
					muted(c, "大小 "+bytesString(r.Size))
					if r.Digest != "" {
						muted(c, "官方校验值 "+r.Digest)
					} else {
						muted(c, "此资源未提供 API 校验值；下载后记录文件 SHA-256。")
					}
				}
				if d.problem != "" {
					ui.Text(c, d.problem).TextColor(c.Theme().Danger)
				}
				ui.Box(c).Grow(1)
				ui.Row(c).FillWidth().Gap(10).Children(func() {
					if action(c, folderIcon, "打开内核目录", false).Clicked() {
						a.openDirectory()
					}
					ui.Box(c).Grow(1)
					if action(c, closeIcon, "取消", false).Clicked() {
						d.open = false
					}
					if action(c, downloadIcon, "下载并安装到目录", true).Disabled(d.loading || len(d.releases) == 0 || a.busy).Clicked() {
						var id int64
						for _, r := range d.releases {
							if d.selected == r.Version+" · "+r.Name {
								id = r.AssetID
							}
						}
						a.request("kernel.download", map[string]int64{"assetId": id}, "内核已下载；扫描后可选用", func(_ json.RawMessage) { a.request("kernel.scan", nil, "内核目录扫描完成", nil) })
						d.open = false
					}
				})
			})
		})
	})
}
