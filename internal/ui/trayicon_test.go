package ui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// makePNG builds a real PNG of the given dimensions for the ICO wrapper tests.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestPngToICOHeaderAndPayload(t *testing.T) {
	src := makePNG(t, 22, 22)
	ico := pngToICO(src)

	if len(ico) != 22+len(src) {
		t.Fatalf("ico length = %d, want header(22)+png(%d) = %d", len(ico), len(src), 22+len(src))
	}

	// ICONDIR: reserved=0, type=1 (icon), count=1.
	if got := binary.LittleEndian.Uint16(ico[0:2]); got != 0 {
		t.Errorf("reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(ico[2:4]); got != 1 {
		t.Errorf("type = %d, want 1 (icon)", got)
	}
	if got := binary.LittleEndian.Uint16(ico[4:6]); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}

	// ICONDIRENTRY.
	if ico[6] != 22 {
		t.Errorf("bWidth = %d, want 22", ico[6])
	}
	if ico[7] != 22 {
		t.Errorf("bHeight = %d, want 22", ico[7])
	}
	if got := binary.LittleEndian.Uint16(ico[10:12]); got != 1 {
		t.Errorf("wPlanes = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint16(ico[12:14]); got != 32 {
		t.Errorf("wBitCount = %d, want 32", got)
	}
	if got := binary.LittleEndian.Uint32(ico[14:18]); got != uint32(len(src)) {
		t.Errorf("dwBytesInRes = %d, want %d", got, len(src))
	}
	if got := binary.LittleEndian.Uint32(ico[18:22]); got != 22 {
		t.Errorf("dwImageOffset = %d, want 22", got)
	}

	// The raw PNG must sit verbatim at the declared offset.
	if !bytes.Equal(ico[22:], src) {
		t.Error("PNG payload at offset 22 does not match the source bytes")
	}
}

func TestPngToICO256IsZero(t *testing.T) {
	// A 256px dimension must encode as the byte 0 per the ICO spec.
	src := makePNG(t, 256, 256)
	ico := pngToICO(src)
	if ico[6] != 0 {
		t.Errorf("bWidth for 256px = %d, want 0", ico[6])
	}
	if ico[7] != 0 {
		t.Errorf("bHeight for 256px = %d, want 0", ico[7])
	}
}

func TestPngDimensions(t *testing.T) {
	src := makePNG(t, 44, 30)
	w, h := pngDimensions(src)
	if w != 44 || h != 30 {
		t.Errorf("pngDimensions = %d x %d, want 44 x 30", w, h)
	}

	// Non-PNG input yields 0,0 (treated as 256 downstream) rather than panicking.
	if w, h := pngDimensions([]byte("not a png")); w != 0 || h != 0 {
		t.Errorf("pngDimensions(garbage) = %d,%d, want 0,0", w, h)
	}
}
