package platform

import (
	"net"

	"golang.org/x/sys/unix"
)

func PeerCredentials(conn *net.UnixConn) (*unix.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var peerErr error
	if err = raw.Control(func(fd uintptr) { cred, peerErr = peerCredentials(fd) }); err != nil {
		return nil, err
	}
	return cred, peerErr
}

func peerCredentials(fd uintptr) (*unix.Ucred, error) {
	return unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
}

func PeerPID(fd uintptr) (int, error) {
	cred, err := peerCredentials(fd)
	if err != nil {
		return 0, err
	}
	return int(cred.Pid), nil
}
