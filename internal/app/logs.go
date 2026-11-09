package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

func logSourceLabel(source string) string {
	switch source {
	case "core":
		return "内核日志"
	case "process":
		return "启动输出"
	case "helper":
		return "后台服务"
	default:
		return source
	}
}
func (a *App) appendLogs(logs []model.Log) {
	for _, entry := range logs {
		if entry.Sequence > a.logAfter {
			a.logs = append(a.logs, entry)
			a.logAfter = entry.Sequence
		}
	}
	if excess := len(a.logs) - model.LogCacheLimit; excess > 0 {
		clear(a.logs[:excess])
		a.logs = a.logs[excess:]
	}
}
func (a *App) filteredLogs() []model.Log {
	rows := []model.Log{}
	query := strings.ToLower(a.logSearch)
	for _, entry := range a.logs {
		if a.logLevel != "全部" && entry.Level != a.logLevel {
			continue
		}
		if a.logSource != "全部来源" && logSourceLabel(entry.Source) != a.logSource {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Message), query) {
			continue
		}
		rows = append(rows, entry)
	}
	slices.Reverse(rows)
	return rows
}

func (a *App) showLogs() {
	a.page = "日志"
	a.paused = false
	a.logSearch = ""
	a.logLevel = "全部"
	a.logSource = "全部来源"
	a.logSelected = 0
	a.logSelection = -1
}

func (a *App) openRuleLogs(rule model.RuleMetric) {
	a.showLogs()
	a.logSource = logSourceLabel("core")
	a.logLevel = "info"
	query := "match " + rule.Type
	if rule.Payload != "" {
		query += "(" + rule.Payload + ")"
	}
	query += " using"
	// A sub-rule's policy is selected at runtime.
	if rule.Proxy != "" && rule.Type != "SubRules" {
		query += " " + rule.Proxy
	}
	a.logSearch = query
	if rows := a.filteredLogs(); len(rows) > 0 {
		a.logSelected = rows[0].Sequence
	}
	a.logSort = ui.SortOrder{}
	a.logRows.ScrollTo(0, ui.Start)
}

func (a *App) openLogs() {
	a.showLogs()
	for i := len(a.logs) - 1; i >= 0; i-- {
		l := a.logs[i]
		if l.Level == "error" && (l.Source == "process" || l.Source == "core") {
			a.logSelected = l.Sequence
			break
		}
	}
}
