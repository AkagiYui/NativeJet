package app

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"log"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type menuBarDisplay struct {
	running bool
	layout  menuBarSpeedLayout
	labels  trafficLabels
}

func (a *App) updateMenuBarDisplay(next menuBarDisplay) {
	prev := a.trayDisplay
	if next == prev {
		return
	}
	stacked := platformName == "macOS" && next.layout == menuBarSpeedStacked
	wasStacked := platformName == "macOS" && prev.layout == menuBarSpeedStacked
	rates, title, tip := "", "", "NativeJet"
	if next.layout != menuBarSpeedOff {
		rates = "↑ " + next.labels.up + "  ↓ " + next.labels.down
		tip += " · 内核实时流量\n" + rates
		if !stacked {
			title = rates
		}
	}
	if next.running != prev.running || stacked != wasStacked || stacked && next.labels != prev.labels {
		icon := menuBarOutlineIcon
		if next.running {
			icon = menuBarIcon
		}
		if stacked {
			stacked, err := stackedMenuBarIcon(icon, next.labels)
			if err == nil {
				icon = stacked
			} else {
				log.Printf("Could not draw the NativeJet menu bar rates: %v", err)
				title = rates
			}
		}
		if err := a.tray.SetIcon(icon, true); err != nil {
			log.Printf("Could not update the NativeJet menu bar icon: %v", err)
			return
		}
	}
	a.tray.SetTitle(title)
	a.tray.SetToolTip(tip)
	a.trayDisplay = next
}

var menuBarRateFont = sync.OnceValues(func() (*opentype.Font, error) {
	return opentype.Parse(gomono.TTF)
})

// MyGo sets the template image to 18 points on macOS. Draw both lines in the
// same 2× image as the rocket so they fit without replacing the status item.
func stackedMenuBarIcon(iconPNG []byte, labels trafficLabels) ([]byte, error) {
	icon, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return nil, err
	}
	f, err := menuBarRateFont()
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	up, down := "↑ "+labels.up, "↓ "+labels.down
	d := font.Drawer{Src: image.Black, Face: face}
	gap, inset := 4, 2
	x := icon.Bounds().Dx() + gap
	// Reserve the typical widest value so other status items don't shift
	// between idle and active traffic.
	width := x + max(d.MeasureString(up), d.MeasureString(down), d.MeasureString("↓ 1023.9 MB/s")).Ceil() + inset
	img := image.NewRGBA(image.Rect(0, 0, width, icon.Bounds().Dy()))
	draw.Draw(img, icon.Bounds(), icon, icon.Bounds().Min, draw.Src)
	d.Dst = img
	d.Dot = fixed.P(x, 15)
	d.DrawString(up)
	d.Dot = fixed.P(x, 32)
	d.DrawString(down)
	var out bytes.Buffer
	err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img)
	return out.Bytes(), err
}
