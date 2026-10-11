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

// TestSetIconPixmapDedupes: an identical pixmap is not sent twice (no property
// set, no NewIcon), a changed one is, and a malformed one is refused.
func TestSetIconPixmapDedupes(t *testing.T) {
	var sent [][]byte
	it := NewDetached(func(w, h int, pix []byte) { sent = append(sent, pix) })
	a := bytes.Repeat([]byte{1, 2, 3, 4}, 4) // 2x2
	b := bytes.Repeat([]byte{4, 3, 2, 1}, 4)

	it.SetIconPixmap(2, 2, a)
	it.SetIconPixmap(2, 2, append([]byte(nil), a...)) // equal content, new slice
	it.SetIconPixmap(2, 2, a)
	if len(sent) != 1 {
		t.Fatalf("identical pixmaps sent %d times, want 1", len(sent))
	}
	it.SetIconPixmap(2, 2, b)
	it.SetIconPixmap(2, 2, a)
	if len(sent) != 3 {
		t.Fatalf("changed pixmaps: %d sends, want 3", len(sent))
	}
	it.SetIconPixmap(2, 2, a[:7]) // wrong length
	it.SetIconPixmap(0, 0, nil)
	if len(sent) != 3 {
		t.Errorf("malformed pixmap reached the host (%d sends)", len(sent))
	}
}

// TestSetIconPNGDedupes: the PNG entry point shares the same guard.
func TestSetIconPNGDedupes(t *testing.T) {
	n := 0
	it := NewDetached(func(w, h int, pix []byte) { n++ })
	data := benchPNG(t)
	it.SetIconPNG(data)
	it.SetIconPNG(data)
	if n != 1 {
		t.Errorf("same PNG sent %d times, want 1", n)
	}
}
