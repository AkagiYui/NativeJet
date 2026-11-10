package app

import "github.com/egoist/mygo/ui"

// Overlays follow Pail's cmd/pail-desktop/overlay.go: everything that floats
// above the window shares one surface style (overlayPanel). Dialogs, the
// spotlight card and popovers all go through here, so a radius, a shadow or
// an outline is decided in one place.

// overlayKind is the kind of surface overlayPanel draws.
type overlayKind int

const (
	// overlayDialog is a modal card: dialogs and the spotlight guide. Radius 10.
	overlayDialog overlayKind = iota
	// overlayPopover is a panel anchored to an element. Radius 8.
	overlayPopover
)

// Radii match Pail: 10 dialogs, 8 popovers. Nested row radius is concentric
// with a popover (outer minus the usual 4px inset).
const (
	radiusDialog  = 10
	radiusPopover = 8
)

// Shadow geometry (x, y, blur, spread). The colour is a token
// (ShadowPopover, ShadowDialog); the dark theme has none.
const (
	shadowPopoverY, shadowPopoverBlur = 8, 24
	shadowDialogY, shadowDialogBlur   = 12, 32
)

// overlayPanel styles panel as an overlay surface of the given kind and
// returns it: fill, radius, and the edge. In the light theme the edge is a
// soft shadow; in the dark theme, where a shadow cannot be seen, it is a 1px
// BorderStrong outline (the shadow token is transparent there). Callers add
// their own size and padding.
func overlayPanel(c *ui.Context, panel ui.Element, kind overlayKind) ui.Element {
	p := palette(c)
	var (
		radius float32
		shadow ui.Color
		y, blr float32
	)
	switch kind {
	case overlayDialog:
		radius, shadow, y, blr = radiusDialog, p.ShadowDialog, shadowDialogY, shadowDialogBlur
	default:
		radius, shadow, y, blr = radiusPopover, p.ShadowPopover, shadowPopoverY, shadowPopoverBlur
	}
	panel.Radius(radius).Background(p.Surface)
	if shadow.A > 0 {
		panel.Shadow(0, y, blr, 0, shadow)
	} else {
		panel.Border(1, p.BorderStrong)
	}
	return panel
}
