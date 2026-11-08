package app

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/storage"
)

const (
	connectionColumnMaxWidth = 4096
	connectionTableSaveDelay = 250 * time.Millisecond
)

var connectionColumns = [...]ui.TableColumn{
	connectionHostColumn:          {ID: "host", Title: "主机", Width: 164, MinWidth: 100},
	connectionProcessColumn:       {ID: "process", Title: "进程", Width: 88, MinWidth: 68},
	connectionChainColumn:         {ID: "chain", Title: "策略", Width: 108, MinWidth: 80},
	connectionDownloadColumn:      {ID: "download", Title: "累计下载", Width: 98, Align: ui.End},
	connectionUploadColumn:        {ID: "upload", Title: "累计上传", Width: 98, Align: ui.End},
	connectionDownloadSpeedColumn: {ID: "downloadSpeed", Title: "下载速度", Width: 106, Align: ui.End},
	connectionUploadSpeedColumn:   {ID: "uploadSpeed", Title: "上传速度", Width: 106, Align: ui.End},
	connectionRuleColumn:          {ID: "rule", Title: "规则", Width: 168, MinWidth: 90},
	connectionStartColumn:         {ID: "start", Title: "连接时间", Width: 164},
	connectionSourceColumn:        {ID: "source", Title: "源地址", Width: 156},
	connectionDestinationColumn:   {ID: "destination", Title: "目标地址", Width: 156},
	connectionTypeColumn:          {ID: "type", Title: "类型", Width: 114},
}

type connectionTablePreferences struct {
	Order          []string           `json:"order"`
	Widths         map[string]float32 `json:"widths,omitempty"`
	Hidden         map[string]bool    `json:"hidden,omitempty"`
	SortColumn     string             `json:"sortColumn,omitempty"`
	SortDescending bool               `json:"sortDescending,omitempty"`
}

func connectionColumnIndex(id string) int {
	return slices.IndexFunc(connectionColumns[:], func(column ui.TableColumn) bool { return column.ID == id })
}

func (p *connectionTablePreferences) normalize() {
	order := make([]string, 0, len(connectionColumns))
	for _, id := range p.Order {
		if connectionColumnIndex(id) >= 0 && !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for _, column := range connectionColumns {
		if !slices.Contains(order, column.ID) {
			order = append(order, column.ID)
		}
	}
	p.Order = order
	for id, hidden := range p.Hidden {
		if !hidden || connectionColumnIndex(id) < 0 {
			delete(p.Hidden, id)
		}
	}
	if len(p.Hidden) == len(connectionColumns) {
		p.Hidden = nil
	}
	for id, width := range p.Widths {
		i := connectionColumnIndex(id)
		if i < 0 || width <= 0 {
			delete(p.Widths, id)
			continue
		}
		p.Widths[id] = min(connectionColumnMaxWidth, max(16, connectionColumns[i].MinWidth, width))
	}
	if connectionColumnIndex(p.SortColumn) < 0 {
		p.SortColumn, p.SortDescending = "", false
	}
}

func (a *App) connectionTablePath() string {
	return filepath.Join(a.data, "connection-columns.json")
}

func (a *App) loadConnectionTablePreferences() {
	var prefs connectionTablePreferences
	data, err := os.ReadFile(a.connectionTablePath())
	if err == nil {
		err = json.Unmarshal(data, &prefs)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.connectionTableError = "无法读取列设置：" + err.Error()
		prefs = connectionTablePreferences{}
	}
	a.applyConnectionTablePreferences(prefs)
}

func (a *App) applyConnectionTablePreferences(prefs connectionTablePreferences) {
	prefs.normalize()
	a.connectionTable = prefs
	a.connectionRows.Columns = ui.TableLayout{Order: prefs.Order, Widths: prefs.Widths}
	a.connectionSort = ui.SortOrder{Column: prefs.SortColumn, Descending: prefs.SortDescending}
}

func (a *App) captureConnectionTablePreferences() bool {
	prefs, layout := &a.connectionTable, &a.connectionRows.Columns
	changed := false
	if !slices.Equal(layout.Order, prefs.Order) {
		// MyGo reports only visible columns after a drag. Keep hidden columns
		// in their slots so showing them again restores their position.
		if len(layout.Order) == len(prefs.Order)-len(prefs.Hidden) {
			order := slices.Clone(prefs.Order)
			visible := 0
			for i, id := range order {
				if !prefs.Hidden[id] {
					order[i] = layout.Order[visible]
					visible++
				}
			}
			prefs.Order = order
			changed = true
		}
		layout.Order = prefs.Order
	}
	if !maps.Equal(layout.Widths, prefs.Widths) {
		prefs.Widths = layout.Widths
		changed = true
	}
	if prefs.SortColumn != a.connectionSort.Column || prefs.SortDescending != a.connectionSort.Descending {
		prefs.SortColumn, prefs.SortDescending = a.connectionSort.Column, a.connectionSort.Descending
		changed = true
	}
	return changed
}

func (a *App) updateConnectionTablePreferences(c *ui.Context) {
	if a.captureConnectionTablePreferences() {
		a.connectionTableSaveAt = c.Now().Add(connectionTableSaveDelay)
	}
	if a.connectionTableSaveAt.IsZero() {
		return
	}
	if remaining := a.connectionTableSaveAt.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	a.flushConnectionTablePreferences()
}

func (a *App) flushConnectionTablePreferences() {
	if !a.captureConnectionTablePreferences() && a.connectionTableSaveAt.IsZero() {
		return
	}
	a.connectionTableSaveAt = time.Time{}
	data, err := json.Marshal(a.connectionTable)
	if err == nil {
		err = storage.Atomic(a.connectionTablePath(), data, 0600)
	}
	if err != nil {
		a.connectionTableError = "无法保存列设置：" + err.Error()
		return
	}
	a.connectionTableError = ""
}

func (a *App) connectionColumnSettings(c *ui.Context) {
	button := action(c.Key("connection-columns"), settingsIcon, "列设置", false).Shrink(0)
	open := ui.Local(button, "open", func() bool { return false })
	if button.Clicked() {
		*open = !*open
	}
	ui.Popover(c, button, open, func() {
		ui.Column(c).Width(224).Padding(8).Gap(5).Children(func() {
			muted(c, "拖动表头调整顺序，拖动列边缘调整宽度。")
			prefs := &a.connectionTable
			for _, column := range connectionColumns {
				visible := !prefs.Hidden[column.ID]
				lastVisible := visible && len(prefs.Hidden) == len(connectionColumns)-1
				if ui.Checkbox(c.Key(column.ID), &visible, column.Title).Label("显示" + column.Title).Disabled(lastVisible).Changed() {
					if visible {
						delete(prefs.Hidden, column.ID)
					} else {
						if prefs.Hidden == nil {
							prefs.Hidden = make(map[string]bool)
						}
						prefs.Hidden[column.ID] = true
					}
					a.connectionTableSaveAt = c.Now().Add(connectionTableSaveDelay)
				}
			}
			if prefs.Hidden[a.connectionSort.Column] {
				if i := connectionColumnIndex(a.connectionSort.Column); i >= 0 {
					muted(c, "当前按隐藏列「"+connectionColumns[i].Title+"」排序。")
				}
			}
			ui.Divider(c)
			if action(c, nil, "恢复默认列设置", false).AlignSelf(ui.Start).Clicked() {
				a.applyConnectionTablePreferences(connectionTablePreferences{})
				a.connectionTableSaveAt = c.Now().Add(connectionTableSaveDelay)
			}
			if a.connectionTableError != "" {
				ui.Text(c, a.connectionTableError).FontSize(12).TextColor(c.Theme().Danger)
			}
		})
	})
}
