//go:build ignore

// Command gen renders the tray icons: a minimalist broadcast/radio-waves glyph
// (a dot with concentric arcs) on a transparent background, in light and dark
// ink and their dimmed (paused) variants, at 22px and 44px.
//
// Run:  go run ./internal/icon/gen
// It writes the PNGs into internal/icon/.
//
// This is our own glyph, NOT the FIP logo (trademark).
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

func main() {
	light := color.NRGBA{0xF5, 0xF5, 0xF5, 0xFF}
	dark := color.NRGBA{0x2B, 0x2B, 0x2B, 0xFF}

	variants := []struct {
		name string
		ink  color.NRGBA
	}{
		{"light", light},
		{"dark", dark},
		{"light_dim", dim(light)},
		{"dark_dim", dim(dark)},
	}
	sizes := []int{22, 44}

	outDir := "internal/icon"
	if _, err := os.Stat(outDir); err != nil {
		outDir = "." // when run from within the package dir
	}

	for _, v := range variants {
		for _, s := range sizes {
			img := render(s, v.ink)
			path := filepath.Join(outDir, "icon_"+v.name+"_"+itoa(s)+".png")
			if err := save(path, img); err != nil {
				log.Fatalf("save %s: %v", path, err)
			}
			log.Printf("wrote %s", path)
		}
	}

	// Launcher badge: light glyph on a dark rounded square, readable on any
	// theme (the transparent monochrome tray glyphs disappear on matching
	// backgrounds in the activities grid). Sizes for hicolor 22/44/128.
	for _, s := range []int{22, 44, 128} {
		img := renderBadge(s, light, color.NRGBA{0x2B, 0x2B, 0x2B, 0xFF})
		path := filepath.Join(outDir, "icon_app_"+itoa(s)+".png")
		if err := save(path, img); err != nil {
			log.Fatalf("save %s: %v", path, err)
		}
		log.Printf("wrote %s", path)
	}
}

// renderBadge draws the glyph over a rounded-square background.
func renderBadge(size int, ink, bg color.NRGBA) *image.NRGBA {
	img := render(size, ink)
	f := float64(size)
	radius := f * 0.22
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			// signed distance to the rounded rectangle (inset 0.5px)
			hw := f/2 - 0.5
			dx := math.Abs(px-f/2) - (hw - radius)
			dy := math.Abs(py-f/2) - (hw - radius)
			if dx < 0 {
				dx = 0
			}
			if dy < 0 {
				dy = 0
			}
			d := math.Hypot(dx, dy) - radius
			cov := clamp01(-d + 0.5)
			if cov <= 0 {
				continue
			}
			// composite glyph over background
			g := img.NRGBAAt(x, y)
			ga := float64(g.A) / 255
			r := float64(g.R)*ga + float64(bg.R)*(1-ga)
			gg := float64(g.G)*ga + float64(bg.G)*(1-ga)
			b := float64(g.B)*ga + float64(bg.B)*(1-ga)
			out.SetNRGBA(x, y, color.NRGBA{uint8(r), uint8(gg), uint8(b), uint8(cov * 255)})
		}
	}
	return out
}

func dim(c color.NRGBA) color.NRGBA {
	c.A = 0x80
	return c
}

// render draws the broadcast glyph at the given size.
func render(size int, ink color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	f := float64(size)

	// Anchor the dot lower-left, arcs opening to the upper-right.
	cx := f * 0.30
	cy := f * 0.72
	dotR := f * 0.11

	// Concentric arc radii and stroke width scale with size.
	stroke := f * 0.075
	arcs := []float64{f * 0.30, f * 0.46, f * 0.62}

	// Arc angular window: from ~ -80° (up) to +10° (right), i.e. upper-right sweep.
	aMin := -80.0 * math.Pi / 180.0
	aMax := 10.0 * math.Pi / 180.0

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) + 0.5
			py := float64(y) + 0.5
			dx := px - cx
			dy := py - cy
			dist := math.Hypot(dx, dy)

			var cov float64

			// Dot (filled circle) with 1px antialias.
			cov = math.Max(cov, coverage(dotR-dist, 1.0))

			// Arcs.
			ang := math.Atan2(dy, dx)
			if ang >= aMin && ang <= aMax {
				for _, r := range arcs {
					d := math.Abs(dist-r) - stroke/2
					cov = math.Max(cov, coverage(-d, 1.0))
				}
			}

			if cov > 0 {
				a := float64(ink.A) / 255.0 * clamp01(cov)
				img.SetNRGBA(x, y, color.NRGBA{ink.R, ink.G, ink.B, uint8(a * 255)})
			}
		}
	}
	return img
}

// coverage returns antialiased coverage in [0,1] for a signed distance `d`
// (positive = inside) over a feather width `w`.
func coverage(d, w float64) float64 {
	return clamp01(d/w + 0.5)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func save(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
