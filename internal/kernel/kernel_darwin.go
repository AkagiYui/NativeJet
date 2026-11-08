package kernel

import (
	"context"
	"debug/macho"
	"errors"
	"nativejet/internal/model"
	"nativejet/internal/platform"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const Executable = "mihomo"
const kernelOpenFlags = os.O_RDONLY

func (m *Manager) probe(ctx context.Context, info os.FileInfo, _ *os.File, item *model.Kernel) {
	path := item.Path
	var err error
	if err = validateExecutable(path); err != nil {
		item.Error = err.Error()
		return
	}
	if info.Mode()&0111 == 0 {
		item.Error = "缺少执行权限：请在 Finder 或终端赋予文件执行权限"
		return
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	cmd := exec.CommandContext(probe, path, "-v")
	cmd.SysProcAttr = platform.Credentials(m.UID, m.GID)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + m.Inbox}
	var buf boundedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	cancel()
	text := strings.TrimSpace(buf.String())
	if err != nil {
		item.Error = "无法读取版本: " + err.Error()
	} else if !strings.Contains(strings.ToLower(text), "mihomo") && !strings.Contains(text, "Clash.Meta") {
		item.Error = "首版支持 Mihomo / Clash.Meta 内核"
	} else {
		item.Version = strings.Split(text, "\n")[0]
	}
	if len(item.Version) > 180 {
		item.Version = item.Version[:180]
	}

}
func validateExecutable(path string) error {
	valid := func(f *macho.File) bool {
		return runtime.GOARCH == "arm64" && f.Cpu == macho.CpuArm64 || runtime.GOARCH == "amd64" && f.Cpu == macho.CpuAmd64
	}
	f, err := macho.Open(path)
	if err == nil {
		defer f.Close()
		if valid(f) {
			return nil
		}
		return errors.New("内核架构与本机不匹配")
	}
	fat, err := macho.OpenFat(path)
	if err != nil {
		return errors.New("不是可识别的 macOS Mach-O 可执行文件")
	}
	defer fat.Close()
	for _, arch := range fat.Arches {
		if valid(arch.File) {
			return nil
		}
	}
	return errors.New("通用内核不包含本机架构")
}

func releaseAsset(name string) bool {
	return strings.HasPrefix(name, "mihomo-darwin-"+runtime.GOARCH) && strings.HasSuffix(name, ".gz") && !strings.Contains(name, "alpha") && (runtime.GOARCH != "amd64" || strings.Contains(name, "compatible"))
}
