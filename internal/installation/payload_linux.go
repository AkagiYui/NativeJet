package installation

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const installedExecutableMode = 0555
const payloadOpenFlags = unix.O_NOFOLLOW | unix.O_NONBLOCK

func prepareIdentity(_ string, payload *Payload) error {
	payload.OwnerUID = os.Getuid()
	return nil
}

func privateStaging(_ string, info os.FileInfo) bool { return info.Mode().Perm()&0077 == 0 }

func validPayloadOwner(_ string, info os.FileInfo, payload Payload) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return payload.OwnerUID > 0 && ok && stat.Uid == uint32(payload.OwnerUID)
}

func manifestFileKey(path string) string { return path }
func secureStaging(string) error         { return nil }
