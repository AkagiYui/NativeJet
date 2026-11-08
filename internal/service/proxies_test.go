package service

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"nativejet/internal/model"
)

func TestProxyDelayQuery(t *testing.T) {
	group := model.Proxy{TestURL: "https://example.test/generate_204", ExpectedStatus: "204"}
	for _, tc := range []struct{ override, address, expected string }{
		{"", group.TestURL, "204"},
		{" http://example.test:8080/probe?a=1&b=x+y ", "http://example.test:8080/probe?a=1&b=x+y", ""},
	} {
		query, err := proxyDelayQuery(group, tc.override)
		if err != nil {
			t.Fatal(err)
		}
		values, err := url.ParseQuery(strings.TrimPrefix(query, "?"))
		if err != nil || values.Get("url") != tc.address || values.Get("expected") != tc.expected || values.Get("timeout") != "5000" {
			t.Errorf("unexpected query: %v, %v", values, err)
		}
	}
	for _, invalid := range []string{"file:///etc/hosts", "https://", "https://user:secret@example.test", "https://example.test/#fragment"} {
		if _, err := proxyDelayQuery(group, invalid); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
}

func TestProxyDelayTargetsResolveProviderMembersWithoutGuessing(t *testing.T) {
	catalog := proxyDelayCatalog{Proxies: map[string]struct{}{"A/B+": {}, "shared": {}}}
	var providers proxyDelayProviders
	if err := json.Unmarshal([]byte(`{"providers":{
		"default":{"vehicleType":"Compatible","proxies":[{"name":"A/B+"}]},
		"provider/1":{"vehicleType":"Inline","proxies":[{"name":"节点"},{"name":"shared"}]}
	}}`), &providers); err != nil {
		t.Fatal(err)
	}
	got, results := proxyDelayTargets([]string{"A/B+", "节点", "shared", "missing", "A/B+"}, catalog, providers, nil)
	want := []proxyDelayTarget{
		{name: "A/B+", path: "/proxies/A%2FB%2B/delay"},
		{name: "节点", path: "/providers/proxies/provider%2F1/%E8%8A%82%E7%82%B9/healthcheck"},
	}
	if !slices.Equal(got, want) || len(results) != 2 || results["shared"].Problem == "" || results["missing"].Problem == "" {
		t.Fatalf("targets = %v, other results = %v", got, results)
	}
	got, results = proxyDelayTargets([]string{"shared"}, catalog, providers, map[string]bool{})
	if len(got) != 1 || got[0].path != "/proxies/shared/delay" || len(results) != 0 {
		t.Fatalf("scoped targets = %v, other results = %v", got, results)
	}
}
