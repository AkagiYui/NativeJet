package app

import (
	"embed"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"nativejet/internal/servicecatalog"
)

//go:embed assets/services/*.png
var serviceIconFiles embed.FS

var serviceIcons = sync.OnceValue(func() map[string]*ui.Bitmap {
	files, _ := serviceIconFiles.ReadDir("assets/services")
	icons := make(map[string]*ui.Bitmap, len(files))
	for _, file := range files {
		data, err := serviceIconFiles.ReadFile("assets/services/" + file.Name())
		if err == nil {
			if bitmap, err := ui.DecodeBitmap(data); err == nil {
				icons[strings.TrimSuffix(file.Name(), ".png")] = bitmap
			}
		}
	}
	return icons
})

func serviceIconImage(c *ui.Context, entry servicecatalog.Entry) {
	if icon := serviceIcons()[entry.IconID()]; icon != nil {
		ui.Image(c, icon).Size(16, 16).Shrink(0)
		return
	}
	first, _ := utf8.DecodeRuneInString(entry.Name)
	ui.Box(c).Size(16, 16).Shrink(0).Radius(4).Background(palette(c).active).Center().Children(func() {
		ui.Text(c, strings.ToUpper(string(first))).FontSize(10).FontWeight(600).TextColor(c.Theme().Accent)
	})
}
