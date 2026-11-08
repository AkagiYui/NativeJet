package kernel

import (
	"archive/zip"
	"bytes"
	"context"
	"debug/buildinfo"
	"debug/pe"
	"errors"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"nativejet/internal/model"
	"nativejet/internal/storage"
)

const Executable = "mihomo.exe"
const kernelOpenFlags = os.O_RDONLY

var releaseVersion = regexp.MustCompile(`-v[0-9]+\.[0-9]+\.[0-9]+(?:[-.][a-zA-Z0-9.-]+)?\.exe$`)

func (m *Manager) probe(_ context.Context, _ os.FileInfo, file *os.File, item *model.Kernel) {
	if err := validatePE(file, item.Name); err != nil {
		item.Error = err.Error()
		return
	}
	// A SYSTEM service must never execute an unapproved binary to identify it.
	info, err := buildinfo.Read(file)
	if err != nil || !(strings.EqualFold(info.Path, "github.com/metacubex/mihomo") ||
		strings.EqualFold(info.Path, "github.com/metacubex/clash.meta")) {
		item.Error = "未找到 Mihomo / Clash.Meta 的 Go 构建信息"
		return
	}
	item.Version = "Mihomo · Windows " + item.Arch
	if version := releaseVersion.FindString(item.Name); version != "" {
		item.Version = "Mihomo " + strings.TrimSuffix(version[1:], ".exe") + "（文件名）"
	} else if info.Main.Version != "" && info.Main.Version != "(devel)" {
		item.Version = "Mihomo " + info.Main.Version
	}
}

func validateExecutable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return validatePE(f, path)
}

func validatePE(reader io.ReaderAt, name string) error {
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return errors.New("Windows 内核必须为 .exe 文件")
	}
	f, err := pe.NewFile(reader)
	if err != nil {
		return errors.New("无法识别 Windows PE 可执行文件")
	}
	defer f.Close()
	machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
	if runtime.GOARCH == "arm64" {
		machine = pe.IMAGE_FILE_MACHINE_ARM64
	}
	if f.Machine != machine {
		return errors.New("内核架构与 NativeJet 不匹配")
	}
	if f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return errors.New("内核必须为 Windows 可执行文件")
	}
	return nil
}

func releaseAsset(name string) bool {
	if !strings.HasSuffix(name, ".zip") {
		return false
	}
	prefix := "mihomo-windows-amd64-v1-"
	if runtime.GOARCH == "arm64" {
		prefix = "mihomo-windows-arm64-"
	}
	suffix, ok := strings.CutPrefix(strings.TrimSuffix(name, ".zip"), prefix)
	return ok && releaseVersion.FindString("-"+suffix+".exe") == "-"+suffix+".exe"
}

func unpack(data []byte, name string) ([]byte, string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", err
	}
	if len(z.File) != 1 {
		return nil, "", errors.New("官方内核 ZIP 必须仅包含一个 EXE")
	}
	file := z.File[0]
	if !file.Mode().IsRegular() || !strings.HasPrefix(file.Name, "mihomo-windows-") ||
		!strings.HasSuffix(file.Name, ".exe") || strings.ContainsAny(file.Name, `/\:`) ||
		file.UncompressedSize64 > MaxBinary {
		return nil, "", errors.New("内核 ZIP 中的文件无效")
	}
	f, err := file.Open()
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	binary, err := io.ReadAll(io.LimitReader(f, MaxBinary+1))
	if err != nil {
		return nil, "", err
	}
	if len(binary) > MaxBinary {
		return nil, "", errors.New("解压后的内核过大")
	}
	return binary, strings.TrimSuffix(name, ".zip") + ".exe", nil
}

func (m *Manager) setDownloadOwner(*os.File) error   { return nil }
func writeExecutable(path string, data []byte) error { return storage.Atomic(path, data, 0700) }
