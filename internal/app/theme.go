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
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).SingleLine().FontWeight(500)
	})
}
