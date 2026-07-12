// Package stats derives listening analytics from the events.jsonl behaviour
// log (internal/events) and renders a self-contained, offline HTML report.
//
// All aggregation is pure and unit-tested over synthetic event slices. The
// report states its own sample size (see Calibration): with a handful of
// sessions the numbers are indicative, not significant, and the UI says so.
package stats

import (
	"sort"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/stations"
)

// Report is the derived model injected into the HTML template. Durations are
// exposed as whole seconds (JSON numbers) to keep the SPA arithmetic trivial.
type Report struct {
	GeneratedAt  time.Time     `json:"generatedAt"`
	Range        Range         `json:"range"`
	Totals       Totals        `json:"totals"`
	Stations     []StationStat `json:"stations"`
	Hourly       [24]int64     `json:"hourly"`  // listening seconds per hour-of-day (local)
	Weekday      [7]int64      `json:"weekday"` // listening seconds per weekday (Sun=0)
	Sessions     SessionStats  `json:"sessions"`
	Transitions  []Transition  `json:"transitions"` // sorted desc by count
	Achievements []Achievement `json:"achievements"`
	Calibration  Calibration   `json:"calibration"`

	// The following blocks are derived from the optional companion inputs
	// (histlog track log, prefs verdict log, enriched.json artist metadata).
	// Each is an omitempty pointer: when its input is absent the block
	// collapses out of the JSON entirely, so a report built from events alone
	// is byte-identical to before these fields existed.
	Epochs    *Epochs        `json:"epochs,omitempty"`
	Enriched  *EnrichedStats `json:"enriched,omitempty"`
	Tastes    *Tastes        `json:"tastes,omitempty"`
	Programme *Programme     `json:"programme,omitempty"`
	Shows     *Shows         `json:"shows,omitempty"`
}

// Range is the observed time span of the log.
type Range struct {
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// Totals are the headline counters.
type Totals struct {
	ListeningSec int64 `json:"listeningSec"` // total time actually playing
	Sessions     int   `json:"sessions"`
	DaysActive   int   `json:"daysActive"`
	Plays        int   `json:"plays"`
	Pauses       int   `json:"pauses"`
	Zaps         int   `json:"zaps"` // station changes while playing
}

// StationStat is per-station listening time and its share of the total.
type StationStat struct {
	Key          string  `json:"key"`
	Display      string  `json:"display"`
	Color        string  `json:"color"` // official webradio brand color, hex
	ListeningSec int64   `json:"listeningSec"`
	Share        float64 `json:"share"` // 0..1 of total listening time
}

// SessionStats summarise session lengths.
type SessionStats struct {
	Count     int   `json:"count"`
	MedianSec int64 `json:"medianSec"`
	MaxSec    int64 `json:"maxSec"`
	MeanSec   int64 `json:"meanSec"`
}

// Transition is one observed station->station hop with its within-row Markov
// probability (P(to | left from)).
type Transition struct {
	From        string  `json:"from"`
	To          string  `json:"to"`
	FromDisplay string  `json:"fromDisplay"`
	ToDisplay   string  `json:"toDisplay"`
	Count       int     `json:"count"`
	Prob        float64 `json:"prob"` // count / row total, 0..1
}

// Calibration reports the sample size so the report never over-claims.
type Calibration struct {
	Events     int `json:"events"`
	Sessions   int `json:"sessions"`
	DaysActive int `json:"daysActive"`
	Zaps       int `json:"zaps"`
}

// session is the internal accumulator during the walk.
type session struct {
	start    time.Time
	end      time.Time
	dur      time.Duration
	zaps     int
	stations map[string]bool
}

// segment is one contiguous stretch of actual playback on a single station,
// reconstructed from play/pause/station_change boundaries. These are the
// honest "was audible" windows: track exposure is the intersection of a
// track's histlog interval with the segments on its station (see enrich.go).
type segment struct {
	station string
	start   time.Time
	end     time.Time
}

// Build derives the full report. evs is the behaviour log (required); hist,
// prf and enr are the optional companion inputs (histlog track log, prefs
// verdict log, enriched.json artist metadata) and may each be nil/empty, in
// which case their derived blocks are omitted and the report is identical to
// the events-only version. now stamps GeneratedAt and anchors time-relative
// achievements (e.g. consecutive-day streaks).
//
// Build is pure: same inputs, same output, no IO. Generate (report.go) does
// the best-effort loading of every input before calling it.
func Build(evs []events.Event, hist []histlog.Entry, prf []prefs.Entry, enr *Enriched, now time.Time) Report {
	sorted := make([]events.Event, len(evs))
	copy(sorted, evs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })

	var (
		playing  bool
		station  string
		segStart time.Time

		perStation = map[string]time.Duration{}
		hourly     [24]int64
		weekday    [7]int64
		transCount = map[string]map[string]int{}
		activeDays = map[string]bool{}

		sessions []session
		cur      *session
		segments []segment

		plays, pauses, zaps int
		total               time.Duration
	)

	// closeSeg attributes [segStart, at] to the current station and session,
	// then advances segStart. No-op if not currently in a segment.
	closeSeg := func(at time.Time) {
		if !playing || segStart.IsZero() || station == "" {
			return
		}
		d := at.Sub(segStart)
		if d > 0 {
			perStation[station] += d
			total += d
			addHourly(&hourly, &weekday, segStart, at)
			markDays(activeDays, segStart, at)
			segments = append(segments, segment{station: station, start: segStart, end: at})
			if cur != nil {
				cur.dur += d
				cur.stations[station] = true
			}
		}
		segStart = at
	}

	startSession := func(at time.Time) {
		cur = &session{start: at, stations: map[string]bool{}}
	}
	endSession := func(at time.Time) {
		if cur == nil {
			return
		}
		cur.end = at
		sessions = append(sessions, *cur)
		cur = nil
	}

	for _, e := range sorted {
		switch e.Kind {
		case events.KindPlay:
			// Count and open a session only on a genuine paused->playing edge.
			// setPlayingUI (the UI chokepoint) may re-emit play on a station
			// switch while already playing; those are idempotent here.
			if !playing {
				plays++
				playing = true
				if e.Station != "" {
					station = e.Station
				}
				segStart = e.TS
				startSession(e.TS)
			}
		case events.KindPause, events.KindAppStop, events.KindQuit:
			if playing {
				if e.Kind == events.KindPause {
					pauses++
				}
				closeSeg(e.TS)
				endSession(e.TS)
				playing = false
			}
		case events.KindStationChange:
			// Markov edge, recorded whether or not we are playing.
			if e.From != "" && e.To != "" && e.From != e.To {
				if transCount[e.From] == nil {
					transCount[e.From] = map[string]int{}
				}
				transCount[e.From][e.To]++
			}
			if playing {
				closeSeg(e.TS) // attribute time up to the hop to the old station
				zaps++
				if cur != nil {
					cur.zaps++
				}
			}
			if e.To != "" {
				station = e.To
			}
		}
	}
	// A log that ends while still playing: close the open segment at the last
	// known event time (we never invent time past the log).
	if playing && len(sorted) > 0 {
		last := sorted[len(sorted)-1].TS
		closeSeg(last)
		endSession(last)
	}

	r := Report{GeneratedAt: now, Hourly: hourly, Weekday: weekday}
	if len(sorted) > 0 {
		r.Range = Range{First: sorted[0].TS, Last: sorted[len(sorted)-1].TS}
	}
	r.Totals = Totals{
		ListeningSec: int64(total.Seconds()),
		Sessions:     len(sessions),
		DaysActive:   len(activeDays),
		Plays:        plays,
		Pauses:       pauses,
		Zaps:         zaps,
	}
	r.Stations = buildStationStats(perStation, total)
	r.Sessions = buildSessionStats(sessions)
	r.Transitions = buildTransitions(transCount)
	r.Calibration = Calibration{Events: len(sorted), Sessions: len(sessions), DaysActive: len(activeDays), Zaps: zaps}

	// Companion-input blocks. Each is nil (omitted) when its input is absent,
	// so an events-only report is unchanged. Exposure per track is computed
	// once (histlog intervals capped and intersected with playback segments)
	// and shared by the epochs, enriched and émission derivations.
	tracks := trackExposure(hist, segments)
	// Achievements are graded after track exposure so the émission badges can
	// read the show facts (histlog show tags joined with playback) alongside the
	// behaviour log.
	r.Achievements = evaluateAchievements(sessions, perStation, activeDays, hourly, zaps, total, showAchievementFacts(tracks, sorted))
	r.Epochs = buildEpochs(tracks)
	r.Enriched = buildEnriched(tracks, enr)
	r.Tastes = buildTastes(prf, sorted, tracks)
	r.Programme = buildProgramme(hist, hourly, int64(total.Seconds()))
	r.Shows = buildShows(tracks)
	return r
}

// addHourly splits [start,end] across hour-of-day and weekday buckets (local
// wall clock), so a segment crossing midnight or an hour boundary is attributed
// proportionally.
func addHourly(hourly *[24]int64, weekday *[7]int64, start, end time.Time) {
	for start.Before(end) {
		next := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), 0, 0, 0, start.Location()).Add(time.Hour)
		seg := end
		if next.Before(seg) {
			seg = next
		}
		secs := int64(seg.Sub(start).Seconds())
		hourly[start.Hour()] += secs
		weekday[int(start.Weekday())] += secs
		start = seg
	}
}

// markDays records every local calendar day touched by [start,end].
func markDays(days map[string]bool, start, end time.Time) {
	d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for !d.After(end) {
		days[d.Format("2006-01-02")] = true
		d = d.AddDate(0, 0, 1)
	}
}

func buildStationStats(per map[string]time.Duration, total time.Duration) []StationStat {
	out := make([]StationStat, 0, len(per))
	for key, d := range per {
		share := 0.0
		if total > 0 {
			share = d.Seconds() / total.Seconds()
		}
		st := stations.ByKey(key)
		out = append(out, StationStat{
			Key:          key,
			Display:      st.Display,
			Color:        st.Color,
			ListeningSec: int64(d.Seconds()),
			Share:        share,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ListeningSec > out[j].ListeningSec })
	return out
}

func buildSessionStats(ss []session) SessionStats {
	if len(ss) == 0 {
		return SessionStats{}
	}
	secs := make([]int64, len(ss))
	var sum, max int64
	for i, s := range ss {
		secs[i] = int64(s.dur.Seconds())
		sum += secs[i]
		if secs[i] > max {
			max = secs[i]
		}
	}
	sort.Slice(secs, func(i, j int) bool { return secs[i] < secs[j] })
	var median int64
	n := len(secs)
	if n%2 == 1 {
		median = secs[n/2]
	} else {
		median = (secs[n/2-1] + secs[n/2]) / 2
	}
	return SessionStats{Count: n, MedianSec: median, MaxSec: max, MeanSec: sum / int64(n)}
}

func buildTransitions(counts map[string]map[string]int) []Transition {
	var out []Transition
	for from, row := range counts {
		var rowTotal int
		for _, c := range row {
			rowTotal += c
		}
		for to, c := range row {
			prob := 0.0
			if rowTotal > 0 {
				prob = float64(c) / float64(rowTotal)
			}
			out = append(out, Transition{
				From:        from,
				To:          to,
				FromDisplay: stations.ByKey(from).Display,
				ToDisplay:   stations.ByKey(to).Display,
				Count:       c,
				Prob:        prob,
			})
		}
	}
	// Deterministic order: count desc, then from/to for stable rendering/tests.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}
