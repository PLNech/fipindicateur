package stats

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
)

// base is a fixed local reference time for deterministic tests.
var base = time.Date(2026, 7, 8, 14, 0, 0, 0, time.Local)

func at(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }

func TestSingleSessionDuration(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(30)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Totals.ListeningSec != 30*60 {
		t.Errorf("listening: got %d want %d", r.Totals.ListeningSec, 30*60)
	}
	if r.Totals.Sessions != 1 {
		t.Errorf("sessions: got %d want 1", r.Totals.Sessions)
	}
	if r.Totals.Plays != 1 || r.Totals.Pauses != 1 {
		t.Errorf("plays/pauses: got %d/%d want 1/1", r.Totals.Plays, r.Totals.Pauses)
	}
	if len(r.Stations) != 1 || r.Stations[0].Key != "fip" || r.Stations[0].ListeningSec != 30*60 {
		t.Errorf("station stat mismatch: %+v", r.Stations)
	}
	if r.Stations[0].Share != 1.0 {
		t.Errorf("single-station share should be 1.0, got %v", r.Stations[0].Share)
	}
}

func TestZapWithinSessionSplitsTime(t *testing.T) {
	// Play fip, after 10min zap to jazz, after 20min more pause.
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindStationChange, From: "fip", To: "jazz", TS: at(10)},
		{Kind: events.KindPause, TS: at(30)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Totals.Sessions != 1 {
		t.Fatalf("a zap must not split the session: got %d sessions", r.Totals.Sessions)
	}
	if r.Totals.Zaps != 1 {
		t.Errorf("zaps: got %d want 1", r.Totals.Zaps)
	}
	got := map[string]int64{}
	for _, s := range r.Stations {
		got[s.Key] = s.ListeningSec
	}
	if got["fip"] != 10*60 || got["jazz"] != 20*60 {
		t.Errorf("time split: fip=%d jazz=%d want 600/1200", got["fip"], got["jazz"])
	}
	if r.Totals.ListeningSec != 30*60 {
		t.Errorf("total listening: got %d want %d", r.Totals.ListeningSec, 30*60)
	}
}

func TestPauseThenPlayIsTwoSessions(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindPause, TS: at(20)},
		{Kind: events.KindPlay, Station: "fip", TS: at(60)},
		{Kind: events.KindPause, TS: at(75)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Totals.Sessions != 2 {
		t.Errorf("sessions: got %d want 2", r.Totals.Sessions)
	}
	if r.Sessions.MaxSec != 20*60 {
		t.Errorf("max session: got %d want %d", r.Sessions.MaxSec, 20*60)
	}
	if r.Sessions.MedianSec != (20*60+15*60)/2 {
		t.Errorf("median: got %d", r.Sessions.MedianSec)
	}
}

func TestOpenSessionClosesAtLastEvent(t *testing.T) {
	// Playing and never paused: attribute time up to the last event only.
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindStationChange, From: "fip", To: "rock", TS: at(45)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Totals.ListeningSec != 45*60 {
		t.Errorf("open session should close at last event: got %d want %d", r.Totals.ListeningSec, 45*60)
	}
	if r.Totals.Sessions != 1 {
		t.Errorf("sessions: got %d want 1", r.Totals.Sessions)
	}
}

func TestTransitionMatrixNormalises(t *testing.T) {
	// fip->jazz twice, fip->rock once. Row fip should normalise to 2/3, 1/3.
	evs := []events.Event{
		{Kind: events.KindStationChange, From: "fip", To: "jazz", TS: at(1)},
		{Kind: events.KindStationChange, From: "fip", To: "jazz", TS: at(2)},
		{Kind: events.KindStationChange, From: "fip", To: "rock", TS: at(3)},
	}
	r := Build(evs, nil, nil, nil, base)
	if len(r.Transitions) != 2 {
		t.Fatalf("expected 2 distinct edges, got %d", len(r.Transitions))
	}
	// Sorted by count desc: fip->jazz first.
	top := r.Transitions[0]
	if top.From != "fip" || top.To != "jazz" || top.Count != 2 {
		t.Errorf("top transition mismatch: %+v", top)
	}
	if top.Prob < 0.66 || top.Prob > 0.67 {
		t.Errorf("prob should be ~0.667, got %v", top.Prob)
	}
	if top.FromDisplay != "FIP" || top.ToDisplay != "Jazz" {
		t.Errorf("display names not resolved: %+v", top)
	}
	var rowSum float64
	for _, tr := range r.Transitions {
		if tr.From == "fip" {
			rowSum += tr.Prob
		}
	}
	if rowSum < 0.999 || rowSum > 1.001 {
		t.Errorf("row probabilities must sum to 1, got %v", rowSum)
	}
}

func TestSelfTransitionIgnored(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindStationChange, From: "fip", To: "fip", TS: at(1)},
	}
	r := Build(evs, nil, nil, nil, base)
	if len(r.Transitions) != 0 {
		t.Errorf("from==to must not create an edge: %+v", r.Transitions)
	}
}

func TestHourlyHistogramCrossesBoundary(t *testing.T) {
	// Play 14:30, pause 15:30: 30 min in hour 14, 30 min in hour 15.
	start := time.Date(2026, 7, 8, 14, 30, 0, 0, time.Local)
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: start},
		{Kind: events.KindPause, TS: start.Add(time.Hour)},
	}
	r := Build(evs, nil, nil, nil, base)
	if r.Hourly[14] != 30*60 || r.Hourly[15] != 30*60 {
		t.Errorf("hourly split: h14=%d h15=%d want 1800/1800", r.Hourly[14], r.Hourly[15])
	}
}

func TestAchievementNightOwlAndMarathon(t *testing.T) {
	// A 2h30 session starting at 1:00am unlocks night_owl and marathon.
	start := time.Date(2026, 7, 8, 1, 0, 0, 0, time.Local)
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: start},
		{Kind: events.KindPause, TS: start.Add(150 * time.Minute)},
	}
	r := Build(evs, nil, nil, nil, base)
	got := map[string]bool{}
	for _, a := range r.Achievements {
		got[a.ID] = a.Unlocked
	}
	if !got["night_owl"] {
		t.Error("night_owl should unlock for 1am listening")
	}
	if !got["marathon"] {
		t.Error("marathon should unlock for a 2h30 session")
	}
	if got["faithful"] {
		t.Error("faithful should stay locked for a single day")
	}
}

func TestAchievementGlobeProgress(t *testing.T) {
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: at(0)},
		{Kind: events.KindStationChange, From: "fip", To: "jazz", TS: at(5)},
		{Kind: events.KindPause, TS: at(10)},
	}
	r := Build(evs, nil, nil, nil, base)
	for _, a := range r.Achievements {
		if a.ID == "globe" {
			if a.Unlocked {
				t.Error("globe should be locked with 2/13 stations")
			}
			if a.Current != 2 || a.Target != 13 {
				t.Errorf("globe progress: got %v/%v want 2/13", a.Current, a.Target)
			}
		}
	}
}

func TestLongestStreak(t *testing.T) {
	days := map[string]bool{
		"2026-07-01": true,
		"2026-07-02": true,
		"2026-07-03": true,
		"2026-07-05": true, // gap
	}
	if got := longestStreak(days); got != 3 {
		t.Errorf("streak: got %d want 3", got)
	}
	if got := longestStreak(map[string]bool{}); got != 0 {
		t.Errorf("empty streak: got %d want 0", got)
	}
}

func TestEmptyLogIsSafe(t *testing.T) {
	r := Build(nil, nil, nil, nil, base)
	if r.Totals.Sessions != 0 || r.Totals.ListeningSec != 0 {
		t.Errorf("empty log should be all-zero: %+v", r.Totals)
	}
	if r.Sessions.Count != 0 {
		t.Errorf("empty sessions: %+v", r.Sessions)
	}
	if len(r.Achievements) == 0 {
		t.Error("achievements list should still render (all locked)")
	}
}
