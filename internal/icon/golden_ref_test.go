package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// The reference pipeline: the original, unoptimized implementations kept
// verbatim as oracles. The optimized paths must reproduce them pixel for pixel.

// refDrawHalo is the original brute-force phantom outline.
func refDrawHalo(img *image.NRGBA, mask []bool, size int) {
	width := haloWidth(size)
	r := int(math.Ceil(width))
	at := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < size && y < size && mask[y*size+x]
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if at(x, y) {
				continue
			}
			best := math.MaxFloat64
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					if at(x+dx, y+dy) {
						if d := math.Hypot(float64(dx), float64(dy)); d < best {
							best = d
						}
					}
				}
			}
			if best > width {
				continue
			}
			f := 1.0
			if best > haloInner {
				f = 1.0 - (best-haloInner)/(width-haloInner)
			}
			a := haloAlpha * clamp01Halo(f)
			if a <= 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{0xFF, 0xFF, 0xFF, uint8(a * 255)})
		}
	}
}

// refBarsImage is the original bar rasterization, with the original halo.
func refBarsImage(h vu.Heights, dark bool, tint color.NRGBA) *image.NRGBA {
	ink := ThemeInk(dark)
	if (tint != color.NRGBA{}) {
		ink = tint
	}
	img := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
	const (
		barW   = 6
		gap    = 4
		bottom = barsSize - 3
		unit   = 3
		stub   = 3
	)
	x0 := (barsSize - (vu.Bars*barW + (vu.Bars-1)*gap)) / 2
	var mask [barsSize * barsSize]bool
	for i := 0; i < vu.Bars; i++ {
		barH := stub + int(h[i])*unit
		xs := x0 + i*(barW+gap)
		for y := bottom - barH; y < bottom; y++ {
			for x := xs; x < xs+barW; x++ {
				mask[y*barsSize+x] = true
			}
		}
	}
	refDrawHalo(img, mask[:], barsSize)
	for y := 0; y < barsSize; y++ {
		for x := 0; x < barsSize; x++ {
			if mask[y*barsSize+x] {
				img.SetNRGBA(x, y, ink)
			}
		}
	}
	return img
}

// refWash is the original Retint, minus the PNG codec: the same per-pixel math
// on an NRGBA image (the PNG round trip is lossless for 8-bit NRGBA).
func refWash(src *image.NRGBA, c color.NRGBA, t float64) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			px := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
			out.SetNRGBA(x, y, mix(px, c, t))
		}
	}
	return out
}

// refPixmap is the original pngToPixmap pixel loop: premultiplied ARGB32 bytes.
func refPixmap(img image.Image) []byte {
	b := img.Bounds()
	pix := make([]byte, 0, 4*b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			pix = append(pix, byte(a>>8), byte(r>>8), byte(g>>8), byte(bl>>8))
		}
	}
	return pix
}

// refTrayPixmap is the whole original per-frame pipeline: render, PNG encode,
// Retint (decode, wash, encode), then the SNI decode to ARGB32. It uses the
// real Retint and real PNG codec, so it is the true end-to-end oracle.
func refTrayPixmap(h vu.Heights, dark bool, tint, wash color.NRGBA, mixT float64) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, refBarsImage(h, dark, tint)); err != nil {
		return nil, err
	}
	tinted := Retint(buf.Bytes(), wash, mixT)
	img, err := png.Decode(bytes.NewReader(tinted))
	if err != nil {
		return nil, err
	}
	return refPixmap(img), nil
}

// goldenInputs is the (heights, tint) grid the golden tests sweep: silence,
// rest, full scale, the envelope's real range, plus tints from zero (theme
// ink) through brand inks to a translucent paused ink.
func goldenInputs() (hs []vu.Heights, tints []color.NRGBA) {
	hs = []vu.Heights{
		{}, {3, 7, 4, 9}, {11, 11, 11, 11}, {0, 1, 0, 2}, {5, 8, 6, 10},
		{11, 0, 11, 0}, {1, 2, 3, 4}, {4, 3, 2, 1}, {9, 9, 9, 9}, {2, 11, 1, 6},
	}
	// Plus a pseudo-random spread of the full state space.
	s := uint32(12345)
	for i := 0; i < 30; i++ {
		var h vu.Heights
		for b := range h {
			s = s*1664525 + 1013904223
			h[b] = uint8((s >> 24) % (vu.MaxHeight + 1))
		}
		hs = append(hs, h)
	}
	tints = []color.NRGBA{
		{}, // theme ink
		{0xE2, 0x00, 0x7A, 0xFF},
		{0xF5, 0xF5, 0xF5, 0xFF},
		{0x2B, 0x2B, 0x2B, 0xFF},
		{0x7F, 0x3A, 0x9B, 0xFF},
		{0xF5, 0xF5, 0xF5, restDimAlpha}, // paused, dimmed ink
		{0x2B, 0x2B, 0x2B, restDimAlpha},
		{0x10, 0xC0, 0x40, 0x80},
	}
	return hs, tints
}
