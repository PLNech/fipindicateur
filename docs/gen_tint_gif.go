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
	"slices"
	"sort"

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

	stepColor := make([]color.NRGBA, tintSteps)
	for step := 0; step < tintSteps; step++ {
		qt := float64(step) / float64(tintSteps-1)
		stepColor[step] = icon.Lerp(rock, jazz, smoothstep(qt))
	}

	holdFrames := holdSec * animFPS
	crossFrames := crossfadeSec * animFPS

	// Pass 1: flatten every frame over the panel background and collect the
	// exact colors used, so the palette can carry them all losslessly.
	frame := 0
	var flats []*image.NRGBA
	used := map[color.NRGBA]bool{}
	add := func(step int) {
		flat := flatten(decodeBars(barsAt(frame), stepColor[step]))
		flats = append(flats, flat)
		for by := 0; by < base; by++ {
			for bx := 0; bx < base; bx++ {
				used[flat.NRGBAAt(bx, by)] = true
			}
		}
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

	// Shared palette: index 0 is the panel background, then one entry per
	// quantized crossfade step (so every frame's ink is an exact palette
	// match), then the remaining flattened colors (the halo rim's blends).
	pal := color.Palette{panelBG}
	for _, c := range stepColor {
		pal = append(pal, c)
	}
	var extra []color.NRGBA
	for c := range used {
		if c != panelBG && !slices.Contains(stepColor, c) {
			extra = append(extra, c)
		}
	}
	sort.Slice(extra, func(i, j int) bool {
		a, b := extra[i], extra[j]
		return a.R < b.R || (a.R == b.R && (a.G < b.G || (a.G == b.G && a.B < b.B)))
	})
	for _, c := range extra {
		pal = append(pal, c)
	}

	// Pass 2: palettize. Every flattened color is in the palette, so Index
	// reproduces the frame exactly.
	g := &gif.GIF{LoopCount: 0} // 0 == loop forever
	canvas := (base + 2*pad) * scale
	for _, flat := range flats {
		p := image.NewPaletted(image.Rect(0, 0, canvas, canvas), pal)
		for by := 0; by < base; by++ {
			for bx := 0; bx < base; bx++ {
				blit(p, (pad+bx)*scale, (pad+by)*scale, flat.NRGBAAt(bx, by), pal)
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 100/animFPS) // centiseconds per frame
		g.Disposal = append(g.Disposal, gif.DisposalNone)
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

// flatten blends one glyph frame over the panel background: the exact pixels
// the tray would draw, ink and phantom rim alike (the rim is a real pixel
// color at partial alpha, not the ink).
func flatten(bars *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, base, base))
	for by := 0; by < base; by++ {
		for bx := 0; bx < base; bx++ {
			px := bars.NRGBAAt(bx, by)
			switch {
			case px.A == 0:
				px = panelBG
			case px.A != 0xFF:
				a := float64(px.A) / 255
				px = color.NRGBA{
					R: uint8(float64(px.R)*a + float64(panelBG.R)*(1-a)),
					G: uint8(float64(px.G)*a + float64(panelBG.G)*(1-a)),
					B: uint8(float64(px.B)*a + float64(panelBG.B)*(1-a)),
					A: 0xFF,
				}
			}
			out.SetNRGBA(bx, by, px)
		}
	}
	return out
}

// blit fills a scale x scale block with the given color's palette index.
func blit(p *image.Paletted, x0, y0 int, c color.NRGBA, pal color.Palette) {
	idx := uint8(pal.Index(c))
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
		flat := flatten(decodeBars(pose, tint))
		ox := (gap + n*(tile+gap)) * scale
		oy := gap * scale
		for by := 0; by < base; by++ {
			for bx := 0; bx < base; bx++ {
				blitNRGBA(out, ox+bx*scale, oy+by*scale, flat.NRGBAAt(bx, by))
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
