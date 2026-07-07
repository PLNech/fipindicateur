package icon

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/PLNech/fipindicateur/internal/vu"
)

func TestBarsIconValidPNG(t *testing.T) {
	b := BarsIcon(vu.Heights{0, 3, 5, 7})
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != barsSize || bounds.Dy() != barsSize {
		t.Errorf("size: got %dx%d want %dx%d", bounds.Dx(), bounds.Dy(), barsSize, barsSize)
	}
}

func TestBarsIconCached(t *testing.T) {
	h := vu.Heights{1, 2, 3, 4}
	b1 := BarsIcon(h)
	b2 := BarsIcon(h)
	if len(b1) == 0 {
		t.Fatal("empty icon")
	}
	// Cache hit returns the same underlying bytes: no re-rasterization.
	if &b1[0] != &b2[0] {
		t.Error("expected cached bytes on second call")
	}
	// A different state is a different frame.
	b3 := BarsIcon(vu.Heights{4, 3, 2, 1})
	if bytes.Equal(b1, b3) {
		t.Error("distinct states should render distinct frames")
	}
}
