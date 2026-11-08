package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"nativejet/internal/model"
	"nativejet/internal/storage"
)

const (
	proxyFilterAll      = "全部节点"
	proxyFilterSuccess  = "测速成功"
	proxyFilterFailed   = "测速失败"
	proxyFilterUntested = "未测速"

	proxyOrderOriginal = "原始顺序"
	proxyOrderFastest  = "延迟从低到高"
	proxyOrderSlowest  = "延迟从高到低"
)

type proxyGroupState struct {
	url, resultsURL, maxDelay, problem string
	filter, sort                       string
	results                            map[string]model.ProxyDelay
}

type proxyTestRun struct {
	group, name string
	cancel      context.CancelFunc
}

func (a *App) loadProxyTestURLs() {
	data, err := os.ReadFile(filepath.Join(a.data, "proxy-test-urls.json"))
	if err == nil {
		_ = json.Unmarshal(data, &a.proxyTestURLs)
	}
}

func (a *App) saveProxyTestURL(group, address string) error {
	if a.proxyTestURLs[group] == address {
		return nil
	}
	next := maps.Clone(a.proxyTestURLs)
	if next == nil {
		next = make(map[string]string)
	}
	if address == "" {
		delete(next, group)
	} else {
		next[group] = address
	}
	data, err := json.Marshal(next)
	if err == nil {
		err = storage.Atomic(filepath.Join(a.data, "proxy-test-urls.json"), data, 0600)
	}
	if err == nil {
		a.proxyTestURLs = next
	}
	return err
}

func (a *App) proxyGroupState(name string) *proxyGroupState {
	if a.proxyGroups == nil {
		a.proxyGroups = make(map[string]*proxyGroupState)
	}
	state := a.proxyGroups[name]
	if state == nil {
		state = &proxyGroupState{url: a.proxyTestURLs[name], filter: proxyFilterAll, sort: proxyOrderOriginal}
		a.proxyGroups[name] = state
	}
	return state
}

func (a *App) resetProxyTests() {
	if run := a.proxyTest; run != nil && run.cancel != nil {
		run.cancel()
	}
	a.proxyTest = nil
	for _, state := range a.proxyGroups {
		state.results, state.resultsURL, state.problem = nil, "", ""
	}
}

func (a *App) startProxyTest(group model.Proxy, state *proxyGroupState, name *string) {
	if a.proxyTest != nil || !a.coreRunning() || a.busy || a.windowContext == nil || a.quitting {
		return
	}
	client := a.client
	override := strings.TrimSpace(state.url)
	address := group.DelayTestURL(override)
	ctx, cancel := context.WithCancel(a.windowContext)
	run := &proxyTestRun{group: group.Name, cancel: cancel}
	if name != nil {
		run.name = *name
	}
	a.proxyTest, state.problem = run, ""
	go func() {
		defer cancel()
		params := map[string]any{"id": group.Name, "name": name, "url": override}
		var results map[string]model.ProxyDelay
		err := client.Call(ctx, "proxies.delay", params, &results)
		mygo.RunOnMain(func() {
			if a.proxyTest != run || client != a.client || a.quitting {
				return
			}
			a.proxyTest = nil
			switch {
			case err == nil:
				if run.name == "" || state.resultsURL != address || state.results == nil {
					state.results = results
				} else {
					for key, value := range results {
						state.results[key] = value
					}
				}
				state.resultsURL = address
				if err := a.saveProxyTestURL(group.Name, override); err != nil {
					state.problem = "无法保存测速 URL：" + err.Error()
				}
			case errors.Is(err, context.Canceled):
				state.problem = "已取消测速"
			default:
				state.problem = err.Error()
			}
			if a.windowVisible() {
				a.win.Invalidate()
			}
		})
	}()
}

func proxyDelayLimit(raw string) (uint16, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	delay, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || delay == 0 {
		return 0, errors.New("最高延迟须为 1–65535 毫秒")
	}
	return uint16(delay), nil
}

func visibleProxyNames(names []string, results map[string]model.ProxyDelay, filter, order string, maxDelay uint16) []string {
	visible := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		result, tested := results[name]
		if (filter == proxyFilterSuccess && result.Delay == 0) || (filter == proxyFilterFailed && result.Problem == "") || (filter == proxyFilterUntested && tested) {
			continue
		}
		if maxDelay > 0 && (result.Delay == 0 || result.Delay > maxDelay) {
			continue
		}
		visible = append(visible, name)
	}
	if order == proxyOrderFastest || order == proxyOrderSlowest {
		slices.SortStableFunc(visible, func(a, b string) int {
			left, right := results[a].Delay, results[b].Delay
			if left == 0 || right == 0 {
				return cmp.Compare(right, left)
			}
			if order == proxyOrderSlowest {
				return cmp.Compare(right, left)
			}
			return cmp.Compare(left, right)
		})
	}
	return visible
}
