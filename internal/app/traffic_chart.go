package app

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"nativejet/internal/model"
)

func (a *App) trafficPanel(c *ui.Context, chartHeight float32) {
	p := palette(c)
	rates := a.trafficLabels()
	sample := a.currentTraffic()
	ui.Column(c.Key("traffic-panel")).FillWidth().Shrink(0).Padding(10, 12, 12).Gap(7).
		BorderWidth(1, 0, 0, 0).BorderColor(p.line).Role(ui.RoleGroup).Label("内核流量速率").Children(func() {
		iconHeading(c, metricsIcon, "流量速率", 12, 2)
		ui.Grid(c).FillWidth().ColumnTracks(ui.Fixed(38), ui.Fr(1), ui.Fr(1)).GapX(8).GapY(4).Children(func() {
			ui.Box(c)
			ui.Text(c, "实时速率").FontSize(10).TextColor(p.TextMuted).SingleLine()
			ui.Text(c, "本次累计").FontSize(10).TextColor(p.TextMuted).SingleLine().Tooltip("当前内核进程的累计流量")
			for _, direction := range [...]struct {
				name, rate string
				total      int64
				color      ui.Color
			}{{"上传", rates.up, sample.UpTotal, p.Accent}, {"下载", rates.down, sample.DownTotal, p.Success}} {
				total := "—"
				if sample.TotalsAvailable {
					total = trafficBytes(direction.total)
				}
				stateLabel(c, direction.name, direction.color)
				ui.Text(c, direction.rate).FontSize(11).FontWeight(600).SingleLine().
					Label(direction.name + "速度：" + direction.rate).Tooltip(direction.rate)
				ui.Text(c, total).FontSize(11).SingleLine().
					Label("累计" + direction.name + "：" + total).Tooltip(total)
			}
		})
		a.trafficChart(c.Key("traffic-plot"), chartHeight)
	})
}

func (a *App) trafficChart(c *ui.Context, height float32) {
	now := time.Now()
	var points []model.TrafficPoint
	if a.trafficClient == a.client {
		points = a.trafficHistory
	}
	trafficPlot(c, points, now.Add(-model.TrafficHistoryWindow), now, height)
}

func trafficPlot(c *ui.Context, points []model.TrafficPoint, start, end time.Time, height float32) {
	p := palette(c)
	var peak int64
	for _, point := range points {
		if !point.At.Before(start) && !point.At.After(end) {
			peak = max(peak, point.Up, point.Down)
		}
	}
	ceiling := max(peak, 1)
	xScale := chart.NewTime(start, end)
	yScale := chart.NewLinear(0, float64(ceiling))
	xs := make([]float32, 0, len(points))
	ups := make([]float32, 0, len(points))
	downs := make([]float32, 0, len(points))
	samples := make([]model.TrafficPoint, 0, len(points))
	var prev time.Time
	for _, point := range points {
		if point.At.Before(start) || point.At.After(end) {
			continue
		}
		if !prev.IsZero() && (point.BreakBefore || point.At.Before(prev)) {
			// MyGo's line paths use a NaN sample to separate runs.
			xs = append(xs, float32(math.NaN()))
			ups = append(ups, 0)
			downs = append(downs, 0)
		}
		xs = append(xs, xScale.At(point.At))
		ups = append(ups, yScale.At(float64(point.Up)))
		downs = append(downs, yScale.At(float64(point.Down)))
		samples = append(samples, point)
		prev = point.At
	}
	// Selection requires ascending positions and only real samples.
	slices.SortStableFunc(samples, func(a, b model.TrafficPoint) int { return a.At.Compare(b.At) })
	sampleXs := make([]float32, len(samples))
	for i, sample := range samples {
		sampleXs[i] = xScale.At(sample.At)
	}

	chart.Plot(c, chart.Axes{}, func(area ui.Element) {
		area.Center().Radius(4).Background(p.Surface).Border(1, p.line).Label("流量绘图区")
		selected := ui.Local(area, "traffic-sample", func() int { return -1 })
		if len(xs) == 0 {
			*selected = -1
			ui.Text(c, "暂无采样").FontSize(11).TextColor(p.TextMuted)
			return
		}
		area.Draw(func(draw *ui.Painter, r ui.Rect) {
			draw.StrokePath(chart.Line(r, xs, downs, chart.CurveLinear), 1.4, p.Success)
			draw.StrokePath(chart.Line(r, xs, ups, chart.CurveLinear), 1.4, p.Accent)
		})
		// Give samples with no adjacent point a visible mark.
		gap := func(i int) bool {
			return i < 0 || i >= len(xs) || math.IsNaN(float64(xs[i]))
		}
		for i, x := range xs {
			if !gap(i) && gap(i-1) && gap(i+1) {
				chart.Point(c, x, downs[i], 2, 2).Radius(1).Background(p.Success)
				chart.Point(c, x, ups[i], 2, 2).Radius(1).Background(p.Accent)
			}
		}
		chart.Select(c, area, selected, sampleXs, func(i int) string {
			sample := samples[i]
			return sample.At.Local().Format("15:04:05") + "，上传 " + trafficRate(sample.Up) + "，下载 " + trafficRate(sample.Down)
		})
		if i := *selected; i >= 0 {
			sample, x := samples[i], sampleXs[i]
			chart.RuleX(c, x, 1).Background(p.TextMuted.Alpha(.55)).PassThrough()
			chart.Point(c, x, yScale.At(float64(sample.Down)), 5, 5).Radius(2.5).Background(p.Success).PassThrough()
			chart.Point(c, x, yScale.At(float64(sample.Up)), 5, 5).Radius(2.5).Background(p.Accent).PassThrough()
			chart.Anchor(c, 1, 1, ui.End, ui.End, func() {
				ui.Column(c).Padding(4, 7).Gap(2).Radius(5).Background(p.Surface).Border(1, p.line).
					PassThrough().Children(func() {
					ui.Text(c, sample.At.Local().Format("15:04:05")).FontSize(10).TextColor(p.TextMuted).NoWrap()
					ui.Text(c, "↑ "+trafficRate(sample.Up)).FontSize(11).TextColor(p.Accent).NoWrap()
					ui.Text(c, "↓ "+trafficRate(sample.Down)).FontSize(11).TextColor(p.Success).NoWrap()
				})
			}).Translate(0, -4).PassThrough()
		}
	}).FillWidth().Height(height).Label("最近 5 分钟的上传和下载速率")
}
