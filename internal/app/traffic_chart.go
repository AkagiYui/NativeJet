package app

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"nativejet/internal/model"
)

func (a *App) trafficPanel(c *ui.Context) {
	p := palette(c)
	rates := a.trafficLabels()
	now := time.Now()
	start := now.Add(-model.TrafficHistoryWindow)
	var points []model.TrafficPoint
	if a.trafficClient == a.client {
		points = a.trafficHistory
		for len(points) > 0 && points[0].At.Before(start) {
			points = points[1:]
		}
	}
	ui.Column(c.Key("traffic-panel")).FillWidth().Shrink(0).Padding(10, 12, 12).Gap(7).
		BorderWidth(1, 0, 0, 0).BorderColor(p.line).Role(ui.RoleGroup).Label("内核流量速率").Children(func() {
		ui.Text(c, "流量速率").FontSize(12).FontWeight(600)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			for _, rate := range [...]struct {
				name, value string
				color       ui.Color
			}{{"上传", rates.up, p.Accent}, {"下载", rates.down, p.Success}} {
				ui.Column(c).Basis(0).Grow(1).MinWidth(0).Gap(2).Children(func() {
					stateLabel(c, rate.name, rate.color)
					ui.Text(c, rate.value).FontSize(12).FontWeight(600).SingleLine().
						Label(rate.name + "速度：" + rate.value).Tooltip(rate.value)
				})
			}
		})
		trafficPlot(c.Key("traffic-plot"), points, start, now)
	})
}

func trafficPlot(c *ui.Context, points []model.TrafficPoint, start, end time.Time) {
	p := palette(c)
	var peak int64
	for _, point := range points {
		if !point.At.After(end) {
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
	}).FillWidth().Height(57).Label("最近 5 分钟的上传和下载速率")
}
