package icon

import (
	"image/color"
	"math"
)

// Station brand tinting for the animated VU glyph. Brand hues are not a tuned
// palette: some melt into the panel surface (gold on a light bar, navy on a
// dark one). Legible keeps the hue but mixes it toward white or black just
// until the ink clears 3:1 non-text contrast against the assumed panel
// surface. This mirrors legible() in internal/stats/report.html.tmpl (8% mix
// steps, at most 12 iterations, so the mixing tops out near 60%).

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

// Legible returns the brand color mixed toward white (dark panel) or black
// (light panel) in 8% steps until it clears 3:1 contrast against the assumed
// panel surface, capped at 12 iterations (so the mixing tops out near 60%).
func Legible(hex string, darkPanel bool) color.NRGBA {
	surface := panelSurfaceLight
	toward := color.NRGBA{0x00, 0x00, 0x00, 0xFF}
	if darkPanel {
		surface = panelSurfaceDark
		toward = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	}
	c := ParseHex(hex)
	for i := 0; i < 12 && contrastRatio(c, surface) < 3; i++ {
		c = mix(c, toward, 0.08)
	}
	return c
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
