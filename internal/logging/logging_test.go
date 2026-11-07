package logging

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nativejet/internal/model"
)

func TestCoreOutputFormats(t *testing.T) {
	for _, test := range []struct{ raw, level, message string }{
		{`time="2026-11-06T11:05:59-08:00" level=error msg="Start TUN listening error: add route: 1.0.0.0/8: file exists"`, "error", "Start TUN listening error: add route: 1.0.0.0/8: file exists"},
		{`level=fatal msg="bad config" path="/tmp/config.yaml"`, "error", "bad config"},
		{`{"level": "warn", "msg": "warning with \"quotes\""}`, "warning", `warning with "quotes"`},
		{`level=warning msg="Mihomo shutting down"`, "warning", "Mihomo shutting down"},
		{`panic: fixture failure`, "error", "panic: fixture failure"},
	} {
		level, message := Parse(test.raw)
		if level != test.level || message != test.message {
			t.Errorf("parse %q: got %q %q want %q %q", test.raw, level, message, test.level, test.message)
		}
	}
}
func TestFailureExplainsActualCoreError(t *testing.T) {
	raw := `level=error msg="Start TUN listening error: configure tun interface: add route: 1.0.0.0/8: file exists"`
	logs := []model.Log{{Source: "process", Level: "error", Message: raw}, {Source: "process", Level: "warning", Message: `level=warning msg="Mihomo shutting down"`}, {Source: "helper", Level: "error", Message: "tun.enable mismatch"}}
	err := Failure("内核启动失败", context.DeadlineExceeded, logs)
	for _, fragment := range []string{"内核启动失败", "TUN 路由冲突", "1.0.0.0/8", "关闭其他代理客户端", "Start TUN listening error"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("missing %q: %v", fragment, err)
		}
	}
	if strings.Contains(err.Error(), "mismatch") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("summary masked root cause or error identity")
	}
	err = Failure("内核启动失败", errors.New("TUN 未能启用"), []model.Log{{Source: "validation", Level: "error", Message: raw}, {Source: "helper", Level: "error", Message: raw}})
	if strings.Contains(err.Error(), "路由冲突") {
		t.Fatal("unrelated errors were used as runtime cause")
	}
	err = Failure("内核启动失败", errors.New("exit"), []model.Log{{Source: "core", Level: "error", Message: "listen tcp :7897: bind: address already in use"}})
	if !strings.Contains(err.Error(), "端口已被占用") || !strings.Contains(err.Error(), ":7897") {
		t.Fatal(err)
	}
}
