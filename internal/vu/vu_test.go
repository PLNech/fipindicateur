package vu

import (
	"math"
	"testing"
)

func TestLevelFromDBClamps(t *testing.T) {
	if l := LevelFromDB(-100); l != 0 {
		t.Errorf("-100 dB: got %f want 0", l)
	}
	if l := LevelFromDB(10); l != 1 {
		t.Errorf("+10 dB: got %f want 1", l)
	}
	if l := LevelFromDB(-60); l != 0 {
		t.Errorf("-60 dB: got %f want 0", l)
	}
	if l := LevelFromDB(0); l != 1 {
		t.Errorf("0 dB: got %f want 1", l)
	}
	if l := LevelFromDB(-30); math.Abs(l-0.5) > 1e-9 {
		t.Errorf("-30 dB: got %f want 0.5", l)
	}
	if l := LevelFromDB(math.NaN()); l != 0 {
		t.Errorf("NaN: got %f want 0", l)
	}
}

func TestLevelFromDBMonotonic(t *testing.T) {
	prev := -1.0
	for db := -70.0; db <= 5.0; db += 0.5 {
		l := LevelFromDB(db)
		if l < prev {
			t.Fatalf("not monotonic at %f dB: %f < %f", db, l, prev)
		}
		prev = l
	}
}

func TestTargetsBoundsAndDeterminism(t *testing.T) {
	for frame := 0; frame < 50; frame++ {
		for _, level := range []float64{0, 0.25, 0.5, 0.75, 1} {
			h1 := Targets(level, frame)
			h2 := Targets(level, frame)
			if h1 != h2 {
				t.Fatalf("not deterministic: %v vs %v", h1, h2)
			}
			for i, v := range h1 {
				if v > MaxHeight {
					t.Fatalf("bar %d out of range: %d (level %f frame %d)", i, v, level, frame)
				}
			}
		}
	}
	// Silence is all-zero regardless of frame.
	if h := Targets(0, 17); h != (Heights{}) {
		t.Errorf("silence should be flat, got %v", h)
	}
}

func TestTargetsVaryAcrossBars(t *testing.T) {
	// At a mid level, bars should not all be equal on every frame (that would
	// mean lockstep movement).
	varied := false
	for frame := 0; frame < 10 && !varied; frame++ {
		h := Targets(0.6, frame)
		for i := 1; i < Bars; i++ {
			if h[i] != h[0] {
				varied = true
				break
			}
		}
	}
	if !varied {
		t.Error("bars move in lockstep")
	}
}

func TestEnvelopeAttackInstant(t *testing.T) {
	got := Envelope(Heights{0, 1, 2, 3}, Heights{7, 7, 7, 7})
	if got != (Heights{7, 7, 7, 7}) {
		t.Errorf("attack should be instant, got %v", got)
	}
}

func TestEnvelopeDecayBounded(t *testing.T) {
	h := Heights{7, 7, 7, 7}
	target := Heights{}
	steps := []Heights{
		{5, 5, 5, 5},
		{3, 3, 3, 3},
		{1, 1, 1, 1},
		{0, 0, 0, 0}, // 1-2 clamps to target 0
		{0, 0, 0, 0}, // stays at floor
	}
	for i, want := range steps {
		h = Envelope(h, target)
		if h != want {
			t.Fatalf("decay step %d: got %v want %v", i, h, want)
		}
	}
}

func TestEnvelopeNeverBelowTarget(t *testing.T) {
	h := Envelope(Heights{7, 7, 7, 7}, Heights{6, 6, 6, 6})
	if h != (Heights{6, 6, 6, 6}) {
		t.Errorf("decay must floor at target, got %v", h)
	}
}

func TestHeightsIsStableMapKey(t *testing.T) {
	// Heights must work as a map key: same values, same bucket.
	m := map[Heights]int{}
	m[Heights{1, 2, 3, 4}]++
	m[Heights{1, 2, 3, 4}]++
	if len(m) != 1 || m[Heights{1, 2, 3, 4}] != 2 {
		t.Errorf("array key not stable: %v", m)
	}
	m[Heights{4, 3, 2, 1}]++
	if len(m) != 2 {
		t.Error("distinct heights should be distinct keys")
	}
}
