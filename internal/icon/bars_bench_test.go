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

// BenchmarkTrayFrameWarm is the animator's per-frame render: the shape is
// memoized, so this is the colour pass plus the pixmap allocation.
func BenchmarkTrayFrameWarm(b *testing.B) {
	wash := ParseHex("#e2007a")
	for _, h := range benchHeights {
		TrayFrame(h, benchTint, wash, 0.9)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TrayFrame(benchHeights[i%len(benchHeights)], benchTint, wash, 0.9)
	}
}

// BenchmarkTrayFrameCold is a shape-memo miss: halo included.
func BenchmarkTrayFrameCold(b *testing.B) {
	wash := ParseHex("#e2007a")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		shapeMu.Lock()
		clear(shapes)
		shapeMu.Unlock()
		b.StartTimer()
		_ = TrayFrame(benchHeights[i%len(benchHeights)], benchTint, wash, 0.9)
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
