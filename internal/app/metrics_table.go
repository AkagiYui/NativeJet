package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

type metricTableState struct {
	rows  ui.ListState
	order ui.SortOrder
}

func metricTable[T any](c *ui.Context, state *metricTableState, columns []ui.TableColumn, label string,
	rows []T, key func(T, int) tableSortKey, cell func(T, int)) {
	state.rows.Sort = &state.order
	rows = sortableTableRows(rows, columns, state.order, key)
	workspaceTable(c, &state.rows, columns, len(rows), label, func(row, col int) { cell(rows[row], col) })
}

func nodeSortKey(node model.NodeMetric, col int) tableSortKey {
	switch col {
	case 0:
		return tableSortKey{text: node.Name}
	case 1:
		return tableSortKey{text: nodeSource(node)}
	case 2:
		return tableSortKey{text: node.Type}
	case 3:
		return tableSortKey{text: nodeHealth(node)}
	case 4:
		delay := latestProbe(node).Delay
		return tableSortKey{number: uint64(delay), missing: delay == 0}
	default:
		return metricDateKey(latestProbe(node).Time)
	}
}

func ruleSortKey(rule model.RuleMetric, col int) tableSortKey {
	switch col {
	case 0:
		return tableSortKey{number: uint64(rule.Index)}
	case 1:
		return tableSortKey{text: rule.Proxy}
	}
	if rule.Extra == nil {
		return tableSortKey{missing: true}
	}
	switch col {
	case 2:
		return tableSortKey{number: rule.Extra.HitCount}
	case 3:
		return tableSortKey{number: rule.Extra.MissCount}
	case 4:
		return metricDateKey(rule.Extra.HitAt)
	default:
		return metricDateKey(rule.Extra.MissAt)
	}
}

func providerSortKey(provider model.ProviderMetric, col int) tableSortKey {
	switch col {
	case 0:
		return tableSortKey{text: provider.Name}
	case 1:
		return tableSortKey{text: providerKind(provider)}
	case 2:
		return tableSortKey{number: uint64(provider.Count)}
	case 3:
		return metricDateKey(provider.UpdatedAt)
	case 4:
		used, ok := subscriptionUsed(provider.Subscription)
		return tableSortKey{number: uint64(used), missing: !ok}
	default:
		if info := provider.Subscription; info != nil && info.Expire > 0 {
			return metricDateKey(time.Unix(info.Expire, 0))
		}
		return tableSortKey{missing: true}
	}
}

func latestProbe(node model.NodeMetric) model.DelaySample {
	if n := len(node.History); n != 0 {
		return node.History[n-1]
	}
	return model.DelaySample{}
}

func nodeSource(node model.NodeMetric) string {
	if node.Provider == "" {
		return "当前配置"
	}
	return node.Provider
}

func nodeHealth(node model.NodeMetric) string {
	if len(node.History) == 0 {
		return "未检测"
	}
	if latestProbe(node).Delay == 0 || node.Alive != nil && !*node.Alive {
		return "不可用"
	}
	return "可用"
}

func providerKind(provider model.ProviderMetric) string {
	if provider.IsRuleSet {
		return "规则集"
	}
	return "代理集"
}

func subscriptionUsed(info *model.SubscriptionQuota) (int64, bool) {
	if info == nil || info.Total <= 0 || info.Upload < 0 || info.Download < 0 || info.Upload > math.MaxInt64-info.Download {
		return 0, false
	}
	return info.Upload + info.Download, true
}

func metricDateKey(t time.Time) tableSortKey {
	return tableSortKey{at: t, missing: !hasMetricTime(t)}
}

func hasMetricTime(t time.Time) bool {
	return !t.IsZero() && !t.Equal(time.Unix(0, 0))
}
