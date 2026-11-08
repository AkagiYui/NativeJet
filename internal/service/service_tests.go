package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"nativejet/internal/model"
	"nativejet/internal/servicecatalog"
	"nativejet/internal/servicetest"
)

func (m *Manager) testServices(ctx context.Context, id string) (model.ServiceTests, error) {
	entries := servicecatalog.All()
	if id != "" {
		entry, ok := servicecatalog.Find(id)
		if !ok {
			return model.ServiceTests{}, errors.New("找不到要测试的服务")
		}
		entries = []servicecatalog.Entry{entry}
	}
	if !m.serviceTest.TryLock() {
		return model.ServiceTests{}, errors.New("服务测试已在运行")
	}
	defer m.serviceTest.Unlock()
	record := m.core.Record()
	if record == nil || !m.core.Running() {
		return model.ServiceTests{}, errors.New("内核未运行")
	}
	values, err := m.revisionValues(record.RevisionID)
	var out model.ServiceTests
	var proxy *url.URL
	if err != nil {
		out.Problem = "无法读取运行中的配置"
	} else if proxy = configuredTestProxy(values); proxy == nil {
		out.Problem = "当前配置没有可供测试的本机 HTTP 或 SOCKS 代理端口"
	}
	out.Results = servicetest.Run(ctx, proxy, m.serviceTestRoute, entries)
	if err := ctx.Err(); err != nil {
		return model.ServiceTests{}, err
	}
	if current := m.core.Record(); current == nil || *current != *record || !m.core.Running() {
		return model.ServiceTests{}, errors.New("测试期间内核已变更，请重新测试")
	}
	return out, nil
}

func (m *Manager) serviceTestRoute(ctx context.Context, source netip.AddrPort) []string {
	data, err := m.core.Call(ctx, http.MethodGet, "/connections", nil)
	if err != nil {
		return nil
	}
	var connections model.Connections
	if json.Unmarshal(data, &connections) != nil {
		return nil
	}
	return serviceTestChain(connections.Connections, source)
}

func serviceTestChain(connections []model.Connection, source netip.AddrPort) []string {
	port := strconv.Itoa(int(source.Port()))
	var match *model.Connection
	for i := range connections {
		conn := &connections[i]
		if conn.Metadata.SourcePort != port {
			continue
		}
		if ip := conn.Metadata.SourceIP; ip != "" {
			addr, err := netip.ParseAddr(ip)
			if err != nil || addr.Unmap() != source.Addr().Unmap() {
				continue
			}
		}
		if match != nil { // The snapshot cannot disambiguate these connections.
			return nil
		}
		match = conn
	}
	if match == nil {
		return nil
	}
	chain := slices.Clone(match.Chains)
	slices.Reverse(chain) // Mihomo lists the exit node before the groups.
	return chain
}

func configuredTestProxy(values map[string]any) *url.URL {
	user := testProxyUser(values)
	host := "127.0.0.1"
	if allowLAN, _ := values["allow-lan"].(bool); allowLAN {
		host, _ = values["bind-address"].(string)
	}
	for _, entry := range []struct{ key, scheme string }{
		{"mixed-port", "http"}, {"port", "http"}, {"socks-port", "socks5"},
	} {
		if proxy := localTestProxy(host, values[entry.key], entry.scheme, user); proxy != nil {
			return proxy
		}
	}
	listeners, _ := values["listeners"].([]any)
	for _, entry := range listeners {
		listener, _ := entry.(map[string]any)
		kind, _ := listener["type"].(string)
		var scheme string
		switch kind {
		case "mixed", "http":
			scheme = "http"
			if cert, _ := listener["certificate"].(string); cert != "" {
				scheme = "https"
			}
		case "socks":
			scheme = "socks5"
		default:
			continue
		}
		listen, _ := listener["listen"].(string)
		credential := user
		if users, ok := listener["users"].([]any); ok {
			credential = nil
			for _, item := range users {
				auth, _ := item.(map[string]any)
				if name, _ := auth["username"].(string); name != "" {
					password, _ := auth["password"].(string)
					credential = url.UserPassword(name, password)
					break
				}
			}
		}
		if proxy := localTestProxy(listen, listener["port"], scheme, credential); proxy != nil {
			return proxy
		}
	}
	return nil
}

func testProxyUser(values map[string]any) *url.Userinfo {
	entries, _ := values["authentication"].([]any)
	for _, entry := range entries {
		if s, _ := entry.(string); s != "" {
			if name, password, ok := strings.Cut(s, ":"); ok && name != "" {
				return url.UserPassword(name, password)
			}
		}
	}
	return nil
}

func localTestProxy(host string, value any, scheme string, user *url.Userinfo) *url.URL {
	port, ok := value.(int)
	if !ok || port < 1 || port > 65535 {
		return nil
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "", "*", "localhost", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || (!ip.IsLoopback() && !interfaceHasAddress(ip)) {
		return nil
	}
	return &url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(port)), User: user}
}

func interfaceHasAddress(ip netip.Addr) bool {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if prefix, err := netip.ParsePrefix(a.String()); err == nil && prefix.Addr().Unmap() == ip.WithZone("").Unmap() {
			return true
		}
	}
	return false
}
