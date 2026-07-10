package ui

import (
	"image/color"
	"testing"
	"time"
)

// TestTintQuantization walks a full 10s transition at fine granularity and
// asserts the animator emits at most tintSteps distinct tints, so the frame
// cache stays bounded and no more than tintSteps extra SetIcon calls fire.
func TestTintQuantization(t *testing.T) {
	an := &animator{}
	an.setTintTarget(color.NRGBA{0xE2, 0x00, 0x7A, 0xFF})
	start := an.tintStart

	seen := map[color.NRGBA]bool{}
	const samples = 600 // ~100x finer than the 6 fps loop would sample
	for i := 0; i <= samples; i++ {
		now := start.Add(time.Duration(i) * tintDur / samples)
		seen[an.quantizedTint(now)] = true
	}
	if len(seen) == 0 {
		t.Fatal("no tints produced")
	}
	if len(seen) > tintSteps {
		t.Errorf("distinct tints = %d, want <= %d", len(seen), tintSteps)
	}
}

// TestTintBeforeFirstTargetIsZero: with no target set, the drawn tint is the
// zero value, which BarsIcon reads as "use theme ink".
func TestTintBeforeFirstTargetIsZero(t *testing.T) {
	an := &animator{}
	if got := an.quantizedTint(time.Now()); got != (color.NRGBA{}) {
		t.Errorf("tint before any target = %v, want zero", got)
	}
}

// TestZapMidTransitionStartsFromCurrent: a second target set mid-fade seeds its
// start from the interpolated current tint, not the old endpoint.
func TestZapMidTransitionStartsFromCurrent(t *testing.T) {
	an := &animator{}
	first := color.NRGBA{0xFF, 0x00, 0x00, 0xFF}
	an.setTintTarget(first)
	// Rewind the start so we are ~halfway through the first fade.
	an.mu.Lock()
	mid := an.currentTintLocked(an.tintStart.Add(tintDur / 2))
	an.tintStart = time.Now().Add(-tintDur / 2)
	an.mu.Unlock()

	an.setTintTarget(color.NRGBA{0x00, 0x00, 0xFF, 0xFF})
	an.mu.Lock()
	from := an.tintFrom
	an.mu.Unlock()
	if from == first {
		t.Error("zap should start from the interpolated current tint, not the old endpoint")
	}
	// from should be near the halfway value of the first fade.
	if from.R == 0 && from.G == 0 && from.B == 0 {
		t.Errorf("unexpected start tint %v (halfway ref %v)", from, mid)
	}
}
