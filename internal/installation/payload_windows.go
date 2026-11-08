package installation

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"

	"nativejet/internal/platform"
)

const installedExecutableMode = 0700
const payloadOpenFlags = os.O_RDONLY

func prepareIdentity(_ string, payload *Payload) error {
	sid, err := platform.CurrentSID()
	payload.OwnerSID = sid
	return err
}

func privateStaging(directory string, _ os.FileInfo) bool { return realDirectory(directory) == nil }

func validPayloadOwner(directory string, _ os.FileInfo, payload Payload) bool {
	if !validOwner(payload.OwnerSID) {
		return false
	}
	sd, err := windows.GetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return false
	}
	if owner.String() == payload.OwnerSID {
		return true
	}
	current, err := platform.CurrentSID()
	return err == nil && current == payload.OwnerSID && owner.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func manifestFileKey(path string) string { return strings.ToLower(path) }

func secureStaging(dir string) error {
	sid, err := platform.CurrentSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(privateACL + "(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
