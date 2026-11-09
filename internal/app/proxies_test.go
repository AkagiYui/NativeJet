package app

import (
	"fmt"
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

func TestProxyPageSplitsNodesAndGroups(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page = proxiesPage
	a.connected = true
	a.snapshot.Status = model.Status{Running: true, ProxyControls: true}
	var nodes []model.NodeMetric
	for i := range 24 {
		nodes = append(nodes, model.NodeMetric{Name: fmt.Sprintf("节点 %02d", i), Type: "Shadowsocks"})
	}
	nodes[0].Provider = "订阅 A"
	a.metrics[metricsNodeTab] = &metricsResult{data: model.Metrics{Nodes: nodes}}
	a.groups = []model.Proxy{{Name: "自动选择", Type: "Selector", All: []string{"节点 00", "节点 01"}}}

	tester := ui.NewTester(a.view, 920, 640)
	for _, width := range []int{920, 1160} {
		tester.SetSize(width, 640)
		node, nodeOK := tester.Find("Shadowsocks · 订阅 A")
		group, groupOK := tester.Find("自动选择")
		sort, sortOK := tester.Find("节点排序")
		count, countOK := tester.Find("1 个策略组")
		if !nodeOK || !groupOK || !sortOK || !countOK {
			t.Fatalf("width %d: missing node, group, group controls, or page header", width)
		}
		if node.X < sidebarWidth || node.X+node.W >= group.X || sort.X+sort.W > float32(width) {
			t.Fatalf("width %d: node %v, group %v, sort control %v", width, node, group, sort)
		}
		if count.Y+count.H > toolbarHeight || group.Y < toolbarHeight || group.Y > toolbarHeight+32 {
			t.Fatalf("width %d: header %v and first group %v should be adjacent", width, count, group)
		}
	}
	group, _ := tester.Find("自动选择")
	node, ok := tester.Find("节点 05")
	if !ok {
		t.Fatal("missing node before scrolling")
	}
	tester.Scroll(node.X+node.W/2, node.Y+node.H/2, 0, 160)
	scrolled, ok := tester.Find("节点 05")
	unchanged, _ := tester.Find("自动选择")
	if !ok || scrolled.Y >= node.Y || unchanged.Y != group.Y {
		t.Fatalf("scrolling the nodes moved node from %v to %v and group from %v to %v", node, scrolled, group, unchanged)
	}

	bar := ui.TitleBar{Height: toolbarHeight + 8, Right: 138}
	tester.SetTitleBar(bar)
	belowCaption, ok := tester.Find("自动选择")
	if !ok || belowCaption.Y != group.Y+bar.Height-toolbarHeight {
		t.Fatalf("group at %v did not move below the window controls", belowCaption)
	}
}

func TestProxyNodesViewShowsOnlyProxyNodes(t *testing.T) {
	a := &App{}
	result := &metricsResult{data: model.Metrics{Nodes: []model.NodeMetric{
		{Name: "COMPATIBLE", Type: "Compatible"},
		{Name: "DIRECT", Type: "Direct"},
		{Name: "本地 DNS", Type: "Dns"},
		{Name: "PASS", Type: "Pass"},
		{Name: "PASS-RULE", Type: "PassRule"},
		{Name: "REJECT", Type: "Reject"},
		{Name: "REJECT-DROP", Type: "RejectDrop"},
		{Name: "重新匹配", Type: "Rematch"},
		{Name: "策略组 A", Type: "Selector"},
	}}}
	a.metrics[metricsNodeTab] = result
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { a.proxyNodesView(c) })
	}, 360, 640)
	if _, ok := tester.Find("暂无代理节点"); !ok {
		t.Fatalf("expected an empty list, got %v", tester.Texts())
	}

	result.data.Nodes = append(result.data.Nodes,
		model.NodeMetric{Name: "本地节点", Type: "Shadowsocks"},
		model.NodeMetric{Name: "PASS", Type: "Vless", Provider: "订阅 A"},
	)
	tester.Frame()
	for _, label := range []string{"本地节点", "PASS", "Vless · 订阅 A"} {
		if _, ok := tester.Find(label); !ok {
			t.Errorf("missing proxy node label %q", label)
		}
	}
	if _, ok := tester.Find("Pass"); ok {
		t.Fatal("displayed the built-in PASS alongside the proxy with the same name")
	}
}
