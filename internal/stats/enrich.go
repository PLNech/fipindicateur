package stats

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PLNech/fipindicateur/internal/histlog"
)

// trackCap bounds a single track's exposure window. FIP tracks are radio-length
// (rarely over ~7 min) and the histlog only records a line on a *change*, so the
// gap to the next same-station line is our best interval estimate. We cap it at
// 8 minutes so a track that is "last before a long pause" (no next line for
// hours) does not claim the whole idle stretch as listening.
const trackCap = 8 * time.Minute

// exposedTrack is one histlog line with its honest exposure: the track's
// interval [Start, End) (End = next same-station line, capped at trackCap)
// intersected with the reconstructed playback segments on that station. A track
// logged while paused, or on a station you were not hearing, contributes zero.
type exposedTrack struct {
	Station  string
	Artist   string
	Title    string
	Year     int
	Label    string
	Start    time.Time
	End      time.Time // capped interval end (for the temporal-join hints)
	Exposure time.Duration
}

// Epochs buckets exposure seconds by the release year of the tracks heard. n is
// the number of tracks that carried a year (coverage honesty): tracks without a
// year are excluded from the buckets and n says how many contributed.
type Epochs struct {
	ByYear   []YearSeconds   `json:"byYear"`
	ByDecade []DecadeSeconds `json:"byDecade"`
	N        int             `json:"n"`
}

// YearSeconds is exposure seconds for a single release year.
type YearSeconds struct {
	Year    int   `json:"year"`
	Seconds int64 `json:"seconds"`
}

// DecadeSeconds is exposure seconds aggregated to a decade (e.g. 1960).
type DecadeSeconds struct {
	Decade  int   `json:"decade"`
	Seconds int64 `json:"seconds"`
}

// EnrichedStats is the artist-metadata view. Labels are derived from the
// histlog Label field alone; genres, countries and the constellation require
// the enriched.json artist metadata (matchRate/nArtists are its passthrough
// coverage figures). Every slice is non-nil so the JSON always carries arrays.
type EnrichedStats struct {
	MatchRate     float64              `json:"matchRate"`
	NArtists      int                  `json:"nArtists"`
	Genres        []GenreSeconds       `json:"genres"`
	Countries     []CountrySeconds     `json:"countries"`
	Labels        []LabelSeconds       `json:"labels"`
	Constellation []ConstellationPoint `json:"constellation"`
}

// GenreSeconds is exposure seconds attributed to a genre. An artist's seconds
// are split evenly across all of its genres (see buildEnriched).
type GenreSeconds struct {
	Name    string `json:"name"`
	Seconds int64  `json:"seconds"`
}

// CountrySeconds is exposure seconds attributed to an artist's country.
type CountrySeconds struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Seconds int64  `json:"seconds"`
}

// LabelSeconds is exposure seconds per record label, with a heuristic indie
// flag (true unless the label name matches a curated major-label marker).
type LabelSeconds struct {
	Name    string `json:"name"`
	Seconds int64  `json:"seconds"`
	Indie   bool   `json:"indie"`
}

// ConstellationPoint is one matched artist placed by its enriched.json coords
// (x = coords[0], y = coords[1], passed through as the companion tool emits
// them). plays is the raw histlog line count for the artist.
type ConstellationPoint struct {
	Name      string   `json:"name"`
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Plays     int      `json:"plays"`
	Genres    []string `json:"genres"`
	Wikipedia string   `json:"wikipedia"`
}

// Enriched mirrors the companion tool's enriched.json: per-raw-artist-name
// Wikidata metadata plus coverage figures. It is an optional input to Build;
// LoadEnriched tolerates a missing or malformed file by returning nil.
type Enriched struct {
	V           int                       `json:"v"`
	GeneratedAt string                    `json:"generated_at"`
	NArtists    int                       `json:"n_artists"`
	NMatched    int                       `json:"n_matched"`
	MatchRate   float64                   `json:"match_rate"`
	Artists     map[string]EnrichedArtist `json:"artists"`
}

// EnrichedArtist is one artist's resolved metadata. coords is a 2-element
// [x, y] placement (empty when unknown).
type EnrichedArtist struct {
	QID         string    `json:"qid"`
	Label       string    `json:"label"`
	Confidence  float64   `json:"confidence"`
	Description string    `json:"description"`
	Genres      []string  `json:"genres"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"`
	Year        int       `json:"year"`
	Wikipedia   string    `json:"wikipedia"`
	Coords      []float64 `json:"coords"`
}

// LoadEnriched reads enriched.json best-effort. A missing or malformed file is
// not an error: it returns nil, and Build simply omits the enriched-only
// figures (genres, countries, constellation, matchRate). Enrichment must never
// gate the core report.
func LoadEnriched(path string) *Enriched {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var e Enriched
	if json.Unmarshal(data, &e) != nil {
		return nil
	}
	return &e
}

// overlap returns the duration shared by the half-open intervals [aStart,aEnd)
// and [bStart,bEnd), or zero when they are disjoint.
func overlap(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	s := aStart
	if bStart.After(s) {
		s = bStart
	}
	e := aEnd
	if bEnd.Before(e) {
		e = bEnd
	}
	if e.After(s) {
		return e.Sub(s)
	}
	return 0
}

// trackExposure turns the histlog into honest per-track exposure. Each track's
// interval runs from its line to the next line *on the same station*, capped at
// trackCap, and is intersected with the playback segments on that station. The
// result is aligned one-to-one with the (sorted) histlog, so plays counts and
// exposure share a single source of truth.
func trackExposure(hist []histlog.Entry, segments []segment) []exposedTrack {
	if len(hist) == 0 {
		return nil
	}
	sorted := make([]histlog.Entry, len(hist))
	copy(sorted, hist)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.Before(sorted[j].TS) })

	segByStation := map[string][]segment{}
	for _, s := range segments {
		segByStation[s.station] = append(segByStation[s.station], s)
	}

	// nextTS[i] = TS of the next later line on the same station (zero if none).
	nextTS := make([]time.Time, len(sorted))
	seen := map[string]time.Time{}
	for i := len(sorted) - 1; i >= 0; i-- {
		if t, ok := seen[sorted[i].Station]; ok {
			nextTS[i] = t
		}
		seen[sorted[i].Station] = sorted[i].TS
	}

	out := make([]exposedTrack, 0, len(sorted))
	for i, e := range sorted {
		start := e.TS
		end := start.Add(trackCap)
		if !nextTS[i].IsZero() && nextTS[i].Before(end) {
			end = nextTS[i]
		}
		var exp time.Duration
		for _, s := range segByStation[e.Station] {
			exp += overlap(start, end, s.start, s.end)
		}
		out = append(out, exposedTrack{
			Station:  e.Station,
			Artist:   e.Artist,
			Title:    e.Title,
			Year:     e.Year,
			Label:    e.Label,
			Start:    start,
			End:      end,
			Exposure: exp,
		})
	}
	return out
}

// buildEpochs buckets track exposure by release year. Returns nil when there
// are no tracks at all (so an events-only report omits the block); when tracks
// exist but none carries a year the block is present with n == 0 and empty
// buckets, which is the calibrated "we heard music but know no dates" signal.
func buildEpochs(tracks []exposedTrack) *Epochs {
	if len(tracks) == 0 {
		return nil
	}
	byYear := map[int]int64{}
	byDecade := map[int]int64{}
	n := 0
	for _, t := range tracks {
		if t.Year <= 0 {
			continue
		}
		n++
		secs := int64(t.Exposure.Seconds())
		byYear[t.Year] += secs
		byDecade[(t.Year/10)*10] += secs
	}
	ep := &Epochs{ByYear: []YearSeconds{}, ByDecade: []DecadeSeconds{}, N: n}
	for y, s := range byYear {
		ep.ByYear = append(ep.ByYear, YearSeconds{Year: y, Seconds: s})
	}
	for d, s := range byDecade {
		ep.ByDecade = append(ep.ByDecade, DecadeSeconds{Decade: d, Seconds: s})
	}
	sort.Slice(ep.ByYear, func(i, j int) bool { return ep.ByYear[i].Year < ep.ByYear[j].Year })
	sort.Slice(ep.ByDecade, func(i, j int) bool { return ep.ByDecade[i].Decade < ep.ByDecade[j].Decade })
	return ep
}

// majorLabelMarkers are substrings (matched case-insensitively) that flag a
// label as belonging to a major group. This is deliberately a coarse heuristic,
// not a definitive corporate-ownership graph: the report captions it as such.
var majorLabelMarkers = []string{
	"universal", "umg", "polydor", "island", "def jam", "interscope",
	"capitol", "emi", "sony", "columbia", "rca", "epic", "arista",
	"legacy", "warner", "wea", "atlantic", "elektra", "asylum",
	"parlophone", "nonesuch", "blue note", "verve", "decca",
	"deutsche grammophon", "mercury",
}

// isIndie is the heuristic: a label is indie unless its name contains a known
// major-label marker as a substring. Coarse by design (it will miss majors it
// does not list and can false-positive on a marker appearing in an indie name).
func isIndie(label string) bool {
	l := strings.ToLower(label)
	for _, m := range majorLabelMarkers {
		if strings.Contains(l, m) {
			return false
		}
	}
	return true
}

// buildEnriched joins per-track exposure onto artist metadata. Labels come from
// the histlog Label field alone; genres, countries and the constellation
// require enr. Returns nil only when there is nothing at all to report (no
// tracks and no enriched metadata).
func buildEnriched(tracks []exposedTrack, enr *Enriched) *EnrichedStats {
	if len(tracks) == 0 && enr == nil {
		return nil
	}
	es := &EnrichedStats{
		Genres:        []GenreSeconds{},
		Countries:     []CountrySeconds{},
		Labels:        []LabelSeconds{},
		Constellation: []ConstellationPoint{},
	}
	if enr != nil {
		es.MatchRate = enr.MatchRate
		es.NArtists = enr.NArtists
	}

	// Labels: exposure seconds per label name (histlog only).
	labelSec := map[string]int64{}
	for _, t := range tracks {
		if t.Label == "" {
			continue
		}
		labelSec[t.Label] += int64(t.Exposure.Seconds())
	}
	for name, sec := range labelSec {
		es.Labels = append(es.Labels, LabelSeconds{Name: name, Seconds: sec, Indie: isIndie(name)})
	}
	sortBySecondsThenName(es.Labels)

	// Per-artist exposure (seconds) and plays (histlog line count).
	artistSec := map[string]int64{}
	artistPlays := map[string]int{}
	for _, t := range tracks {
		if t.Artist == "" {
			continue
		}
		artistSec[t.Artist] += int64(t.Exposure.Seconds())
		artistPlays[t.Artist]++
	}

	if enr != nil {
		genreSec := map[string]int64{}
		type ctryAgg struct {
			code, name string
			sec        int64
		}
		countryAgg := map[string]*ctryAgg{}
		for raw, sec := range artistSec {
			a, ok := enr.Artists[raw]
			if !ok {
				continue
			}
			// Genres: split the artist's seconds evenly across its genres. An
			// artist commonly carries several (e.g. "jazz", "bossa nova"); with
			// no per-track genre we cannot know which applied, so an even split
			// is the least-assuming attribution. The integer remainder is
			// handed to the first genres deterministically so the total is
			// conserved exactly.
			if n := int64(len(a.Genres)); n > 0 {
				per := sec / n
				rem := sec % n
				for gi, g := range a.Genres {
					add := per
					if int64(gi) < rem {
						add++
					}
					genreSec[g] += add
				}
			}
			if a.CountryCode != "" || a.Country != "" {
				key := a.CountryCode
				if key == "" {
					key = a.Country
				}
				if countryAgg[key] == nil {
					countryAgg[key] = &ctryAgg{code: a.CountryCode, name: a.Country}
				}
				countryAgg[key].sec += sec
			}
		}
		for name, sec := range genreSec {
			es.Genres = append(es.Genres, GenreSeconds{Name: name, Seconds: sec})
		}
		for _, c := range countryAgg {
			es.Countries = append(es.Countries, CountrySeconds{Code: c.code, Name: c.name, Seconds: c.sec})
		}
		// Constellation: one point per matched artist that carries coords (a
		// scatter needs a position). plays comes from the histlog line count.
		for raw, plays := range artistPlays {
			a, ok := enr.Artists[raw]
			if !ok || len(a.Coords) < 2 {
				continue
			}
			name := a.Label
			if name == "" {
				name = raw
			}
			es.Constellation = append(es.Constellation, ConstellationPoint{
				Name:      name,
				X:         a.Coords[0],
				Y:         a.Coords[1],
				Plays:     plays,
				Genres:    topGenres(a.Genres),
				Wikipedia: a.Wikipedia,
			})
		}
		sort.Slice(es.Genres, func(i, j int) bool {
			if es.Genres[i].Seconds != es.Genres[j].Seconds {
				return es.Genres[i].Seconds > es.Genres[j].Seconds
			}
			return es.Genres[i].Name < es.Genres[j].Name
		})
		sort.Slice(es.Countries, func(i, j int) bool {
			if es.Countries[i].Seconds != es.Countries[j].Seconds {
				return es.Countries[i].Seconds > es.Countries[j].Seconds
			}
			return es.Countries[i].Code < es.Countries[j].Code
		})
		// Deterministic constellation order: plays desc, then name.
		sort.Slice(es.Constellation, func(i, j int) bool {
			if es.Constellation[i].Plays != es.Constellation[j].Plays {
				return es.Constellation[i].Plays > es.Constellation[j].Plays
			}
			return es.Constellation[i].Name < es.Constellation[j].Name
		})
	}
	return es
}

// topGenres caps a genre list to the first three so constellation tooltips stay
// legible. Order is the enriched.json order (Wikidata gives no ranking).
func topGenres(g []string) []string {
	if len(g) <= 3 {
		return g
	}
	return g[:3]
}

// sortBySecondsThenName orders labels by seconds desc, then name asc.
func sortBySecondsThenName(ls []LabelSeconds) {
	sort.Slice(ls, func(i, j int) bool {
		if ls[i].Seconds != ls[j].Seconds {
			return ls[i].Seconds > ls[j].Seconds
		}
		return ls[i].Name < ls[j].Name
	})
}
