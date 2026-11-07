package service

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

func TestMetricsFromRunningController(t *testing.T) {
	m := newTestManager(t)
	installFixture(t, m)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	p, err := m.SaveProfile(model.Profile{Name: "metrics", Kind: "local", Content: "mode: rule\nexternal-controller: " + address + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SaveOverride("", "tun:\n  enable: false\nsecret: test-token\n"); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err = m.Apply(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		if _, err = m.Core(ctx, http.MethodGet, "/connections", nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	server := &Server{}
	sample := func(section string) model.Metrics {
		t.Helper()
		params, _ := json.Marshal(map[string]string{"section": section})
		out, err := server.dispatch(ctx, os.Getuid(), m, ipc.Request{Method: "metrics", Params: params})
		if err != nil {
			t.Fatal(err)
		}
		metrics := out.(model.Metrics)
		if metrics.PID != m.Snapshot().Status.PID {
			t.Fatalf("unexpected PID %d", metrics.PID)
		}
		return metrics
	}
	overview := sample(model.MetricsOverview)
	if overview.Memory == nil || *overview.Memory != 22020096 ||
		overview.ActiveConnections == nil || *overview.ActiveConnections != 0 {
		t.Fatalf("expected real RSS and a valid zero connection count: %+v", overview)
	}
	nodes := sample(model.MetricsNodes)
	if len(nodes.Nodes) != 2 || len(nodes.Nodes[1].History) != 1 || nodes.Nodes[1].History[0].Delay != 0 ||
		nodes.Nodes[1].Alive == nil || *nodes.Nodes[1].Alive || nodes.Nodes[1].TestURL != "https://example.com" {
		t.Fatalf("expected the provider-specific probe, including failures: %+v", nodes.Nodes)
	}
	rules := sample(model.MetricsRules)
	if len(rules.Rules) != 2 || rules.Rules[0].Extra == nil || rules.Rules[0].Extra.HitCount != 8 || rules.Rules[1].Extra != nil {
		t.Fatalf("expected rule counts only where reported: %+v", rules.Rules)
	}
	providers := sample(model.MetricsProviders)
	if len(providers.Providers) != 1 || providers.Providers[0].Count != 1 || providers.Providers[0].Subscription == nil ||
		providers.Providers[0].Subscription.Total != 800 || len(providers.Problems) != 1 || providers.Problems[0] != "规则集" {
		t.Fatalf("expected proxy quota despite an unsupported rule provider endpoint: %+v", providers)
	}
}
