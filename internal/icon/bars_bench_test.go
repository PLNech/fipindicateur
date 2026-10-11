package icon

import (
	"image/color"
	"testing"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// benchHeights is a handful of realistic envelope poses.
var benchHeights = []vu.Heights{
	{3, 7, 4, 9}, {5, 8, 6, 10}, {0, 1, 0, 2}, {11, 9, 10, 8},
}

var benchTint = color.NRGBA{0xE2, 0x00, 0x7A, 0xFF}

// BenchmarkRenderBars is a frame-cache miss: raster, halo and PNG encode.
func BenchmarkRenderBars(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = renderBars(benchHeights[i%len(benchHeights)], true, benchTint)
	}
}

// BenchmarkBarsIconHit is the steady state: a cached frame lookup.
func BenchmarkBarsIconHit(b *testing.B) {
	for _, h := range benchHeights {
		BarsIcon(h, benchTint)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BarsIcon(benchHeights[i%len(benchHeights)], benchTint)
	}
}

// BenchmarkRetint is the FIP wash applied to every frame before it ships.
func BenchmarkRetint(b *testing.B) {
	png := renderBars(benchHeights[0], true, benchTint)
	wash := ParseHex("#e2007a")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Retint(png, wash, 0.9)
	}
}
