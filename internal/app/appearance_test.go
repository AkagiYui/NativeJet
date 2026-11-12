package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAppearancePreservesThemeWhenChangingTraySpeed(t *testing.T) {
	a := &App{data: t.TempDir()}
	if err := os.WriteFile(a.appearancePath(), []byte(`{"menuBarSpeedLayout":"stacked"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.loadAppearance()
	if want := (appearancePreferences{Theme: mygo.ThemeSystem, MenuBarSpeedLayout: menuBarSpeedStacked}); a.appearance != want {
		t.Fatalf("preferences loaded as %+v, want %+v", a.appearance, want)
	}
	prefs := a.appearance
	prefs.Theme = mygo.ThemeDark
	if !a.saveAppearance(prefs) {
		t.Fatal(a.appearanceError)
	}
	tester := ui.NewTester(a.menuBarSettings, 700, 200)
	wantLayout := menuBarSpeedSideBySide
	if platformName == "Windows" {
		tester.Key(0, ui.KeyTab)
		if !tester.Focused("托盘速率提示") {
			t.Fatal("tray speed switch did not receive focus")
		}
		tester.Key(0, ui.KeySpace)
		wantLayout = menuBarSpeedOff
	} else {
		tester.Key(0, ui.KeyTab)
		if !tester.Focused("单列（上下排列）") {
			t.Fatal("the active tray layout did not receive focus")
		}
		tester.Key(0, ui.KeyRight)
		if !tester.Focused("双列（左右并排）") {
			t.Fatal("Right arrow did not focus the next tray layout")
		}
	}
	reloaded := &App{data: a.data}
	reloaded.loadAppearance()
	if want := (appearancePreferences{Theme: mygo.ThemeDark, MenuBarSpeedLayout: wantLayout}); reloaded.appearance != want {
		t.Fatalf("preferences reloaded as %+v, want %+v", reloaded.appearance, want)
	}
}
