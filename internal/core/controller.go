package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"nativejet/internal/platform"
)

// Controller describes how to reach a listener from the effective YAML.
type Controller struct {
	address, baseURL, secret string
	unix                     bool
	problem                  string
}

func ConfiguredController(values map[string]any, home string) Controller {
	if socket, _ := values["external-controller-unix"].(string); socket != "" {
		if !filepath.IsAbs(socket) {
			socket = filepath.Join(home, socket)
		}
		return Controller{address: socket, baseURL: "http://core", unix: true}
	}
	field, scheme := "external-controller", "http"
	address, _ := values[field].(string)
	if address == "" {
		field, scheme = "external-controller-tls", "https"
		address, _ = values[field].(string)
	}
	if address == "" {
		return Controller{problem: "当前配置未启用 Mihomo 控制接口。"}
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" || port == "0" {
		return Controller{problem: field + " 未指定可连接的地址和端口。"}
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	address = net.JoinHostPort(host, port)
	secret, _ := values["secret"].(string)
	return Controller{address: address, baseURL: scheme + "://" + address, secret: secret}
}

func (c Controller) occupied(ctx context.Context) bool {
	// Avoid sending controller requests to a TCP listener predating this core.
	if c.address == "" || c.unix {
		return false
	}
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (c Controller) request(ctx context.Context, pid int, method, path string, body io.Reader) (*http.Response, error) {
	if c.address == "" {
		if c.problem != "" {
			return nil, errors.New(c.problem)
		}
		return nil, errors.New("无法读取运行中内核的控制接口配置")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	dialer := net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true}
	if c.unix {
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, "unix", c.address)
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
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return client.Do(req)
}

func (s *Supervisor) Call(ctx context.Context, method, path string, payload any) ([]byte, error) {
	s.mu.Lock()
	controller, record := s.controller, s.record
	s.mu.Unlock()
	if record == nil {
		return nil, errors.New("内核未运行")
	}
	if record.ControllerOccupied {
		return nil, errors.New("配置的控制端口在启动内核前已被占用")
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	resp, err := controller.request(ctx, record.PID, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("内核 API %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}
