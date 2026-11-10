package app

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The tests of the motion (motion.go): the tester's clock stands still unless
// the test moves it. They mirror Pail's motion_test.go for the overlays this
// app has: fade in where they rest, go at once on close, and nowhere else
// animates.

// TestDialogScrimFadesIn proves the overlay fade-in renders: the scrim that
// dims the window starts transparent, reaches partial opacity partway through
// baseAnim, and is fully dimmed once it rests.
func TestDialogScrimFadesIn(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.importing = &importForm{app: a, open: true, name: "测试配置", interval: "24", editor: newEditor("", false)}
	tt := ui.NewTester(a.view, 920, 640)

	// A corner pixel sits under the scrim but outside the centered card.
	corner := func() uint8 {
		img := tt.Image()
		c := img.RGBAAt(4, 4)
		return luminance(c)
	}
	// The clock is frozen at the open instant: the scrim must not dim yet.
	before := corner()
	if before < 200 {
		t.Fatalf("scrim dims the window in its first frame (luminance %d)", before)
	}

	tt.Advance(125 * time.Millisecond)
	mid := corner()
	if mid >= before || mid == 0 {
		t.Fatalf("scrim did not fade partway in (luminance %d -> %d)", before, mid)
	}

	tt.Advance(250 * time.Millisecond)
	after := corner()
	if after >= mid {
		t.Fatalf("scrim did not finish fading in (luminance %d -> %d -> %d)", before, mid, after)
	}
}

// TestDialogFadesInWhereItRests: the card's layout in the first fading frame
// matches the layout at rest — nothing scales or slides.
func TestDialogFadesInWhereItRests(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.nameDialog = nameDialogState{open: true, title: "重命名全局覆盖", name: "测试"}
	tt := ui.NewTester(a.view, 920, 640)

	first, ok := tt.Find("取消")
	if !ok {
		t.Fatal("cancel not found on first frame")
	}
	tt.Advance(time.Second)
	tt.Frame()
	rest, ok := tt.Find("取消")
	if !ok {
		t.Fatal("cancel not found at rest")
	}
	if first != rest {
		t.Errorf("cancel moved or scaled during fade: first %+v rest %+v", first, rest)
	}
}

// TestClosingDialogIsGoneAtOnce: a dialog that closes leaves in that frame —
// no fade-out, no pending animation frames.
func TestClosingDialogIsGoneAtOnce(t *testing.T) {
	a, err := New(Options{Data: t.TempDir(), Helper: "/unused", Development: true})
	if err != nil {
		t.Fatal(err)
	}
	a.nameDialog = nameDialogState{open: true, title: "重命名全局覆盖", name: "测试"}
	tt := ui.NewTester(a.view, 920, 640)
	tt.Advance(time.Second)
	tt.Frame()
	if !tt.HasText("全局覆盖名称") {
		t.Fatal("dialog did not open")
	}

	a.nameDialog.open = false
	tt.Frame()
	if tt.HasText("全局覆盖名称") {
		t.Error("closed dialog is still drawn")
	}
	if tt.FramePending() {
		t.Error("window asks for frames after close: something fades out")
	}
}

// TestOnlyTheTokenMoves keeps every view except motion.go from asking mygo
// for an animation. Progress-bar easing, when added, must be listed here the
// way Pail lists transfers.go.
func TestOnlyTheTokenMoves(t *testing.T) {
	anim := regexp.MustCompile(`\.(AnimatePose|AnimateTo|AnimateColor|Transition|ContentTransition)\(|ui\.Animation\{|SpringOptions`)
	allowed := map[string]string{
		"motion.go": "",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		// `go test` may run with a different cwd; resolve next to this file.
		dir := filepath.Dir(mustAbs(t))
		files, err = filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") || !anim.MatchString(line) {
				continue
			}
			if ok, listed := allowed[base]; listed && (ok == "" || strings.Contains(line, ok)) {
				continue
			}
			t.Errorf("%s:%d animates outside motion.go: %s", base, i+1, strings.TrimSpace(line))
		}
	}
}

func mustAbs(t *testing.T) string {
	t.Helper()
	// Any source file in this package; motion.go is the one we care about.
	p, err := filepath.Abs("motion.go")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func luminance(c color.RGBA) uint8 {
	return uint8((int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000)
}
