package installation

import (
	"os"
	"syscall"

	"nativejet/internal/platform"
)

const installedExecutableMode = 0555
const payloadOpenFlags = os.O_RDONLY

func prepareIdentity(path string, payload *Payload) error {
	requirement, err := platform.SigningRequirement(path)
	payload.OwnerUID = os.Getuid()
	payload.HelperRequirement = requirement
	return err
}

func privateStaging(_ string, info os.FileInfo) bool { return info.Mode().Perm()&0077 == 0 }

func validPayloadOwner(_ string, info os.FileInfo, payload Payload) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return payload.OwnerUID > 0 && ok && stat.Uid == uint32(payload.OwnerUID)
}

func manifestFileKey(path string) string { return path }

func secureStaging(string) error { return nil }
