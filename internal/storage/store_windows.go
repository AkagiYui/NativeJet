package storage

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func RuntimeDirectory(data string) string { return filepath.Join(data, "runtime") }

func checkDirectory(dir string, _ os.FileInfo) error {
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(path)
	if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("数据目录不能为重解析点")
	}
	return err
}

func replaceFile(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

const DefaultOverride = "# 所有影响内核的默认设置均在此处可见。\nmixed-port: 7897\nallow-lan: false\nlog-level: info\nexternal-controller-pipe: '\\\\.\\pipe\\NativeJet-Mihomo'\ntun:\n  enable: true\n  stack: gvisor\n  auto-route: true\n  auto-detect-interface: true\n"
