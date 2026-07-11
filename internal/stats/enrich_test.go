package stats

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/prefs"
)

// playPause is one hour of continuous fip playback: a single segment [0,60min)
// that fully covers the histlog intervals in the scenarios below.
func playPauseHour() []events.Event {
	return []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(60)},
	}
}

// scenarioHist is the shared enrichment fixture: three fip tracks inside the
// playing hour. Track B's next line is 15 min away, so its interval is capped.
func scenarioHist() []histlog.Entry {
	return []histlog.Entry{
		{Station: "fip", Artist: "Artist A", Title: "A1", Year: 1968, Label: "Blue Note Records", TS: at(0)},
		{Station: "fip", Artist: "Artist B", Title: "B1", Year: 1975, Label: "Tiny Indie", TS: at(5)},
		{Station: "fip", Artist: "Artist A", Title: "A2", Year: 1968, TS: at(20)},
	}
}

func scenarioEnriched() *Enriched {
	return &Enriched{
		MatchRate: 0.83,
		NArtists:  283,
		Artists: map[string]EnrichedArtist{
			"Artist A": {
				Label:       "Artist A Full",
				Genres:      []string{"jazz", "blues"},
				Country:     "France",
				CountryCode: "FR",
				Coords:      []float64{0.1, 0.2},
				Wikipedia:   "https://en.wikipedia.org/wiki/Artist_A",
			},
			"Artist B": {
				Genres:      []string{"rock"},
				Country:     "United States",
				CountryCode: "US",
				Coords:      []float64{0.5, 0.6},
			},
		},
	}
}

func TestTrackIntervalCappedAtEightMinutes(t *testing.T) {
	r := Build(playPauseHour(), scenarioHist(), nil, nil, base)
	if r.Enriched == nil {
		t.Fatal("enriched block should be present when histlog exists")
	}
	// Track B (1975) has its next line 15 min later; exposure must cap at 8min.
	var y1975 int64
	for _, ys := range r.Epochs.ByYear {
		if ys.Year == 1975 {
			y1975 = ys.Seconds
		}
	}
	if y1975 != 8*60 {
		t.Errorf("capped exposure: got %ds want 480s (8min cap)", y1975)
	}
}

func TestEpochsBucketing(t *testing.T) {
	r := Build(playPauseHour(), scenarioHist(), nil, nil, base)
	if r.Epochs == nil {
		t.Fatal("epochs should be present")
	}
	if r.Epochs.N != 3 {
		t.Errorf("epochs n: got %d want 3 (all three tracks carry a year)", r.Epochs.N)
	}
	byYear := map[int]int64{}
	for _, ys := range r.Epochs.ByYear {
		byYear[ys.Year] = ys.Seconds
	}
	// A1 [0,5)=300 + A2 [20,28)=480 => 1968 total 780; B1 capped => 1975 = 480.
	if byYear[1968] != 780 {
		t.Errorf("1968 exposure: got %d want 780", byYear[1968])
	}
	if byYear[1975] != 480 {
		t.Errorf("1975 exposure: got %d want 480", byYear[1975])
	}
	byDecade := map[int]int64{}
	for _, ds := range r.Epochs.ByDecade {
		byDecade[ds.Decade] = ds.Seconds
	}
	if byDecade[1960] != 780 || byDecade[1970] != 480 {
		t.Errorf("decade buckets: 1960=%d 1970=%d want 780/480", byDecade[1960], byDecade[1970])
	}
}

func TestEpochsExcludesTracksWithoutYear(t *testing.T) {
	hist := []histlog.Entry{
		{Station: "fip", Artist: "X", Year: 1990, TS: at(0)},
		{Station: "fip", Artist: "Y", TS: at(3)}, // no year
	}
	r := Build(playPauseHour(), hist, nil, nil, base)
	if r.Epochs.N != 1 {
		t.Errorf("epochs n should count only year-bearing tracks: got %d want 1", r.Epochs.N)
	}
}

func TestGenreSecondsSplitEvenly(t *testing.T) {
	r := Build(playPauseHour(), scenarioHist(), nil, scenarioEnriched(), base)
	genres := map[string]int64{}
	for _, g := range r.Enriched.Genres {
		genres[g.Name] = g.Seconds
	}
	// Artist A exposure 780 split evenly across jazz+blues => 390 each.
	if genres["jazz"] != 390 || genres["blues"] != 390 {
		t.Errorf("genre split: jazz=%d blues=%d want 390/390", genres["jazz"], genres["blues"])
	}
	// Artist B exposure 480, single genre rock.
	if genres["rock"] != 480 {
		t.Errorf("rock: got %d want 480", genres["rock"])
	}
}

func TestGenreSplitConservesTotalWithRemainder(t *testing.T) {
	// A single artist with an odd exposure across 3 genres: remainder must be
	// distributed so the parts sum exactly to the whole.
	hist := []histlog.Entry{
		{Station: "fip", Artist: "Z", TS: at(0)},
	}
	// Only one track, no next line: interval [0,8min), fully inside the hour.
	enr := &Enriched{Artists: map[string]EnrichedArtist{
		"Z": {Genres: []string{"a", "b", "c"}},
	}}
	r := Build(playPauseHour(), hist, nil, enr, base)
	var sum int64
	for _, g := range r.Enriched.Genres {
		sum += g.Seconds
	}
	if sum != 8*60 {
		t.Errorf("genre split must conserve total: got %d want 480", sum)
	}
}

func TestCountriesAndConstellation(t *testing.T) {
	r := Build(playPauseHour(), scenarioHist(), nil, scenarioEnriched(), base)
	countries := map[string]int64{}
	for _, c := range r.Enriched.Countries {
		countries[c.Code] = c.Seconds
	}
	if countries["FR"] != 780 || countries["US"] != 480 {
		t.Errorf("countries: FR=%d US=%d want 780/480", countries["FR"], countries["US"])
	}
	if r.Enriched.MatchRate != 0.83 || r.Enriched.NArtists != 283 {
		t.Errorf("enriched passthrough: matchRate=%v nArtists=%d want 0.83/283", r.Enriched.MatchRate, r.Enriched.NArtists)
	}
	plays := map[string]int{}
	for _, p := range r.Enriched.Constellation {
		plays[p.Name] = p.Plays
	}
	// Artist A: 2 histlog lines, resolved label used as name.
	if plays["Artist A Full"] != 2 {
		t.Errorf("constellation plays for A: got %d want 2", plays["Artist A Full"])
	}
	// Artist B: 1 line, no resolved label => raw name.
	if plays["Artist B"] != 1 {
		t.Errorf("constellation plays for B: got %d want 1", plays["Artist B"])
	}
}

func TestLabelIndieHeuristic(t *testing.T) {
	r := Build(playPauseHour(), scenarioHist(), nil, nil, base)
	indie := map[string]bool{}
	secs := map[string]int64{}
	for _, l := range r.Enriched.Labels {
		indie[l.Name] = l.Indie
		secs[l.Name] = l.Seconds
	}
	if indie["Blue Note Records"] {
		t.Error("'Blue Note Records' contains a major marker, should be indie=false")
	}
	if !indie["Tiny Indie"] {
		t.Error("'Tiny Indie' matches no major marker, should be indie=true")
	}
	// Label seconds track exposure: A1 [0,5)=300; A2 has no label.
	if secs["Blue Note Records"] != 300 {
		t.Errorf("Blue Note seconds: got %d want 300", secs["Blue Note Records"])
	}
}

func TestImplicitEarlyPauseBoundary(t *testing.T) {
	hist := []histlog.Entry{{Station: "fip", Artist: "X", TS: at(0)}}

	// Pause at 59s: inside the first minute => an early-pause hint.
	evs59 := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(0).Add(59 * time.Second)},
	}
	r59 := Build(evs59, hist, nil, nil, base)
	if r59.Tastes.Implicit.EarlyPauses != 1 {
		t.Errorf("pause at 59s should be an early pause: got %d want 1", r59.Tastes.Implicit.EarlyPauses)
	}

	// Pause at 61s: past the first minute => not an early pause.
	evs61 := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(0).Add(61 * time.Second)},
	}
	r61 := Build(evs61, hist, nil, nil, base)
	if r61.Tastes.Implicit.EarlyPauses != 0 {
		t.Errorf("pause at 61s should not be an early pause: got %d want 0", r61.Tastes.Implicit.EarlyPauses)
	}
}

func TestImplicitZapOut(t *testing.T) {
	hist := []histlog.Entry{{Station: "fip", Artist: "X", TS: at(0)}}
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindStationChange, From: "fip", To: "jazz", TS: at(2)}, // track X still under way
	}
	r := Build(evs, hist, nil, nil, base)
	if r.Tastes.Implicit.ZapOuts != 1 {
		t.Errorf("zap away mid-track should be a zap-out hint: got %d want 1", r.Tastes.Implicit.ZapOuts)
	}
	if r.Tastes.Implicit.N != 1 {
		t.Errorf("implicit n should equal tracks considered: got %d want 1", r.Tastes.Implicit.N)
	}
}

func TestExplicitTastes(t *testing.T) {
	prf := []prefs.Entry{
		{Artist: "A", Title: "1", Verdict: prefs.Like, TS: at(1)},
		{Artist: "B", Title: "2", Verdict: prefs.Like, TS: at(3)},
		{Artist: "C", Title: "3", Verdict: prefs.Dislike, TS: at(2)},
	}
	r := Build(nil, nil, prf, nil, base)
	if r.Tastes == nil {
		t.Fatal("tastes should be present with prefs only")
	}
	if r.Tastes.Likes != 2 || r.Tastes.Dislikes != 1 {
		t.Errorf("verdict counts: likes=%d dislikes=%d want 2/1", r.Tastes.Likes, r.Tastes.Dislikes)
	}
	if len(r.Tastes.Items) != 3 {
		t.Fatalf("items: got %d want 3", len(r.Tastes.Items))
	}
	// Most recent first: B at(3), C at(2), A at(1).
	if r.Tastes.Items[0].Artist != "B" || r.Tastes.Items[2].Artist != "A" {
		t.Errorf("items should be newest-first: got %q..%q", r.Tastes.Items[0].Artist, r.Tastes.Items[2].Artist)
	}
	// Prefs but no histlog: implicit hints have nothing to consider.
	if r.Tastes.Implicit.N != 0 {
		t.Errorf("implicit n with no histlog: got %d want 0", r.Tastes.Implicit.N)
	}
	// No histlog and no enriched: those blocks must stay absent.
	if r.Epochs != nil || r.Enriched != nil {
		t.Error("epochs/enriched must be nil when only prefs are provided")
	}
}

func TestItemsCapAtTwenty(t *testing.T) {
	var prf []prefs.Entry
	for i := 0; i < 25; i++ {
		prf = append(prf, prefs.Entry{Artist: "A", Verdict: prefs.Like, TS: at(i)})
	}
	r := Build(nil, nil, prf, nil, base)
	if len(r.Tastes.Items) != 20 {
		t.Errorf("items should cap at 20: got %d", len(r.Tastes.Items))
	}
	if r.Tastes.Likes != 25 {
		t.Errorf("counts should reflect all verdicts, not just the shown 20: got %d want 25", r.Tastes.Likes)
	}
}

func TestNilInputsProduceNoNewKeys(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(30)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Epochs != nil || r.Enriched != nil || r.Tastes != nil {
		t.Fatal("all companion inputs nil must leave all new blocks nil")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"\"epochs\"", "\"enriched\"", "\"tastes\""} {
		if strings.Contains(string(data), key) {
			t.Errorf("events-only report JSON must not contain %s", key)
		}
	}
}

func TestLoadEnrichedTolerant(t *testing.T) {
	if LoadEnriched("/nonexistent/path/enriched.json") != nil {
		t.Error("missing file should yield nil, not a panic or partial value")
	}
}
