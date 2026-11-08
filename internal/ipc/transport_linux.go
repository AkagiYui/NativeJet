package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"nativejet/internal/platform"
)

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, _, err := c.verifiedSocket(ctx)
	return conn, err
}

func (c *Client) verifiedSocket(ctx context.Context) (net.Conn, int, error) {
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return nil, 0, fmt.Errorf("无法连接后台服务: %w", err)
	}
	expectedUID := 0
	if c.Development {
		expectedUID = os.Getuid()
	}
	cred, err := platform.PeerCredentials(conn.(*net.UnixConn))
	if err == nil && cred.Uid == uint32(expectedUID) && cred.Pid > 0 {
		return conn, int(cred.Pid), nil
	}
	_ = conn.Close()
	return nil, 0, errors.New("后台服务身份验证失败")
}

func (c *Client) StopDevelopmentHelper(ctx context.Context, expectedPath string) error {
	if !c.Development {
		return errors.New("不允许直接停止系统 helper")
	}
	conn, pid, err := c.verifiedSocket(ctx)
	if err != nil {
		return err
	}
	_ = conn.Close()
	expected, err := filepath.EvalSymlinks(expectedPath)
	if err != nil {
		return err
	}
	expected, err = filepath.Abs(expected)
	if err != nil {
		return err
	}
	path, started, err := platform.ProcessIdentity(pid)
	// dpkg can replace the executable while an older user helper is running.
	if err != nil || (path != expected && path != expected+" (deleted)") {
		return errors.New("连接并非当前 NativeJet 的开发 helper")
	}
	if err = c.Call(ctx, "service.shutdown", nil, nil); err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, current, err := platform.ProcessIdentity(pid)
			if err != nil || current != started {
				return nil
			}
		}
	}
}
