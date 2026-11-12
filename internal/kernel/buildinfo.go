package kernel

import (
	"debug/buildinfo"
	"io"
	"regexp"
	"strings"

	"nativejet/internal/model"
)

const UnknownVersion = model.KernelVersionUnknown

var fileVersion = regexp.MustCompile(`-v[0-9]+\.[0-9]+\.[0-9]+(?:[-.][a-zA-Z0-9.-]+)?$`)

// Build metadata identifies a candidate; it does not establish its provenance.
// Never execute a file to fill in missing metadata.
func inspectBuildInfo(reader io.ReaderAt, item *model.Kernel) {
	item.Version = UnknownVersion
	info, err := buildinfo.Read(reader)
	if err != nil || !(strings.EqualFold(info.Path, "github.com/metacubex/mihomo") ||
		strings.EqualFold(info.Path, "github.com/metacubex/clash.meta")) {
		item.Error = "未找到 Mihomo / Clash.Meta 的 Go 构建信息"
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		item.Version = "Mihomo " + info.Main.Version
	} else if version := fileVersion.FindString(strings.TrimSuffix(item.Name, ".exe")); version != "" {
		item.Version = "Mihomo " + version[1:] + "（文件名）"
	}
}
