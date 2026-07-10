//go:build ignore

// Command gen_tint_gif renders the tray VU glyph's station-color crossfade to
// an animated GIF, and a strip of every station's legible tint to a PNG, using
// the app's real icon code (internal/icon, internal/vu). Pixel-honest: no
// screen capture, every frame is the exact bitmap the tray would draw.
//
// Run:  go run docs/gen_tint_gif.go
// Writes docs/tint-transition.gif and docs/station-colors.png.
//
// The GIF mirrors internal/ui/animator.go exactly: a ~1s hold on Rock, the 10s
// crossfade at 6 fps with smoothstep easing quantized to 16 steps, a ~1s hold
// on Jazz, looping forever. The 4 VU bars move to a deterministic sum-of-sines
// pattern (no math/rand), quantized to the 12 height levels.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/vu"
)

// These mirror internal/ui/animator.go so the doc GIF matches what ships.
const (
	animFPS      = 6
	tintSteps    = 16
	crossfadeSec = 10
	holdSec      = 1
)

// Render geometry: the 44px glyph on a dark panel with padding, upscaled 4x
// nearest-neighbour so it reads at README size.
const (
	base  = 44 // matches icon.barsSize
	pad   = 4  // dark-panel padding around the glyph, in base pixels
	scale = 4  // nearest-neighbour upscale factor
)

var panelBG = color.NRGBA{0x26, 0x26, 0x26, 0xFF} // matches icon.panelSurfaceDark

func main() {
	if err := writeGIF(outPath("tint-transition.gif")); err != nil {
		log.Fatalf("tint-transition.gif: %v", err)
	}
	if err := writeStrip(outPath("station-colors.png")); err != nil {
		log.Fatalf("station-colors.png: %v", err)
	}
}

// outPath resolves a docs/ output path whether run from the repo root or from
// within docs/.
func outPath(name string) string {
	if _, err := os.Stat("docs"); err == nil {
		return filepath.Join("docs", name)
	}
	return name
}

// smoothstep eases 0..1 with a symmetric ease-in-out (mirrors animator.go).
func smoothstep(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

// quantizedTintColor is the crossfade tint for progress t in 0..1, snapped to
// one of tintSteps discrete positions, exactly as animator.quantizedTint does.
func quantizedTintColor(from, to color.NRGBA, t float64) color.NRGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	step := math.Round(t * float64(tintSteps-1))
	qt := step / float64(tintSteps-1)
	return icon.Lerp(from, to, smoothstep(qt))
}

// barsAt derives a deterministic, music-like 4-bar pose for a global frame
// index: sums of sines mapped into 0..vu.MaxHeight. No randomness so the GIF is
// byte-reproducible.
func barsAt(frame int) vu.Heights {
	var h vu.Heights
	f := float64(frame)
	for i := 0; i < vu.Bars; i++ {
		phase := float64(i) * 1.1
		s := 0.5 + 0.45*math.Sin(0.55*f+phase)
		s *= 0.65 + 0.35*(0.5+0.5*math.Sin(0.21*f+phase*1.7))
		s += 0.12 * math.Sin(1.27*f+phase*0.5)
		switch {
		case s < 0:
			s = 0
		case s > 1:
			s = 1
		}
		q := int(math.Round(s * float64(vu.MaxHeight)))
		if q > vu.MaxHeight {
			q = vu.MaxHeight
		}
		h[i] = uint8(q)
	}
	return h
}

// decodeBars renders the glyph via the real icon code and decodes it to NRGBA.
// A non-zero tint makes every bar pixel exactly that tint at full alpha, so we
// can read shape from alpha and paint the frame's own quantized tint.
func decodeBars(h vu.Heights, tint color.NRGBA) *image.NRGBA {
	img, err := png.Decode(bytes.NewReader(icon.BarsIcon(h, tint)))
	if err != nil {
		log.Fatalf("decode bars: %v", err)
	}
	if nr, ok := img.(*image.NRGBA); ok {
		return nr
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

// writeGIF renders the hold/crossfade/hold sequence and encodes a looping GIF.
func writeGIF(path string) error {
	rock := icon.Legible("#f93446", true) // Rock brand ink, made legible on dark
	jazz := icon.Legible("#13898d", true) // Jazz brand ink, made legible on dark

	// Shared palette: index 0 is the panel background, then one entry per
	// quantized crossfade step (so every frame's ink is an exact palette match).
	pal := color.Palette{panelBG}
	stepColor := make([]color.NRGBA, tintSteps)
	for step := 0; step < tintSteps; step++ {
		qt := float64(step) / float64(tintSteps-1)
		c := icon.Lerp(rock, jazz, smoothstep(qt))
		stepColor[step] = c
		pal = append(pal, c)
	}
	stepIndex := func(step int) uint8 { return uint8(1 + step) } // palette index

	holdFrames := holdSec * animFPS
	crossFrames := crossfadeSec * animFPS

	g := &gif.GIF{LoopCount: 0} // 0 == loop forever
	frame := 0
	add := func(step int) {
		p := composite(decodeBars(barsAt(frame), stepColor[step]), stepIndex(step), pal)
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 100/animFPS) // centiseconds per frame
		g.Disposal = append(g.Disposal, gif.DisposalNone)
		frame++
	}

	for i := 0; i < holdFrames; i++ { // hold on Rock (step 0)
		add(0)
	}
	for i := 0; i < crossFrames; i++ { // 10s crossfade, quantized progress
		t := float64(i) / float64(crossFrames)
		step := int(math.Round(t * float64(tintSteps-1)))
		add(step)
	}
	for i := 0; i < holdFrames; i++ { // hold on Jazz (last step)
		add(tintSteps - 1)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := gif.EncodeAll(f, g); err != nil {
		return err
	}
	log.Printf("wrote %s (%d frames)", path, len(g.Image))
	return nil
}

// composite paints one glyph frame onto the padded, upscaled paletted canvas:
// background everywhere, ink pixels (alpha > 0) become inkIdx.
func composite(bars *image.NRGBA, inkIdx uint8, pal color.Palette) *image.Paletted {
	canvas := base + 2*pad
	out := image.NewPaletted(image.Rect(0, 0, canvas*scale, canvas*scale), pal)
	// out is zero-filled == palette index 0 == panel background.
	for by := 0; by < base; by++ {
		for bx := 0; bx < base; bx++ {
			if bars.NRGBAAt(bx, by).A == 0 {
				continue
			}
			blit(out, (pad+bx)*scale, (pad+by)*scale, inkIdx)
		}
	}
	return out
}

// blit fills a scale x scale block with the given palette index.
func blit(p *image.Paletted, x0, y0 int, idx uint8) {
	for dy := 0; dy < scale; dy++ {
		for dx := 0; dx < scale; dx++ {
			p.SetColorIndex(x0+dx, y0+dy, idx)
		}
	}
}

// writeStrip renders every station's legible tint over one fixed VU pose, side
// by side on the dark panel, upscaled 4x.
func writeStrip(path string) error {
	pose := vu.Heights{7, 10, 5, 9} // one pleasant fixed pose
	const gap = 6                   // base pixels of panel between glyphs

	tile := base
	stripW := (len(stations.All)*tile + (len(stations.All)+1)*gap) * scale
	stripH := (tile + 2*gap) * scale
	out := image.NewNRGBA(image.Rect(0, 0, stripW, stripH))
	fill(out, panelBG)

	for n, s := range stations.All {
		tint := icon.Legible(s.Color, true)
		bars := decodeBars(pose, tint)
		ox := (gap + n*(tile+gap)) * scale
		oy := gap * scale
		for by := 0; by < base; by++ {
			for bx := 0; bx < base; bx++ {
				px := bars.NRGBAAt(bx, by)
				if px.A == 0 {
					continue
				}
				blitNRGBA(out, ox+bx*scale, oy+by*scale, tint)
			}
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		return err
	}
	log.Printf("wrote %s (%d stations)", path, len(stations.All))
	return nil
}

func fill(img *image.NRGBA, c color.NRGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func blitNRGBA(img *image.NRGBA, x0, y0 int, c color.NRGBA) {
	for dy := 0; dy < scale; dy++ {
		for dx := 0; dx < scale; dx++ {
			img.SetNRGBA(x0+dx, y0+dy, c)
		}
	}
}
