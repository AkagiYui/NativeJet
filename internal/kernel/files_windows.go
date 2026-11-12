package kernel

import (
	"errors"
	"os"
	"syscall"
)

func checkManagedInfo(info os.FileInfo, _ bool) error {
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); !ok || data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("受管理内核文件或目录不能为重解析点")
	}
	// System-service directories inherit the private ACL installed by the installer.
	return nil
}
