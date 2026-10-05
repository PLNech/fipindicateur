package icon

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
)

// CoverIcon renders an album cover as a menu icon: decode whatever image
// format the stdlib knows (Radio France serves JPEG), area-average downscale
// to fit size (never upscale: a smaller cover stays its own size), encode
// PNG (what the dbusmenu icon-data property expects). Any decode, scale or
// encode failure returns nil: no icon beats a broken one, and a nil return
// also means "clear the icon" for the caller.
//
// The average is alpha-weighted over unpremultiplied samples, so a cover with
// transparency does not bleed its transparent pixels' colour into the average
// (the box filter's classic dark-halo trap).
func CoverIcon(b []byte, size int) []byte {
	if len(b) == 0 || size <= 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil
	}
	dstW, dstH := srcW, srcH
	if dstW > size {
		dstW = size
	}
	if dstH > size {
		dstH = size
	}
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	out := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		y0 := bounds.Min.Y + dy*srcH/dstH
		y1 := bounds.Min.Y + (dy+1)*srcH/dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < dstW; dx++ {
			x0 := bounds.Min.X + dx*srcW/dstW
			x1 := bounds.Min.X + (dx+1)*srcW/dstW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var rs, gs, bs, as, n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, a := img.At(x, y).RGBA()
					if a == 0 {
						continue // transparent: no colour to average in
					}
					// Unpremultiply to the 8-bit channels the output holds,
					// so averaging mixes plain colours, not premultiplied
					// ones against varying alphas.
					rs += uint64(unpremul(r, a))
					gs += uint64(unpremul(g, a))
					bs += uint64(unpremul(bl, a))
					as += uint64(a >> 8)
					n++
				}
			}
			if n == 0 || as == 0 {
				continue // fully transparent box stays transparent
			}
			out.SetNRGBA(dx, dy, color.NRGBA{
				R: uint8(rs / n),
				G: uint8(gs / n),
				B: uint8(bs / n),
				A: uint8(as / n),
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil
	}
	return buf.Bytes()
}

// unpremul converts one premultiplied 16-bit channel back to its 8-bit
// unpremultiplied value. alpha is the pixel's 16-bit alpha.
func unpremul(c, alpha uint32) uint8 {
	return uint8(math.Round(float64(c) * 255 / float64(alpha)))
}
