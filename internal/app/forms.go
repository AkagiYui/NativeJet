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
	win                                *mygo.Window
	tab                                int
	name, url, path, interval, problem string
	editor                             *editor.State
	busy                               bool
}

func (a *App) addConfig() {
	if a.dirty() {
		a.problem = "请先保存或撤销当前编辑，再添加配置"
		return
	}
	f := &importForm{app: a, name: "新的配置", interval: "24", editor: newEditor("配置内容", false)}
	f.win = mygo.NewWindow(mygo.WindowOptions{Title: "添加配置", Width: 720, Height: 650, MinWidth: 560, MinHeight: 520, Content: ui.View(f.view)})
}
func (f *importForm) view(c *ui.Context) {
	useWorkspaceTheme(c)
	ui.Column(c).Fill().Padding(22).Gap(16).Children(func() {
		heading(c, "添加配置", "订阅原文只读；文件和粘贴内容保存为可编辑的本地配置。")
		ui.Segmented(c, &f.tab, "订阅地址", "添加文件", "粘贴内容")
		ui.Column(c).FillWidth().Gap(5).Children(func() { muted(c, "配置名称"); ui.TextInput(c, &f.name).FillWidth().Label("配置名称") })
		switch f.tab {
		case 0:
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				muted(c, "订阅地址")
				ui.TextInput(c, &f.url).Placeholder("https://…/profile.yaml").FillWidth().Label("订阅地址")
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "更新间隔（小时）")
				ui.TextInput(c, &f.interval).Width(90).Label("订阅更新间隔")
				muted(c, "0 为仅手动更新")
			})
			muted(c, "订阅更新不会自动应用。修改网络参数时请使用覆盖配置。")
			ui.Box(c).Grow(1)
		case 1:
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "选择 YAML 文件").Disabled(f.busy).Clicked() {
					f.pick()
				}
				muted(c, f.path)
			})
			muted(c, "导入应用内副本，保留原文件；之后可直接编辑和保存。")
			f.editor.ReadOnly = true
			editor.View(c, f.editor).Grow(1).FillWidth()
		case 2:
			ui.Row(c).Gap(10).Children(func() {
				if ui.Button(c, "从剪贴板粘贴").Disabled(f.busy).Clicked() {
					f.editor.Buffer.Reset(mygo.Clipboard.ReadText())
				}
				if ui.Button(c, "填入空白配置模板").Disabled(f.busy).Clicked() {
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
			if ui.Button(c, "取消").Disabled(f.busy).Clicked() {
				f.win.Close()
			}
			label := "添加本地配置"
			if f.tab == 0 {
				label = "下载并添加订阅"
			}
			if f.busy {
				label = "正在处理…"
			}
			if ui.PrimaryButton(c, label).Disabled(f.busy).Clicked() {
				f.submit()
			}
		})
	})
}
func (f *importForm) pick() {
	go func() {
		// YAML is validated by the helper. Accept extensionless and .conf files as
		// well; macOS UTType associations for YAML vary between installed apps.
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: f.win, Title: "添加配置文件", Message: "选择包含 YAML 配置的文件；导入时会校验内容。"})
		if err != nil {
			f.win.Update(func() { f.problem = err.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		file, err := os.Open(paths[0])
		if err != nil {
			f.win.Update(func() { f.problem = err.Error() })
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		file.Close()
		if err == nil && len(data) > 4<<20 {
			err = errors.New("配置文件不能超过 4 MB")
		}
		f.win.Update(func() {
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
	if strings.TrimSpace(f.name) == "" {
		f.problem = "请输入配置名称"
		return
	}
	method := "profile.save"
	params := map[string]any{"profile": model.Profile{Name: f.name, Kind: "local", Content: f.editor.Buffer.String()}}
	if f.tab == 0 {
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
		f.win.Update(func() {
			f.busy = false
			f.app.busy = false
			f.app.win.Invalidate()
			if err != nil {
				f.problem = err.Error()
				return
			}
			f.app.selected = p.ID
			f.app.tab = 0
			f.app.accept(snap, true)
			f.app.message = "配置已添加"
			f.app.problem = ""
			f.app.win.Invalidate()
			f.win.Close()
		})
	}()
}

func (a *App) downloadDialog() {
	var releases []model.Release
	var selected string
	var problem string
	loading := true
	var window *mygo.Window
	window = mygo.NewWindow(mygo.WindowOptions{Title: "下载内核", Width: 700, Height: 440, Content: ui.View(func(c *ui.Context) {
		useWorkspaceTheme(c)
		ui.Column(c).Fill().Padding(22).Gap(16).Children(func() {
			heading(c, "下载 Mihomo", "来源：MetaCubeX/mihomo · GitHub Releases")
			if loading {
				muted(c, "正在获取适合本机架构的稳定版本…")
			} else if len(releases) == 0 {
				muted(c, "没有可用版本，可关闭后重试或手动放入内核目录。")
			} else {
				names := []string{}
				lookup := map[string]model.Release{}
				for _, r := range releases {
					text := r.Version + " · " + r.Name
					names = append(names, text)
					lookup[text] = r
				}
				ui.Select(c, &selected, names).FillWidth().Label("内核版本")
				r := lookup[selected]
				muted(c, "大小 "+bytesString(r.Size))
				if r.Digest != "" {
					muted(c, "官方校验值 "+r.Digest)
				} else {
					muted(c, "此资源未提供 API 校验值；下载后记录文件 SHA-256。")
				}
			}
			if problem != "" {
				ui.Text(c, problem).TextColor(c.Theme().Danger)
			}
			ui.Box(c).Grow(1)
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				if ui.Button(c, "打开内核目录").Clicked() {
					a.openDirectory()
				}
				ui.Box(c).Grow(1)
				if ui.Button(c, "取消").Clicked() {
					window.Close()
				}
				if ui.PrimaryButton(c, "下载并安装到目录").Disabled(loading || len(releases) == 0 || a.busy).Clicked() {
					var id int64
					for _, r := range releases {
						if selected == r.Version+" · "+r.Name {
							id = r.AssetID
						}
					}
					a.request("kernel.download", map[string]int64{"assetId": id}, "内核已下载；扫描后可选用", func(_ json.RawMessage) { a.request("kernel.scan", nil, "内核目录扫描完成", nil) })
					window.Close()
				}
			})
		})
	})})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var items []model.Release
		err := a.client.Call(ctx, "kernel.releases", nil, &items)
		window.Update(func() {
			loading = false
			if err != nil {
				problem = err.Error()
			} else {
				releases = items
				if len(items) > 0 {
					selected = items[0].Version + " · " + items[0].Name
				}
			}
		})
	}()
}
