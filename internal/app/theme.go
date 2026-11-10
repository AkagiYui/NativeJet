package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	toolbarHeight         = 44
	controlHeight         = 28
	statusBarHeight       = controlHeight + 8
	inspectorHeaderHeight = controlHeight + 8
	controlRadius         = 6
	sidebarWidth          = 208
	inspectorWidth        = 254
)

type workspacePalette struct {
	ui.Theme
	rail, line, active, rowSelection ui.Color
	// Overlay tokens (Pail overlay.go / theme.go). Scrim dims the window behind
	// a modal. BorderStrong outlines dark overlays where a shadow would vanish.
	// Shadow* colour the light-theme soft edge; they are transparent in dark.
	BorderStrong                ui.Color
	Scrim                       ui.Color
	ShadowPopover, ShadowDialog ui.Color
}

var paper = workspacePalette{
	Theme: ui.Theme{
		Background: ui.Hex("#f9f8f5"), Surface: ui.Hex("#fffefa"),
		SurfaceHover: ui.Hex("#eeece6"), SurfacePressed: ui.Hex("#e4e1d8"),
		Border: ui.Hex("#c9c5bc"), Text: ui.Hex("#242520"), TextMuted: ui.Hex("#62625a"),
		Accent: ui.Hex("#385c91"), AccentHover: ui.Hex("#304f7f"), AccentPressed: ui.Hex("#29436d"), AccentText: ui.Hex("#ffffff"),
		Danger: ui.Hex("#a13232"), Warning: ui.Hex("#805512"), Success: ui.Hex("#36714e"),
		Selection: ui.Hex("#dbe5f2"), Focus: ui.Hex("#547db4"),
		Inverse: ui.Hex("#282922"), InverseText: ui.Hex("#faf9f5"),
		Scrollbar: ui.RGBA(53, 52, 42, .34), ScrollbarWidth: 6, Radius: controlRadius, Spacing: 4, FontSize: 13,
	},
	rail: ui.Hex("#f0eee7"), line: ui.Hex("#dedbd3"), active: ui.Hex("#e3e0d6"), rowSelection: ui.Hex("#dbe5f2"),
	BorderStrong:  ui.Hex("#a8a499"),
	Scrim:         ui.Hex("#141311").Alpha(0.32),
	ShadowPopover: ui.RGBA(0, 0, 0, 0.14),
	ShadowDialog:  ui.RGBA(0, 0, 0, 0.18),
}

var graphite = workspacePalette{
	Theme: ui.Theme{
		Dark:       true,
		Background: ui.Hex("#222321"), Surface: ui.Hex("#2b2c29"),
		SurfaceHover: ui.Hex("#323430"), SurfacePressed: ui.Hex("#3b3e39"),
		Border: ui.Hex("#595c54"), Text: ui.Hex("#f1f0e9"), TextMuted: ui.Hex("#b5b6ab"),
		Accent: ui.Hex("#9dbbea"), AccentHover: ui.Hex("#b1c9ef"), AccentPressed: ui.Hex("#88a8db"), AccentText: ui.Hex("#1b2638"),
		Danger: ui.Hex("#f29b96"), Warning: ui.Hex("#e0b574"), Success: ui.Hex("#93c9a7"),
		Selection: ui.Hex("#384a63"), Focus: ui.Hex("#9dbbea"),
		Inverse: ui.Hex("#eeeae0"), InverseText: ui.Hex("#242520"),
		Scrollbar: ui.RGBA(241, 240, 233, .34), ScrollbarWidth: 6, Radius: controlRadius, Spacing: 4, FontSize: 13,
	},
	rail: ui.Hex("#1c1d1b"), line: ui.Hex("#373934"), active: ui.Hex("#373a34"), rowSelection: ui.Hex("#384a63"),
	BorderStrong:  ui.Hex("#6b6e64"),
	Scrim:         ui.Hex("#000000").Alpha(0.5),
	ShadowPopover: ui.Transparent,
	ShadowDialog:  ui.Transparent,
}

func palette(c *ui.Context) *workspacePalette {
	if c.Theme().Dark {
		return &graphite
	}
	return &paper
}

func useWorkspaceTheme(c *ui.Context) {
	c.SetTheme(&palette(c).Theme)
}

func workspaceTable(c *ui.Context, state *ui.ListState, columns []ui.TableColumn, n int, label string, cell func(row, col int)) {
	p := palette(c)
	t := *c.Theme()
	t.Accent, t.AccentText, t.SurfacePressed = p.rowSelection, t.Text, p.active
	t.Border = p.line
	table := ui.Themed(c, &t).Grow(1).FillWidth().MinWidth(0).MinHeight(0)
	if state != nil && state.Sort != nil {
		order := ui.Local(table, "table-sort", func() ui.SortOrder { return *state.Sort })
		if *order != *state.Sort {
			*order = *state.Sort
			state.ScrollTo(0, ui.Start)
		}
	}
	table.Children(func() {
		ui.Table(c, state, columns, n, cell).Fill().Label(label)
	})
}

func workspaceEditor(c *ui.Context, state *editor.State) ui.Element {
	return editor.View(c, state).FillWidth().MinHeight(0)
}

func action(c *ui.Context, icon *ui.SVG, label string, primary bool) ui.Element {
	var b ui.Element
	if primary {
		b = ui.PrimaryButton(c, "")
	} else {
		b = ui.Button(c, "")
	}
	return b.Label(label).Height(controlHeight).Padding(0, 10).Gap(6).Children(func() {
		ui.Icon(c, icon).Size(14, 14).Shrink(0)
		ui.Text(c, label).SingleLine().FontWeight(500)
	})
}

type iconChoice struct {
	label string
	icon  *ui.SVG
}

func iconTabs(c *ui.Context, selected *int, label string, choices ...iconChoice) ui.Element {
	t := c.Theme()
	parts := ui.TabsBase(c, selected, len(choices))
	return parts.List.Label(label).Gap(4).Children(func() {
		for i, choice := range choices {
			tab := parts.Tab(i).Label(choice.label).Height(34).PaddingX(8).Gap(5).Center().Radius(controlRadius)
			tab.TextColor(t.TextMuted)
			if i == *selected {
				tab.TextColor(t.Text).FontWeight(600)
				tab.DrawOver(func(p *ui.Painter, r ui.Rect) {
					p.Fill(ui.Rect{X: r.X + 5, Y: r.Y + r.H - 2, W: r.W - 10, H: 2}, t.Accent, 1)
				})
			}
			if tab.Hovered() {
				tab.Background(t.SurfaceHover).TextColor(t.Text)
			}
			tab.Children(func() {
				ui.Icon(c, choice.icon).Size(13, 13).Shrink(0)
				ui.Text(c, choice.label).SingleLine()
			})
		}
	})
}

func iconSegments(c *ui.Context, selected *int, label string, choices ...iconChoice) ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(choices))
	return parts.Track.Label(label).Padding(2).Gap(2).Radius(controlRadius).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		for i, choice := range choices {
			segment := parts.Segment(i).Label(choice.label).Height(26).PaddingX(8).Gap(5).Radius(4)
			switch {
			case i == *selected:
				segment.Background(t.Selection).FontWeight(600)
			case segment.Hovered():
				segment.Background(t.SurfaceHover)
			}
			segment.Children(func() {
				ui.Icon(c, choice.icon).Size(13, 13).Shrink(0)
				ui.Text(c, choice.label).SingleLine()
			})
		}
	})
}
