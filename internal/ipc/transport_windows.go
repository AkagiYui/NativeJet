package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"nativejet/internal/buildinfo"
	"nativejet/internal/platform"
)

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, _, err := c.verifiedPipe(ctx)
	return conn, err
}

func (c *Client) verifiedPipe(ctx context.Context) (net.Conn, uint32, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := platform.DialPipe(ctx, c.Socket)
	if err != nil {
		return nil, 0, fmt.Errorf("无法连接后台服务: %w", err)
	}
	pid, err := platform.PipeServerPID(conn)
	if err == nil && pid != 0 {
		if c.Development {
			sid, userErr := platform.CurrentSID()
			peer, peerErr := platform.ProcessSID(pid)
			if userErr == nil && peerErr == nil && sid == peer {
				if c.Requirement == "" {
					return conn, pid, nil
				}
				path, _, identityErr := platform.ProcessIdentity(int(pid))
				if identityErr == nil && platform.SamePath(path, c.Requirement) {
					return conn, pid, nil
				}
			}
		} else if service, scmErr := platform.ServicePID(buildinfo.HelperID); scmErr == nil && service == pid {
			return conn, pid, nil
		}
	}
	_ = conn.Close()
	return nil, 0, errors.New("后台服务身份验证失败")
}

func (c *Client) StopDevelopmentHelper(ctx context.Context, expectedPath string) error {
	if !c.Development {
		return errors.New("不允许直接停止系统 helper")
	}
	conn, pid, err := c.verifiedPipe(ctx)
	if err != nil {
		return err
	}
	_ = conn.Close()
	path, started, err := platform.ProcessIdentity(int(pid))
	if err != nil || !platform.SamePath(path, expectedPath) {
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
			_, current, e := platform.ProcessIdentity(int(pid))
			if errors.Is(e, os.ErrProcessDone) || e != nil || current != started {
				return nil
			}
		}
	}
}
