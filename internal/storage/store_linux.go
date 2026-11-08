package storage

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func RuntimeDirectory(data string) string {
	dir := filepath.Join(data, "runtime")
	if len(filepath.Join(dir, "mihomo.sock")) < 108 {
		return dir
	}
	sum := sha256.Sum256([]byte(data))
	return filepath.Join(os.TempDir(), fmt.Sprintf("nativejet-%d-%x", os.Geteuid(), sum[:8]))
}

func checkDirectory(dir string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("数据目录不属于当前服务用户: %s", dir)
	}
	return os.Chmod(dir, 0700)
}

func replaceFile(source, target string) error { return os.Rename(source, target) }

const DefaultOverride = "# 所有影响内核的默认设置均在此处可见。\nmixed-port: 7897\nallow-lan: false\nlog-level: info\nexternal-controller-unix: mihomo.sock\ntun:\n  enable: true\n  stack: mixed\n  auto-route: true\n  auto-detect-interface: true\n"
