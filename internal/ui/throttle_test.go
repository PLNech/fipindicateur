package ui

import (
	"sync"
	"testing"
	"time"
)

// collector is a push sink that records everything it receives, safely from any
// goroutine (time.AfterFunc fires on its own goroutine).
type collector struct {
	mu   sync.Mutex
	vals []string
}

func (c *collector) push(v string) {
	c.mu.Lock()
	c.vals = append(c.vals, v)
	c.mu.Unlock()
}

func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.vals))
	copy(out, c.vals)
	return out
}

func TestThrottleDedupe(t *testing.T) {
	c := &collector{}
	th := newThrottle(time.Second, c.push)
	base := time.Unix(0, 0)

	if !th.updateAt("a", base) {
		t.Fatal("first distinct value should push immediately")
	}
	// Same value again well after the interval: must be deduped, not pushed.
	if th.updateAt("a", base.Add(2*time.Second)) {
		t.Fatal("identical value should be deduped, not pushed")
	}
	if got := c.snapshot(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected single push [a], got %v", got)
	}
}

func TestThrottleLeadingEdgeAfterInterval(t *testing.T) {
	c := &collector{}
	th := newThrottle(time.Second, c.push)
	base := time.Unix(100, 0)

	th.updateAt("a", base)
	// A different value once the interval has elapsed pushes immediately.
	if !th.updateAt("b", base.Add(time.Second)) {
		t.Fatal("distinct value after the interval should push immediately")
	}
	if got := c.snapshot(); len(got) != 2 || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestThrottleCoalescesBurst(t *testing.T) {
	c := &collector{}
	// Short interval so the trailing timer fires quickly under test.
	th := newThrottle(40*time.Millisecond, c.push)
	now := time.Now()

	th.updateAt("a", now)                          // leading push
	th.updateAt("b", now.Add(5*time.Millisecond))  // too soon -> pending
	th.updateAt("c", now.Add(10*time.Millisecond)) // too soon -> supersedes pending

	// Only "a" so far; the burst is still pending.
	if got := c.snapshot(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("during the burst only the leading push should be visible, got %v", got)
	}

	// Wait for the trailing edge to deliver the latest value ("c"), not "b".
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := c.snapshot(); len(got) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := c.snapshot()
	if len(got) != 2 || got[1] != "c" {
		t.Fatalf("trailing edge should deliver the latest value [a c], got %v", got)
	}
}

func TestThrottleZeroMinDisablesDebounceKeepsDedupe(t *testing.T) {
	c := &collector{}
	th := newThrottle(0, c.push)
	base := time.Unix(0, 0)

	th.updateAt("a", base)
	th.updateAt("a", base) // dedupe
	th.updateAt("b", base) // distinct -> push (no debounce)
	if got := c.snapshot(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("zero-min throttle should push every distinct value, got %v", got)
	}
}
