package app

import (
	"net/url"
	"testing"
)

func TestParseSubscriptionLink(t *testing.T) {
	address := "https://subscriptions.example.com/sub?token=a%2Bb&flag=meta"
	query := url.Values{"url": {address}, "name": {"测试 & 订阅"}}
	link, err := parseSubscriptionLink("clash://install-config/?" + query.Encode())
	if err != nil || link.address != address || link.name != "测试 & 订阅" {
		t.Fatalf("parsed link = %+v, %v", link, err)
	}

	query.Del("name")
	link, err = parseSubscriptionLink("clash://install-config?" + query.Encode())
	if err != nil || link.name != "subscriptions.example.com" {
		t.Fatalf("link without a name = %+v, %v", link, err)
	}
}

func TestRejectUnsupportedSubscriptionLinks(t *testing.T) {
	for _, raw := range []string{
		"nativejet://install-config?url=https://example.com",
		"clash://update-config?url=https://example.com",
		"clash://install-config/extra?url=https://example.com",
		"clash://install-config?url=https://example.com&url=https://other.example.com",
		"clash://install-config?url=%zz",
		"clash://install-config?name=test",
		"clash://install-config?url=http://example.com",
		"clash://install-config?url=file:///tmp/profile.yaml",
		"clash://install-config?url=https://user@example.com",
	} {
		if _, err := parseSubscriptionLink(raw); err == nil {
			t.Errorf("accepted unsupported link %q", raw)
		}
	}
}
