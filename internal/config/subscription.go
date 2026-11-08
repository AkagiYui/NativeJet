package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var shareLinkStart = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// SubscriptionYAML preserves YAML as received. A link list has only one
// equivalent configuration field: proxies. A failed conversion is atomic.
func SubscriptionYAML(raw string) (string, error) {
	if len(raw) > MaxBytes {
		return "", errors.New("订阅超过 4 MB")
	}
	if !utf8.ValidString(raw) {
		return "", errors.New("订阅必须为 UTF-8 文本")
	}
	body := strings.TrimPrefix(raw, "\ufeff")
	if strings.TrimSpace(body) == "" {
		return "", errors.New("订阅内容为空")
	}
	if !shareLinkStart.MatchString(strings.TrimSpace(body)) {
		_, _, yamlErr := Parse(raw)
		if yamlErr == nil {
			return raw, nil
		}
		decoded, err := decodeLinkBase64(strings.Join(strings.Fields(body), ""))
		if err != nil || !shareLinkStart.MatchString(strings.TrimSpace(strings.TrimPrefix(decoded, "\ufeff"))) {
			return "", fmt.Errorf("订阅须为 Clash/Mihomo YAML、明文节点列表或 Base64 节点列表: %w", yamlErr)
		}
		body = strings.TrimPrefix(decoded, "\ufeff")
	}

	var proxies []map[string]any
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		proxy, err := parseShareLink(line)
		if err != nil {
			return "", fmt.Errorf("节点列表第 %d 行: %w", i+1, err)
		}
		proxies = append(proxies, proxy)
	}
	out, err := yaml.Marshal(map[string]any{"proxies": proxies})
	if err != nil {
		return "", err
	}
	// Enforce the same size and structure limits as other configuration sources.
	if _, _, err := Parse(string(out)); err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeLinkBase64(text string) (string, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if data, err := enc.DecodeString(text); err == nil && utf8.Valid(data) {
			return string(data), nil
		}
	}
	return "", errors.New("无效的 Base64 文本")
}
