package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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

// TestRetint pins the prototype-branding pass: every non-transparent pixel
// moves toward the brand hue by the asked fraction (alpha preserved),
// transparent pixels stay transparent, and a non-PNG input comes back
// untouched rather than blanked.
func TestRetint(t *testing.T) {
	// A 2x1 PNG: one opaque red pixel, one transparent pixel.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xFF, A: 0xFF})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	src := buf.Bytes()

	out, err := png.Decode(bytes.NewReader(Retint(src, color.NRGBA{G: 0xFF, A: 0xFF}, 0.5)))
	if err != nil {
		t.Fatalf("Retint output is not a PNG: %v", err)
	}
	if got := out.Bounds(); got.Dx() != 2 || got.Dy() != 1 {
		t.Fatalf("Retint changed the size: %v", got)
	}
	r, g, b, a := out.At(0, 0).RGBA()
	if uint8(a>>8) != 0xFF {
		t.Errorf("alpha changed: %d, want 255", uint8(a>>8))
	}
	// Red (255,0) mixed 50% toward green (0,255): both channels near 128.
	if uint8(r>>8) < 120 || uint8(r>>8) > 136 || uint8(g>>8) < 120 || uint8(g>>8) > 136 || uint8(b>>8) != 0 {
		t.Errorf("pixel did not blend halfway toward the brand hue: r=%d g=%d b=%d", uint8(r>>8), uint8(g>>8), uint8(b>>8))
	}
	if _, _, _, a2 := out.At(1, 0).RGBA(); a2 != 0 {
		t.Errorf("transparent pixel became opaque: %d", uint8(a2>>8))
	}

	// A broken input is passed through, never blanked.
	if got := Retint([]byte("not a png"), color.NRGBA{}, 0.5); string(got) != "not a png" {
		t.Errorf("non-PNG input was mangled: %q", got)
	}
}
