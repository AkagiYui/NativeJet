package installation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"nativejet/internal/platform"
)

var (
	programFiles, programFilesErr = windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	programData, programDataErr   = windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	installRoot                   = filepath.Join(programFiles, "NativeJet")
	dataRoot                      = filepath.Join(programData, "NativeJet")
	RegistrationPath              = filepath.Join(installRoot, "service.json")
)

const privateACL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
const installACL = privateACL + "(A;OICI;FRFX;;;BU)"
const dataRootACL = privateACL + "(A;;FRFX;;;BU)"

type Registration struct {
	Version           int      `json:"version"`
	OwnerSID          string   `json:"ownerSid"`
	HelperSHA256      string   `json:"helperSha256"`
	HelperRequirement string   `json:"-"`
	ApprovedKernels   []string `json:"approvedKernels"`
}

func validOwner(value string) bool {
	sid, err := windows.StringToSid(value)
	if err != nil || sid.String() != value {
		return false
	}
	return strings.HasPrefix(value, "S-1-5-21-") || strings.HasPrefix(value, "S-1-12-1-")
}

func (r Registration) HelperPath() string {
	return filepath.Join(installRoot, "helper-"+r.HelperSHA256+".exe")
}

func (r Registration) Data() string { return filepath.Join(dataRoot, "users", r.OwnerSID) }
func (r Registration) Inbox() string {
	return filepath.Join(dataRoot, "kernels-"+r.OwnerSID)
}

func (r Registration) inboxACL() string {
	// The owner can list/create files and modify contents. The directory itself
	// remains administrator-owned and cannot be replaced by the owner.
	return privateACL + "(A;;0x1200af;;;" + r.OwnerSID + ")(A;OICIIO;0x1301bf;;;" + r.OwnerSID + ")"
}

func LoadRegistration() (Registration, error) {
	var r Registration
	if err := errors.Join(programFilesErr, programDataErr); err != nil {
		return r, err
	}
	if err := checkProtectedDirectory(installRoot, installACL); err != nil {
		return r, err
	}
	info, err := os.Lstat(RegistrationPath)
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return r, errors.New("后台服务注册记录无效")
	}
	data, err := os.ReadFile(RegistrationPath)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(data, &r); err != nil {
		return r, err
	}
	if r.Version != 1 || !validOwner(r.OwnerSID) || !digestPattern.MatchString(r.HelperSHA256) {
		return r, errors.New("后台服务注册身份无效")
	}
	for _, id := range r.ApprovedKernels {
		if !digestPattern.MatchString(id) {
			return r, errors.New("内核授权记录无效")
		}
	}
	r.HelperRequirement = r.HelperSHA256
	return r, nil
}

func realDirectory(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attr, err := windows.GetFileAttributes(name)
	if err != nil {
		return err
	}
	if attr&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("后台服务目录必须是真实目录")
	}
	return nil
}

func checkProtectedDirectory(path, acl string) error {
	if err := realDirectory(path); err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	_, actual, ok := strings.Cut(sd.String(), "D:")
	if !ok || !(owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid)) {
		return errors.New("后台服务目录所有者不可信")
	}
	want, err := windows.SecurityDescriptorFromString(acl)
	if err != nil {
		return err
	}
	_, expected, _ := strings.Cut(want.String(), "D:")
	if strings.ReplaceAll(actual, "PAI(", "P(") != expected {
		return errors.New("后台服务目录 ACL 与安装策略不一致: " + path)
	}
	return nil
}

func protectDirectory(path, acl string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("O:BA" + acl)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: sd}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	if err = windows.CreateDirectory(name, &attributes); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	return checkProtectedDirectory(path, acl)
}

func (r Registration) VerifyServiceDirectories() error {
	for _, dir := range []struct{ path, acl string }{
		{installRoot, installACL}, {dataRoot, dataRootACL}, {filepath.Join(dataRoot, "users"), privateACL},
		{r.Data(), privateACL}, {r.Inbox(), r.inboxACL()},
	} {
		if err := checkProtectedDirectory(dir.path, dir.acl); err != nil {
			return err
		}
	}
	return nil
}

func (r Registration) VerifyOwner() error {
	sid, err := platform.CurrentSID()
	if err != nil {
		return err
	}
	if sid != r.OwnerSID {
		return errors.New("后台服务已授权给另一位 Windows 用户")
	}
	return nil
}
