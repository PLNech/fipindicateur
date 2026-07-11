package stats

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
)

// hbase anchors the programme tests in daytime so a repeat is "strict" unless a
// test deliberately places the later airing at night.
var hbase = time.Date(2026, 7, 8, 14, 0, 0, 0, time.Local)

func hentry(station, artist, title string, t time.Time) histlog.Entry {
	return histlog.Entry{Station: station, Artist: artist, Title: title, TS: t}
}

func TestRepeatWithin48hSameStationIsCaught(t *testing.T) {
	// Same track, same station, 47h apart, later airing in daytime: a catch.
	hist := []histlog.Entry{
		hentry("fip", "The Cure", "Just Like Heaven", hbase),
		hentry("fip", "Other", "Filler", hbase.Add(time.Hour)),
		hentry("fip", "The Cure", "Just Like Heaven", hbase.Add(47*time.Hour)),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme == nil {
		t.Fatal("programme block should be present with history")
	}
	if got := r.Programme.Rule48h.StrictRepeats; got != 1 {
		t.Errorf("strict repeats: got %d want 1", got)
	}
	if got := r.Programme.Rule48h.NightRepeats; got != 0 {
		t.Errorf("night repeats: got %d want 0", got)
	}
	if len(r.Programme.Rule48h.Caught) != 1 {
		t.Fatalf("caught list: got %d want 1", len(r.Programme.Rule48h.Caught))
	}
	c := r.Programme.Rule48h.Caught[0]
	if c.Artist != "The Cure" || c.Title != "Just Like Heaven" {
		t.Errorf("caught details wrong: %+v", c)
	}
	if c.GapSec != int64(47*time.Hour/time.Second) {
		t.Errorf("gap: got %d want %d", c.GapSec, int64(47*time.Hour/time.Second))
	}
	if !r.Programme.SameStation {
		t.Error("sameStation should be true when the histlog carries stations")
	}
}

func TestRepeatBeyond48hIsNotCaught(t *testing.T) {
	hist := []histlog.Entry{
		hentry("fip", "Air", "La Femme d'Argent", hbase),
		hentry("fip", "Filler", "x", hbase.Add(time.Hour)),
		hentry("fip", "Air", "La Femme d'Argent", hbase.Add(49*time.Hour)),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme.Rule48h.StrictRepeats != 0 || r.Programme.Rule48h.NightRepeats != 0 {
		t.Errorf("49h apart must not be a repeat: strict=%d night=%d",
			r.Programme.Rule48h.StrictRepeats, r.Programme.Rule48h.NightRepeats)
	}
}

func TestRepeatDifferentStationsIsNotCaught(t *testing.T) {
	// Same pair, within 48h, but on two different antennas: not a rule test.
	hist := []histlog.Entry{
		hentry("fip", "Nina Simone", "Feeling Good", hbase),
		hentry("jazz", "Nina Simone", "Feeling Good", hbase.Add(2*time.Hour)),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme.Rule48h.StrictRepeats != 0 || r.Programme.Rule48h.NightRepeats != 0 {
		t.Errorf("cross-station repeat must not count: strict=%d night=%d",
			r.Programme.Rule48h.StrictRepeats, r.Programme.Rule48h.NightRepeats)
	}
}

func TestRepeatAtNightIsNightClass(t *testing.T) {
	// First airing daytime, second at 02:00 the next night: expected loop.
	night := time.Date(2026, 7, 9, 2, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		hentry("fip", "Bonobo", "Kong", hbase),
		hentry("fip", "Filler", "x", hbase.Add(time.Hour)),
		hentry("fip", "Bonobo", "Kong", night),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme.Rule48h.NightRepeats != 1 {
		t.Errorf("night repeats: got %d want 1", r.Programme.Rule48h.NightRepeats)
	}
	if r.Programme.Rule48h.StrictRepeats != 0 {
		t.Errorf("a 02:00 second airing must be night-class, got %d strict", r.Programme.Rule48h.StrictRepeats)
	}
	if len(r.Programme.Rule48h.Caught) != 0 {
		t.Errorf("night repeats are not listed as caught: %+v", r.Programme.Rule48h.Caught)
	}
}

func TestNormalizationMatchesCaseAndSpace(t *testing.T) {
	hist := []histlog.Entry{
		hentry("fip", "The  Cure", "Just Like Heaven", hbase),
		hentry("fip", "Filler", "x", hbase.Add(time.Hour)),
		hentry("fip", "  the cure ", "  just like heaven  ", hbase.Add(3*time.Hour)),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme.Rule48h.StrictRepeats != 1 {
		t.Errorf("case/space-different pair should match as a repeat: got %d", r.Programme.Rule48h.StrictRepeats)
	}
}

func TestConsecutiveRelogIsNotARepeat(t *testing.T) {
	// The same track logged twice back-to-back (metadata refresh) is one
	// airing, not a repeat.
	hist := []histlog.Entry{
		hentry("fip", "Massive Attack", "Teardrop", hbase),
		hentry("fip", "Massive Attack", "Teardrop", hbase.Add(2*time.Minute)),
	}
	r := Build(nil, hist, nil, nil, base)
	if r.Programme.Rule48h.StrictRepeats != 0 || r.Programme.Rule48h.NightRepeats != 0 {
		t.Errorf("consecutive re-log must not be a repeat: strict=%d night=%d",
			r.Programme.Rule48h.StrictRepeats, r.Programme.Rule48h.NightRepeats)
	}
	if r.Programme.Rule48h.TracksChecked != 1 {
		t.Errorf("consecutive re-log should collapse to one airing: checked=%d", r.Programme.Rule48h.TracksChecked)
	}
}

func TestConducteursSplitsCuratedVsNight(t *testing.T) {
	// 1h listening in daytime (14:00-15:00) + 1h in the night loop (02:00-03:00).
	day := time.Date(2026, 7, 8, 14, 0, 0, 0, time.Local)
	nite := time.Date(2026, 7, 9, 2, 0, 0, 0, time.Local)
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: day},
		{Kind: events.KindPause, TS: day.Add(time.Hour)},
		{Kind: events.KindPlay, Station: "fip", TS: nite},
		{Kind: events.KindPause, TS: nite.Add(time.Hour)},
	}
	hist := []histlog.Entry{hentry("fip", "x", "y", day)} // presence triggers the block
	r := Build(evs, hist, nil, nil, base)
	c := r.Programme.Conducteurs
	if c.CuratedSec != 3600 {
		t.Errorf("curated seconds: got %d want 3600", c.CuratedSec)
	}
	if c.NightSec != 3600 {
		t.Errorf("night seconds: got %d want 3600", c.NightSec)
	}
	if r.Programme.Rule48h.ObservedSec != 7200 {
		t.Errorf("observed seconds: got %d want 7200", r.Programme.Rule48h.ObservedSec)
	}
}

func TestBlocksEstimateFromCuratedHours(t *testing.T) {
	// ~6h of daytime listening: two 3h blocks traversed.
	day := time.Date(2026, 7, 8, 10, 0, 0, 0, time.Local)
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: day},
		{Kind: events.KindPause, TS: day.Add(6 * time.Hour)},
	}
	hist := []histlog.Entry{hentry("fip", "x", "y", day)}
	r := Build(evs, hist, nil, nil, base)
	if r.Programme.Conducteurs.BlocksEstimate != 2 {
		t.Errorf("blocks: got %d want 2", r.Programme.Conducteurs.BlocksEstimate)
	}
}

func TestNoHistoryOmitsProgramme(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: hbase},
		{Kind: events.KindPause, TS: hbase.Add(time.Hour)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Programme != nil {
		t.Errorf("programme must be omitted without history, got %+v", r.Programme)
	}
}
