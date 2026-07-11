package stats

import (
	"sort"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/prefs"
)

// Tastes summarises how the listener reacted to what they heard. It combines
// the explicit verdict log (prefs: deliberate like / dislike) with soft,
// aggregate-only implicit hints mined from the behaviour x history join.
type Tastes struct {
	Likes    int         `json:"likes"`
	Dislikes int         `json:"dislikes"`
	Items    []TasteItem `json:"items"` // up to 20 most recent explicit verdicts
	Implicit Implicit    `json:"implicit"`
}

// TasteItem is one explicit verdict. These are the only per-track lines the
// report exposes: implicit hints are reported as counts, never attributed to a
// specific track (see Implicit).
type TasteItem struct {
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	Verdict string    `json:"verdict"` // "like" | "dislike"
	TS      time.Time `json:"ts"`
}

// Implicit holds soft behavioural HINTS, not verdicts. They are deliberately
// aggregate: a zap-out or an early pause suggests a track may not have landed,
// but a pause is ambiguous (the room may just need silence, a phone call, a
// coffee) and a zap may be curiosity, not rejection. We therefore report only
// counts, never which track, and the report captions them as hints.
//
//   - ZapOuts:     a station change away while a track was under way on the
//     station being left.
//   - EarlyPauses: a pause within the first 60 seconds of a track.
//   - N:           histlog tracks considered (the denominator for the hints).
type Implicit struct {
	ZapOuts     int `json:"zapOuts"`
	EarlyPauses int `json:"earlyPauses"`
	N           int `json:"n"`
}

// earlyPauseWindow is how soon after a track starts a pause reads as "early".
const earlyPauseWindow = 60 * time.Second

// buildTastes assembles explicit counts + recent items from prf and the
// implicit hints from the events x tracks join. Returns nil only when there is
// nothing to report (no verdicts and no tracks), so an events-only report omits
// the block entirely.
func buildTastes(prf []prefs.Entry, evs []events.Event, tracks []exposedTrack) *Tastes {
	if len(prf) == 0 && len(tracks) == 0 {
		return nil
	}
	t := &Tastes{Items: []TasteItem{}}
	for _, p := range prf {
		switch p.Verdict {
		case prefs.Like:
			t.Likes++
		case prefs.Dislike:
			t.Dislikes++
		}
	}
	// Up to the 20 most recent explicit verdicts, newest first.
	recent := make([]prefs.Entry, len(prf))
	copy(recent, prf)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].TS.After(recent[j].TS) })
	for i, p := range recent {
		if i >= 20 {
			break
		}
		t.Items = append(t.Items, TasteItem{
			Artist:  p.Artist,
			Title:   p.Title,
			Verdict: p.Verdict,
			TS:      p.TS,
		})
	}
	t.Implicit = buildImplicit(evs, tracks)
	return t
}

// buildImplicit mines the soft hints. evs is expected already sorted (Build
// sorts it). Each event contributes at most one hint (first matching track),
// so counts never exceed the number of qualifying events.
func buildImplicit(evs []events.Event, tracks []exposedTrack) Implicit {
	im := Implicit{N: len(tracks)}
	if len(tracks) == 0 {
		return im
	}
	for _, e := range evs {
		switch e.Kind {
		case events.KindStationChange:
			// zapOut: leaving a station while one of its tracks is under way.
			if e.From == "" {
				continue
			}
			for _, tr := range tracks {
				if tr.Station != e.From {
					continue
				}
				if !e.TS.Before(tr.Start) && e.TS.Before(tr.End) {
					im.ZapOuts++
					break
				}
			}
		case events.KindPause:
			// earlyPause: a pause landing inside the first minute of a track.
			for _, tr := range tracks {
				if !e.TS.Before(tr.Start) && e.TS.Before(tr.End) && e.TS.Sub(tr.Start) < earlyPauseWindow {
					im.EarlyPauses++
					break
				}
			}
		}
	}
	return im
}
