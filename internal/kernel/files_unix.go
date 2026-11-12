//go:build darwin || linux

package kernel

import (
	"errors"
	"os"
	"syscall"
)

func checkManagedInfo(info os.FileInfo, executable bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("受管理内核文件或目录不属于当前服务用户")
	}
	if info.Mode().Perm()&0022 != 0 || executable && info.Mode().Perm()&0222 != 0 {
		return errors.New("受管理内核文件或目录可被写入，请重新选用内核")
	}
	if executable && info.Mode().Perm()&0111 == 0 {
		return errors.New("受管理内核缺少执行权限")
	}
	return nil
}
