//go:build linux

package sni

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
)

// refPixmap is the original generic conversion: every pixel through
// color.Color.RGBA(). The fast paths must match it byte for byte.
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

// TestImageToPixmapMatchesGeneric covers the NRGBA and RGBA fast paths and the
// generic fallback (Gray here) with random pixels, including semi-transparent.
func TestImageToPixmapMatchesGeneric(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	nrgba := image.NewNRGBA(image.Rect(0, 0, 13, 7))
	rgba := image.NewRGBA(image.Rect(2, 3, 15, 10)) // non-zero origin
	gray := image.NewGray(image.Rect(0, 0, 5, 4))
	rng.Read(nrgba.Pix)
	rng.Read(gray.Pix)
	for y := rgba.Rect.Min.Y; y < rgba.Rect.Max.Y; y++ {
		for x := rgba.Rect.Min.X; x < rgba.Rect.Max.X; x++ {
			a := uint8(rng.Intn(256))
			rgba.SetRGBA(x, y, color.RGBA{uint8(rng.Intn(int(a) + 1)), uint8(rng.Intn(int(a) + 1)), uint8(rng.Intn(int(a) + 1)), a})
		}
	}
	for name, img := range map[string]image.Image{"nrgba": nrgba, "rgba": rgba, "gray": gray} {
		if got, want := imageToPixmap(img).Pix, refPixmap(img); !bytes.Equal(got, want) {
			t.Errorf("%s: fast pixmap differs from the generic conversion", name)
		}
	}
}

// TestPNGToPixmapMatchesGeneric is the same check through a real PNG decode.
func TestPNGToPixmapMatchesGeneric(t *testing.T) {
	data := benchPNG(t)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	pm, err := pngToPixmap(data)
	if err != nil {
		t.Fatal(err)
	}
	if pm.W != 44 || pm.H != 44 || !bytes.Equal(pm.Pix, refPixmap(img)) {
		t.Errorf("pngToPixmap differs from the generic conversion (%dx%d)", pm.W, pm.H)
	}
}
