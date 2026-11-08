package config

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func parseVMessJSON(encoded string) (map[string]any, error) {
	body, err := decodeLinkBase64(encoded)
	if err != nil || !json.Valid([]byte(body)) {
		return nil, errors.New("VMess 节点应包含 Base64 编码的 JSON 对象")
	}
	fields, _, err := Parse(body)
	if err != nil {
		return nil, errors.New("VMess 节点 JSON 对象无效")
	}
	params := make(linkParams, len(fields))
	for key, value := range fields {
		switch key {
		case "v", "ps", "add", "port", "id", "aid", "scy", "net", "type", "host", "path", "tls", "sni", "alpn", "fp", "pbk", "sid":
		default:
			return nil, errors.New("VMess 节点包含无法转换的 JSON 字段")
		}
		switch v := value.(type) {
		case string:
			params[key] = v
		case int:
			if key != "v" && key != "port" && key != "aid" {
				return nil, errors.New("VMess 节点包含无法转换的数字字段")
			}
			params[key] = strconv.Itoa(v)
		default:
			return nil, errors.New("VMess 节点包含无法转换的 JSON 字段")
		}
	}
	if version, ok := params.take("v"); ok && version != "2" && version != "1" {
		return nil, errors.New("暂不支持该 VMess 链接版本")
	}
	node := map[string]any{"type": "vmess"}
	params.move(node, map[string]string{"ps": "name", "add": "server", "id": "uuid", "scy": "cipher"})
	if port, ok := params.take("port"); ok {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return nil, errors.New("VMess 节点端口格式无效")
		}
		node["port"] = int(number)
	}
	if aid, ok := params.take("aid"); ok {
		number, err := strconv.ParseUint(aid, 10, 16)
		if err != nil {
			return nil, errors.New("VMess 节点 aid 格式无效")
		}
		node["alterId"] = int(number)
	}

	// In v2rayN JSON, net is the transport and type describes its header
	// (or its gRPC mode). Empty host/path on TCP are unused editor fields.
	network, hasNetwork := params.take("net")
	header, hasHeader := params.take("type")
	if network == "grpc" {
		if hasHeader && header != "none" {
			params["mode"] = header
		}
		if err := params.rename("path", "serviceName"); err != nil {
			return nil, err
		}
		if params["host"] == "" {
			delete(params, "host")
		}
	} else if hasHeader && header != "" && header != "none" {
		params["headerType"] = header
	}
	if network == "tcp" && header != "http" {
		for _, key := range []string{"host", "path"} {
			if params[key] == "" {
				delete(params, key)
			}
		}
	}
	if hasNetwork && network != "" {
		params["type"] = network
	}
	if err := params.rename("tls", "security"); err != nil {
		return nil, err
	}
	if err := mapVLinkOptions(node, params, "vmess"); err != nil {
		return nil, err
	}
	return node, params.remaining()
}

func parseShadowsocksLink(body string) (map[string]any, error) {
	if authority, _, _ := strings.Cut(body, "?"); !strings.Contains(strings.SplitN(authority, "#", 2)[0], "@") {
		// Legacy form wraps method:password@host:port in a single Base64 value.
		end := len(body)
		if i := strings.IndexAny(body, "?#"); i >= 0 {
			end = i
		}
		decoded, err := decodeLinkBase64(body[:end])
		if err != nil {
			return nil, errors.New("Shadowsocks 节点认证格式无效")
		}
		at := strings.LastIndexByte(decoded, '@')
		if at < 0 {
			return nil, errors.New("Shadowsocks 节点缺少服务器地址")
		}
		cipher, password, ok := strings.Cut(decoded[:at], ":")
		if !ok {
			return nil, errors.New("Shadowsocks 节点认证格式无效")
		}
		body = url.UserPassword(cipher, password).String() + "@" + decoded[at+1:] + body[end:]
	}
	u, node, params, err := parseLinkURL("ss://" + body)
	if err != nil {
		return nil, err
	}
	if u.User == nil {
		return nil, errors.New("Shadowsocks 节点缺少认证信息")
	}
	cipher := u.User.Username()
	password, ok := u.User.Password()
	if !ok {
		decoded, err := decodeLinkBase64(cipher)
		if err == nil {
			cipher, password, ok = strings.Cut(decoded, ":")
		}
		if !ok {
			return nil, errors.New("Shadowsocks 节点认证格式无效")
		}
	}
	node["type"], node["cipher"], node["password"] = "ss", cipher, password
	if plugin, ok := params.take("plugin"); ok {
		if err := mapShadowsocksPlugin(node, plugin); err != nil {
			return nil, err
		}
	}
	return node, params.remaining()
}

func mapShadowsocksPlugin(node map[string]any, plugin string) error {
	parts := strings.Split(plugin, ";")
	name := parts[0]
	params := make(linkParams, len(parts)-1)
	for _, part := range parts[1:] {
		key, value, hasValue := strings.Cut(part, "=")
		if _, duplicate := params[key]; duplicate || key == "" {
			return errors.New("Shadowsocks 插件参数格式无效")
		}
		if !hasValue {
			if key != "tls" && key != "mux" {
				return errors.New("Shadowsocks 插件参数缺少值")
			}
			value = "true"
		}
		params[key] = value
	}
	opts := map[string]any{}
	switch name {
	case "simple-obfs", "obfs-local", "obfs":
		name = "obfs"
		params.move(opts, map[string]string{"obfs": "mode", "obfs-host": "host"})
	case "v2ray-plugin":
		params.move(opts, map[string]string{"mode": "mode", "host": "host", "path": "path"})
		if err := params.boolean(opts, "tls", "tls"); err != nil {
			return err
		}
		if err := params.boolean(opts, "mux", "mux"); err != nil {
			return err
		}
	default:
		return errors.New("暂不支持该 Shadowsocks 插件")
	}
	if err := params.remaining(); err != nil {
		return err
	}
	node["plugin"] = name
	if len(opts) != 0 {
		node["plugin-opts"] = opts
	}
	return nil
}

func parseShadowsocksRLink(encoded string) (map[string]any, error) {
	body, err := decodeLinkBase64(encoded)
	if err != nil {
		return nil, err
	}
	address, query, _ := strings.Cut(body, "/?")
	parts := strings.Split(address, ":")
	if len(parts) < 6 {
		return nil, errors.New("ShadowsocksR 节点格式无效")
	}
	host := strings.Trim(strings.Join(parts[:len(parts)-5], ":"), "[]")
	port, protocol, cipher, obfs, encodedPassword := parts[len(parts)-5], parts[len(parts)-4], parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
	password, err := decodeLinkBase64(encodedPassword)
	if err != nil {
		return nil, err
	}
	_, node, _, err := parseLinkURL("ssr://" + net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	node["type"], node["password"] = "ssr", password
	node["protocol"], node["cipher"], node["obfs"] = protocol, cipher, obfs
	params, err := parseLinkParams(query)
	if err != nil {
		return nil, err
	}
	for from, to := range map[string]string{"remarks": "name", "obfsparam": "obfs-param", "protoparam": "protocol-param"} {
		if value, ok := params.take(from); ok {
			text, err := decodeLinkBase64(value)
			if err != nil {
				return nil, err
			}
			node[to] = text
		}
	}
	return node, params.remaining()
}
