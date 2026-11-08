package config

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestSubscriptionPreservesYAML(t *testing.T) {
	for _, raw := range []string{
		"# nodes\nproxies: []\n",
		"# profile\nproxies: []\nproxy-providers: {}\nrules: ['MATCH,DIRECT']\ndns: {enable: false}\nfuture: null\n",
		"\ufeffmode: rule\r\n",
	} {
		got, err := SubscriptionYAML(raw)
		if err != nil || got != raw {
			t.Fatalf("YAML was rewritten: %q, %v", got, err)
		}
	}
}

func TestNodeSubscriptionContainsOnlySourceFields(t *testing.T) {
	ssAuth := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret"))
	vmess := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"same","add":"vm.example","port":"8443","id":"test-id","aid":0,"net":"ws","type":"none","host":"cdn.example","path":"/in","tls":"tls"}`))
	raw := "\nss://" + ssAuth + "@[2001:db8::1]:443#same\r\n" +
		"vless://test-id@v.example:8443?type=tcp&security=reality&pbk=key&sid=1234&allowInsecure=0\r\n" +
		"vmess://" + vmess + "\r\n" +
		"hy2://secret@h.example:443?obfs=salamander&obfs-password=p%2Bq#H%20two\r\n"
	want := map[string]any{"proxies": []any{
		map[string]any{"type": "ss", "server": "2001:db8::1", "port": 443, "name": "same", "cipher": "aes-128-gcm", "password": "secret"},
		map[string]any{"type": "vless", "server": "v.example", "port": 8443, "uuid": "test-id", "network": "tcp", "tls": true,
			"skip-cert-verify": false, "reality-opts": map[string]any{"public-key": "key", "short-id": "1234"}},
		map[string]any{"type": "vmess", "server": "vm.example", "port": 8443, "uuid": "test-id", "name": "same", "alterId": 0, "network": "ws", "tls": true,
			"ws-opts": map[string]any{"path": "/in", "headers": map[string]any{"Host": "cdn.example"}}},
		map[string]any{"type": "hysteria2", "server": "h.example", "port": 443, "password": "secret",
			"obfs": "salamander", "obfs-password": "p+q", "name": "H two"},
	}}
	variants := map[string]string{"plain": raw}
	for name, encoding := range map[string]*base64.Encoding{"standard": base64.StdEncoding, "url safe unpadded": base64.RawURLEncoding} {
		encoded := encoding.EncodeToString([]byte(raw))
		variants[name] = encoded[:31] + "\r\n" + encoded[31:]
	}
	for name, body := range variants {
		t.Run(name, func(t *testing.T) {
			yaml, err := SubscriptionYAML(body)
			if err != nil {
				t.Fatal(err)
			}
			values, _, err := Parse(yaml)
			if err != nil || !reflect.DeepEqual(values, want) {
				t.Fatalf("unexpected subscription YAML: %v\n%s", err, yaml)
			}
		})
	}
}

func TestNodeSubscriptionRejectsLossyConversion(t *testing.T) {
	first := "trojan://secret@example.com:443#first"
	for _, node := range []string{
		"unsupported://secret@example.com:443",
		"vless://secret@example.com:443?type=grpc&authority=example.org",
		"vless://secret@example.com:443?security=tls&fp=chrome&fp=firefox",
		"trojan://secret@example.com:99999",
		"unexpected content",
	} {
		out, err := SubscriptionYAML(first + "\n" + node)
		if err == nil || out != "" || !strings.Contains(err.Error(), "第 2 行") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid conversion result %q, %v", out, err)
		}
	}
}
