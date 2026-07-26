package ui

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
)

// TestVolCoalescerLeadingEdge: the first record of a gesture lands
// immediately, so a lone preset click or wheel notch is never delayed.
func TestVolCoalescerLeadingEdge(t *testing.T) {
	var got []int
	c := &volCoalescer{min: 500 * time.Millisecond, record: func(e events.Event) { got = append(got, e.Value) }}
	now := time.Now()
	if !c.submitAt(events.Event{Kind: events.KindVolume, Value: 40}, now) {
		t.Fatal("first submit should record on the leading edge")
	}
	if len(got) != 1 || got[0] != 40 {
		t.Fatalf("recorded %v, want [40]", got)
	}
}

// TestVolCoalescerBurstKeepsLatest: a drag's intermediate values collapse;
// the flush (trailing edge or exit) records the LATEST value exactly once.
func TestVolCoalescerBurstKeepsLatest(t *testing.T) {
	var got []int
	c := &volCoalescer{min: 500 * time.Millisecond, record: func(e events.Event) { got = append(got, e.Value) }}
	now := time.Now()
	c.submitAt(events.Event{Kind: events.KindVolume, Value: 40}, now) // leading
	for i, v := range []int{45, 50, 55, 60} {
		if c.submitAt(events.Event{Kind: events.KindVolume, Value: v}, now.Add(time.Duration(i+1)*100*time.Millisecond)) {
			t.Fatalf("value %d inside the window should be deferred", v)
		}
	}
	c.flush() // stands in for the trailing timer
	if len(got) != 2 || got[0] != 40 || got[1] != 60 {
		t.Fatalf("recorded %v, want [40 60] (start level, settled level)", got)
	}
}

// TestVolCoalescerQuietGapReopensLeading: after the window has elapsed with
// nothing pending, the next gesture records immediately again.
func TestVolCoalescerQuietGapReopensLeading(t *testing.T) {
	var got []int
	c := &volCoalescer{min: 500 * time.Millisecond, record: func(e events.Event) { got = append(got, e.Value) }}
	now := time.Now()
	c.submitAt(events.Event{Kind: events.KindVolume, Value: 40}, now)
	if !c.submitAt(events.Event{Kind: events.KindVolume, Value: 70}, now.Add(600*time.Millisecond)) {
		t.Fatal("a submit after the window should record on the leading edge")
	}
	if len(got) != 2 || got[1] != 70 {
		t.Fatalf("recorded %v, want [40 70]", got)
	}
}

// TestVolCoalescerFlushEmpty: flushing with nothing pending records nothing
// (the exit path always flushes).
func TestVolCoalescerFlushEmpty(t *testing.T) {
	var got []int
	c := &volCoalescer{min: 500 * time.Millisecond, record: func(e events.Event) { got = append(got, e.Value) }}
	c.flush()
	if len(got) != 0 {
		t.Fatalf("recorded %v, want nothing", got)
	}
}
