package service

import (
	"encoding/json"
	"net/netip"
	"slices"
	"testing"

	"nativejet/internal/config"
	"nativejet/internal/model"
)

func TestServiceTestsUseConfiguredProxy(t *testing.T) {
	for _, fixture := range []struct{ yaml, want string }{
		{"mixed-port: 24511\nport: 24512\nbind-address: '[::]'\n", "http://127.0.0.1:24511"},
		{"port: 24512\nallow-lan: true\nbind-address: '[::]'\nauthentication: ['me:a:b']\n", "http://me:a%3Ab@[::1]:24512"},
		{"socks-port: 24513\n", "socks5://127.0.0.1:24513"},
		{"listeners:\n  - {type: mixed, port: 24514, listen: 127.0.0.1, users: []}\nauthentication: ['me:secret']\n", "http://127.0.0.1:24514"},
		{"external-controller: '127.0.0.1:24515'\ntun: {enable: true}\n", ""},
		{"mixed-port: 24511\nallow-lan: true\nbind-address: 192.0.2.1\n", ""},
	} {
		values, _, err := config.Parse(fixture.yaml)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		if proxy := configuredTestProxy(values); proxy != nil {
			got = proxy.String()
		}
		if got != fixture.want {
			t.Errorf("YAML %q: proxy = %q, want %q", fixture.yaml, got, fixture.want)
		}
	}
}

func TestServiceTestsReadActualChainForProbeSocket(t *testing.T) {
	var data model.Connections
	if err := json.Unmarshal([]byte(`{"connections":[
		{"metadata":{"sourceIP":"192.0.2.1","sourcePort":"53001"},"chains":["unrelated"]},
		{"metadata":{"sourceIP":"127.0.0.1","sourcePort":"53001"},"chains":["Singapore","AI","Default"]},
		{"metadata":{"sourceIP":"127.0.0.1","sourcePort":"53002"},"chains":["DIRECT"]}
	]}`), &data); err != nil {
		t.Fatal(err)
	}
	if got := serviceTestChain(data.Connections, netip.MustParseAddrPort("127.0.0.1:53001")); !slices.Equal(got, []string{"Default", "AI", "Singapore"}) {
		t.Errorf("got chain %v", got)
	}
	if got := serviceTestChain(data.Connections, netip.MustParseAddrPort("127.0.0.1:53003")); len(got) != 0 {
		t.Errorf("closed connection reported a chain: %v", got)
	}
}
