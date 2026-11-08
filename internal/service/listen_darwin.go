package service

import (
	"context"
	"errors"
	"fmt"
	"nativejet/internal/buildinfo"
	"nativejet/internal/platform"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"
)

type authenticatedListener struct {
	net.Listener
	requirement string
	development bool
	installed   bool
}

func (l *authenticatedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		u, ok := conn.(*net.UnixConn)
		if !ok {
			conn.Close()
			continue
		}
		raw, err := u.SyscallConn()
		if err != nil {
			conn.Close()
			continue
		}
		var uid int
		var auth error
		err = raw.Control(func(fd uintptr) { uid, auth = platform.Peer(fd, l.requirement) })
		if err != nil || auth != nil || (l.development && uid != os.Getuid()) || (!l.development && !l.installed && uid == 0) {
			conn.Close()
			continue
		}
		return &authenticatedConn{Conn: conn, uid: uid}, nil
	}
}

func (s *Server) managerOptions(uid int) (Options, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return Options{}, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Options{}, err
	}
	data := s.Data
	inbox := s.Inbox
	if data == "" {
		data = filepath.Join("/Library/Application Support/NativeJet/users", strconv.Itoa(uid))
	}
	if inbox == "" {
		inbox = filepath.Join(u.HomeDir, "Library/Application Support/NativeJet/kernels")
	}
	return Options{Data: data, Inbox: inbox, UID: uid, GID: gid, ApprovedKernels: s.ApprovedKernels}, nil
}
func (s *Server) listen(ctx context.Context) (net.Listener, func(), error) {
	requirement := s.PeerRequirement
	if !s.Development {
		if os.Geteuid() != 0 {
			return nil, nil, errors.New("生产 helper 必须由已授权的 LaunchDaemon 启动")
		}
		if s.Installed {
			if s.ApprovedKernels == nil {
				return nil, nil, errors.New("已安装服务缺少内核授权记录")
			}
			requirement = ""
		} else if requirement == "" {
			var err error
			requirement, err = platform.Requirement(buildinfo.AppID, s.TeamID)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if s.Socket == "" {
		return nil, nil, errors.New("未指定 IPC socket")
	}
	if _, err := os.Lstat(s.Socket); err == nil {
		conn, err := net.DialTimeout("unix", s.Socket, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil, nil, errors.New("后台服务已经运行")
		}
		if err = os.Remove(s.Socket); err != nil {
			return nil, nil, err
		}
	}
	listener, err := net.Listen("unix", s.Socket)
	if err != nil {
		return nil, nil, err
	}
	mode := os.FileMode(0666)
	if s.Development {
		mode = 0600
	}
	if err = os.Chmod(s.Socket, mode); err != nil {
		listener.Close()
		_ = os.Remove(s.Socket)
		return nil, nil, err
	}
	// Reconcile every persisted owner before accepting another user's start
	// request; an old core can outlive an unexpected helper termination.
	if !s.Development {
		entries, _ := os.ReadDir("/Library/Application Support/NativeJet/users")
		for _, entry := range entries {
			uid, err := strconv.Atoi(entry.Name())
			if err != nil || uid <= 0 || !entry.IsDir() {
				continue
			}
			if _, err := s.manager(ctx, uid); err != nil {
				s.recoveryError = fmt.Sprintf("无法恢复已有用户 %d 的后台状态: %v", uid, err)
			}
		}
	}

	return &authenticatedListener{Listener: listener, requirement: requirement, development: s.Development, installed: s.Installed}, func() { _ = listener.Close(); _ = os.Remove(s.Socket) }, nil
}
