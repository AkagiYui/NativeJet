package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"nativejet/internal/platform"
)

// StopDevelopmentHelper only signals the ordinary-user helper identified by
// the kernel's peer credentials and the expected executable/start timestamp.
func (c *Client) StopDevelopmentHelper(ctx context.Context, expectedPath string) error {
	if !c.Development {
		return errors.New("不允许直接停止系统 helper")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err != nil {
		return err
	}
	var uid, pid int
	var auth error
	if err = raw.Control(func(fd uintptr) {
		uid, auth = platform.Peer(fd, "")
		if auth == nil {
			pid, auth = platform.PeerPID(fd)
		}
	}); err != nil {
		return err
	}
	if auth != nil || uid != os.Getuid() {
		return errors.New("开发 helper 身份不符")
	}
	path, stamp, err := platform.ProcessIdentity(pid)
	if err != nil {
		return err
	}
	expected, err := filepath.EvalSymlinks(expectedPath)
	if err != nil {
		return err
	}
	if path != expected {
		return errors.New("连接并非当前 NativeJet 的开发 helper")
	}
	again, started, err := platform.ProcessIdentity(pid)
	if err != nil || again != path || started != stamp {
		return errors.New("开发 helper 进程身份已变化")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err = process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		current, start, e := platform.ProcessIdentity(pid)
		if e != nil || current != path || start != stamp {
			return nil
		}
	}
}
