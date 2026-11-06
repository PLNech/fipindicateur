package icon

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/PLNech/fipindicateur/internal/vu"
)

func TestBarsIconValidPNG(t *testing.T) {
	b := BarsIcon(vu.Heights{0, 3, 5, 7}, color.NRGBA{})
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != barsSize || bounds.Dy() != barsSize {
		t.Errorf("size: got %dx%d want %dx%d", bounds.Dx(), bounds.Dy(), barsSize, barsSize)
	}
}

func TestBarsIconCached(t *testing.T) {
	h := vu.Heights{1, 2, 3, 4}
	b1 := BarsIcon(h, color.NRGBA{})
	b2 := BarsIcon(h, color.NRGBA{})
	if len(b1) == 0 {
		t.Fatal("empty icon")
	}
	// Cache hit returns the same underlying bytes: no re-rasterization.
	if &b1[0] != &b2[0] {
		t.Error("expected cached bytes on second call")
	}
	// A different state is a different frame.
	b3 := BarsIcon(vu.Heights{4, 3, 2, 1}, color.NRGBA{})
	if bytes.Equal(b1, b3) {
		t.Error("distinct states should render distinct frames")
	}
}

func TestBarsIconTintDistinct(t *testing.T) {
	h := vu.Heights{1, 2, 3, 4}
	plain := BarsIcon(h, color.NRGBA{})
	tinted := BarsIcon(h, color.NRGBA{0xE2, 0x00, 0x7A, 0xFF})
	if bytes.Equal(plain, tinted) {
		t.Error("a non-zero tint must recolor the bars")
	}
	// The tint joins the cache key: same tint, same bytes.
	again := BarsIcon(h, color.NRGBA{0xE2, 0x00, 0x7A, 0xFF})
	if &tinted[0] != &again[0] {
		t.Error("expected cached bytes for the same (heights, tint)")
	}
}

// TestRenderBarsThemeSplit is the dark-mode evidence: the two panel themes
// produce different pixels (different ink, and a halo wherever the platform
// draws one on dark), so a misdetection cannot hide behind "both look fine".
func TestRenderBarsThemeSplit(t *testing.T) {
	h := vu.Heights{3, 7, 4, 9}
	hasInk := func(t *testing.T, data []byte, want color.NRGBA) bool {
		t.Helper()
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("not a valid PNG: %v", err)
		}
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if c := img.At(x, y); c == want {
					return true
				}
			}
		}
		return false
	}
	dark := renderBars(h, true, color.NRGBA{})
	light := renderBars(h, false, color.NRGBA{})
	if bytes.Equal(dark, light) {
		t.Error("dark and light panels must render different glyphs")
	}
	// The theme ink itself flips: light ink on a dark panel, dark on light.
	if !hasInk(t, dark, ThemeInk(true)) {
		t.Error("the dark-panel render must carry the light theme ink")
	}
	if !hasInk(t, light, ThemeInk(false)) {
		t.Error("the light-panel render must carry the dark theme ink")
	}
}
