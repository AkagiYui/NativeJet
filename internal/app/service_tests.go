package app

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
)

const serviceTestsPage = "服务测试"

var serviceTestEntries = servicecatalog.All()

type serviceTestRun struct {
	id     string
	cancel context.CancelFunc
}

type serviceTestState struct {
	run      *serviceTestRun
	results  map[string]model.ServiceTestResult
	problem  string
	finished time.Time
	err      error
	search   string
	table    ui.ListState
	sort     ui.SortOrder
}

func (a *App) resetServiceTests() {
	if run := a.serviceTests.run; run != nil && run.cancel != nil {
		run.cancel()
	}
	a.serviceTests = serviceTestState{search: a.serviceTests.search, sort: a.serviceTests.sort}
}

func (a *App) startServiceTests(id string) {
	state := &a.serviceTests
	if state.run != nil || !a.coreRunning() || a.busy || a.windowContext == nil || a.quitting {
		return
	}
	client := a.client
	timeout := 20 * time.Second
	if id == "" {
		timeout = 95 * time.Second
	}
	ctx, cancel := context.WithTimeout(a.windowContext, timeout)
	run := &serviceTestRun{id: id, cancel: cancel}
	state.run, state.err = run, nil
	go func() {
		defer cancel()
		var result model.ServiceTests
		err := client.Call(ctx, "services.test", map[string]string{"id": id}, &result)
		mygo.RunOnMain(func() {
			if state.run != run || client != a.client || a.quitting {
				return
			}
			state.run = nil
			if err == nil {
				if state.results == nil {
					state.results = make(map[string]model.ServiceTestResult, len(result.Results))
				}
				for _, item := range result.Results {
					state.results[item.ID] = item
				}
				state.problem, state.finished = result.Problem, time.Now()
			} else if !errors.Is(err, context.Canceled) {
				state.err = err
			}
			if a.windowVisible() {
				a.win.Invalidate()
			}
		})
	}()
}

func (a *App) serviceTestsView(c *ui.Context) {
	state := &a.serviceTests
	supported := a.snapshot.Status.ServiceCatalog
	updateNeeded := a.connected && !supported
	ready := a.coreRunning() && supported
	workspaceHeader(c, serviceTestIcon, serviceTestsPage, func() {
		if updateNeeded {
			stateLabel(c, "更新后台服务后即可测试", c.Theme().Warning)
		} else if state.run != nil {
			stateLabel(c, "正在测试", c.Theme().Accent)
		} else if !state.finished.IsZero() {
			muted(c, "上次测试："+state.finished.Format("15:04:05"))
		}
		if ui.TextInput(c.Key("service-search"), &state.search).Width(170).MinWidth(110).Shrink(1).
			Height(controlHeight).Placeholder("搜索服务或地址").Label("搜索服务").Changed() {
			state.table.ScrollTo(0, ui.Start)
		}
		if run := state.run; run != nil {
			if action(c, closeIcon, "取消测试", false).Shrink(0).Disabled(run.cancel == nil).Clicked() {
				run.cancel()
				run.cancel = nil
			}
		} else if ready && action(c, scanIcon, "测试全部", true).Shrink(0).Disabled(a.busy).Clicked() {
			a.startServiceTests("")
		}
		if updateNeeded && action(c, settingsIcon, "打开设置", false).Shrink(0).Clicked() {
			a.page = settingsPage
			a.refreshService()
		}
	})
	modifier := "Ctrl"
	if runtime.GOOS == "darwin" {
		modifier = "⌘"
	}
	linkHint := modifier + " 点击地址，或聚焦后按回车打开"
	problem := state.problem
	if state.err != nil {
		problem = state.err.Error()
	} else if !a.connected {
		problem = "连接后台服务后即可测试"
	} else if supported && !a.coreRunning() {
		problem = "启动内核后即可测试"
	}
	if problem != "" {
		commandRow(c, func() { stateLabel(c, problem, c.Theme().Warning) })
	}
	entries := serviceTestEntries
	if query := strings.ToLower(strings.TrimSpace(state.search)); query != "" {
		entries = nil
		for _, entry := range serviceTestEntries {
			if strings.Contains(strings.ToLower(entry.Name), query) || strings.Contains(strings.ToLower(entry.Site), query) {
				entries = append(entries, entry)
			}
		}
	}
	if len(entries) == 0 {
		emptyState(c, serviceTestIcon, "没有匹配的服务", "", nil)
		return
	}
	columns := []ui.TableColumn{
		{Title: "服务", Width: 145}, {Title: "服务地址", MinWidth: 136}, {Title: "可用性", Width: 97},
		{Title: "耗时", Width: 76, Align: ui.End}, {Title: "地区", Width: 57}, {Title: "实际线路", MinWidth: 122},
	}
	state.table.Sort = &state.sort
	entries = sortableTableRows(entries, columns, state.sort, func(entry servicecatalog.Entry, col int) tableSortKey {
		result, tested := state.results[entry.ID]
		return serviceTestSortKey(entry, result, tested, col)
	})
	linkColor := palette(c).Accent
	workspaceTable(c, &state.table, columns, len(entries), "服务测试结果", func(row, col int) {
		entry := entries[row]
		result, tested := state.results[entry.ID]
		value := "未知"
		switch col {
		case 0:
			button := ui.ButtonBase(c.Key("test-" + entry.ID)).FillWidth().MinWidth(0).Gap(6).AlignItems(ui.Center).Justify(ui.Start).
				Disabled(!ready || state.run != nil || a.busy).Label("重测 " + entry.Name).Tooltip(entry.Name + " · 单独重测").Cursor(ui.CursorPointer)
			button.Children(func() {
				serviceIconImage(c, entry)
				ui.Text(c, entry.Name).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				ui.Icon(c, scanIcon).Size(12, 12).Shrink(0)
			})
			if button.Clicked() {
				a.startServiceTests(entry.ID)
			}
			return
		case 1:
			link := ui.ButtonBase(c.Key("site-" + entry.ID)).FillWidth().MinWidth(0).Gap(4).Justify(ui.Start).Cursor(ui.CursorPointer).
				Role(ui.RoleLink).Label(entry.Site).Description(linkHint).Tooltip(entry.Site + "\n" + linkHint)
			// A pointer release requires a modifier. Keyboard and assistive
			// activation use the link's ordinary press action.
			pointerRelease := ui.Local(link, "pointer-release", func() bool { return false })
			link.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerUp && ev.Button == 0 {
					*pointerRelease = true
				}
				return false
			})
			link.Children(func() {
				ui.Icon(c, externalLinkIcon).Size(12, 12).TextColor(linkColor).Shrink(0)
				ui.Text(c, strings.TrimPrefix(entry.Site, "https://")).FontSize(12).SingleLine().MinWidth(0).TextColor(linkColor)
			})
			fromPointer := *pointerRelease
			*pointerRelease = false
			if link.Clicked() && (!fromPointer || link.ClickModifiers()&ui.Cmd != 0) {
				c.OpenURLThen(entry.Site, func(err error) {
					if err != nil {
						state.err = fmt.Errorf("无法打开服务地址：%w", err)
					}
				})
			}
			return
		case 2:
			if run := state.run; run != nil && (run.id == "" || run.id == entry.ID) {
				stateLabel(c, "测试中", c.Theme().Accent)
				return
			}
			serviceTestStatus(c, result, tested)
			return
		case 3:
			if result.ElapsedMS > 0 {
				value = fmt.Sprintf("%d ms", result.ElapsedMS)
			}
		case 4:
			if result.Region != "" {
				value = result.Region
			}
		case 5:
			if len(result.Route) > 0 {
				value = strings.Join(result.Route, " → ")
			}
		}
		if !tested {
			value = "—"
		}
		ui.Text(c, value).FontSize(12).SingleLine().Tooltip(value)
	})
}

func serviceTestSortKey(entry servicecatalog.Entry, result model.ServiceTestResult, tested bool, col int) tableSortKey {
	switch col {
	case 0:
		return tableSortKey{text: strings.ToLower(entry.Name)}
	case 1:
		return tableSortKey{text: strings.ToLower(strings.TrimPrefix(entry.Site, "https://"))}
	case 2:
		return tableSortKey{text: serviceTestStatusLabel(result, tested), missing: !tested}
	case 3:
		return tableSortKey{number: uint64(max(0, result.ElapsedMS)), missing: result.ElapsedMS <= 0}
	case 4:
		return tableSortKey{text: result.Region, missing: result.Region == ""}
	default:
		return tableSortKey{text: strings.Join(result.Route, " → "), missing: len(result.Route) == 0}
	}
}

func serviceTestStatusLabel(result model.ServiceTestResult, tested bool) string {
	if !tested {
		return "未测试"
	}
	if result.Evidence == "region" {
		switch result.RegionSupport {
		case model.ServiceAvailable:
			return "地区支持"
		case model.ServiceBlocked:
			return "地区受限"
		}
	}
	switch result.Availability {
	case model.ServiceAvailable:
		return "可用"
	case model.ServicePartial:
		return "部分可用"
	case model.ServiceBlocked:
		return "受限"
	case model.ServiceReachable:
		return "仅可达"
	default:
		return "未知"
	}
}

func serviceTestStatus(c *ui.Context, result model.ServiceTestResult, tested bool) {
	t := c.Theme()
	value, color := serviceTestStatusLabel(result, tested), t.TextMuted
	switch value {
	case "可用":
		color = t.Success
	case "部分可用", "地区受限":
		color = t.Warning
	case "受限":
		color = t.Danger
	case "地区支持":
		color = t.Accent
	}
	basis := map[string]string{
		"response": "依据服务响应", "redirect": "依据服务的重定向", "reach": "收到 HTTP 响应", "region": "依据服务识别的地区，功能可用性未知",
	}[result.Evidence]
	if result.Detail != "" {
		basis = strings.TrimSpace(basis + " " + result.Detail)
	}
	ui.Row(c).AlignItems(ui.Center).Tooltip(basis).Children(func() { stateLabel(c, value, color) })
}
