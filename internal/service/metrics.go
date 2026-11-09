package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"nativejet/internal/model"
)

type probeHistory struct {
	Alive   *bool               `json:"alive"`
	History []model.DelaySample `json:"history"`
}

type metricProxy struct {
	probeHistory
	Name  string                  `json:"name"`
	Type  string                  `json:"type"`
	All   []string                `json:"all"`
	Extra map[string]probeHistory `json:"extra"`
}

type metricProxyProvider[P any] struct {
	VehicleType  string                   `json:"vehicleType"`
	Proxies      []P                      `json:"proxies"`
	TestURL      string                   `json:"testUrl"`
	UpdatedAt    time.Time                `json:"updatedAt"`
	Subscription *model.SubscriptionQuota `json:"subscriptionInfo"`
}

func readMetric[T any](ctx context.Context, m *Manager, path string) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var out T
	data, err := m.core.Call(ctx, http.MethodGet, path, nil)
	if err == nil {
		err = json.Unmarshal(data, &out)
	}
	return out, err
}

func (m *Manager) metrics(ctx context.Context, section string) (model.Metrics, error) {
	record := m.core.Record()
	if record == nil || !m.core.Running() {
		return model.Metrics{}, errors.New("内核未运行")
	}
	out := model.Metrics{PID: record.PID}
	switch section {
	case model.MetricsOverview:
		var rss int64
		var memoryErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			probe, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
			defer cancel()
			rss, memoryErr = m.core.ReadMemory(probe)
		})
		connections, err := readMetric[struct {
			Connections []struct{} `json:"connections"`
		}](ctx, m, "/connections")
		if metricOK(&out, "活动连接", err) {
			count := len(connections.Connections)
			out.ActiveConnections = &count
		}
		wg.Wait()
		if metricOK(&out, "内核内存", memoryErr) {
			out.Memory = &rss
		}
	case model.MetricsNodes:
		m.nodeMetrics(ctx, &out)
	case model.MetricsRules:
		result, err := readMetric[struct {
			Rules []model.RuleMetric `json:"rules"`
		}](ctx, m, "/rules")
		if metricOK(&out, "规则统计", err) {
			out.Rules = result.Rules
		}
	case model.MetricsProviders:
		m.providerMetrics(ctx, &out)
	default:
		return model.Metrics{}, errors.New("未知指标类别")
	}
	if current := m.core.Record(); current == nil || *current != *record {
		return model.Metrics{}, errors.New("内核状态已变化，正在等待下一次采样")
	}
	return out, nil
}

func metricOK(out *model.Metrics, label string, err error) bool {
	if err == nil {
		return true
	}
	out.Problems = append(out.Problems, label)
	return false
}

func (m *Manager) nodeMetrics(ctx context.Context, out *model.Metrics) {
	proxies, err := readMetric[struct {
		Proxies map[string]metricProxy `json:"proxies"`
	}](ctx, m, "/proxies")
	if metricOK(out, "配置节点", err) {
		for name, proxy := range proxies.Proxies {
			if proxy.All == nil {
				out.Nodes = append(out.Nodes, proxy.node(name, "", ""))
			}
		}
	}
	providers, err := readMetric[struct {
		Providers map[string]metricProxyProvider[metricProxy] `json:"providers"`
	}](ctx, m, "/providers/proxies")
	if metricOK(out, "代理集节点", err) {
		for name, provider := range providers.Providers {
			if isInternalProxyProvider(name, provider.VehicleType) {
				continue
			}
			for _, proxy := range provider.Proxies {
				out.Nodes = append(out.Nodes, proxy.node(proxy.Name, name, provider.TestURL))
			}
		}
	}
	slices.SortFunc(out.Nodes, func(a, b model.NodeMetric) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Provider, b.Provider))
	})
}

func (p metricProxy) node(name, provider, testURL string) model.NodeMetric {
	result := model.NodeMetric{Name: name, Type: p.Type, Provider: provider, Alive: p.Alive, History: p.History}
	if probe, ok := p.Extra[testURL]; ok && testURL != "" {
		result.TestURL, result.Alive, result.History = testURL, probe.Alive, probe.History
	}
	return result
}

func (m *Manager) providerMetrics(ctx context.Context, out *model.Metrics) {
	proxies, err := readMetric[struct {
		Providers map[string]metricProxyProvider[struct{}] `json:"providers"`
	}](ctx, m, "/providers/proxies")
	if metricOK(out, "代理集", err) {
		for name, provider := range proxies.Providers {
			if !isInternalProxyProvider(name, provider.VehicleType) {
				out.Providers = append(out.Providers, model.ProviderMetric{
					Name: name, Count: len(provider.Proxies), UpdatedAt: provider.UpdatedAt, Subscription: provider.Subscription,
				})
			}
		}
	}
	rules, err := readMetric[struct {
		Providers map[string]struct {
			RuleCount int       `json:"ruleCount"`
			UpdatedAt time.Time `json:"updatedAt"`
		} `json:"providers"`
	}](ctx, m, "/providers/rules")
	if metricOK(out, "规则集", err) {
		for name, provider := range rules.Providers {
			out.Providers = append(out.Providers, model.ProviderMetric{
				Name: name, IsRuleSet: true, Count: provider.RuleCount, UpdatedAt: provider.UpdatedAt,
			})
		}
	}
	slices.SortFunc(out.Providers, func(a, b model.ProviderMetric) int {
		if a.IsRuleSet != b.IsRuleSet {
			if a.IsRuleSet {
				return 1
			}
			return -1
		}
		return cmp.Compare(a.Name, b.Name)
	})
}
