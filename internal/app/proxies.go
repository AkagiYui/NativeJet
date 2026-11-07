package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

const proxiesPage = "策略组"

var checkIcon = outlineIcon(`<path d="m5 12 4 4L19 6"/>`)

func (a *App) proxiesView(c *ui.Context) {
	workspaceHeader(c, proxiesIcon, proxiesPage, func() {
		if a.connected && a.snapshot.Status.Running && a.groups != nil && a.proxiesError == nil {
			muted(c, fmt.Sprintf("%d 个策略组", len(a.groups)))
		}
	})
	switch {
	case !a.connected:
		emptyState(c, proxiesIcon, "后台服务未连接", "连接后台服务后即可读取内核策略组。", nil)
		return
	case !a.snapshot.Status.Running:
		emptyState(c, proxiesIcon, "内核未运行", "启动内核后，这里会显示策略组及其候选节点。", nil)
		return
	case a.proxiesError != nil:
		emptyState(c, proxiesIcon, "无法读取策略组", a.proxiesError.Error(), nil)
		return
	case a.groups == nil:
		emptyState(c, proxiesIcon, "正在读取策略组", "等待 Mihomo 控制接口返回。", nil)
		return
	case len(a.groups) == 0:
		emptyState(c, proxiesIcon, "暂无策略组", "当前运行的配置没有策略组。", nil)
		return
	}

	const (
		padding  = 16
		gap      = 8
		tileMinW = 184
	)
	w, _ := c.Size()
	columns := max(1, int((w-sidebarWidth-2*padding+gap)/(tileMinW+gap)))
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Padding(padding).
		ScrollbarInsets(0, 4, 0, 0).Children(func() {
		ui.Column(c).FillWidth().Gap(20).Children(func() {
			for _, group := range a.groups {
				a.proxyGroup(c.Key(group.Name), group, columns, gap)
			}
		})
	})
}

func (a *App) proxyGroup(c *ui.Context, group model.Proxy, columns int, gap float32) {
	p := palette(c)
	manual := group.Type == "Selector"
	ui.Column(c).FillWidth().Gap(gap).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, group.Name).FontSize(15).FontWeight(600).SingleLine().Tooltip(group.Name)
				mode := "内核管理"
				if manual {
					mode = "手动选择"
				}
				muted(c, fmt.Sprintf("%s · %s · %d 个选项", group.Type, mode, len(group.All)))
			})
			if group.Now != "" {
				current := "当前 · " + group.Now
				ui.Text(c, current).FontSize(12).TextColor(p.TextMuted).MaxWidth(260).SingleLine().Tooltip(current)
			}
		})
		grid := ui.Grid(c).Columns(columns).FillWidth().Gap(gap).Label(group.Name + "的选项")
		if manual {
			grid.Role(ui.RoleRadioGroup).FocusGroup(ui.Horizontal | ui.Vertical)
		}
		selected := group.Now
		grid.Children(func() {
			for i, name := range group.All {
				active := name == group.Now
				var tile ui.Element
				if manual {
					tile = ui.RadioBase(c.Key(i), &selected, name).Label(name).Disabled(a.busy)
					if tile.Changed() && name != group.Now {
						a.request("proxies.select", map[string]string{"id": group.Name, "name": name}, "策略组已更新", nil)
					}
				} else {
					tile = ui.Row(c.Key(i))
					if active {
						tile.Label(name + "，当前使用")
					}
				}
				fill, border, weight := p.Surface, p.line, 400
				if active {
					fill, border, weight = p.rowSelection, p.Accent, 600
				} else if manual && tile.Hovered() && !a.busy {
					fill = p.SurfaceHover
				}
				tile.Height(40).PaddingX(10).Gap(6).AlignItems(ui.Center).Radius(controlRadius).
					Background(fill).Border(1, border).Tooltip(name).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, name).SingleLine().FontWeight(weight)
					})
					if active {
						ui.Icon(c, checkIcon).Size(14, 14).TextColor(p.Accent).Shrink(0)
					}
				})
			}
		})
	})
}
