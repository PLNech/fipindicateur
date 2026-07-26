package ui

import (
	"sync"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
)

// volumeRecordMin is the coalescing window for KindVolume records: within one
// slider drag the first applied level is recorded immediately (leading edge)
// and the rest collapse into one trailing record carrying the latest value.
// The volume itself is still APPLIED on every command; only the event log is
// coalesced, so a drag reads as a gesture (start level, settled level) rather
// than dozens of intermediate steps.
const volumeRecordMin = 500 * time.Millisecond

// volCoalescer rate-limits event records to one per min interval, leading
// edge immediate, latest value guaranteed on the trailing edge. It is the
// events-log sibling of throttle (which coalesces SNI label pushes): same
// leading/trailing shape, but carrying a full events.Event.
type volCoalescer struct {
	min    time.Duration
	record func(events.Event)

	mu      sync.Mutex
	lastAt  time.Time
	pending *events.Event
	timer   *time.Timer
}

// submit records e now (leading edge) or queues it as the trailing record,
// replacing any queued predecessor. Uses the wall clock.
func (c *volCoalescer) submit(e events.Event) { c.submitAt(e, time.Now()) }

// submitAt is submit with an injectable clock (for tests). Returns true when
// the event was recorded immediately.
func (c *volCoalescer) submitAt(e events.Event, now time.Time) bool {
	c.mu.Lock()
	if c.timer == nil && now.Sub(c.lastAt) >= c.min {
		c.lastAt = now
		rec := c.record
		c.mu.Unlock()
		if rec != nil {
			rec(e)
		}
		return true
	}
	c.pending = &e
	if c.timer == nil {
		wait := c.min - now.Sub(c.lastAt)
		if wait < 0 {
			wait = 0
		}
		c.timer = time.AfterFunc(wait, c.flush)
	}
	c.mu.Unlock()
	return false
}

// flush delivers the pending event, if any, right away. Called by the
// trailing timer, and by OnExit so the settled value of a drag interrupted by
// quitting still lands in the log before the recorder closes.
func (c *volCoalescer) flush() {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	e := c.pending
	c.pending = nil
	if e != nil {
		c.lastAt = time.Now()
	}
	rec := c.record
	c.mu.Unlock()
	if e != nil && rec != nil {
		rec(*e)
	}
}
