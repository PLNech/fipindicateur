package metadata

import (
	"testing"
	"time"
)

func TestPeekFresh(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	tr := NowPlaying{Artist: "A", Title: "T"}
	cases := []struct {
		name string
		p    peeked
		want bool
	}{
		{"empty is never fresh", peeked{NowPlaying{End: now.Add(time.Hour)}, now}, false},
		{"before its end", peeked{withEnd(tr, now.Add(time.Minute)), now.Add(-time.Minute)}, true},
		{"just past the end, inside the grace", peeked{withEnd(tr, now.Add(-5*time.Second)), now.Add(-time.Minute)}, true},
		{"long past the end", peeked{withEnd(tr, now.Add(-time.Minute)), now.Add(-2 * time.Minute)}, false},
		{"no end, recent", peeked{tr, now.Add(-time.Minute)}, true},
		{"no end, old", peeked{tr, now.Add(-10 * time.Minute)}, false},
	}
	for _, c := range cases {
		if got := peekFresh(c.p, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func withEnd(np NowPlaying, e time.Time) NowPlaying { np.End = e; return np }

func TestPeekerLatest(t *testing.T) {
	p := newPeeker(nil)
	if _, ok := p.Latest("jazz"); ok {
		t.Fatal("nothing peeked yet")
	}
	p.store("jazz", NowPlaying{Artist: "A", Title: "T", End: time.Now().Add(time.Minute)})
	if np, ok := p.Latest("jazz"); !ok || np.Title != "T" {
		t.Fatalf("got %+v %v", np, ok)
	}
}
