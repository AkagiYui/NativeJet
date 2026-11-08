package app

import (
	"github.com/egoist/mygo/ui"
	"nativejet/internal/ipc"
	"nativejet/internal/model"
)

const geoDataPage = "GeoData"

type geoResult struct {
	client *ipc.Client
	data   model.GeoData
	err    error
}

func geoPID(status model.Status) int {
	if status.Running {
		return status.PID
	}
	return 0
}

func (a *App) geoUpdateAction(c *ui.Context) {
	result := a.geoData
	ready := a.coreRunning() && a.snapshot.Status.GeoData && result != nil &&
		result.client == a.client && result.err == nil && result.data.SourceError == "" &&
		result.data.PID == a.snapshot.Status.PID
	if action(c, downloadIcon, "立即更新", true).Disabled(a.busy || !ready).Clicked() {
		a.request("geo.update", map[string]int{"pid": result.data.PID}, "Mihomo 已完成 GeoData 更新检查", nil)
	}
}

var geoLabels = map[string]string{
	"mmdb":    "GeoIP · MMDB",
	"geoip":   "GeoIP · DAT",
	"geosite": "GeoSite",
	"asn":     "ASN",
}

func (a *App) geoDataView(c *ui.Context) {
	workspaceHeader(c, geoDataIcon, geoDataPage, func() { a.geoUpdateAction(c) })
	switch {
	case !a.connected:
		emptyState(c, geoDataIcon, "后台服务未连接", "连接后即可读取 GeoData。", nil)
		return
	case !a.snapshot.Status.GeoData:
		emptyState(c, geoDataIcon, "后台服务需要更新", "请在「设置」中更新后台服务以启用 GeoData。", nil)
		return
	case a.geoData == nil || a.geoData.client != a.client:
		emptyState(c, geoDataIcon, "正在读取 GeoData", "", nil)
		return
	case a.geoData.err != nil:
		emptyState(c, geoDataIcon, "无法读取 GeoData", a.geoData.err.Error(), nil)
		return
	}

	data := a.geoData.data
	commandRow(c, func() {
		switch {
		case data.SourceError != "":
			muted(c, data.SourceError)
		case data.DatMode == nil:
			muted(c, "内核未报告 GeoIP 格式")
		default:
			format := "MMDB"
			if *data.DatMode {
				format = "DAT"
			}
			muted(c, "当前 GeoIP 格式 · "+format)
		}
	})
	detailScroll(c, func() {
		property(c, "内核运行目录", a.snapshot.Status.RuntimeDirectory)
		muted(c, "下载源由运行中的 Mihomo 报告。本地文件可能来自先前的下载。")
		if a.coreRunning() && data.SourceError != "" {
			muted(c, "如需启用控制接口，可在覆盖配置中添加 external-controller-unix: mihomo.sock 并应用。")
		}
		if data.FileError != "" {
			muted(c, "无法检查运行目录 · "+data.FileError)
		}
		for _, file := range data.Files {
			ui.Divider(c)
			ui.Text(c, geoLabels[file.Kind]).FontSize(13).FontWeight(600)
			source := file.URL
			if source == "" && data.SourceError == "" {
				source = "内核未报告"
			}
			property(c, "生效下载源", source)
			fileStatus := "状态不可用"
			if data.FileError == "" {
				fileStatus = geoFileLabel(file)
			}
			property(c, "本地文件", fileStatus)
		}
	})
}

func geoFileLabel(file model.GeoFile) string {
	switch {
	case file.Name == "":
		return "未找到文件"
	case file.Problem != "":
		return file.Name + " · " + file.Problem
	case file.Size == 0:
		return file.Name + " · 空文件 · 修改于 " + file.ModifiedAt.Local().Format("2006-01-02 15:04:05")
	default:
		return file.Name + " · " + bytesString(file.Size) + " · 修改于 " + file.ModifiedAt.Local().Format("2006-01-02 15:04:05")
	}
}
