package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Station brand tinting for the animated VU glyph. Brand hues are not a tuned
// palette: some melt into the panel surface (gold on a light bar, navy on a
// dark one). Legible keeps the hue at full HSL saturation (the brand colors
// are timid: jazz's teal is S=0.76 out of the box) and moves only the
// lightness, up on a dark panel and down on a light one, just until the ink
// clears 3:1 non-text contrast against the assumed panel surface. The old
// toward-white/toward-black RGB mixing reached the same ratio but bled the
// color out: hip-hop landed at S=0.15 gray-mauve on a dark bar.

// Assumed panel surfaces. GNOME's default top bar is near-black; a light theme
// gives a near-white bar. We cannot read the exact pixel, so we tune against
// these representative surfaces (matching the darkPanel probe's two modes).
var (
	panelSurfaceDark  = color.NRGBA{0x26, 0x26, 0x26, 0xFF}
	panelSurfaceLight = color.NRGBA{0xF2, 0xF2, 0xF2, 0xFF}
)

// ParseHex parses a #rrggbb (or #rgb) hex string into an opaque NRGBA. On
// malformed input it returns a neutral gray, mirroring the report's hexRgb().
func ParseHex(s string) color.NRGBA {
	h := s
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return color.NRGBA{0x80, 0x80, 0x80, 0xFF}
	}
	v, err := parseHexDigits(h)
	if err {
		return color.NRGBA{0x80, 0x80, 0x80, 0xFF}
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xFF}
}

// parseHexDigits parses 6 hex digits into a 24-bit value. The bool reports a
// parse error (true == malformed), so ParseHex can fall back to gray.
func parseHexDigits(h string) (uint32, bool) {
	var v uint32
	for i := 0; i < len(h); i++ {
		c := h[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, true
		}
		v = v<<4 | d
	}
	return v, false
}

// relLum is the WCAG relative luminance of an NRGBA (alpha ignored).
func relLum(c color.NRGBA) float64 {
	lin := func(u uint8) float64 {
		v := float64(u) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrastRatio is the WCAG contrast ratio between two colors (>= 1).
func contrastRatio(a, b color.NRGBA) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// mixChan blends one channel from v toward w by fraction t, rounding.
func mixChan(v, w uint8, t float64) uint8 {
	return uint8(math.Round(float64(v) + (float64(w)-float64(v))*t))
}

// mix blends a toward b by fraction t (0..1), preserving a's alpha.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: mixChan(a.R, b.R, t),
		G: mixChan(a.G, b.G, t),
		B: mixChan(a.B, b.B, t),
		A: a.A,
	}
}

// hslOf converts an NRGBA to HSL (h in degrees, s and l in 0..1).
func hslOf(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max, min := r, r
	for _, v := range []float64{g, b} {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
		h *= 60
	case g:
		h = (b-r)/d*60 + 120
	default:
		h = (r-g)/d*60 + 240
	}
	return h, s, l
}

// hslColorOf converts HSL back to an opaque NRGBA.
func hslColorOf(h, s, l float64) color.NRGBA {
	if s <= 0 {
		v := uint8(math.Round(l * 255))
		return color.NRGBA{v, v, v, 0xFF}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	chanAt := func(t float64) float64 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		default:
			return p
		}
	}
	hk := h / 360
	to8 := func(v float64) uint8 { return uint8(math.Round(v * 255)) }
	return color.NRGBA{
		R: to8(chanAt(hk + 1.0/3)),
		G: to8(chanAt(hk)),
		B: to8(chanAt(hk - 1.0/3)),
		A: 0xFF,
	}
}

// Legible returns the station ink for the assumed panel surface: the brand
// hue at full HSL saturation, with the lightness walked up (dark panel) or
// down (light panel) just until the ink clears 3:1 contrast. Lightness moves
// are monotone in luminance at fixed hue and saturation, so a bounded linear
// walk cannot overshoot the ratio and come back. Achromatic input keeps its
// (hueless) gray: there is no saturation to boost.
func Legible(hex string, darkPanel bool) color.NRGBA {
	surface := panelSurfaceLight
	if darkPanel {
		surface = panelSurfaceDark
	}
	h, s, l := hslOf(ParseHex(hex))
	if s >= 0.02 {
		s = 1 // the whole point: full-strength hue, never a pastel
	}
	step := 0.01
	if !darkPanel {
		step = -0.01
	}
	c := hslColorOf(h, s, l)
	for i := 0; i < 100 && contrastRatio(c, surface) < 3; i++ {
		l += step
		if l < 0 || l > 1 {
			break // white or black necessarily clears; the guard is paranoia
		}
		c = hslColorOf(h, s, l)
	}
	return c
}

// inkIsColored reports whether an ink is a saturated brand color rather than
// the neutral theme ink (or its dimmed pause variant). The halo branches on
// it: see haloInk.
func inkIsColored(c color.NRGBA) bool {
	_, s, _ := hslOf(c)
	return s >= 0.3
}

// Lerp linearly interpolates between two colors, t clamped to 0..1.
func Lerp(a, b color.NRGBA, t float64) color.NRGBA {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return color.NRGBA{
		R: mixChan(a.R, b.R, t),
		G: mixChan(a.G, b.G, t),
		B: mixChan(a.B, b.B, t),
		A: mixChan(a.A, b.A, t),
	}
}

// Retint recolors every non-transparent pixel of a PNG toward c by fraction
// t, preserving the alpha channel (and leaving fully transparent pixels
// alone). It is the prototype-branding step: a whole icon family takes the
// one hue without touching its renderer. On any decode or encode failure the
// input bytes are returned untouched, so a branding pass can never blank the
// tray.
func Retint(b []byte, c color.NRGBA, t float64) []byte {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	bounds := img.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue // keep the transparent background transparent
			}
			px := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
			out.SetNRGBA(x, y, mix(px, c, t))
		}
	}
	var buf bytes.Buffer
	if png.Encode(&buf, out) != nil {
		return b
	}
	return buf.Bytes()
}
