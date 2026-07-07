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
