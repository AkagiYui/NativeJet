package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"nativejet/internal/platform"
)

type authenticatedListener struct {
	net.Listener
	uid int
}

func (l *authenticatedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		cred, err := platform.PeerCredentials(conn.(*net.UnixConn))
		if err == nil && cred.Uid == uint32(l.uid) && cred.Pid > 0 {
			return &authenticatedConn{Conn: conn, uid: l.uid}, nil
		}
		_ = conn.Close()
	}
}

func (s *Server) managerOptions(uid int) (Options, error) {
	if uid != s.OwnerUID || s.Data == "" || s.Inbox == "" {
		return Options{}, errors.New("未指定已授权用户或后台服务数据目录")
	}
	return Options{Data: s.Data, Inbox: s.Inbox, UID: uid, GID: s.OwnerGID,
		Privileged: s.Installed, ApprovedKernels: s.ApprovedKernels}, nil
}

func (s *Server) listen(ctx context.Context) (net.Listener, func(), error) {
	if s.Development {
		if os.Geteuid() == 0 || s.OwnerUID != os.Geteuid() {
			return nil, nil, errors.New("普通用户 helper 必须以当前登录用户运行")
		}
	} else if os.Geteuid() != 0 || !s.Installed || s.OwnerUID <= 0 || s.ApprovedKernels == nil {
		return nil, nil, errors.New("系统服务缺少授权记录")
	}
	if !filepath.IsAbs(s.Socket) {
		return nil, nil, errors.New("必须指定 IPC socket 的绝对路径")
	}
	dir := filepath.Dir(s.Socket)
	if s.Development {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, nil, err
		}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		return nil, nil, errors.New("IPC socket 目录所有者或权限不正确")
	}

	// Serialize replacement of a stale socket. Closing the lock without
	// deleting it keeps concurrent starters on the same inode.
	lock, err := unix.Open(s.Socket+".lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (net.Listener, func(), error) {
		_ = unix.Close(lock)
		return nil, nil, err
	}
	if err = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fail(fmt.Errorf("后台服务 socket 已被占用: %w", err))
	}
	if info, err = os.Lstat(s.Socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fail(errors.New("IPC socket 路径被其他文件占用"))
		}
		if err = os.Remove(s.Socket); err != nil {
			return fail(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}

	listener, err := net.Listen("unix", s.Socket)
	if err != nil {
		return fail(err)
	}
	cleanup := func() { _ = listener.Close(); _ = unix.Close(lock) }
	if err = os.Chmod(s.Socket, 0600); err == nil && s.Installed {
		err = os.Chown(s.Socket, s.OwnerUID, s.OwnerGID)
	}
	if err == nil {
		_, err = s.manager(ctx, s.OwnerUID)
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return &authenticatedListener{Listener: listener, uid: s.OwnerUID}, cleanup, nil
}

func servicePrivileged(o Options) bool { return o.Privileged }
func prepareInbox(o Options) error     { return os.MkdirAll(o.Inbox, 0700) }
