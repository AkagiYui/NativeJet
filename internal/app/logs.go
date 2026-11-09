package app

import (
	"slices"
	"strings"

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
func (a *App) openLogs() {
	a.page = "日志"
	a.paused = false
	a.logSearch = ""
	a.logLevel = "全部"
	a.logSource = "全部来源"
	a.logSelected = 0
	a.logSelection = -1
	for i := len(a.logs) - 1; i >= 0; i-- {
		l := a.logs[i]
		if l.Level == "error" && (l.Source == "process" || l.Source == "core") {
			a.logSelected = l.Sequence
			break
		}
	}
}
