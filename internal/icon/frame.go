package icon

import (
	"image"
	"image/color"
	"sync"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// Raw tray frames for the animator. The PNG route (BarsIcon, then Retint, then
// the tray's own PNG decode) paid two codec round trips per frame to end up at
// the premultiplied ARGB32 pixmap the StatusNotifierItem wants. TrayFrame goes
// straight there, and is pinned pixel for pixel to that route by the golden
// tests (golden_ref_test.go).
//
// A frame splits in two: the shape, which depends on the bar heights only
// (glyph and halo layout, the expensive part), and the colour pass, which maps
// each shape pixel through the ink and the brand wash. Shapes are memoized; the
// colour pass over the 44x44 canvas is cheap enough to redo per tint step.

// Frame is a ready-to-publish tray pixmap: W*H pixels of premultiplied ARGB32,
// bytes in network order. Treat it as immutable.
type Frame struct {
	W, H int
	Pix  []byte
}

// inkClass marks a glyph (bar) pixel in a shape. Every other non-zero class is
// the alpha of a white halo pixel, which tops out at haloAlpha*255 = 140.
const inkClass = 255

// shapeCap bounds the shape memo: ~2 KB each, so about 1 MB at the limit. A
// full memo is dropped wholesale; the next frames repopulate it.
const shapeCap = 512

var (
	shapeMu sync.Mutex
	shapes  = map[vu.Heights]*[barsSize * barsSize]uint8{}
)

// shapeOf returns the class map for the heights: 0 transparent, inkClass for
// a bar pixel, otherwise the halo alpha. It reuses drawHalo, so the halo is
// the very one the PNG renderer draws.
func shapeOf(h vu.Heights) *[barsSize * barsSize]uint8 {
	shapeMu.Lock()
	if s, ok := shapes[h]; ok {
		shapeMu.Unlock()
		return s
	}
	shapeMu.Unlock()

	mask := barMask(h)
	halo := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
	drawHalo(halo, mask[:], barsSize)
	s := new([barsSize * barsSize]uint8)
	for i := range s {
		if mask[i] {
			s[i] = inkClass
		} else {
			s[i] = halo.Pix[4*i+3]
		}
	}

	shapeMu.Lock()
	if len(shapes) >= shapeCap {
		clear(shapes)
	}
	shapes[h] = s
	shapeMu.Unlock()
	return s
}

// washPixel is the colour of one source pixel after the brand wash, as the
// ARGB32 bytes the tray receives. It mirrors Retint on the PNG route: a fully
// transparent pixel stays untouched, anything else is mixed toward wash by t
// from its premultiplied 8-bit components, keeping its alpha.
func washPixel(src, wash color.NRGBA, t float64) [4]byte {
	r, g, b, a := src.RGBA()
	if a == 0 {
		return [4]byte{}
	}
	px := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	r, g, b, a = mix(px, wash, t).RGBA()
	return [4]byte{byte(a >> 8), byte(r >> 8), byte(g >> 8), byte(b >> 8)}
}

// TrayFrame renders the bars for the given heights and ink straight to the
// tray pixmap, with the brand wash (mixed toward wash by mixT) already applied.
// A zero tint means the theme ink, as in BarsIcon.
func TrayFrame(h vu.Heights, tint, wash color.NRGBA, mixT float64) *Frame {
	ink := ThemeInk(panelIsDark())
	if (tint != color.NRGBA{}) {
		ink = tint
	}
	shape := shapeOf(h)

	// One washed value per class actually met, computed on first use.
	var (
		lut  [256][4]byte
		seen [256]bool
	)
	pix := make([]byte, 4*barsSize*barsSize)
	for i, c := range shape {
		if c == 0 {
			continue // transparent, already zero
		}
		if !seen[c] {
			src := color.NRGBA{0xFF, 0xFF, 0xFF, c}
			if c == inkClass {
				src = ink
			}
			lut[c] = washPixel(src, wash, mixT)
			seen[c] = true
		}
		copy(pix[4*i:], lut[c][:])
	}
	return &Frame{W: barsSize, H: barsSize, Pix: pix}
}
