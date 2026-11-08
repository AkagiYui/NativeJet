package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSpotlightGuideKeepsNavigationInTheCard(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.page = "日志"
	tt := ui.NewTester(a.view, 920, 640)
	nav, ok := tt.Find("内核")
	if !ok {
		t.Fatal("kernel navigation control not found")
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	assertStep := func(progress, page string) {
		t.Helper()
		if !tt.HasText(progress) || a.displayedPage() != page || a.page != "日志" {
			t.Fatalf("expected %s on %s, displayed=%q original=%q texts=%q", progress, page, a.displayedPage(), a.page, tt.Texts())
		}
	}
	assertClosed := func() {
		t.Helper()
		if a.spotlight.open || tt.HasText("快速上手") || a.displayedPage() != "日志" {
			t.Fatal("guide did not close and restore the original page")
		}
	}

	click("使用引导")
	assertStep("0 / 3", "日志")
	for _, size := range [][2]int{{920, 640}, {1160, 800}} {
		tt.SetSize(size[0], size[1])
		card, ok := tt.Find("使用引导 · 欢迎使用 NativeJet")
		if !ok || card.X*2+card.W != float32(size[0]) || card.Y*2+card.H != float32(size[1]) {
			t.Fatalf("welcome card is not centered at %v: %v", size, card)
		}
	}
	click("开始引导")
	assertStep("1 / 3", "内核")
	tt.ClickAt(nav.X+nav.W/2, nav.Y+nav.H/2)
	assertStep("1 / 3", "内核")
	click("上一步")
	assertStep("0 / 3", "日志")
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("开始引导") && !tt.Focused("跳过") {
		t.Fatal("Tab left the guide card")
	}
	click("开始引导")
	click("下一步")
	assertStep("2 / 3", "配置")
	click("上一步")
	assertStep("1 / 3", "内核")
	click("下一步")
	click("下一步")
	assertStep("3 / 3", "配置")
	click("完成")
	assertClosed()

	click("使用引导")
	assertStep("0 / 3", "日志")
	tt.Key(0, ui.KeyEscape)
	assertClosed()
	click("使用引导")
	click("跳过")
	assertClosed()
}

func TestSpotlightGuideRemembersDismissalAcrossLaunches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		actions []string
	}{
		{name: "skip", actions: []string{"跳过"}},
		{name: "finish", actions: []string{"开始引导", "下一步", "下一步", "完成"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := Options{Data: t.TempDir(), Helper: "/unused", Development: true}
			launch := func() *ui.Tester {
				t.Helper()
				a, err := New(options)
				if err != nil {
					t.Fatal(err)
				}
				a.spotlight.showOnStartup()
				return ui.NewTester(a.view, 920, 640)
			}
			first := launch()
			if !first.HasText("0 / 3") {
				t.Fatal("first launch did not show the welcome step")
			}
			first.Key(0, ui.KeyEscape)
			if first.HasText("0 / 3") {
				t.Fatal("Escape did not close the guide")
			}
			second := launch()
			if !second.HasText("0 / 3") {
				t.Fatal("closing with Escape suppressed the guide on the next launch")
			}
			for _, action := range tc.actions {
				if err := second.Click(action); err != nil {
					t.Fatal(err)
				}
			}
			third := launch()
			if third.HasText("快速上手") {
				t.Fatal("the guide reappeared after dismissal")
			}
			if err := third.Click("使用引导"); err != nil || !third.HasText("0 / 3") {
				t.Fatalf("the guide link did not reopen the welcome step: %v", err)
			}
		})
	}
}
