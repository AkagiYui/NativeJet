package kernel

import (
	"context"
	"debug/macho"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"

	"nativejet/internal/model"
)

const Executable = "mihomo"
const kernelOpenFlags = unix.O_NOFOLLOW | unix.O_NONBLOCK

func (m *Manager) probe(_ context.Context, info os.FileInfo, reader io.ReaderAt, item *model.Kernel) {
	image, err := machoImage(reader)
	if err != nil {
		item.Error = err.Error()
		return
	}
	if info.Mode()&0111 == 0 {
		item.Error = "缺少执行权限：请在 Finder 或终端赋予文件执行权限"
		return
	}
	inspectBuildInfo(image, item)
}

// Select the image macOS will execute. Metadata from another architecture in
// a universal binary must not identify the native image.
func machoImage(reader io.ReaderAt) (io.ReaderAt, error) {
	valid := func(f *macho.File) bool {
		return runtime.GOARCH == "arm64" && f.Cpu == macho.CpuArm64 || runtime.GOARCH == "amd64" && f.Cpu == macho.CpuAmd64
	}
	if f, err := macho.NewFile(reader); err == nil {
		if f.Type != macho.TypeExec {
			return nil, errors.New("内核必须为 macOS 可执行文件")
		}
		if !valid(f) {
			return nil, errors.New("内核架构与本机不匹配")
		}
		return reader, nil
	}
	fat, err := macho.NewFatFile(reader)
	if err != nil {
		return nil, errors.New("不是可识别的 macOS Mach-O 可执行文件")
	}
	for _, arch := range fat.Arches {
		if valid(arch.File) {
			if arch.Type != macho.TypeExec {
				return nil, errors.New("内核必须为 macOS 可执行文件")
			}
			return io.NewSectionReader(reader, int64(arch.Offset), int64(arch.Size)), nil
		}
	}
	return nil, errors.New("通用内核不包含本机架构")
}

func releaseAsset(name string) bool {
	return strings.HasPrefix(name, "mihomo-darwin-"+runtime.GOARCH) && strings.HasSuffix(name, ".gz") && !strings.Contains(name, "alpha") && (runtime.GOARCH != "amd64" || strings.Contains(name, "compatible"))
}
