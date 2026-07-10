package icon

import (
	"image/color"
	"testing"

	"github.com/PLNech/fipindicateur/internal/stations"
)

func TestParseHex(t *testing.T) {
	cases := []struct {
		in   string
		want color.NRGBA
	}{
		{"#e2007a", color.NRGBA{0xE2, 0x00, 0x7A, 0xFF}},
		{"e2007a", color.NRGBA{0xE2, 0x00, 0x7A, 0xFF}},
		{"#FFD700", color.NRGBA{0xFF, 0xD7, 0x00, 0xFF}},
		{"#abc", color.NRGBA{0xAA, 0xBB, 0xCC, 0xFF}},
		{"", color.NRGBA{0x80, 0x80, 0x80, 0xFF}},
		{"#zzzzzz", color.NRGBA{0x80, 0x80, 0x80, 0xFF}},
		{"#12345", color.NRGBA{0x80, 0x80, 0x80, 0xFF}},
	}
	for _, c := range cases {
		if got := ParseHex(c.in); got != c.want {
			t.Errorf("ParseHex(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLerpEndpointsAndClamp(t *testing.T) {
	a := color.NRGBA{0, 0, 0, 0xFF}
	b := color.NRGBA{100, 200, 50, 0xFF}
	if got := Lerp(a, b, 0); got != a {
		t.Errorf("Lerp t=0 = %v, want %v", got, a)
	}
	if got := Lerp(a, b, 1); got != b {
		t.Errorf("Lerp t=1 = %v, want %v", got, b)
	}
	// Clamping below 0 and above 1 pins to the endpoints.
	if got := Lerp(a, b, -0.5); got != a {
		t.Errorf("Lerp t<0 = %v, want %v", got, a)
	}
	if got := Lerp(a, b, 2); got != b {
		t.Errorf("Lerp t>1 = %v, want %v", got, b)
	}
	// Midpoint rounds each channel.
	mid := Lerp(a, b, 0.5)
	if mid != (color.NRGBA{50, 100, 25, 0xFF}) {
		t.Errorf("Lerp t=0.5 = %v, want {50 100 25 255}", mid)
	}
}

// TestLegibleContrast asserts every station brand color, once run through
// Legible, clears 3:1 contrast against the assumed panel surface in both modes.
func TestLegibleContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		surface := panelSurfaceLight
		if dark {
			surface = panelSurfaceDark
		}
		for _, s := range stations.All {
			c := Legible(s.Color, dark)
			if cr := contrastRatio(c, surface); cr < 3 {
				t.Errorf("station %q color %s: contrast %.2f < 3 (dark=%v)",
					s.Key, s.Color, cr, dark)
			}
		}
	}
}
