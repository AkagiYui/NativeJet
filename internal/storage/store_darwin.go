package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func RuntimeDirectory(data string) string {
	dir := filepath.Join(data, "runtime")
	// Leave room for local controller names within the Unix socket path limit.
	if len(dir) > 84 {
		sum := sha256.Sum256([]byte(data))
		dir = filepath.Join(os.TempDir(), "nativejet-"+hex.EncodeToString(sum[:8]))
	}
	return dir
}

func checkDirectory(dir string, info os.FileInfo) error {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != uint32(os.Geteuid()) {
		return errors.New("数据目录不属于当前服务用户，拒绝接管")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	return nil
}
func replaceFile(source, target string) error { return os.Rename(source, target) }

const DefaultOverride = "# 所有影响内核的默认设置均在此处可见。\nmixed-port: 7897\nallow-lan: false\nlog-level: info\ntun:\n  enable: true\n  stack: mixed\n  auto-route: true\n  auto-detect-interface: true\n"
