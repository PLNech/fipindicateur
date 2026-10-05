package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestCoverIcon pins the album-cover-to-menu-icon pass: the output is a PNG
// capped at the asked size (never upscaled), the colour of a flat source
// survives the box filter, transparency stays transparent, and any failure
// mode returns nil (no icon beats a broken one).
func TestCoverIcon(t *testing.T) {
	// A 64x64 flat magenta source, half of it fully transparent.
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				src.SetNRGBA(x, y, color.NRGBA{R: 0xE2, G: 0x00, B: 0x7A, A: 0xFF})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	out, err := png.Decode(bytes.NewReader(CoverIcon(buf.Bytes(), 32)))
	if err != nil {
		t.Fatalf("CoverIcon output is not a PNG: %v", err)
	}
	if got := out.Bounds(); got.Dx() != 32 || got.Dy() != 32 {
		t.Fatalf("CoverIcon bounds = %v, want 32x32 (downscale)", got)
	}
	r, g, b, a := out.At(8, 8).RGBA() // left half: opaque magenta
	if uint8(r>>8) != 0xE2 || uint8(g>>8) != 0x00 || uint8(b>>8) != 0x7A || uint8(a>>8) != 0xFF {
		t.Errorf("flat magenta did not survive the box filter: r=%d g=%d b=%d a=%d",
			uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
	}
	if _, _, _, a2 := out.At(24, 8).RGBA(); a2 != 0 {
		t.Errorf("transparent half became opaque: %d", uint8(a2>>8))
	}

	// A source smaller than the asked size keeps its own size (no upscale).
	small := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	var buf2 bytes.Buffer
	if err := png.Encode(&buf2, small); err != nil {
		t.Fatal(err)
	}
	out2, err := png.Decode(bytes.NewReader(CoverIcon(buf2.Bytes(), 32)))
	if err != nil {
		t.Fatalf("small source: %v", err)
	}
	if got := out2.Bounds(); got.Dx() != 16 || got.Dy() != 16 {
		t.Errorf("small source was upscaled: %v, want its own 16x16", got)
	}

	// Failure modes return nil, never a half-rendered icon.
	if got := CoverIcon(nil, 32); got != nil {
		t.Errorf("empty input returned %d bytes, want nil", len(got))
	}
	if got := CoverIcon([]byte("not an image"), 32); got != nil {
		t.Errorf("garbage input returned %d bytes, want nil", len(got))
	}
	if got := CoverIcon(buf.Bytes(), 0); got != nil {
		t.Errorf("size 0 returned %d bytes, want nil", len(got))
	}
}
