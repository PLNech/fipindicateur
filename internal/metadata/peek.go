package metadata

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

// peekGrace is how long after a track's declared end a peeked entry still
// counts: the next poll is due pollPad after the end, plus network slack.
const peekGrace = 20 * time.Second

// Peeker keeps the latest livemeta answer for every station, not only the
// tuned one, so a zap can say what is on air at the destination the moment
// you land there, instead of "En écoute" first and the track a poll later. It
// asks the same public endpoint the tuned station's watcher asks, at the same
// rhythm (each track's own end), and sends nothing about the listener.
type Peeker struct {
	lm *LivemetaProvider

	mu   sync.Mutex
	seen map[string]peeked
}

type peeked struct {
	np      NowPlaying
	fetched time.Time
}

func newPeeker(lm *LivemetaProvider) *Peeker {
	return &Peeker{lm: lm, seen: map[string]peeked{}}
}

// peekFresh reports whether a peeked answer can still be trusted at now: it
// must carry a track, and either its declared end has not long passed, or (no
// end declared) it was fetched within the longest poll interval.
func peekFresh(p peeked, now time.Time) bool {
	if p.np.Empty() {
		return false
	}
	if p.np.End.IsZero() {
		return now.Sub(p.fetched) <= pollMax
	}
	return now.Before(p.np.End.Add(peekGrace))
}

// Latest returns what is on air at the station per the last answer, when it
// is still fresh.
func (p *Peeker) Latest(key string) (NowPlaying, bool) {
	p.mu.Lock()
	v, ok := p.seen[key]
	p.mu.Unlock()
	if !ok || !peekFresh(v, time.Now()) {
		return NowPlaying{}, false
	}
	return v.np, true
}

func (p *Peeker) store(key string, np NowPlaying) {
	p.mu.Lock()
	p.seen[key] = peeked{np: np, fetched: time.Now()}
	p.mu.Unlock()
}

// Start polls every station that has a livemeta id until ctx ends. Starts are
// staggered so thirteen requests never leave in one burst; errors back off
// like Watch does and never crash anything.
func (p *Peeker) Start(ctx context.Context, all []stations.Station) {
	i := 0
	for _, s := range all {
		if s.MetaID == 0 {
			continue
		}
		go p.run(ctx, s, time.Duration(i)*700*time.Millisecond)
		i++
	}
}

func (p *Peeker) run(ctx context.Context, s stations.Station, delay time.Duration) {
	backoff := backoffStart
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		np, err := p.lm.fetch(ctx, s.MetaID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("metadata: peek %s error: %v (backoff %s)", s.Key, err, backoff)
			delay = backoff
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
			continue
		}
		backoff = backoffStart
		p.store(s.Key, np)
		delay = nextDelay(np.End, time.Now())
	}
}
