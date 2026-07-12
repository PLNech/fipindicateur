package metadata

import (
	"context"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

// livemetaStaleAfter: if livemeta hasn't produced an update in this long, the
// ICY fallback is allowed to emit (covers persistent livemeta errors).
const livemetaStaleAfter = 3 * time.Minute

// Manager composes the Livemeta and ICY providers into one NowPlaying stream.
// It prefers livemeta and falls back to ICY for no-id stations or when
// livemeta goes stale. The player should feed titles via PushTitle.
type Manager struct {
	livemeta *LivemetaProvider
	icy      *ICYProvider
}

// NewManager builds a Manager with default providers.
func NewManager() *Manager {
	return &Manager{
		livemeta: NewLivemeta(),
		icy:      NewICY(),
	}
}

// PushTitle forwards an icecast title to the ICY provider.
func (m *Manager) PushTitle(title string) { m.icy.PushTitle(title) }

// Watch merges both providers for the given station into a single channel.
func (m *Manager) Watch(ctx context.Context, station stations.Station) <-chan NowPlaying {
	out := make(chan NowPlaying, 1)
	lm := m.livemeta.Watch(ctx, station)
	ic := m.icy.Watch(ctx, station)

	go func() {
		defer close(out)
		// Seed with now so the ICY fallback stays quiet during the startup
		// grace window; if livemeta is genuinely dead for livemetaStaleAfter,
		// ICY takes over. For no-id stations ICY always emits.
		lastLivemeta := time.Now()
		// lastLM is the most recent livemeta result, kept so the ICY fallback
		// can inherit its programme context during show-only broadcasts.
		var lastLM NowPlaying
		for {
			select {
			case <-ctx.Done():
				return
			case np, ok := <-lm:
				if !ok {
					lm = nil
					continue
				}
				lastLivemeta = time.Now()
				lastLM = np
				emit(ctx, out, np)
			case np, ok := <-ic:
				if !ok {
					ic = nil
					continue
				}
				// A show with no song grain (e.g. "Fip Tape"): livemeta keeps
				// emitting fresh, so it never goes stale, yet it carries no
				// track. Let ICY supply the title and graft livemeta's Show onto
				// it, so the programme stays on air while the real track shows.
				showOnly := lastLM.Show != nil && lastLM.Empty()
				stale := station.MetaID == 0 || time.Since(lastLivemeta) > livemetaStaleAfter
				if showOnly {
					np.Show = lastLM.Show
					np.UpcomingShows = lastLM.UpcomingShows
				}
				if stale || showOnly {
					emit(ctx, out, np)
				}
			}
			if lm == nil && ic == nil {
				return
			}
		}
	}()
	return out
}

func emit(ctx context.Context, out chan<- NowPlaying, np NowPlaying) {
	select {
	case out <- np:
	case <-ctx.Done():
	}
}
