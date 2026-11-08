package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

const metricsPage = "内核指标"

var metricSections = [...]string{model.MetricsOverview, model.MetricsNodes, model.MetricsRules, model.MetricsProviders}

type metricsResult struct {
	data model.Metrics
	err  error
}

func (a *App) metricsView(c *ui.Context) {
	workspaceHeader(c, metricsIcon, metricsPage, func() {
		ui.Tabs(c, &a.metricsTab, "概览", "节点", "规则", "集合").Shrink(0).Label("指标类别")
	})
	if !a.connected {
		emptyState(c, metricsIcon, "后台服务未连接", "连接后台服务后即可读取内核指标。", nil)
		return
	}
	if !a.coreRunning() {
		emptyState(c, metricsIcon, "内核未运行", "启动内核后即可读取指标。", nil)
		return
	}

	result := a.metrics[a.metricsTab]
	ready := result != nil
	var data model.Metrics
	if ready {
		data = result.data
		if result.err != nil {
			if a.metricsTab != 0 {
				emptyState(c, metricsIcon, "无法读取指标", result.err.Error(), nil)
				return
			}
			data.Problems = []string{"内核内存", "活动连接"}
		}
	}
	if a.metricsTab == 0 {
		a.metricsOverview(c, data)
		return
	}
	if !ready {
		emptyState(c, metricsIcon, "正在读取指标", "", nil)
		return
	}
	if len(data.Problems) != 0 {
		commandRow(c, func() { metricProblems(c, data.Problems) })
	}
	switch metricSections[a.metricsTab] {
	case model.MetricsNodes:
		metricsNodes(c.Key("nodes"), &a.nodeMetricsTable, data.Nodes, len(data.Problems) != 0)
	case model.MetricsRules:
		metricsRules(c.Key("rules"), &a.ruleMetricsTable, data.Rules, len(data.Problems) != 0)
	case model.MetricsProviders:
		metricsProviders(c.Key("providers"), &a.providerMetricsTable, data.Providers, len(data.Problems) != 0)
	}
}

func (a *App) metricsOverview(c *ui.Context, data model.Metrics) {
	p := palette(c)
	rates := a.trafficLabels()
	traffic := a.currentTraffic()
	upTotal, downTotal, memory, connections := "—", "—", "—", "—"
	if traffic.TotalsAvailable {
		upTotal, downTotal = trafficBytes(traffic.UpTotal), trafficBytes(traffic.DownTotal)
	}
	if data.Memory != nil {
		memory = trafficBytes(*data.Memory)
	}
	if data.ActiveConnections != nil {
		connections = strconv.Itoa(*data.ActiveConnections)
	}
	ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Padding(16).Children(func() {
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			ui.Grid(c).FillWidth().Columns(3).Gap(8).Children(func() {
				for _, card := range [...]struct {
					title, value string
					color        ui.Color
				}{
					{"上传速率", rates.up, p.Accent}, {"累计上传", upTotal, p.Text}, {"内核内存 (RSS)", memory, p.Text},
					{"下载速率", rates.down, p.Success}, {"累计下载", downTotal, p.Text}, {"活动连接", connections, p.Text},
				} {
					ui.Column(c).Padding(10, 12).Gap(4).MinWidth(0).Radius(controlRadius).Background(p.Surface).Border(1, p.line).Children(func() {
						muted(c, card.title)
						ui.Text(c, card.value).FontSize(17).FontWeight(600).TextColor(card.color).SingleLine().
							Label(card.title + " " + card.value)
					})
				}
			})
			if len(data.Problems) != 0 {
				metricProblems(c, data.Problems)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, "最近 5 分钟流量").FontWeight(600)
				stateLabel(c, "上传", p.Accent)
				stateLabel(c, "下载", p.Success)
				ui.Box(c).Grow(1)
				if action(c, nil, "查看连接", false).Clicked() {
					a.page = "连接"
				}
			})
			a.trafficChart(c.Key("metrics-traffic"), 190)
		})
	})
}

func metricProblems(c *ui.Context, problems []string) {
	stateLabel(c, strings.Join(problems, "、")+"暂不可用", c.Theme().Warning)
}

func metricsNodes(c *ui.Context, state *metricTableState, nodes []model.NodeMetric, unavailable bool) {
	if len(nodes) == 0 {
		metricEmpty(c, "暂无节点", unavailable)
		return
	}
	columns := []ui.TableColumn{
		{Title: "节点", MinWidth: 140}, {Title: "来源", MinWidth: 96},
		{Title: "协议", Width: 88}, {Title: "健康检查", Width: 85},
		{Title: "最近延迟", Width: 82, Align: ui.End}, {Title: "检查时间", Width: 124},
	}
	metricTable(c, state, columns, "节点指标", nodes, nodeSortKey, func(node model.NodeMetric, col int) {
		latest := latestProbe(node)
		switch col {
		case 0:
			metricText(c, node.Name)
		case 1:
			metricText(c, nodeSource(node))
		case 2:
			metricText(c, node.Type)
		case 3:
			t := c.Theme()
			status := nodeHealth(node)
			color := t.TextMuted
			switch status {
			case "不可用":
				color = t.Danger
			case "可用":
				color = t.Success
			}
			stateLabel(c, status, color)
		case 4:
			value := delayText(latest.Delay)
			tip := []string{}
			if node.TestURL != "" {
				tip = append(tip, node.TestURL)
			}
			for i := len(node.History) - 1; i >= 0; i-- {
				sample := node.History[i]
				delay := "失败"
				if sample.Delay != 0 {
					delay = delayText(sample.Delay)
				}
				tip = append(tip, sample.Time.Local().Format("15:04:05")+"  "+delay)
			}
			ui.Text(c, value).FontSize(12).SingleLine().Tooltip(strings.Join(tip, "\n"))
		case 5:
			metricText(c, metricTime(latest.Time))
		}
	})
}

func metricsRules(c *ui.Context, state *metricTableState, rules []model.RuleMetric, unavailable bool) {
	if len(rules) == 0 {
		metricEmpty(c, "暂无规则", unavailable)
		return
	}
	columns := []ui.TableColumn{
		{Title: "规则", MinWidth: 140}, {Title: "策略", MinWidth: 88},
		{Title: "命中", Width: 64, Align: ui.End}, {Title: "未命中", Width: 64, Align: ui.End},
		{Title: "最近命中", Width: 124}, {Title: "最近未命中", Width: 124},
	}
	metricTable(c, state, columns, "规则指标", rules, ruleSortKey, func(rule model.RuleMetric, col int) {
		name := fmt.Sprintf("%d · %s", rule.Index+1, rule.Type)
		if rule.Payload != "" {
			name += " · " + rule.Payload
		}
		if rule.Extra != nil && rule.Extra.Disabled {
			name = "已禁用 · " + name
		}
		values := [6]string{name, rule.Proxy, "—", "—", "—", "—"}
		if extra := rule.Extra; extra != nil {
			values[2], values[3] = strconv.FormatUint(extra.HitCount, 10), strconv.FormatUint(extra.MissCount, 10)
			values[4], values[5] = metricTime(extra.HitAt), metricTime(extra.MissAt)
		}
		metricText(c, values[col])
	})
}

func metricsProviders(c *ui.Context, state *metricTableState, providers []model.ProviderMetric, unavailable bool) {
	if len(providers) == 0 {
		metricEmpty(c, "暂无代理集或规则集", unavailable)
		return
	}
	columns := []ui.TableColumn{
		{Title: "名称", MinWidth: 140}, {Title: "类型", Width: 68},
		{Title: "条目", Width: 62, Align: ui.End}, {Title: "最近更新", Width: 124},
		{Title: "订阅已用 / 额度", MinWidth: 116}, {Title: "订阅到期", Width: 124},
	}
	metricTable(c, state, columns, "集合指标", providers, providerSortKey, func(provider model.ProviderMetric, col int) {
		quota, expiry := "—", "—"
		if used, ok := subscriptionUsed(provider.Subscription); ok {
			quota = trafficBytes(used) + " / " + trafficBytes(provider.Subscription.Total)
		}
		if info := provider.Subscription; info != nil {
			if info.Expire > 0 {
				expiry = metricTime(time.Unix(info.Expire, 0))
			}
		}
		values := [6]string{provider.Name, providerKind(provider), strconv.Itoa(provider.Count), metricTime(provider.UpdatedAt), quota, expiry}
		metricText(c, values[col])
	})
}

func metricEmpty(c *ui.Context, title string, unavailable bool) {
	if unavailable {
		title = "暂无可显示的指标"
	}
	emptyState(c, metricsIcon, title, "", nil)
}

func metricText(c *ui.Context, text string) {
	if text == "" {
		text = "—"
	}
	ui.Text(c, text).FontSize(12).SingleLine().Tooltip(text)
}

func metricTime(t time.Time) string {
	if !hasMetricTime(t) {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func delayText(ms uint16) string {
	if ms == 0 {
		return "—"
	}
	return strconv.Itoa(int(ms)) + " ms"
}
