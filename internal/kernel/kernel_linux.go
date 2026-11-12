package kernel

import (
	"context"
	"debug/elf"
	"errors"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"

	"nativejet/internal/model"
)

const Executable = "mihomo"
const kernelOpenFlags = unix.O_NOFOLLOW | unix.O_NONBLOCK

var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-.][a-zA-Z0-9.-]+)?$`)

func (m *Manager) probe(_ context.Context, info os.FileInfo, reader io.ReaderAt, item *model.Kernel) {
	if err := validateELF(reader); err != nil {
		item.Error = err.Error()
		return
	}
	if info.Mode().Perm()&0111 == 0 {
		item.Error = "缺少执行权限：请对内核文件执行 chmod +x"
		return
	}
	inspectBuildInfo(reader, item)
}

func validateELF(reader io.ReaderAt) error {
	f, err := elf.NewFile(reader)
	if err != nil || f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB ||
		(f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN) || f.Entry == 0 ||
		(f.OSABI != elf.ELFOSABI_NONE && f.OSABI != elf.ELFOSABI_LINUX) {
		return errors.New("不是可识别的 Linux ELF 可执行文件")
	}
	if !(runtime.GOARCH == "amd64" && f.Machine == elf.EM_X86_64 ||
		runtime.GOARCH == "arm64" && f.Machine == elf.EM_AARCH64) {
		return errors.New("内核架构与 NativeJet 不匹配")
	}
	return nil
}

func releaseAsset(name string) bool {
	stem, ok := strings.CutSuffix(name, ".gz")
	if !ok {
		return false
	}
	prefixes := []string{"mihomo-linux-arm64-"}
	if runtime.GOARCH == "amd64" {
		prefixes = []string{"mihomo-linux-amd64-v1-", "mihomo-linux-amd64-compatible-"}
	}
	for _, prefix := range prefixes {
		if version, ok := strings.CutPrefix(stem, prefix); ok && releaseVersion.MatchString(version) {
			return true
		}
	}
	return false
}
