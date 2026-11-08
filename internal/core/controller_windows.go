package core

import (
	"context"
	"errors"
	"net"
	"time"

	"nativejet/internal/platform"
)

func localController(values map[string]any, _ string) (Controller, bool) {
	if pipe, _ := values["external-controller-pipe"].(string); pipe != "" {
		if !platform.ValidPipe(pipe) {
			return Controller{problem: "external-controller-pipe 必须为有效的 Windows 本地命名管道。"}, true
		}
		return Controller{address: pipe, baseURL: "http://core", local: true}, true
	}
	if socket, _ := values["external-controller-unix"].(string); socket != "" {
		http, _ := values["external-controller"].(string)
		https, _ := values["external-controller-tls"].(string)
		if http == "" && https == "" {
			return Controller{problem: "Windows 请在覆盖配置中设置 external-controller-pipe 或回环 HTTP 控制接口。"}, true
		}
	}
	return Controller{}, false
}

func dialLocalController(ctx context.Context, address string, pid int) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := platform.DialPipe(ctx, address)
	if err != nil {
		return nil, err
	}
	peer, err := platform.PipeServerPID(conn)
	if err == nil && int(peer) == pid {
		return conn, nil
	}
	_ = conn.Close()
	return nil, errors.New("配置的命名管道未连接到当前内核")
}
