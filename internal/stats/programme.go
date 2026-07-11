package stats

import (
	"sort"
	"strings"
	"time"

	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/stations"
)

// repeatWindow is FIP's claimed no-repeat horizon: a track is (they say) never
// played twice within 48 hours on the same antenna. We test the claim against
// what the listener actually heard.
const repeatWindow = 48 * time.Hour

// caughtCap bounds how many strict repeats the report lists (the rest collapse
// into the count).
const caughtCap = 5

// blockHours is the length of one hand-crafted music block: FIP programmers
// build three-hour music sequences by hand (Luc Frelon: "On fabrique
// artisanalement des enchaînements de trois heures de musique").
const blockHours = 3

// Programme is the FIP-programming view of the listening data: the 48h
// no-repeat rule tested against what was actually heard, plus an estimate of
// the hand-crafted three-hour music blocks traversed. It is derived from the
// histlog track log alone and is nil (omitted) when there is no history, so an
// events-only report is unchanged.
type Programme struct {
	Rule48h     Rule48h     `json:"rule48h"`
	Conducteurs Conducteurs `json:"conducteurs"`
	// SameStation reports whether repeats were tested within each antenna
	// (true: the honest test, since each webradio programmes independently) or
	// across all stations at once (false: only when the histlog carried no
	// station field, which the report then discloses as "toutes stations
	// confondues").
	SameStation bool `json:"sameStation"`
}

// Rule48h is the verdict on the no-repeat claim. Durations are whole seconds
// (matching the Totals convention); the report converts to hours for copy.
type Rule48h struct {
	StrictRepeats int            `json:"strictRepeats"` // repeats whose later airing fell in daytime (a genuine catch)
	NightRepeats  int            `json:"nightRepeats"`  // repeats whose later airing fell in the night loop (expected)
	TracksChecked int            `json:"tracksChecked"` // distinct airings examined (consecutive re-logs collapsed)
	ObservedSec   int64          `json:"observedSec"`   // total listening time observed (coverage honesty)
	Caught        []CaughtRepeat `json:"caught"`        // the strict repeats, most flagrant first, capped at caughtCap
}

// CaughtRepeat is one daytime same-track repeat within the 48h window.
type CaughtRepeat struct {
	Artist      string `json:"artist"`
	Title       string `json:"title"`
	Station     string `json:"station"`     // station key
	StationName string `json:"stationName"` // display name
	GapSec      int64  `json:"gapSec"`      // seconds between the two airings
}

// Conducteurs estimates the hand-crafted programming traversed. Only daytime
// listening (07h-22h) counts as curated airtime; the night hours are the
// automated rerun of the day ("la boucle de nuit"), not live curation.
type Conducteurs struct {
	CuratedSec     int64 `json:"curatedSec"`     // listening seconds in 07h-22h
	NightSec       int64 `json:"nightSec"`       // listening seconds in 22h-07h
	BlocksEstimate int   `json:"blocksEstimate"` // curated hours / 3, rounded
}

// isNightHour reports whether a local hour-of-day falls in the automated night
// loop (22:00-07:00). Hour 6 (06:00-07:00) is still night; hour 7 is daytime.
func isNightHour(h int) bool { return h >= 22 || h < 7 }

// normField lowercases, trims, and collapses internal whitespace so that
// "  The   Cure " and "the cure" match.
func normField(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// buildProgramme derives the programming view from the histlog. hourly is the
// local hour-of-day listening split (already computed by the walk) and totalSec
// is the total observed listening time. Returns nil when there is no history,
// so the block collapses out of an events-only report exactly like epochs.
func buildProgramme(hist []histlog.Entry, hourly [24]int64, totalSec int64) *Programme {
	if len(hist) == 0 {
		return nil
	}

	sorted := make([]histlog.Entry, len(hist))
	copy(sorted, hist)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })

	// Detect repeats per station. Each webradio programmes on its own, so a
	// cross-station coincidence is not a test of the rule. When the histlog
	// carries no station field at all, everything groups under "" and we test
	// all stations confounded (SameStation=false, disclosed in the report).
	sameStation := false
	byStation := map[string][]histlog.Entry{}
	for _, e := range sorted {
		if e.Station != "" {
			sameStation = true
		}
		byStation[e.Station] = append(byStation[e.Station], e)
	}

	var (
		strict, night int
		checked       int
	)
	caught := []CaughtRepeat{} // non-nil so the JSON always carries an array

	for st, entries := range byStation {
		// Collapse consecutive re-logs of the same track into one airing: FIP
		// never plays a title twice back-to-back, so adjacent identical lines
		// are the same airing logged twice (metadata refresh), not a repeat.
		lastSeen := map[string]time.Time{} // normKey -> time of previous airing on this station
		prevKey := ""
		for _, e := range entries {
			art := normField(e.Artist)
			tit := normField(e.Title)
			if art == "" && tit == "" {
				prevKey = "" // an unnamed line breaks any run
				continue
			}
			key := art + "\x1f" + tit
			if key == prevKey {
				continue // same airing, still on air
			}
			prevKey = key
			checked++
			if prev, ok := lastSeen[key]; ok {
				gap := e.TS.Sub(prev)
				if gap > 0 && gap < repeatWindow {
					if isNightHour(e.TS.Hour()) {
						night++
					} else {
						strict++
						caught = append(caught, CaughtRepeat{
							Artist:      e.Artist,
							Title:       e.Title,
							Station:     st,
							StationName: stations.ByKey(st).Display,
							GapSec:      int64(gap.Seconds()),
						})
					}
				}
			}
			lastSeen[key] = e.TS
		}
	}

	// Most flagrant first (smallest gap), then a stable tiebreak, then cap.
	sort.SliceStable(caught, func(i, j int) bool {
		if caught[i].GapSec != caught[j].GapSec {
			return caught[i].GapSec < caught[j].GapSec
		}
		if caught[i].Artist != caught[j].Artist {
			return caught[i].Artist < caught[j].Artist
		}
		return caught[i].Title < caught[j].Title
	})
	if len(caught) > caughtCap {
		caught = caught[:caughtCap]
	}

	// Conducteurs: split observed listening into curated daytime vs night loop.
	var curatedSec, nightSec int64
	for h := 0; h < 24; h++ {
		if isNightHour(h) {
			nightSec += hourly[h]
		} else {
			curatedSec += hourly[h]
		}
	}
	blocks := int((float64(curatedSec)/3600.0)/blockHours + 0.5)

	return &Programme{
		Rule48h: Rule48h{
			StrictRepeats: strict,
			NightRepeats:  night,
			TracksChecked: checked,
			ObservedSec:   totalSec,
			Caught:        caught,
		},
		Conducteurs: Conducteurs{
			CuratedSec:     curatedSec,
			NightSec:       nightSec,
			BlocksEstimate: blocks,
		},
		SameStation: sameStation,
	}
}
