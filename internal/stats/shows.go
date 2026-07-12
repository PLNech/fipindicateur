package stats

import "sort"

// Shows is the "émissions" view of the listening data: time spent inside FIP's
// named programmes, aggregated by the stable conceptUuid so the same recurring
// show heard on different nights counts as one. It is derived from the histlog
// track log (whose lines carry the programme they aired within) intersected
// with the reconstructed playback segments, and is nil (omitted) when no track
// was heard inside any show, so an events-only report, a webradio-only history,
// or an older log without show tags all collapse the block out entirely.
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

// showAccum is the internal per-concept accumulator during the walk.
type showAccum struct {
	name     string
	sec      int64
	tracks   int
	evenings map[string]bool
}

// buildShows aggregates per-programme listening time from the exposed tracks.
// Only tracks actually heard (exposure > 0) inside a show (a non-empty
// conceptUuid) contribute. Returns nil when no such track exists, so the
// section is simply absent when there is no programme data, mirroring
// buildProgramme and the other companion blocks.
func buildShows(tracks []exposedTrack) *Shows {
	if len(tracks) == 0 {
		return nil
	}

	byConcept := map[string]*showAccum{}
	var totalSec, inShowSec int64
	for _, t := range tracks {
		secs := int64(t.Exposure.Seconds())
		if secs <= 0 {
			continue
		}
		totalSec += secs
		if t.ShowConcept == "" {
			continue // heard, but in the normal rotation, not a programme
		}
		inShowSec += secs
		a := byConcept[t.ShowConcept]
		if a == nil {
			a = &showAccum{evenings: map[string]bool{}}
			byConcept[t.ShowConcept] = a
		}
		if a.name == "" && t.Show != "" {
			a.name = t.Show
		}
		a.sec += secs
		a.tracks++
		a.evenings[t.Start.Format("2006-01-02")] = true
	}

	if len(byConcept) == 0 {
		return nil // no programme heard: collapse the block
	}

	shows := make([]ShowStat, 0, len(byConcept))
	for concept, a := range byConcept {
		share := 0.0
		if inShowSec > 0 {
			share = float64(a.sec) / float64(inShowSec)
		}
		name := a.name
		if name == "" {
			name = concept // never blank in the report, even if a name was missing
		}
		shows = append(shows, ShowStat{
			Concept:      concept,
			Name:         name,
			ListeningSec: a.sec,
			Evenings:     len(a.evenings),
			Tracks:       a.tracks,
			Share:        share,
		})
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
		Distinct:    len(byConcept),
	}
}
