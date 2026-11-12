package app

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

const proxiesPage = "策略组"

func (a *App) proxiesView(c *ui.Context) {
	a.workspaceHeader(c, proxiesIcon, proxiesPage, func() {
		if a.coreRunning() && a.groups != nil && a.proxiesError == nil {
			muted(c, fmt.Sprintf("%d 个策略组", len(a.groups)))
		}
	})
	switch {
	case !a.connected:
		emptyState(c, proxiesIcon, "后台服务未连接", "连接后台服务后即可读取代理节点和策略组。", nil)
		return
	case !a.snapshot.Status.Running:
		emptyState(c, proxiesIcon, "内核未运行", "启动内核后，这里会显示代理节点和策略组。", nil)
		return
	}

	width, _ := c.Size()
	contentWidth := width - sidebarWidth
	nodesWidth := contentWidth / 3
	ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c.Key("nodes")).Width(nodesWidth).Shrink(0).MinHeight(0).
			BorderWidth(0, 1, 0, 0).BorderColor(palette(c).line).Children(func() {
			a.proxyNodesView(c)
		})
		ui.Column(c.Key("groups")).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			a.proxyGroupsView(c, contentWidth-nodesWidth)
		})
	})
}

func (a *App) proxyNodesView(c *ui.Context) {
	result := a.metrics[metricsNodeTab]
	switch {
	case result == nil:
		emptyState(c, proxiesIcon, "正在读取代理节点", "", nil)
		return
	case result.err != nil:
		emptyState(c, proxiesIcon, "无法读取代理节点", result.err.Error(), nil)
		return
	}
	if len(result.data.Problems) > 0 {
		ui.Row(c).FillWidth().Padding(8, 16).Children(func() { metricProblems(c, result.data.Problems) })
	}
	nodes := make([]model.NodeMetric, 0, len(result.data.Nodes))
	for _, node := range result.data.Nodes {
		switch node.Type {
		case "Compatible", "Direct", "Dns", "Pass", "PassRule", "Reject", "RejectDrop", "Rematch",
			"Fallback", "LoadBalance", "Relay", "Selector", "URLTest":
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		emptyState(c, proxiesIcon, "暂无代理节点", "", nil)
		return
	}

	p := palette(c)
	ui.List(c, nil, len(nodes), func(i int) {
		node := nodes[i]
		detail := node.Type
		if node.Provider != "" {
			detail += " · " + node.Provider
		}
		ui.Row(c).FillWidth().Height(46).PaddingX(10).Gap(6).AlignItems(ui.Center).
			Radius(controlRadius).Background(p.Surface).Border(1, p.line).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, node.Name).SingleLine().Tooltip(node.Name)
				ui.Text(c, detail).FontSize(11).TextColor(p.TextMuted).SingleLine().Tooltip(detail)
			})
			if delay := latestProbe(node).Delay; delay != 0 {
				ui.Text(c, delayText(delay)).FontSize(11).TextColor(p.TextMuted).SingleLine().Shrink(0)
			}
		})
	}).Grow(1).FillWidth().MinHeight(0).Padding(16).Gap(8).ScrollbarInsets(0, 4, 0, 0).Label("代理节点")
}

func (a *App) proxyGroupsView(c *ui.Context, width float32) {
	switch {
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
		tileMinW = 230
	)
	columns := max(1, int((width-2*padding+gap)/(tileMinW+gap)))
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
	pinnable := group.CanPin()
	state := a.proxyGroupState(group.Name)
	results := state.results
	if state.resultsURL != group.DelayTestURL(state.url) {
		results = nil
	}
	maxDelay, limitErr := proxyDelayLimit(state.maxDelay)
	names := visibleProxyNames(group.All, results, state.filter, state.sort, maxDelay)
	ui.Column(c).FillWidth().Gap(gap).Children(func() {
		ui.Row(c.Key("heading")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				iconHeading(c, proxiesIcon, group.Name, 15, 2).Tooltip(group.Name)
				mode := "内核管理"
				if manual {
					mode = "手动选择"
				} else if group.Fixed != "" {
					mode = "已固定 · " + group.Fixed
				} else if pinnable {
					mode = "自动选择 · 点击节点固定"
				}
				muted(c, fmt.Sprintf("%s · %s · %d 个选项", group.Type, mode, len(group.All)))
			})
			if group.Now != "" {
				current := "当前 · " + group.Now
				ui.Text(c, current).FontSize(12).TextColor(p.TextMuted).MaxWidth(220).SingleLine().Tooltip(current)
			}
			if pinnable && group.Fixed != "" && action(c.Key("unpin"), unpinIcon, "解除固定", false).Disabled(a.busy).Clicked() {
				a.request("proxies.unpin", map[string]string{"id": group.Name}, "已恢复自动选择", nil)
			}
		})
		a.proxyTestControls(c, group, state, limitErr)
		if len(names) == 0 {
			muted(c.Key("no-matches"), "没有符合筛选条件的节点")
			return
		}
		grid := ui.Grid(c.Key("members")).Columns(columns).FillWidth().Gap(gap).Label(group.Name + "的选项")
		if manual {
			grid.FocusGroup(ui.Horizontal | ui.Vertical)
		}
		selected := group.Now
		grid.Children(func() {
			for _, name := range names {
				active := name == group.Now
				fill, border := p.Surface, p.line
				if active {
					fill, border = p.rowSelection, p.Accent
				}
				ui.Row(c.Key(name)).Height(46).Gap(3).PaddingX(5).AlignItems(ui.Center).
					Radius(controlRadius).Background(fill).Border(1, border).Children(func() {
					control := c.Key("select")
					var tile ui.Element
					switch {
					case manual:
						tile = ui.RadioBase(control, &selected, name).Label(name).Disabled(a.busy)
						if tile.Changed() && name != group.Now {
							a.request("proxies.select", map[string]string{"id": group.Name, "name": name}, "策略组已更新", nil)
						}
					case pinnable && name != group.Fixed:
						tile = ui.ButtonBase(control).Label("固定 " + name).Disabled(a.busy).Cursor(ui.CursorPointer)
						if tile.Clicked() {
							a.request("proxies.pin", map[string]string{"id": group.Name, "name": name}, "已更新固定节点", nil)
						}
					default:
						tile = ui.Row(control)
					}
					tile.Grow(1).MinWidth(0).FillHeight().Gap(4).PaddingX(5).AlignItems(ui.Center).
						Tooltip(proxyMemberTooltip(name, pinnable, name == group.Fixed)).Children(func() {
						if pinnable && !manual {
							ui.Icon(c, pinIcon).Size(12, 12).TextColor(p.TextMuted).Shrink(0)
						}
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							weight := 400
							if active {
								weight = 600
							}
							ui.Text(c, name).SingleLine().FontWeight(weight)
							delay := results[name]
							label := proxyDelayLabel(delay)
							if name == group.Fixed {
								label = "已固定 · " + label
							}
							ui.Text(c, label).FontSize(11).TextColor(p.TextMuted).SingleLine().Tooltip(label)
						})
						if active {
							ui.Icon(c, checkIcon).Size(14, 14).TextColor(p.Accent).Shrink(0)
						}
					})
					if action(c.Key("test"), metricsIcon, "测速", false).Label("测速 " + name).
						Shrink(0).Disabled(a.busy || a.proxyTest != nil).Clicked() {
						a.startProxyTest(group, state, &name)
					}
				})
			}
		})
	})
}

func proxyMemberTooltip(name string, pinnable, fixed bool) string {
	if fixed {
		return name + " · 已固定"
	}
	if pinnable {
		return "固定 " + name
	}
	return name
}

func (a *App) proxyTestControls(c *ui.Context, group model.Proxy, state *proxyGroupState, limitErr error) {
	run := a.proxyTest
	testingGroup := run != nil && run.group == group.Name
	defaultURL := group.DelayTestURL("")
	ui.Row(c.Key("url")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "测速 URL").FontSize(12).Shrink(0)
		ui.TextInput(c.Key("input"), &state.url).Grow(1).MinWidth(120).Height(controlHeight).
			Label(group.Name + "的测速 URL").Placeholder(defaultURL).Tooltip("留空使用 " + defaultURL).Disabled(testingGroup)
		if testingGroup {
			label := "取消测速"
			if run.name != "" {
				label = "取消单节点测速"
			}
			if action(c.Key("cancel"), closeIcon, label, false).Shrink(0).Disabled(run.cancel == nil).Clicked() {
				run.cancel()
				run.cancel = nil
			}
		} else if action(c.Key("test-all"), metricsIcon, "整组测速", false).Shrink(0).Disabled(a.busy || run != nil).Clicked() {
			a.startProxyTest(group, state, nil)
		}
	})
	ui.Row(c.Key("filter")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Select(c.Key("status"), &state.filter, []string{proxyFilterAll, proxyFilterSuccess, proxyFilterFailed, proxyFilterUntested}).Width(117).Label("按测速结果筛选")
		ui.TextInput(c.Key("max"), &state.maxDelay).Width(140).Height(controlHeight).Placeholder("最高延迟（ms）").Label("最高延迟（毫秒）")
		ui.Select(c.Key("sort"), &state.sort, []string{proxyOrderOriginal, proxyOrderFastest, proxyOrderSlowest}).Width(138).Label("节点排序")
	})
	if limitErr != nil {
		stateLabel(c.Key("invalid-delay"), limitErr.Error(), c.Theme().Warning)
	}
	if state.problem != "" {
		stateLabel(c.Key("problem"), state.problem, c.Theme().Warning)
	} else if testingGroup {
		label := "正在测量整组延迟"
		if run.name != "" {
			label = "正在测速 · " + run.name
		}
		stateLabel(c.Key("testing"), label, c.Theme().Accent)
	}
}

func proxyDelayLabel(result model.ProxyDelay) string {
	if result.Problem != "" {
		return result.Problem
	}
	if result.Delay == 0 {
		return "未测速"
	}
	return strconv.Itoa(int(result.Delay)) + " ms"
}
