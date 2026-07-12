package stats

import (
	"sort"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
)

// Shows is the "émissions" view of the listening data: time spent inside FIP's
// named programmes, aggregated by the stable conceptUuid so the same recurring
// show heard on different nights counts as one. It is derived from two grains
// (see deriveShowListening): the histlog track log (lines carry the programme
// they aired within) and the show_change boundaries of the behaviour log, each
// intersected with the reconstructed playback segments. It is nil (omitted)
// when no listening fell inside any show, so an events-only log without
// programme boundaries, a webradio-only history, or an older log without show
// tags all collapse the block out entirely.
type Shows struct {
	Shows []ShowStat `json:"shows"` // per-programme, sorted desc by listening seconds
	// InShowSec is total listening time spent inside any programme; TotalSec is
	// all observed listening (the denominator). InShowShare is the fraction of
	// listening spent in programmes rather than the normal rotation.
	InShowSec   int64   `json:"inShowSec"`
	TotalSec    int64   `json:"totalSec"`
	InShowShare float64 `json:"inShowShare"` // 0..1
	Distinct    int     `json:"distinct"`    // number of distinct programmes heard
}

// ShowStat is one recurring programme's listening footprint.
type ShowStat struct {
	Concept      string  `json:"concept"` // conceptUuid (stable identity across nights)
	Name         string  `json:"name"`    // date-free display name
	ListeningSec int64   `json:"listeningSec"`
	Evenings     int     `json:"evenings"` // distinct calendar dates the show was heard
	Tracks       int     `json:"tracks"`   // tracks actually heard within the show
	Share        float64 `json:"share"`    // of in-show listening time, 0..1
}

// showSpan is one on-air programme interval reconstructed from the behaviour
// log's show_change boundaries, before intersection with playback.
type showSpan struct {
	concept string
	station string
	start   time.Time
	end     time.Time
}

// showSpansFromEvents walks the (sorted) behaviour log and rebuilds the
// programme intervals: a span opens at a show_change whose To is a concept and
// closes at the next show_change (or at an app lifecycle boundary: the app's
// show state does not survive a restart, so a span left open at shutdown must
// never absorb the next day's listening). A log that ends mid-show closes at
// its last event, mirroring the segment walk: we never invent time past the
// log. show_change is only recorded while actually playing, so a span opened
// during a paused stretch can start up to one livemeta poll late; the error is
// bounded by the poll interval and only ever undercounts.
func showSpansFromEvents(evs []events.Event) []showSpan {
	var out []showSpan
	var cur *showSpan
	closeAt := func(at time.Time) {
		if cur == nil {
			return
		}
		if at.After(cur.start) {
			cur.end = at
			out = append(out, *cur)
		}
		cur = nil
	}
	var last time.Time
	for _, e := range evs {
		last = e.TS
		switch e.Kind {
		case events.KindShowChange:
			closeAt(e.TS)
			if e.To != "" {
				cur = &showSpan{concept: e.To, station: e.Station, start: e.TS}
			}
		case events.KindAppStart, events.KindAppStop, events.KindQuit:
			closeAt(e.TS)
		}
	}
	closeAt(last)
	return out
}

// showListening is the merged per-programme listening derivation shared by
// buildShows and showAchievementFacts. Two grains measure time inside a
// programme: the histlog lines tagged with a show (per-track) and the
// show_change boundary spans intersected with playback (per-programme). For a
// given (concept, day) the span grain, when present, supersedes the track
// grain: it covers the same listening plus the gaps between tracks, and it is
// the only grain during émissions that broadcast no per-song metadata at all
// (continuous mixes like "Fip Tape": no song steps in livemeta, no inline ICY
// titles on the Radio France streams). Track lines still supply the display
// name and the tracks-heard count; a bare show tag with no artist and no title
// is the show-start marker the app writes for such émissions, identity only.
type showListening struct {
	secs          map[string]map[string]int64 // concept -> day -> listening seconds
	names         map[string]string           // concept -> display name (histlog)
	trackCounts   map[string]int              // concept -> tracks actually heard
	sundayEvening bool                        // in-show listening on a Sunday 18h-24h
	nightSec      int64                       // in-show listening in hours 0h-5h
}

// deriveShowListening intersects the show spans with the playback segments and
// merges the result with the tagged-track exposure, per (concept, day).
func deriveShowListening(tracks []exposedTrack, spans []showSpan, segments []segment) showListening {
	sl := showListening{
		secs:        map[string]map[string]int64{},
		names:       map[string]string{},
		trackCounts: map[string]int{},
	}
	addCell := func(concept, day string, secs int64) {
		if sl.secs[concept] == nil {
			sl.secs[concept] = map[string]int64{}
		}
		sl.secs[concept][day] += secs
	}
	// markHours splits [s,e) on hour boundaries so the day attribution and the
	// hour-of-day facts stay exact across midnight (same walk as addHourly).
	markHours := func(concept string, s, e time.Time, mark map[string]map[string]bool) {
		for s.Before(e) {
			next := time.Date(s.Year(), s.Month(), s.Day(), s.Hour(), 0, 0, 0, s.Location()).Add(time.Hour)
			chunk := e
			if next.Before(chunk) {
				chunk = next
			}
			secs := int64(chunk.Sub(s).Seconds())
			if secs > 0 {
				day := s.Format("2006-01-02")
				addCell(concept, day, secs)
				if mark[concept] == nil {
					mark[concept] = map[string]bool{}
				}
				mark[concept][day] = true
				if s.Hour() < 5 {
					sl.nightSec += secs
				}
				if s.Weekday() == time.Sunday && s.Hour() >= 18 {
					sl.sundayEvening = true
				}
			}
			s = chunk
		}
	}

	segByStation := map[string][]segment{}
	for _, s := range segments {
		segByStation[s.station] = append(segByStation[s.station], s)
	}
	// spanDays marks the (concept, day) cells the span grain covers, so the
	// track grain defers to it there (no double counting).
	spanDays := map[string]map[string]bool{}
	for _, sp := range spans {
		segs := segByStation[sp.station]
		if sp.station == "" {
			segs = segments // an unstationed boundary intersects everything
		}
		for _, sg := range segs {
			s, e := sp.start, sp.end
			if sg.start.After(s) {
				s = sg.start
			}
			if sg.end.Before(e) {
				e = sg.end
			}
			markHours(sp.concept, s, e, spanDays)
		}
	}

	for _, t := range tracks {
		secs := int64(t.Exposure.Seconds())
		if secs <= 0 || t.ShowConcept == "" {
			continue
		}
		if sl.names[t.ShowConcept] == "" && t.Show != "" {
			sl.names[t.ShowConcept] = t.Show
		}
		// A line with a title or an artist is a track heard within the
		// programme; a bare tag is the show-start marker (name only, above).
		if t.Title != "" || t.Artist != "" {
			sl.trackCounts[t.ShowConcept]++
		}
		day := t.Start.Format("2006-01-02")
		if spanDays[t.ShowConcept][day] {
			continue // the span grain already measures this (concept, day)
		}
		addCell(t.ShowConcept, day, secs)
		if t.Start.Hour() < 5 {
			sl.nightSec += secs
		}
		if t.Start.Weekday() == time.Sunday && t.Start.Hour() >= 18 {
			sl.sundayEvening = true
		}
	}
	return sl
}

// buildShows aggregates per-programme listening time from the merged
// derivation. totalSec is all observed listening (the segment total), the
// honest denominator whether or not every heard minute carried a track line.
// Returns nil when no listening fell inside any programme, so the section is
// simply absent when there is no programme data, mirroring buildProgramme and
// the other companion blocks.
func buildShows(sl showListening, totalSec int64) *Shows {
	var inShowSec int64
	shows := make([]ShowStat, 0, len(sl.secs))
	for concept, days := range sl.secs {
		var sec int64
		for _, s := range days {
			sec += s
		}
		if sec <= 0 {
			continue
		}
		inShowSec += sec
		name := sl.names[concept]
		if name == "" {
			name = concept // never blank in the report, even if a name was missing
		}
		shows = append(shows, ShowStat{
			Concept:      concept,
			Name:         name,
			ListeningSec: sec,
			Evenings:     len(days),
			Tracks:       sl.trackCounts[concept],
		})
	}
	if len(shows) == 0 {
		return nil // no programme heard: collapse the block
	}
	for i := range shows {
		if inShowSec > 0 {
			shows[i].Share = float64(shows[i].ListeningSec) / float64(inShowSec)
		}
	}
	// Deterministic order: listening time desc, then name, then concept, so the
	// render and the tests are stable.
	sort.SliceStable(shows, func(i, j int) bool {
		if shows[i].ListeningSec != shows[j].ListeningSec {
			return shows[i].ListeningSec > shows[j].ListeningSec
		}
		if shows[i].Name != shows[j].Name {
			return shows[i].Name < shows[j].Name
		}
		return shows[i].Concept < shows[j].Concept
	})

	inShowShare := 0.0
	if totalSec > 0 {
		inShowShare = float64(inShowSec) / float64(totalSec)
	}
	return &Shows{
		Shows:       shows,
		InShowSec:   inShowSec,
		TotalSec:    totalSec,
		InShowShare: inShowShare,
		Distinct:    len(shows),
	}
}
