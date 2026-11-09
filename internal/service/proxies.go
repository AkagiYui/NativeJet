package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"nativejet/internal/model"
)

const (
	proxyDelayTimeout = 5 * time.Second
	proxyDelayWorkers = 12
)

func proxyPathSegment(name string) string {
	// Keep even dot-only names within their route parameter.
	switch name {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	default:
		return strings.ReplaceAll(url.PathEscape(name), "+", "%2B")
	}
}

func (m *Manager) proxyGroup(ctx context.Context, id string) (model.Proxy, error) {
	if id == "" {
		return model.Proxy{}, errors.New("请选择策略组")
	}
	return readMetric[model.Proxy](ctx, m, "/proxies/"+proxyPathSegment(id))
}

func (m *Manager) setProxy(ctx context.Context, method, id string, name *string) error {
	m.op.Lock()
	defer m.op.Unlock()
	group, err := m.proxyGroup(ctx, id)
	if err != nil {
		return err
	}
	path := "/proxies/" + proxyPathSegment(id)
	if method == "proxies.unpin" && group.CanPin() {
		_, err = m.Core(ctx, http.MethodDelete, path, nil)
		return err
	}
	if method == "proxies.select" && group.Type == "Selector" || method == "proxies.pin" && group.CanPin() {
		if name != nil && *name != "" && slices.Contains(group.All, *name) {
			_, err = m.Core(ctx, http.MethodPut, path, map[string]string{"name": *name})
			return err
		}
	}
	return errors.New("该策略组不支持此节点操作")
}

func proxyDelayQuery(group model.Proxy, override string) (string, error) {
	address := group.DelayTestURL(override)
	parsed, err := url.Parse(address)
	if err != nil || len(address) > 2048 || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("测速 URL 必须是有效的 HTTP 或 HTTPS 地址，且不含用户名、密码或片段")
	}
	query := url.Values{"url": {address}, "timeout": {strconv.FormatInt(proxyDelayTimeout.Milliseconds(), 10)}}
	if address == group.TestURL && group.ExpectedStatus != "" {
		query.Set("expected", group.ExpectedStatus)
	}
	return "?" + query.Encode(), nil
}

type proxyDelayCatalog struct {
	Proxies map[string]struct{} `json:"proxies"`
}

type proxyDelayProviders struct {
	Providers map[string]struct {
		VehicleType string `json:"vehicleType"`
		Proxies     []struct {
			Name string `json:"name"`
		} `json:"proxies"`
	} `json:"providers"`
}

type proxyDelayTarget struct {
	name string
	path string
}

func isInternalProxyProvider(name, vehicleType string) bool {
	// Mihomo wraps existing nodes and groups in Compatible providers.
	return name == "default" || vehicleType == "Compatible"
}

func proxyDelayTargets(names []string, catalog proxyDelayCatalog, providers proxyDelayProviders, scope map[string]bool) ([]proxyDelayTarget, map[string]model.ProxyDelay) {
	paths := make(map[string]string, len(names))
	for _, name := range names {
		if _, ok := catalog.Proxies[name]; ok {
			paths[name] = "/proxies/" + proxyPathSegment(name) + "/delay"
		}
	}
	for provider, entry := range providers.Providers {
		if isInternalProxyProvider(provider, entry.VehicleType) || (scope != nil && !scope[provider]) {
			continue
		}
		for _, node := range entry.Proxies {
			if _, exists := paths[node.Name]; exists {
				paths[node.Name] = "" // The group API identifies members by name only.
			} else {
				paths[node.Name] = "/providers/proxies/" + proxyPathSegment(provider) + "/" + proxyPathSegment(node.Name) + "/healthcheck"
			}
		}
	}
	results := make(map[string]model.ProxyDelay, len(names))
	targets := make([]proxyDelayTarget, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		switch path, found := paths[name]; {
		case !found:
			results[name] = model.ProxyDelay{Problem: "找不到可测速的节点"}
		case path == "":
			results[name] = model.ProxyDelay{Problem: "存在同名节点，无法确定测速目标"}
		default:
			targets = append(targets, proxyDelayTarget{name: name, path: path})
		}
	}
	return targets, results
}

// Mihomo returns names, without their provider, in a group's member list.
// Use the applied group's explicit provider scope to rule out unrelated same-name nodes.
func (m *Manager) proxyDelayProviderScope(revision, name string) map[string]bool {
	values, err := m.revisionValues(revision)
	if err != nil {
		return nil
	}
	groups, _ := values["proxy-groups"].([]any)
	for _, entry := range groups {
		group, _ := entry.(map[string]any)
		if group["name"] != name {
			continue
		}
		for _, key := range []string{"include-all", "include-all-providers"} {
			if value := group[key]; value != nil && value != false {
				return nil
			}
		}
		scope := make(map[string]bool)
		if raw := group["use"]; raw != nil {
			use, ok := raw.([]any)
			if !ok {
				return nil
			}
			for _, value := range use {
				provider, ok := value.(string)
				if !ok {
					return nil
				}
				scope[provider] = true
			}
		}
		return scope
	}
	if name == "GLOBAL" {
		return map[string]bool{} // Mihomo's implicit GLOBAL uses only top-level proxies.
	}
	return nil
}

func (m *Manager) testProxyDelay(ctx context.Context, id string, name *string, address string) (map[string]model.ProxyDelay, error) {
	if !m.proxyTest.TryLock() {
		return nil, errors.New("已有节点测速在运行")
	}
	defer m.proxyTest.Unlock()
	record := m.core.Record()
	if record == nil || !m.core.Running() {
		return nil, errors.New("内核未运行")
	}
	group, err := m.proxyGroup(ctx, id)
	if err != nil {
		return nil, err
	}
	names := group.All
	if len(names) == 0 {
		return nil, errors.New("策略组没有可测速的节点")
	}
	if name != nil {
		if *name == "" || !slices.Contains(names, *name) {
			return nil, errors.New("该节点不在策略组中")
		}
		names = []string{*name}
	}
	query, err := proxyDelayQuery(group, address)
	if err != nil {
		return nil, err
	}
	catalog, err := readMetric[proxyDelayCatalog](ctx, m, "/proxies")
	if err != nil {
		return nil, err
	}
	providers, err := readMetric[proxyDelayProviders](ctx, m, "/providers/proxies")
	if err != nil {
		return nil, err
	}
	targets, results := proxyDelayTargets(names, catalog, providers, m.proxyDelayProviderScope(record.RevisionID, id))

	// Mihomo's /group/{name}/delay endpoint clears URLTest and Fallback pins.
	// Test members individually so measuring delay preserves the fixed choice.
	var workers sync.WaitGroup
	var resultsMu sync.Mutex
	jobs := make(chan proxyDelayTarget)
	for range min(proxyDelayWorkers, len(targets)) {
		workers.Go(func() {
			for target := range jobs {
				if current := m.core.Record(); ctx.Err() != nil || current == nil || *current != *record {
					continue
				}
				result := m.measureProxyDelay(ctx, target.path+query)
				resultsMu.Lock()
				results[target.name] = result
				resultsMu.Unlock()
			}
		})
	}
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		jobs <- target
	}
	close(jobs)
	workers.Wait()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if current := m.core.Record(); current == nil || *current != *record || !m.core.Running() {
		return nil, errors.New("测速期间内核已变更，请重新测试")
	}
	return results, nil
}

func (m *Manager) measureProxyDelay(ctx context.Context, path string) model.ProxyDelay {
	ctx, cancel := context.WithTimeout(ctx, proxyDelayTimeout+2*time.Second)
	defer cancel()
	var result model.ProxyDelay
	data, err := m.Core(ctx, http.MethodGet, path, nil)
	if err != nil || json.Unmarshal(data, &result) != nil || result.Delay == 0 {
		return model.ProxyDelay{Problem: "测速失败或超时"}
	}
	return model.ProxyDelay{Delay: result.Delay}
}
