package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/model"
)

func TestProxyDelayFiltersAndSort(t *testing.T) {
	names := []string{"missing", "slow", "failed", "fast", "slow"}
	results := map[string]model.ProxyDelay{
		"slow": {Delay: 150}, "fast": {Delay: 42}, "failed": {Problem: "测速失败或超时"},
	}
	for _, tc := range []struct {
		filter, order string
		limit         uint16
		want          []string
	}{
		{proxyFilterAll, proxyOrderFastest, 0, []string{"fast", "slow", "missing", "failed"}},
		{proxyFilterAll, proxyOrderSlowest, 0, []string{"slow", "fast", "missing", "failed"}},
		{proxyFilterAll, proxyOrderOriginal, 42, []string{"fast"}},
		{proxyFilterFailed, proxyOrderOriginal, 0, []string{"failed"}},
		{proxyFilterUntested, proxyOrderOriginal, 0, []string{"missing"}},
		{proxyFilterSuccess, proxyOrderOriginal, 0, []string{"slow", "fast"}},
	} {
		if got := visibleProxyNames(names, results, tc.filter, tc.order, tc.limit); !slices.Equal(got, tc.want) {
			t.Errorf("%s/%s/%d: got %v, want %v", tc.filter, tc.order, tc.limit, got, tc.want)
		}
	}
}

func TestProxyGroupShowsControlsAndOnlyMatchingURLResults(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page = proxiesPage
	a.connected = true
	a.snapshot.Status = model.Status{Running: true, ProxyControls: true}
	a.groups = []model.Proxy{{
		Name: "自动", Type: "URLTest", Now: "节点 A", Fixed: "节点 A",
		All: []string{"节点 A", "节点 B"},
	}}
	state := a.proxyGroupState("自动")
	state.resultsURL = a.groups[0].DelayTestURL("")
	state.results = map[string]model.ProxyDelay{"节点 A": {Delay: 42}}
	tester := ui.NewTester(a.view, 920, 640)
	for _, label := range []string{"整组测速", "测速 节点 A", "解除固定", "固定 节点 B", "已固定 · 42 ms"} {
		if _, ok := tester.Find(label); !ok {
			t.Errorf("missing %q", label)
		}
	}
	state.url = "https://example.test/"
	tester.Frame()
	if _, ok := tester.Find("已固定 · 42 ms"); ok {
		t.Fatal("displayed latency from a different URL")
	}
	a.snapshot.Status.ProxyControls = false
	tester.Frame()
	if _, ok := tester.Find("整组测速"); ok {
		t.Fatal("displayed controls for an old helper")
	}
}
