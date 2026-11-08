package service

import (
	"context"
	"errors"
	"net"
	"os"

	"nativejet/internal/platform"
)

func (s *Server) managerOptions(_ int) (Options, error) {
	if s.Data == "" || s.Inbox == "" {
		return Options{}, errors.New("未指定后台服务数据目录")
	}
	return Options{Data: s.Data, Inbox: s.Inbox, Privileged: s.Installed, ApprovedKernels: s.ApprovedKernels}, nil
}

func (s *Server) listen(ctx context.Context) (net.Listener, func(), error) {
	if !s.Development && (!s.Installed || s.ApprovedKernels == nil || s.OwnerSID == "") {
		return nil, nil, errors.New("系统服务缺少授权记录")
	}
	listener, err := platform.ListenPipe(s.Socket, s.OwnerSID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.manager(ctx, 0); err != nil {
		_ = listener.Close()
		return nil, nil, err
	}
	return listener, func() { _ = listener.Close() }, nil
}

func servicePrivileged(o Options) bool { return o.Privileged }
func prepareInbox(o Options) error     { return os.MkdirAll(o.Inbox, 0700) }
