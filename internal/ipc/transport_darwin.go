package ipc

import (
	"context"
	"errors"
	"fmt"
	"nativejet/internal/buildinfo"
	"nativejet/internal/platform"
	"net"
	"os"
	"time"
)

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return nil, fmt.Errorf("无法连接后台服务: %w", err)
	}
	u := conn.(*net.UnixConn)
	raw, err := u.SyscallConn()
	if err != nil {
		conn.Close()
		return nil, err
	}
	requirement := c.Requirement
	if !c.Development && requirement == "" {
		requirement, err = platform.Requirement(buildinfo.HelperID, c.TeamID)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}
	var uid int
	var auth error
	if err = raw.Control(func(fd uintptr) { uid, auth = platform.Peer(fd, requirement) }); err != nil {
		conn.Close()
		return nil, err
	}
	expected := 0
	if c.Development {
		expected = os.Getuid()
	}
	if auth != nil || uid != expected {
		conn.Close()
		return nil, errors.New("后台服务身份验证失败")
	}
	return conn, nil
}
