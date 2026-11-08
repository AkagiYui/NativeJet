package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type linkParams map[string]string

func (p linkParams) take(key string) (string, bool) {
	value, ok := p[key]
	delete(p, key)
	return value, ok
}

func (p linkParams) move(dst map[string]any, fields map[string]string) {
	for from, to := range fields {
		if value, ok := p.take(from); ok {
			dst[to] = value
		}
	}
}

func (p linkParams) rename(from, to string) error {
	if value, ok := p.take(from); ok {
		if previous, exists := p[to]; exists && previous != value {
			return fmt.Errorf("参数 %s 与 %s 冲突", from, to)
		}
		p[to] = value
	}
	return nil
}

func (p linkParams) boolean(dst map[string]any, from, to string) error {
	if value, ok := p.take(from); ok {
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("参数 %s 应为布尔值", from)
		}
		dst[to] = flag
	}
	return nil
}

func (p linkParams) remaining() error {
	if len(p) == 0 {
		return nil
	}
	keys := make([]string, 0, len(p))
	for key := range p {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	// Parameter names normally describe the problem. Hide arbitrary query
	// keys, which may themselves contain credentials or control characters.
	for i, key := range keys {
		if len(key) > 32 || strings.ContainsFunc(key, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
		}) {
			keys[i] = "（未知参数）"
		}
	}
	return fmt.Errorf("无法转换参数 %s", strings.Join(keys, "、"))
}

func parseLinkParams(raw string) (linkParams, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, errors.New("节点参数格式无效")
	}
	params := make(linkParams, len(query))
	for name, values := range query {
		if len(values) != 1 {
			return nil, errors.New("节点包含重复参数")
		}
		params[name] = values[0]
	}
	return params, nil
}

func parseShareLink(line string) (map[string]any, error) {
	scheme, body, ok := strings.Cut(line, "://")
	if !ok {
		return nil, errors.New("应为节点链接")
	}
	scheme = strings.ToLower(scheme)
	switch scheme {
	case "vmess":
		if !strings.Contains(body, "@") {
			return parseVMessJSON(body)
		}
	case "ss":
		return parseShadowsocksLink(body)
	case "ssr":
		return parseShadowsocksRLink(body)
	case "vless", "trojan", "hysteria", "hy2", "hysteria2", "tuic", "anytls", "socks", "socks5", "http", "https":
	default:
		return nil, errors.New("暂不支持该节点链接协议")
	}

	u, node, params, err := parseLinkURL(scheme + "://" + body)
	if err != nil {
		return nil, err
	}
	node["type"] = scheme
	switch scheme {
	case "vless", "vmess", "trojan", "anytls":
		credential := "uuid"
		if scheme == "trojan" || scheme == "anytls" {
			credential = "password"
		}
		if err = linkCredential(node, u.User, credential); err != nil {
			return nil, err
		}
		if err = mapVLinkOptions(node, params, scheme); err != nil {
			return nil, err
		}
	case "hysteria", "hy2", "hysteria2":
		if scheme == "hy2" {
			node["type"] = "hysteria2"
		}
		if err = mapHysteria(node, params, u.User, scheme); err != nil {
			return nil, err
		}
	case "tuic":
		if u.User != nil {
			if password, ok := u.User.Password(); ok {
				node["uuid"], node["password"] = u.User.Username(), password
			} else {
				node["token"] = u.User.Username()
			}
		}
		params.move(node, map[string]string{"congestion_control": "congestion-controller", "udp_relay_mode": "udp-relay-mode"})
		if err = params.boolean(node, "disable_sni", "disable-sni"); err != nil {
			return nil, err
		}
		if err = params.boolean(node, "reduce_rtt", "reduce-rtt"); err != nil {
			return nil, err
		}
		if err = mapTLSParams(node, params, "sni", true); err != nil {
			return nil, err
		}
	case "socks", "socks5", "http", "https":
		if scheme == "socks" {
			node["type"] = "socks5"
		} else if scheme == "https" {
			node["type"], node["tls"] = "http", true
		}
		if u.User != nil {
			node["username"] = u.User.Username()
			if password, ok := u.User.Password(); ok {
				node["password"] = password
			}
		}
		serverName := ""
		if scheme == "http" || scheme == "https" {
			serverName = "sni"
		}
		if err = mapTLSParams(node, params, serverName, false); err != nil {
			return nil, err
		}
	}
	if err = params.remaining(); err != nil {
		return nil, err
	}
	return node, nil
}

func parseLinkURL(line string) (*url.URL, map[string]any, linkParams, error) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" || (u.Path != "" && u.Path != "/") {
		return nil, nil, nil, errors.New("节点地址格式无效")
	}
	node := map[string]any{"server": u.Hostname()}
	if port := u.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return nil, nil, nil, errors.New("节点端口格式无效")
		}
		node["port"] = int(number)
	}
	if strings.Contains(line, "#") {
		node["name"] = u.Fragment
	}
	params, err := parseLinkParams(u.RawQuery)
	return u, node, params, err
}

func linkCredential(node map[string]any, user *url.Userinfo, field string) error {
	if user == nil {
		return nil
	}
	value := user.Username()
	if extra, ok := user.Password(); ok {
		if field != "password" {
			return errors.New("节点包含无法转换的认证参数")
		}
		value += ":" + extra
	}
	node[field] = value
	return nil
}

func mapTLSParams(node map[string]any, params linkParams, serverName string, supportsALPN bool) error {
	if err := params.rename("allowInsecure", "insecure"); err != nil {
		return err
	}
	params.move(node, map[string]string{"pcs": "fingerprint"})
	if serverName != "" {
		params.move(node, map[string]string{"sni": serverName})
	}
	if supportsALPN {
		if alpn, ok := params.take("alpn"); ok {
			protocols := []string{}
			if alpn != "" {
				protocols = strings.Split(alpn, ",")
			}
			node["alpn"] = protocols
		}
	}
	return params.boolean(node, "insecure", "skip-cert-verify")
}

func mapVLinkOptions(node map[string]any, params linkParams, scheme string) error {
	serverName := "sni"
	switch scheme {
	case "vless", "vmess":
		serverName = "servername"
		if scheme == "vless" {
			params.move(node, map[string]string{"flow": "flow", "encryption": "encryption"})
		} else {
			params.move(node, map[string]string{"encryption": "cipher"})
		}
	}
	if scheme == "anytls" {
		if err := params.rename("hpkp", "pcs"); err != nil {
			return err
		}
	}
	if err := mapTLSParams(node, params, serverName, true); err != nil {
		return err
	}
	params.move(node, map[string]string{"fp": "client-fingerprint"})
	security, hasSecurity := params.take("security")
	if hasSecurity {
		switch security {
		case "", "none":
			if serverName == "sni" {
				return errors.New("此协议的 TLS 模式无法关闭")
			}
			node["tls"] = false
		case "tls", "reality":
			if serverName == "servername" {
				node["tls"] = true
			}
		default:
			return errors.New("暂不支持该 TLS 模式")
		}
	}
	if scheme != "anytls" {
		reality := map[string]any{}
		params.move(reality, map[string]string{"pbk": "public-key", "sid": "short-id"})
		if security == "reality" || len(reality) != 0 {
			if hasSecurity && security != "reality" {
				return errors.New("REALITY 参数与 TLS 模式冲突")
			}
			node["reality-opts"] = reality
		}
	} else if security == "reality" {
		return errors.New("该协议不支持 REALITY")
	}
	if network, ok := params.take("type"); ok {
		if scheme == "anytls" || (scheme == "trojan" && network != "tcp" && network != "raw" && network != "ws" && network != "grpc") ||
			(scheme != "vless" && (network == "xhttp" || network == "splithttp")) {
			return errors.New("该协议不支持此传输类型")
		}
		return mapLinkTransport(node, params, network)
	}
	return nil
}

func mapLinkTransport(node map[string]any, params linkParams, network string) error {
	header, hasHeader := params.take("headerType")
	if network == "raw" {
		network = "tcp"
	}
	if hasHeader && header != "" && header != "none" {
		if header != "http" || (network != "tcp" && network != "http") {
			return errors.New("暂不支持该传输头类型")
		}
		network = "http"
	}
	opts := map[string]any{}
	switch network {
	case "tcp":
	case "ws", "httpupgrade":
		if network == "httpupgrade" {
			network = "ws"
			opts["v2ray-http-upgrade"] = true
		}
		params.move(opts, map[string]string{"path": "path"})
		if host, ok := params.take("host"); ok {
			opts["headers"] = map[string]string{"Host": host}
		}
	case "grpc":
		params.move(opts, map[string]string{"serviceName": "grpc-service-name"})
		if mode, ok := params.take("mode"); ok && mode != "" && mode != "gun" {
			return errors.New("暂不支持该 gRPC 模式")
		}
	case "h2":
		params.move(opts, map[string]string{"path": "path"})
		if host, ok := params.take("host"); ok {
			opts["host"] = strings.Split(host, ",")
		}
	case "http":
		if path, ok := params.take("path"); ok {
			opts["path"] = strings.Split(path, ",")
		}
		if host, ok := params.take("host"); ok {
			opts["headers"] = map[string][]string{"Host": strings.Split(host, ",")}
		}
	case "xhttp", "splithttp":
		network = "xhttp"
		params.move(opts, map[string]string{"path": "path", "host": "host", "mode": "mode"})
	default:
		return errors.New("暂不支持该传输类型")
	}
	node["network"] = network
	if len(opts) != 0 {
		node[network+"-opts"] = opts
	}
	return nil
}

func mapHysteria(node map[string]any, params linkParams, user *url.Userinfo, scheme string) error {
	for from, to := range map[string]string{"peer": "sni", "upmbps": "up", "downmbps": "down"} {
		if err := params.rename(from, to); err != nil {
			return err
		}
	}
	if err := mapTLSParams(node, params, "sni", true); err != nil {
		return err
	}
	params.move(node, map[string]string{"up": "up", "down": "down", "mport": "ports"})
	if scheme == "hysteria" {
		params.move(node, map[string]string{"auth": "auth-str", "protocol": "protocol"})
		if obfs, ok := params.take("obfsParam"); ok {
			if mode, hasMode := params.take("obfs"); hasMode && mode != "xplus" {
				return errors.New("暂不支持该 Hysteria 混淆类型")
			}
			node["obfs"] = obfs
		} else if mode, ok := params.take("obfs"); ok {
			if mode == "xplus" {
				return errors.New("Hysteria 混淆缺少 obfsParam")
			}
			node["obfs"] = mode
		}
		if _, ok := node["auth-str"]; ok && user != nil {
			return errors.New("节点包含重复的认证参数")
		}
		if user != nil {
			auth := user.Username()
			if password, ok := user.Password(); ok {
				auth += ":" + password
			}
			node["auth-str"] = auth
		}
		return nil
	}
	params.move(node, map[string]string{"obfs": "obfs", "obfs-password": "obfs-password"})
	return linkCredential(node, user, "password")
}
