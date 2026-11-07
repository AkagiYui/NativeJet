package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestAppearancePreservesThemeWhenChangingMenuBarLayout(t *testing.T) {
	a := &App{data: t.TempDir()}
	if err := os.WriteFile(a.appearancePath(), []byte(`{"menuBarSpeedLayout":"stacked"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.loadAppearance()
	if want := (appearancePreferences{Theme: mygo.ThemeSystem, MenuBarSpeedLayout: menuBarSpeedStacked}); a.appearance != want {
		t.Fatalf("legacy preferences loaded as %+v, want %+v", a.appearance, want)
	}
	prefs := a.appearance
	prefs.Theme = mygo.ThemeDark
	if !a.saveAppearance(prefs) {
		t.Fatal(a.appearanceError)
	}
	tester := ui.NewTester(a.appearanceSettings, 700, 200)
	if err := tester.Click("双列（左右并排）"); err != nil {
		t.Fatal(err)
	}
	reloaded := &App{data: a.data}
	reloaded.loadAppearance()
	if want := (appearancePreferences{Theme: mygo.ThemeDark, MenuBarSpeedLayout: menuBarSpeedSideBySide}); reloaded.appearance != want {
		t.Fatalf("preferences reloaded as %+v, want %+v", reloaded.appearance, want)
	}
}
