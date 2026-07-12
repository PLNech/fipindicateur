package stats

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
)

// hshow is a histlog line carrying a programme tag.
func hshow(station, artist, title, show, concept string, t time.Time) histlog.Entry {
	return histlog.Entry{Station: station, Artist: artist, Title: title, Show: show, ShowConcept: concept, TS: t}
}

// buildFromHist runs the full pure derivation over one continuous play session
// spanning the given histlog, so the tracks pick up real playback exposure.
func buildFromHist(hist []histlog.Entry, from, to time.Time) Report {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: hist[0].Station, TS: from},
		{Kind: events.KindPause, TS: to},
	}
	return Build(evs, hist, nil, nil, to)
}

func TestShowsAggregateByConcept(t *testing.T) {
	// Two airings of the same recurring show on two different nights, plus one
	// rotation track outside any show. The two airings collapse under one
	// conceptUuid; the rotation track is counted in the total but not in-show.
	d1 := time.Date(2026, 7, 10, 21, 0, 0, 0, time.Local)
	d2 := time.Date(2026, 7, 11, 21, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		hshow("fip", "A1", "T1", "Club Jazzafip", "c-jazz", d1),
		hshow("fip", "A2", "T2", "Club Jazzafip", "c-jazz", d1.Add(10*time.Minute)),
		{Station: "fip", Artist: "R", Title: "Rot", TS: d1.Add(20 * time.Minute)}, // rotation, no show
		hshow("fip", "A3", "T3", "Club Jazzafip", "c-jazz", d2),
	}
	r := buildFromHist(hist, d1, d2.Add(30*time.Minute))
	if r.Shows == nil {
		t.Fatal("expected a shows block")
	}
	if r.Shows.Distinct != 1 {
		t.Errorf("distinct shows: got %d want 1", r.Shows.Distinct)
	}
	if len(r.Shows.Shows) != 1 {
		t.Fatalf("expected 1 show stat, got %d", len(r.Shows.Shows))
	}
	s := r.Shows.Shows[0]
	if s.Concept != "c-jazz" || s.Name != "Club Jazzafip" {
		t.Errorf("show identity: %+v", s)
	}
	if s.Evenings != 2 {
		t.Errorf("evenings: got %d want 2 (two distinct nights)", s.Evenings)
	}
	if s.Tracks < 3 {
		t.Errorf("tracks: got %d want >=3 in-show tracks", s.Tracks)
	}
	if s.ListeningSec <= 0 || r.Shows.InShowSec <= 0 {
		t.Errorf("expected positive in-show listening: %+v", r.Shows)
	}
	// The rotation track lifts the total above the in-show time, so the in-show
	// share is strictly between 0 and 1.
	if !(r.Shows.InShowShare > 0 && r.Shows.InShowShare < 1) {
		t.Errorf("in-show share should be a proper fraction, got %v", r.Shows.InShowShare)
	}
	if r.Shows.TotalSec <= r.Shows.InShowSec {
		t.Errorf("total (%d) should exceed in-show (%d) because of the rotation track", r.Shows.TotalSec, r.Shows.InShowSec)
	}
}

func TestShowsSortedByListening(t *testing.T) {
	d := time.Date(2026, 7, 10, 21, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		// Short show first in time, long show second: the long one must sort first.
		hshow("fip", "A", "shortA", "Short", "c-short", d),
		hshow("fip", "B", "longB", "Long", "c-long", d.Add(5*time.Minute)),
		hshow("fip", "B2", "longB2", "Long", "c-long", d.Add(60*time.Minute)),
	}
	r := buildFromHist(hist, d, d.Add(120*time.Minute))
	if r.Shows == nil || len(r.Shows.Shows) != 2 {
		t.Fatalf("expected 2 shows, got %+v", r.Shows)
	}
	if r.Shows.Shows[0].Concept != "c-long" {
		t.Errorf("longest-listened show should sort first: %+v", r.Shows.Shows)
	}
	// Shares within the in-show total sum to ~1.
	sum := 0.0
	for _, s := range r.Shows.Shows {
		sum += s.Share
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("in-show shares should sum to ~1, got %v", sum)
	}
}

func TestShowsNilWithoutShowData(t *testing.T) {
	// A history with no programme tags (older format, or webradio-only) yields
	// no shows block: the section is simply absent.
	d := time.Date(2026, 7, 10, 21, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		{Station: "jazz", Artist: "A", Title: "T", TS: d},
		{Station: "jazz", Artist: "B", Title: "U", TS: d.Add(10 * time.Minute)},
	}
	r := buildFromHist(hist, d, d.Add(30*time.Minute))
	if r.Shows != nil {
		t.Errorf("no programme data should collapse the block, got %+v", r.Shows)
	}
}

func TestShowsAbsentFromEventsOnlyReport(t *testing.T) {
	// The hard criterion: a report built from an old events-only log (no
	// history at all) generates with no shows block.
	r := Build([]events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: base},
		{Kind: events.KindPause, TS: base.Add(30 * time.Minute)},
	}, nil, nil, nil, base)
	if r.Shows != nil {
		t.Errorf("events-only report must omit shows, got %+v", r.Shows)
	}
}
