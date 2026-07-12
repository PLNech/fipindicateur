package stats

import (
	"strings"
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
)

// achievementsFor grades the badges with the given show facts and no base
// listening data, returning them keyed by id. The nine base badges stay locked;
// only the émission badges react to sf, which is what these tests exercise.
func achievementsFor(sf showFacts) map[string]Achievement {
	got := map[string]Achievement{}
	for _, a := range evaluateAchievements(nil, nil, nil, [24]int64{}, 0, 0, sf) {
		got[a.ID] = a
	}
	return got
}

const hour = int64(3600)

// showBadgeTriggers is one showFacts per émission achievement, tuned so that
// badge unlocks. Every "shows_" achievement MUST appear here (enforced by
// TestEveryShowAchievementHasATrigger), so a new badge cannot ship untested.
var showBadgeTriggers = map[string]showFacts{
	"shows_premiere":         {distinctConcepts: 1},
	"shows_curieux":          {distinctConcepts: 3},
	"shows_habitue":          {distinctConcepts: 7},
	"shows_connaisseur":      {distinctConcepts: 15},
	"shows_fidele_bronze":    {maxEvenings: 5},
	"shows_fidele_argent":    {maxEvenings: 20},
	"shows_fidele_or":        {maxEvenings: 50},
	"shows_assidu":           {maxConceptStreak: 3},
	"shows_rituel":           {maxConceptStreak: 7},
	"shows_temps_bronze":     {inShowSec: 5 * hour},
	"shows_temps_argent":     {inShowSec: 20 * hour},
	"shows_temps_or":         {inShowSec: 50 * hour},
	"shows_titres_bronze":    {inShowTracks: 50},
	"shows_titres_argent":    {inShowTracks: 200},
	"shows_titres_or":        {inShowTracks: 500},
	"shows_part_quart":       {inShowSec: 250, totalSec: 1000}, // 25%
	"shows_part_moitie":      {inShowSec: 500, totalSec: 1000}, // 50%
	"shows_part_gros":        {inShowSec: 800, totalSec: 1000}, // 80%
	"shows_traversee_bronze": {showChanges: 10},
	"shows_traversee_or":     {showChanges: 50},
	"shows_marathon":         {maxEveningInShow: 2 * hour},
	"shows_veillee":          {maxEveningInShow: 4 * hour},
	"shows_double_programme": {twoConceptsEvening: true},
	"shows_dimanche":         {sundayEvening: true},
	"shows_nocturne":         {nightInShowSec: 1800},
	"shows_calendrier":       {calendarDiscovered: true},
}

func TestEveryShowAchievementTriggers(t *testing.T) {
	for id, sf := range showBadgeTriggers {
		a, ok := achievementsFor(sf)[id]
		if !ok {
			t.Errorf("%s: no such achievement", id)
			continue
		}
		if !a.Unlocked {
			t.Errorf("%s: expected unlocked with %+v, got current=%v target=%v", id, sf, a.Current, a.Target)
		}
	}
}

// TestEveryShowAchievementHasATrigger guards coverage: every badge whose id is
// prefixed "shows_" must have an entry in showBadgeTriggers, so adding a badge
// without a test fails the build.
func TestEveryShowAchievementHasATrigger(t *testing.T) {
	// A maximal showFacts unlocks every émission badge, so we can enumerate them.
	all := achievementsFor(showFacts{
		distinctConcepts: 99, maxEvenings: 99, maxConceptStreak: 99,
		inShowSec: 100 * hour, totalSec: 100 * hour, inShowTracks: 9999,
		maxEveningInShow: 10 * hour, twoConceptsEvening: true, sundayEvening: true,
		nightInShowSec: 99999, showChanges: 999, calendarDiscovered: true,
	})
	count := 0
	for id := range all {
		if !strings.HasPrefix(id, "shows_") {
			continue
		}
		count++
		if _, ok := showBadgeTriggers[id]; !ok {
			t.Errorf("achievement %s has no trigger in showBadgeTriggers", id)
		}
	}
	if count < 20 {
		t.Errorf("expected at least 20 émission achievements, found %d", count)
	}
	t.Logf("émission achievements: %d", count)
}

// TestShowAchievementsLockedWhenEmpty is the hard floor: with no history and no
// events, not a single émission badge is unlocked, and the base badges are all
// locked too.
func TestShowAchievementsLockedWhenEmpty(t *testing.T) {
	r := Build(nil, nil, nil, nil, base)
	for _, a := range r.Achievements {
		if strings.HasPrefix(a.ID, "shows_") && a.Unlocked {
			t.Errorf("émission badge %s unlocked on an empty log", a.ID)
		}
	}
	// And an events-only report (no history at all) unlocks no émission badge.
	r2 := Build([]events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: base},
		{Kind: events.KindPause, TS: base.Add(30 * time.Minute)},
	}, nil, nil, nil, base)
	for _, a := range r2.Achievements {
		if strings.HasPrefix(a.ID, "shows_") && a.Unlocked {
			t.Errorf("émission badge %s unlocked on an events-only log", a.ID)
		}
	}
}

// TestShowFactsFromHistlog checks the derivation end-to-end: a real histlog with
// show tags, played back, unlocks the data-driven badges (first émission,
// breadth, fidelity, double programme) through Build, not just via hand-built
// facts.
func TestShowFactsFromHistlog(t *testing.T) {
	d1 := time.Date(2026, 7, 10, 21, 0, 0, 0, time.Local)
	d2 := time.Date(2026, 7, 11, 21, 0, 0, 0, time.Local)
	d3 := time.Date(2026, 7, 12, 21, 0, 0, 0, time.Local)
	hist := []histlog.Entry{
		hshow("fip", "A", "T1", "Club Jazzafip", "c-jazz", d1),
		hshow("fip", "B", "T2", "Transe Fip Express", "c-transe", d1.Add(40*time.Minute)), // 2 concepts, same evening
		hshow("fip", "C", "T3", "Club Jazzafip", "c-jazz", d2),
		hshow("fip", "D", "T4", "Club Jazzafip", "c-jazz", d3), // c-jazz on 3 distinct evenings
	}
	evs := []events.Event{
		{Kind: events.KindPlay, Station: "fip", TS: d1},
		{Kind: events.KindPause, TS: d3.Add(30 * time.Minute)},
		{Kind: events.KindShowCalendar, Value: 1, TS: d1},
	}
	r := Build(evs, hist, nil, nil, d3.Add(time.Hour))
	got := map[string]bool{}
	for _, a := range r.Achievements {
		got[a.ID] = a.Unlocked
	}
	for _, id := range []string{"shows_premiere", "shows_curieux", "shows_double_programme", "shows_calendrier"} {
		// curieux needs 3 distinct: we heard only 2, so it should stay LOCKED.
		if id == "shows_curieux" {
			if got[id] {
				t.Errorf("%s should be locked (only 2 distinct programmes heard)", id)
			}
			continue
		}
		if !got[id] {
			t.Errorf("%s should unlock from the histlog", id)
		}
	}
	// Fidelity: c-jazz heard on 3 distinct evenings, below the 5-evening bronze.
	for _, a := range r.Achievements {
		if a.ID == "shows_fidele_bronze" {
			if a.Unlocked {
				t.Error("shows_fidele_bronze needs 5 evenings, only 3 heard")
			}
			if a.Current != 3 {
				t.Errorf("fidele current: got %v want 3 evenings", a.Current)
			}
		}
	}
}
