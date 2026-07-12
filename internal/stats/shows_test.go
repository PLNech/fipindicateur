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
	// A report built from an old events-only log (no history, no show_change
	// boundaries) generates with no shows block.
	r := Build([]events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: base},
		{Kind: events.KindPause, TS: base.Add(30 * time.Minute)},
	}, nil, nil, nil, base)
	if r.Shows != nil {
		t.Errorf("events-only report must omit shows, got %+v", r.Shows)
	}
}

// tracklessSundayEvents is a Sunday evening spent inside a programme that
// broadcasts no tracklist (a continuous mix like "Fip Tape"): the only signals
// are behaviour events, play plus the show_change boundaries. 2026-07-12 is a
// Sunday; the show runs 20:01 to 22:01 while playing until 22:30.
func tracklessSundayEvents(start time.Time) []events.Event {
	return []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: start},
		{Kind: events.KindShowChange, Station: "fip", From: "", To: "c-fiptape", TS: start.Add(time.Minute)},
		{Kind: events.KindShowChange, Station: "fip", From: "c-fiptape", To: "", TS: start.Add(121 * time.Minute)},
		{Kind: events.KindPause, TS: start.Add(150 * time.Minute)},
	}
}

func TestShowTimeFromEventsAloneTracklessShow(t *testing.T) {
	// The hard criterion for émissions without a tracklist: zero tagged track
	// lines, only show_change boundaries crossed with playback, must still
	// accumulate show time, produce the shows block and unlock the
	// Sunday-evening badge.
	start := time.Date(2026, 7, 12, 20, 0, 0, 0, time.Local) // a Sunday
	r := Build(tracklessSundayEvents(start), nil, nil, nil, start.Add(3*time.Hour))
	if r.Shows == nil {
		t.Fatal("expected a shows block from the show_change boundaries alone")
	}
	if r.Shows.Distinct != 1 || len(r.Shows.Shows) != 1 {
		t.Fatalf("expected exactly one programme, got %+v", r.Shows)
	}
	s := r.Shows.Shows[0]
	if s.Concept != "c-fiptape" {
		t.Errorf("concept: got %q want c-fiptape", s.Concept)
	}
	// Two hours on air while playing: the span grain measures it in full.
	if s.ListeningSec < 7100 || s.ListeningSec > 7300 {
		t.Errorf("listening: got %ds want ~7200 (the 2h span)", s.ListeningSec)
	}
	if s.Tracks != 0 {
		t.Errorf("no track was heard, got %d", s.Tracks)
	}
	// With no histlog at all the name honestly falls back to the concept id.
	if s.Name != "c-fiptape" {
		t.Errorf("name fallback: got %q want the concept id", s.Name)
	}
	if s.Evenings != 1 {
		t.Errorf("evenings: got %d want 1", s.Evenings)
	}
	// The denominator is all observed listening (150 min), so the share is a
	// proper fraction.
	if !(r.Shows.InShowShare > 0.7 && r.Shows.InShowShare < 0.9) {
		t.Errorf("in-show share: got %v want ~0.8 (120 of 150 min)", r.Shows.InShowShare)
	}
	unlocked := map[string]bool{}
	for _, a := range r.Achievements {
		unlocked[a.ID] = a.Unlocked
	}
	if !unlocked["shows_dimanche"] {
		t.Error("shows_dimanche must unlock from a trackless Sunday-evening programme")
	}
	if !unlocked["shows_premiere"] {
		t.Error("shows_premiere must unlock: a programme was heard")
	}
	if !unlocked["shows_marathon"] {
		t.Error("shows_marathon must unlock: 2h of programme in one evening")
	}
}

func TestShowMarkerNamesTheTracklessShow(t *testing.T) {
	// The show-start marker (a bare show tag, no artist and no title) supplies
	// the display name; the listening time still comes from the boundaries (the
	// marker's own capped interval must not bound it), and it counts no track.
	start := time.Date(2026, 7, 12, 20, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		{Station: "fip", Show: "Fip Tape", ShowConcept: "c-fiptape", TS: start.Add(time.Minute)},
	}
	r := Build(tracklessSundayEvents(start), hist, nil, nil, start.Add(3*time.Hour))
	if r.Shows == nil || len(r.Shows.Shows) != 1 {
		t.Fatalf("expected the one programme, got %+v", r.Shows)
	}
	s := r.Shows.Shows[0]
	if s.Name != "Fip Tape" {
		t.Errorf("name: got %q want Fip Tape (from the marker)", s.Name)
	}
	if s.Tracks != 0 {
		t.Errorf("the marker is identity, not a track: got %d tracks", s.Tracks)
	}
	if s.ListeningSec < 7100 {
		t.Errorf("listening: got %ds want the full ~7200s span, not the marker's capped interval", s.ListeningSec)
	}
}

func TestShowSpanSupersedesTracksSameDay(t *testing.T) {
	// A programme with BOTH boundaries and tagged tracks on the same evening
	// must not double count: per (concept, day) the span grain supersedes the
	// track grain, and the tagged tracks still count as tracks heard.
	start := time.Date(2026, 7, 10, 21, 0, 0, 0, time.Local)
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: start},
		{Kind: events.KindShowChange, Station: "fip", From: "", To: "c-jazz", TS: start},
		{Kind: events.KindShowChange, Station: "fip", From: "c-jazz", To: "", TS: start.Add(60 * time.Minute)},
		{Kind: events.KindPause, TS: start.Add(90 * time.Minute)},
	}
	hist := []histlog.Entry{
		hshow("fip", "A1", "T1", "Club Jazzafip", "c-jazz", start.Add(5*time.Minute)),
		hshow("fip", "A2", "T2", "Club Jazzafip", "c-jazz", start.Add(15*time.Minute)),
	}
	r := Build(evs, hist, nil, nil, start.Add(2*time.Hour))
	if r.Shows == nil || len(r.Shows.Shows) != 1 {
		t.Fatalf("expected the one programme, got %+v", r.Shows)
	}
	s := r.Shows.Shows[0]
	// The hour-long span, once: not span + track exposure.
	if s.ListeningSec < 3500 || s.ListeningSec > 3700 {
		t.Errorf("listening: got %ds want ~3600 (the span, counted once)", s.ListeningSec)
	}
	if s.Tracks != 2 {
		t.Errorf("tracks: got %d want 2", s.Tracks)
	}
	if s.Name != "Club Jazzafip" {
		t.Errorf("name: got %q want Club Jazzafip", s.Name)
	}
}

func TestShowSpanClosedByAppLifecycle(t *testing.T) {
	// A span left open at shutdown must not absorb the next day's listening:
	// the app's show state does not survive a restart.
	d1 := time.Date(2026, 7, 12, 20, 0, 0, 0, time.Local)
	d2 := d1.Add(14 * time.Hour) // next morning, plain rotation
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: d1},
		{Kind: events.KindShowChange, Station: "fip", From: "", To: "c-fiptape", TS: d1.Add(time.Minute)},
		{Kind: events.KindQuit, TS: d1.Add(60 * time.Minute)}, // quit mid-show
		{Kind: events.KindPlay, Station: "fip", TS: d2},
		{Kind: events.KindPause, TS: d2.Add(120 * time.Minute)},
	}
	r := Build(evs, nil, nil, nil, d2.Add(3*time.Hour))
	if r.Shows == nil || len(r.Shows.Shows) != 1 {
		t.Fatalf("expected the one programme, got %+v", r.Shows)
	}
	s := r.Shows.Shows[0]
	// Only the 59 minutes up to the quit, none of the morning's two hours.
	if s.ListeningSec < 3400 || s.ListeningSec > 3600 {
		t.Errorf("listening: got %ds want ~3540 (bounded by the quit)", s.ListeningSec)
	}
	if s.Evenings != 1 {
		t.Errorf("evenings: got %d want 1 (the morning is not the show)", s.Evenings)
	}
}
