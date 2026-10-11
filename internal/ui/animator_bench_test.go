//go:build linux

package ui

import (
	"image/color"
	"testing"

	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/sni"
	"github.com/PLNech/fipindicateur/internal/vu"
)

// benchFrames is a deterministic walk of realistic animator frames: the real
// envelope over a swelling level, with the ink advancing through the tint steps.
// Consecutive frames always differ so no dedupe short-circuits the benchmark.
func benchFrames() []frameKey {
	from := color.NRGBA{0xF5, 0xF5, 0xF5, 0xFF}
	to := color.NRGBA{0xE2, 0x00, 0x7A, 0xFF}
	var (
		out  []frameKey
		prev vu.Heights
	)
	for i := 0; i < 96; i++ {
		level := 0.35 + 0.3*float64(i%12)/12
		h := vu.Envelope(prev, vu.Targets(level, i))
		prev = h
		qt := float64(i/6%tintSteps) / float64(tintSteps-1)
		k := frameKey{h: h, tint: icon.Lerp(from, to, smoothstep(qt))}
		if len(out) > 0 && out[len(out)-1] == k {
			continue
		}
		out = append(out, k)
	}
	return out
}

// withSink installs a bus-less SNI item for the duration of a benchmark and
// returns a counter of icon updates it received.
func withSink(b *testing.B) *int {
	b.Helper()
	n := new(int)
	it := sni.NewDetached(func(w, h int, pix []byte) { *n++ })
	sniMu.Lock()
	old := sniItem
	sniItem = it
	sniMu.Unlock()
	b.Cleanup(func() {
		sniMu.Lock()
		sniItem = old
		sniMu.Unlock()
	})
	return n
}

// BenchmarkAnimFrame is one animator frame end to end with warm caches, minus
// audio and the bus: frame lookup, tint wash, ARGB repack, handoff to the SNI.
func BenchmarkAnimFrame(b *testing.B) {
	n := withSink(b)
	a := &App{}
	frames := benchFrames()
	for _, k := range frames { // warm every cache
		a.drawFrame(k)
	}
	*n = 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.drawFrame(frames[i%len(frames)])
	}
	b.StopTimer()
	if *n == 0 {
		b.Fatal("no icon reached the sink")
	}
}
