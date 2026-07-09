package ui

import (
	"sync"
	"time"
)

// throttle coalesces rapid string updates into a gentle stream of pushes. It
// serves two purposes for the system-tray / SNI surface, which a buggy
// appindicator extension can choke on under churn:
//
//  1. dedupe: an identical consecutive value is never pushed twice;
//  2. debounce: pushes are rate-limited to at most one per min interval, and
//     the latest value always lands on the trailing edge (no update is lost,
//     only intermediate ones between two ticks are collapsed).
//
// The first change is pushed immediately (leading edge), so the UI stays
// responsive; a burst that follows is collapsed to a single trailing push once
// min has elapsed. The push callback runs off the throttle's lock so it may do
// D-Bus work without risking a deadlock.
type throttle struct {
	min  time.Duration
	push func(string)

	mu       sync.Mutex
	last     string // last value actually pushed
	haveLast bool
	lastAt   time.Time
	pending  string // latest value waiting for the trailing edge
	havePend bool
	timer    *time.Timer
}

// newThrottle builds a throttle that pushes via fn, rate-limited to one push
// per min. A min of 0 disables debouncing (dedupe still applies).
func newThrottle(min time.Duration, fn func(string)) *throttle {
	return &throttle{min: min, push: fn}
}

// update submits a new value for pushing, using the wall clock.
func (t *throttle) update(v string) { t.updateAt(v, time.Now()) }

// updateAt is update with an injectable clock (for tests). It returns true when
// the value was pushed immediately (leading edge), false when it was deduped or
// deferred to the trailing edge.
func (t *throttle) updateAt(v string, now time.Time) bool {
	t.mu.Lock()

	// Dedupe against the most recent known value (pending if a trailing push is
	// queued, otherwise the last pushed one).
	latest := t.last
	if t.havePend {
		latest = t.pending
	}
	if t.haveLast && v == latest {
		t.mu.Unlock()
		return false
	}

	if !t.haveLast || t.min <= 0 || now.Sub(t.lastAt) >= t.min {
		t.markPushed(v, now)
		fn := t.push
		t.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return true
	}

	// Too soon: keep the value as pending and arm a trailing flush.
	t.pending = v
	t.havePend = true
	if t.timer == nil {
		wait := t.min - now.Sub(t.lastAt)
		if wait < 0 {
			wait = 0
		}
		t.timer = time.AfterFunc(wait, t.flush)
	}
	t.mu.Unlock()
	return false
}

// markPushed records a push (caller holds the lock).
func (t *throttle) markPushed(v string, now time.Time) {
	t.last = v
	t.haveLast = true
	t.lastAt = now
	t.havePend = false
}

// flush delivers the pending value on the trailing edge.
func (t *throttle) flush() {
	t.mu.Lock()
	t.timer = nil
	if !t.havePend {
		t.mu.Unlock()
		return
	}
	v := t.pending
	t.markPushed(v, time.Now())
	fn := t.push
	t.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}
