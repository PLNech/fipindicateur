package icon

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// TestRenderBarsMatchesReference: the PNG renderer (optimized halo included)
// produces the very same pixels as the original brute-force implementation.
func TestRenderBarsMatchesReference(t *testing.T) {
	hs, tints := goldenInputs()
	for _, dark := range []bool{false, true} {
		for _, h := range hs {
			for _, tint := range tints {
				got := renderBars(h, dark, tint)
				var want bytes.Buffer
				if err := png.Encode(&want, refBarsImage(h, dark, tint)); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want.Bytes()) {
					t.Fatalf("renderBars(%v, dark=%v, tint=%v) differs from the reference", h, dark, tint)
				}
			}
		}
	}
}

// TestDrawHaloMatchesReference compares the halo layer alone, on the glyph
// shapes plus an arbitrary mask touching the canvas edges.
func TestDrawHaloMatchesReference(t *testing.T) {
	hs, _ := goldenInputs()
	for _, h := range hs {
		got := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
		want := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
		mask := barMask(h)
		drawHalo(got, mask[:], barsSize)
		refDrawHalo(want, mask[:], barsSize)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("halo for %v differs from the reference", h)
		}
	}
	edge := make([]bool, barsSize*barsSize)
	for i := 0; i < barsSize; i++ {
		edge[i] = true                         // top row
		edge[i*barsSize+barsSize-1] = i%3 == 0 // right column, dashed
	}
	got := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
	want := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))
	drawHalo(got, edge, barsSize)
	refDrawHalo(want, edge, barsSize)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("halo for an edge-touching mask differs from the reference")
	}
}
