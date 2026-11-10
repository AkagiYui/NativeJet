package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Motion is a faithful port of Pail's cmd/pail-desktop/motion.go. The
// interface barely moves: a state change shows in the frame it happens.
// Two things ease, both on the one token below (250 ms, CSS ease-out):
//
//   - an overlay coming in (dialog and its scrim, popover, spotlight) fades
//     in where it rests; nothing scales or slides, and it goes at once when
//     it closes;
//   - the fill of a progress bar follows the progress (when the app draws
//     one: AnimateTo with baseAnim, nowhere else).
//
// With the system's "reduce motion" preference mygo keeps only fades of
// opacity, which is all the fade-in is. TestOnlyTheTokenMoves keeps every
// other view from animating.

// durBase is the one duration of the interface.
const durBase = 250 * time.Millisecond

// easeOut is the one curve: CSS ease-out, a quick start that settles softly.
var easeOut = ui.CubicBezier(0, 0, 0.58, 1)

// baseAnim is durBase on easeOut.
var baseAnim = ui.Animation{Duration: durBase, Ease: easeOut}

// mounted reports whether el was laid out in an earlier frame: false in the
// frame that builds it first.
func mounted(el ui.Element) bool {
	b := el.Bounds()
	return b.W > 0 || b.H > 0
}

// fadeIn fades el, an overlay, in over baseAnim in the frame it first shows
// and holds it at rest after. It is a pose, which mygo applies when it
// paints: overlays lay out again after transitions, and a transition's Enter
// plays only for an element whose parent was there in the frame before, so a
// transition could not bring an overlay in.
func fadeIn(c *ui.Context, el ui.Element) {
	if !mounted(el) {
		el.AnimatePose(ui.Pose{Fade: 1}, baseAnim)
		c.AnimationFrame() // the next frame heads for rest
		return
	}
	el.AnimatePose(ui.Pose{}, baseAnim)
}

// modalLayer shows a dialog as ui.DialogBase does (the backdrop takes the
// presses outside the card and Escape, the focus goes into it and back) with
// the scrim and the card fading in over baseAnim. A dialog that closes is
// gone in that frame, so it never holds the keyboard or the pointer. key
// tells the dialogs apart; fn styles the backdrop (it only lays the card out:
// its colour is the scrim) and the card, and builds the card's content.
func modalLayer(c *ui.Context, key string, open *bool, scrim ui.Color, fn func(backdrop, panel ui.Element)) ui.Element {
	if *open {
		ui.Overlay(c, func() {
			s := ui.Box(c.Key("scrim:" + key)).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(scrim)
			fadeIn(c, s)
		})
	}
	return ui.DialogBase(c.Key("modal:"+key), open, func(backdrop, panel ui.Element) {
		fadeIn(c, panel)
		fn(backdrop, panel)
	})
}
