package icon

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// withPanel forces the cached panel-darkness probe for one test.
func withPanel(t *testing.T, dark bool) {
	t.Helper()
	panelIsDark() // run the once
	old := panelDark
	panelDark = dark
	t.Cleanup(func() { panelDark = old })
}

// TestTrayFrameMatchesReference is the golden test: TrayFrame must equal the
// original pipeline (render, PNG, Retint, PNG decode, ARGB32) byte for byte,
// for a spread of heights, inks (including zero tint and translucent inks),
// both panel themes, and several washes.
func TestTrayFrameMatchesReference(t *testing.T) {
	hs, tints := goldenInputs()
	washes := []struct {
		c color.NRGBA
		t float64
	}{
		{ParseHex("#e2007a"), 0.9}, // the shipped FIP wash
		{ParseHex("#e2007a"), 0},
		{ParseHex("#e2007a"), 0.35},
		{color.NRGBA{0x20, 0x90, 0xF0, 0xFF}, 1},
	}
	n := 0
	for _, dark := range []bool{false, true} {
		withPanel(t, dark)
		for hi, h := range hs {
			for _, tint := range tints {
				// The hand-picked poses meet every wash; the random spread
				// only the shipped one (the reference pipeline is slow).
				ws := washes
				if hi >= 10 {
					ws = washes[:1]
				}
				for _, w := range ws {
					want, err := refTrayPixmap(h, dark, tint, w.c, w.t)
					if err != nil {
						t.Fatal(err)
					}
					got := TrayFrame(h, tint, w.c, w.t)
					if got.W != barsSize || got.H != barsSize {
						t.Fatalf("frame is %dx%d, want %d", got.W, got.H, barsSize)
					}
					if !bytes.Equal(got.Pix, want) {
						t.Fatalf("TrayFrame(%v, dark=%v, tint=%v, wash=%v@%v) differs from the reference pipeline",
							h, dark, tint, w.c, w.t)
					}
					n++
				}
			}
		}
	}
	t.Logf("%d (heights, tint, theme, wash) frames identical to the reference", n)
}

// TestShapeMemoBounded: the shape memo never grows past its cap and keeps
// serving correct frames after it is dropped.
func TestShapeMemoBounded(t *testing.T) {
	wash := ParseHex("#e2007a")
	for i := 0; i < 3*shapeCap; i++ {
		h := vu.Heights{uint8(i % 12), uint8(i / 12 % 12), uint8(i / 144 % 12), uint8(i / 1728 % 12)}
		TrayFrame(h, color.NRGBA{}, wash, 0.9)
	}
	shapeMu.Lock()
	n := len(shapes)
	shapeMu.Unlock()
	if n > shapeCap {
		t.Errorf("shape memo holds %d entries, cap is %d", n, shapeCap)
	}
	h := vu.Heights{3, 7, 4, 9}
	want, err := refTrayPixmap(h, panelIsDark(), color.NRGBA{}, wash, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(TrayFrame(h, color.NRGBA{}, wash, 0.9).Pix, want) {
		t.Error("frame wrong after the memo was dropped")
	}
}
