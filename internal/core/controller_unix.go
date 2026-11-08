//go:build darwin || linux

package core

import (
	"context"
	"errors"
	"nativejet/internal/platform"
	"net"
	"path/filepath"
	"time"
)

func localController(values map[string]any, home string) (Controller, bool) {
	if socket, _ := values["external-controller-unix"].(string); socket != "" {
		if !filepath.IsAbs(socket) {
			socket = filepath.Join(home, socket)
		}
		return Controller{address: socket, baseURL: "http://core", local: true}, true
	}
	return Controller{}, false
}
func dialLocalController(ctx context.Context, address string, pid int) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", address)
	if err != nil {
		return nil, err
	}
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err == nil {
		var peer int
		err = raw.Control(func(fd uintptr) { peer, _ = platform.PeerPID(fd) })
		if err == nil && peer == pid {
			return conn, nil
		}
	}
	_ = conn.Close()
	return nil, errors.New("配置的 Unix socket 未连接到当前内核")
}
