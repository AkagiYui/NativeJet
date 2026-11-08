package app

import (
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/storage"
)

const (
	spotlightWelcome = iota
	spotlightKernel
	spotlightConfig
	spotlightStart
)

var spotlightSteps = [...]struct {
	page, title, detail string
	at, self            ui.Anchor
}{
	spotlightWelcome: {
		title:  "欢迎使用 NativeJet",
		detail: "NativeJet 是一款 Mihomo / Clash.Meta 桌面客户端，可管理代理配置、切换策略组，并查看实时连接和流量。接下来三步会带你找到下载内核、添加配置和启动内核的入口。",
	},
	spotlightKernel: {
		page: "内核", title: "准备 Mihomo 内核",
		detail: "从这里下载 Mihomo。已有内核文件时，也可以放进内核目录，重新扫描后选用。",
		at:     ui.AnchorBottomRight, self: ui.AnchorTopRight,
	},
	spotlightConfig: {
		page: "配置", title: "添加并应用配置",
		detail: "导入订阅地址、YAML 文件，或粘贴配置内容。添加后，点击右上角的「应用配置」。",
		at:     ui.AnchorBottomRight, self: ui.AnchorTopRight,
	},
	spotlightStart: {
		page: "配置", title: "启动并查看状态",
		detail: "选好内核并应用配置后，在侧栏启动内核。启动后，这里会显示运行状态和停止按钮。",
		at:     ui.AnchorRight, self: ui.AnchorLeft,
	},
}

type spotlightGuide struct {
	open          bool
	step          int
	targets       [len(spotlightSteps)]ui.Handle
	dismissedPath string
}

func (g *spotlightGuide) showOnStartup() {
	if _, err := os.Stat(g.dismissedPath); errors.Is(err, os.ErrNotExist) {
		g.show()
	}
}

func (g *spotlightGuide) show() {
	g.open, g.step = true, spotlightWelcome
}

func (g *spotlightGuide) dismiss(c *ui.Context) {
	g.open = false
	if err := storage.Atomic(g.dismissedPath, nil, 0600); err != nil {
		log.Printf("Could not save usage guide dismissal: %v", err)
		c.Toast("无法保存引导状态，下次启动时可能再次显示")
	}
}

func (a *App) displayedPage() string {
	if a.spotlight.open && a.spotlight.step != spotlightWelcome {
		return spotlightSteps[a.spotlight.step].page
	}
	return a.page
}

func (g *spotlightGuide) view(c *ui.Context) {
	if !g.open {
		return
	}
	step := spotlightSteps[g.step]
	var target ui.Element
	if g.step != spotlightWelcome {
		target = c.Resolve(g.targets[g.step])
		if !target.Valid() {
			return
		}
	}
	colors := palette(c)
	ui.Overlay(c, func() {
		ui.Box(c.Key("spotlight")).Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Draw(func(p *ui.Painter, window ui.Rect) {
				drawSpotlight(p, window, target.Bounds(), colors.Accent)
			}).Children(func() {
			// The wrapper attaches to the target outside the modal. The card
			// establishes its own focus scope so that Tab stays in the guide.
			placement := ui.Box(c)
			width := float32(334)
			switch {
			case !target.Valid():
				placement.Attach(ui.AnchorCenter, ui.AnchorCenter)
				width = 390
			case step.at == ui.AnchorRight:
				placement.AttachTo(target, step.at, step.self).Margin(0, 0, 0, 16)
			default:
				placement.AttachTo(target, step.at, step.self).Margin(16, 0, 0, 0)
			}
			placement.Children(func() {
				card := ui.Column(c).Width(width).Padding(16).Gap(9).Radius(12).
					Background(colors.Surface).Border(1, colors.Border).
					Shadow(0, 10, 26, 0, ui.RGBA(0, 0, 0, .34)).Modal().Role(ui.RoleDialog).
					Label("使用引导 · " + step.title)
				card.Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Icon(c, guideIcon).Size(15, 15).TextColor(colors.Accent)
						ui.Text(c, "快速上手").FontSize(12).FontWeight(600).TextColor(colors.Accent)
						ui.Box(c).Grow(1)
						ui.Textf(c, "%d / %d", g.step, len(spotlightSteps)-1).Padding(3, 8).
							Radius(999).Background(colors.Selection).TextColor(colors.Text).FontSize(11)
					})
					ui.Text(c, step.title).FontSize(19).FontWeight(600)
					ui.Text(c, step.detail).FontSize(13).TextColor(colors.TextMuted)
					g.controls(c, card)
				})
			})
		})
	})
}

func (g *spotlightGuide) controls(c *ui.Context, card ui.Element) {
	ui.Row(c).FillWidth().Padding(5, 0, 0).AlignItems(ui.Center).Gap(6).Children(func() {
		skip := action(c.Key("guide-skip"), nil, "跳过", false)
		ui.Box(c).Grow(1)
		var back ui.Element
		if g.step != spotlightWelcome {
			back = action(c.Key("guide-back"), nil, "上一步", false)
		}
		last := g.step == len(spotlightSteps)-1
		label := "下一步"
		switch {
		case g.step == spotlightWelcome:
			label = "开始引导"
		case last:
			label = "完成"
		}
		next := action(c.Key("guide-next"), nil, label, true)
		switch {
		case skip.Clicked():
			g.dismiss(c)
		case card.OverlayShortcut(0, ui.KeyEscape):
			g.open = false
		case back.Clicked():
			g.step--
		case next.Clicked():
			if last {
				g.dismiss(c)
			} else {
				g.step++
			}
		}
	})
}

func drawSpotlight(p *ui.Painter, window, target ui.Rect, accent ui.Color) {
	dim := ui.RGBA(15, 19, 25, .64)
	if target.W <= 0 || target.H <= 0 {
		p.Fill(window, dim, 0)
		return
	}
	const padding, radius = 5, 9
	left, top := target.X-padding, target.Y-padding
	right, bottom := target.X+target.W+padding, target.Y+target.H+padding
	// The opposite winding of the inner contour leaves the target clear.
	mask := new(ui.Path)
	mask.MoveTo(window.X, window.Y).LineTo(window.X+window.W, window.Y).
		LineTo(window.X+window.W, window.Y+window.H).LineTo(window.X, window.Y+window.H).Close()
	mask.MoveTo(left+radius, top).QuadTo(left, top, left, top+radius).
		LineTo(left, bottom-radius).QuadTo(left, bottom, left+radius, bottom).
		LineTo(right-radius, bottom).QuadTo(right, bottom, right, bottom-radius).
		LineTo(right, top+radius).QuadTo(right, top, right-radius, top).Close()
	p.FillPath(mask, dim)
	hole := ui.Rect{X: left, Y: top, W: right - left, H: bottom - top}
	p.Shadow(hole, radius, 0, 0, 12, 1, accent)
	p.Stroke(hole, accent, radius, 2)
}
