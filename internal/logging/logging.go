// Package logging interprets core output without discarding the original logs.
package logging

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"nativejet/internal/model"
)

var levelPattern = regexp.MustCompile(`(?i)(?:^|\s)level=("[a-z]+"|[a-z]+)`)
var routePattern = regexp.MustCompile(`(?i)add route:\s*([0-9a-f:.]+/\d+):\s*file exists`)

func Level(s string) string {
	switch strings.ToLower(s) {
	case "warn", "warning":
		return "warning"
	case "fatal", "panic", "error":
		return "error"
	case "debug", "trace":
		return "debug"
	default:
		return "info"
	}
}

// Parse supports Mihomo's text and JSON output. Callers retain raw separately.
func Parse(raw string) (level, message string) {
	raw = strings.TrimSpace(raw)
	message = raw
	level = "info"
	var value struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
	}
	if json.Unmarshal([]byte(raw), &value) == nil && value.Message != "" {
		return Level(value.Level), value.Message
	}
	if match := levelPattern.FindStringSubmatch(raw); len(match) > 1 {
		level = Level(strings.Trim(match[1], `"`))
	}
	if _, rest, ok := strings.Cut(raw, "msg="); ok {
		if quoted, err := strconv.QuotedPrefix(rest); err == nil {
			if text, err := strconv.Unquote(quoted); err == nil {
				message = text
			}
		} else {
			message = strings.TrimSpace(rest)
		}
	}
	lower := strings.ToLower(message)
	if strings.HasPrefix(lower, "panic:") || strings.HasPrefix(lower, "fatal:") {
		level = "error"
	}
	return level, message
}

type failure struct {
	stage, reason string
	cause         error
}

func (f *failure) Error() string { return f.stage + "：" + f.reason }
func (f *failure) Unwrap() error { return f.cause }

// Failure extracts the core's own error from the current operation's logs.
func Failure(stage string, cause error, logs []model.Log) error {
	if cause == nil {
		return nil
	}
	best, score := "", 0
	for _, entry := range logs {
		if entry.Source != "process" && entry.Source != "core" {
			continue
		}
		parsed, message := Parse(entry.Message)
		if Level(entry.Level) != "error" && parsed != "error" {
			continue
		}
		priority := 1
		lower := strings.ToLower(message)
		if strings.Contains(lower, "tun") {
			priority = 2
		}
		if routePattern.MatchString(message) {
			priority = 3
		}
		if message != "" && priority >= score {
			best = message
			score = priority
		}
	}
	if best == "" {
		return &failure{stage: stage, reason: cause.Error(), cause: cause}
	}
	reason := best
	if match := routePattern.FindStringSubmatch(best); len(match) > 1 && strings.Contains(strings.ToLower(best), "tun") {
		reason = "TUN 路由冲突：路由 " + match[1] + " 已存在。请先关闭其他代理客户端的 TUN 后重试。\n内核报错：" + best
	} else if strings.Contains(strings.ToLower(best), "address already in use") {
		reason = "监听地址或端口已被占用。请检查其他代理客户端或监听程序。\n内核报错：" + best
	}
	if len([]rune(reason)) > 1600 {
		reason = string([]rune(reason)[:1600]) + "…（完整内容见日志）"
	}
	return &failure{stage: stage, reason: reason, cause: cause}
}
