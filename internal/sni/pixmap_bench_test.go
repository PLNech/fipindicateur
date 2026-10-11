//go:build linux

package sni

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// benchPNG is a 44x44 NRGBA PNG with a semi-transparent halo and opaque bars,
// the shape of the tray frames.
func benchPNG(tb testing.TB) []byte {
	tb.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 44, 44))
	for y := 0; y < 44; y++ {
		for x := 0; x < 44; x++ {
			switch {
			case x >= 8 && x < 36 && y >= 10 && y < 41:
				img.SetNRGBA(x, y, color.NRGBA{0xE2, 0x00, 0x7A, 0xFF})
			case x >= 5 && x < 39 && y >= 7:
				img.SetNRGBA(x, y, color.NRGBA{0xFF, 0xFF, 0xFF, 0x60})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

// BenchmarkPNGToPixmap is the decode + ARGB32 repack of one frame.
func BenchmarkPNGToPixmap(b *testing.B) {
	data := benchPNG(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pngToPixmap(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetIconPNGDetached is the whole SNI half of a frame minus the bus.
func BenchmarkSetIconPNGDetached(b *testing.B) {
	// Two different icons alternate so the identical-pixmap guard never hides
	// the decode and send cost.
	img, _ := png.Decode(bytes.NewReader(benchPNG(b)))
	flipped := image.NewNRGBA(img.Bounds())
	for y := 0; y < 44; y++ {
		for x := 0; x < 44; x++ {
			flipped.Set(x, y, img.At(43-x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, flipped); err != nil {
		b.Fatal(err)
	}
	data := [2][]byte{benchPNG(b), buf.Bytes()}
	it := NewDetached(func(w, h int, pix []byte) {})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it.SetIconPNG(data[i&1])
	}
}
