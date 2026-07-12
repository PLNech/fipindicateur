package stats

import (
	"sort"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/stations"
)

// showFacts are the scalars the émission (programme) achievements are graded
// on. Every one is derived strictly from logged data: the histlog show tags
// (via the exposed tracks, so only time actually heard counts) and the events
// log (show_change boundaries, the calendar toggle). Nothing here needs a datum
// the app does not store, so a badge can never depend on a phantom signal.
type showFacts struct {
	distinctConcepts   int   // distinct programmes heard (by conceptUuid)
	maxEvenings        int   // most distinct evenings spent on a single programme
	maxConceptStreak   int   // longest consecutive-day run on a single programme
	inShowSec          int64 // total listening time inside programmes
	totalSec           int64 // total observed listening (denominator for share)
	inShowTracks       int   // tracks heard within programmes
	maxEveningInShow   int64 // most in-programme listening within one calendar day
	twoConceptsEvening bool  // >= 2 distinct programmes heard on the same day
	sundayEvening      bool  // a programme heard on a Sunday evening (18h-24h)
	nightInShowSec     int64 // in-programme listening in the deep night (0h-5h)
	showChanges        int   // programme boundaries witnessed while listening
	calendarDiscovered bool  // the calendar setting was toggled at least once
}

// showAchievementFacts derives the programme scalars from the merged
// per-(concept, day) listening derivation (see deriveShowListening: histlog
// show tags and show_change boundary spans, each joined with playback) and the
// behaviour log. Only time actually heard contributes, so the grades reflect
// listening, not mere presence in the schedule. totalSec is all observed
// listening (the segment total), the denominator for the share badges.
func showAchievementFacts(sl showListening, evs []events.Event, totalSec int64) showFacts {
	var sf showFacts
	sf.totalSec = totalSec
	sf.sundayEvening = sl.sundayEvening
	sf.nightInShowSec = sl.nightSec

	eveningConcepts := map[string]map[string]bool{}
	eveningInShow := map[string]int64{}
	for concept, days := range sl.secs {
		dates := map[string]bool{}
		for day, secs := range days {
			if secs <= 0 {
				continue
			}
			sf.inShowSec += secs
			dates[day] = true
			if eveningConcepts[day] == nil {
				eveningConcepts[day] = map[string]bool{}
			}
			eveningConcepts[day][concept] = true
			eveningInShow[day] += secs
		}
		if len(dates) == 0 {
			continue
		}
		sf.distinctConcepts++
		if len(dates) > sf.maxEvenings {
			sf.maxEvenings = len(dates)
		}
		if s := longestStreak(dates); s > sf.maxConceptStreak {
			sf.maxConceptStreak = s
		}
	}
	for _, n := range sl.trackCounts {
		sf.inShowTracks += n
	}
	for _, concepts := range eveningConcepts {
		if len(concepts) >= 2 {
			sf.twoConceptsEvening = true
		}
	}
	for _, sec := range eveningInShow {
		if sec > sf.maxEveningInShow {
			sf.maxEveningInShow = sec
		}
	}

	for _, e := range evs {
		switch e.Kind {
		case events.KindShowChange:
			sf.showChanges++
		case events.KindShowCalendar:
			sf.calendarDiscovered = true
		}
	}
	return sf
}

// Achievement is one unlockable badge. Locked badges still carry Current/Target
// so the report can show progress (calibrated: you see how close you are).
type Achievement struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Desc     string  `json:"desc"`
	Emoji    string  `json:"emoji"`
	Unlocked bool    `json:"unlocked"`
	Current  float64 `json:"current"`
	Target   float64 `json:"target"`
}

// evaluateAchievements applies the badge rules to the derived session data.
// Rules are intentionally simple and deterministic so they unit-test cleanly.
func evaluateAchievements(
	ss []session,
	perStation map[string]time.Duration,
	activeDays map[string]bool,
	hourly [24]int64,
	zaps int,
	total time.Duration,
	sf showFacts,
) []Achievement {
	// Derived scalars.
	stationsHeard := len(perStation)
	maxSession := time.Duration(0)
	maxZapsInSession := 0
	for _, s := range ss {
		if s.dur > maxSession {
			maxSession = s.dur
		}
		if s.zaps > maxZapsInSession {
			maxZapsInSession = s.zaps
		}
	}
	streak := longestStreak(activeDays)
	nightSec := hourly[0] + hourly[1] + hourly[2] + hourly[3] + hourly[4]
	earlySec := hourly[5] + hourly[6]
	topShare := 0.0
	if total > 0 {
		for _, d := range perStation {
			if s := d.Seconds() / total.Seconds(); s > topShare {
				topShare = s
			}
		}
	}
	explorerBest := maxStationsInADay(ss)
	totalHours := total.Hours()

	badge := func(id, name, desc, emoji string, current, target float64) Achievement {
		return Achievement{
			ID: id, Name: name, Desc: desc, Emoji: emoji,
			Unlocked: current >= target && target > 0,
			Current:  current, Target: target,
		}
	}

	// In-show share of total listening, 0..100 (0 when nothing was heard).
	inShowShare := 0.0
	if sf.totalSec > 0 {
		inShowShare = float64(sf.inShowSec) / float64(sf.totalSec) * 100
	}
	inShowHours := float64(sf.inShowSec) / 3600.0
	maxEveningHours := float64(sf.maxEveningInShow) / 3600.0

	out := []Achievement{
		badge("night_owl", "Oiseau de nuit", "Ecouter entre 1h et 5h du matin", "\U0001F989",
			float64(nightSec), 60), // >= 1 min after 1am
		badge("early_bird", "Leve-tot", "Ecouter avant 7h du matin", "\U0001F305",
			float64(earlySec), 60),
		badge("globe", "Tour du monde", "Ecouter les 13 webradios FIP", "\U0001F30D",
			float64(stationsHeard), float64(len(stations.All))),
		badge("explorer", "Explorateur", "Ecouter 5 radios differentes en une journee", "\U0001F9ED",
			float64(explorerBest), 5),
		badge("zapper", "Zappeur", "Changer de radio 10 fois dans une session", "⚡",
			float64(maxZapsInSession), 10),
		badge("marathon", "Marathon", "Une session d'ecoute de 2h ou plus", "\U0001F3C3",
			maxSession.Hours(), 2),
		badge("faithful", "Fidele", "Ecouter 7 jours consecutifs", "\U0001F4C5",
			float64(streak), 7),
		badge("purist", "Puriste", "Passer 90% du temps sur une seule radio", "\U0001F3AF",
			topShare*100, 90),
		badge("melomane", "Melomane", "Cumuler 10h d'ecoute", "\U0001F3B6",
			totalHours, 10),
	}

	// Emission (programme) achievements. All graded on showFacts, i.e. strictly
	// on logged data: histlog show tags and the events boundaries. Tiered
	// families (bronze/argent/or) each count as one insignia.
	out = append(out,
		// The very first programme heard.
		badge("shows_premiere", "Premiere emission", "Entendre ta premiere emission FIP", "",
			float64(sf.distinctConcepts), 1),

		// Breadth: distinct recurring programmes heard.
		badge("shows_curieux", "Curieux", "Entendre 3 emissions differentes", "",
			float64(sf.distinctConcepts), 3),
		badge("shows_habitue", "Habitue", "Entendre 7 emissions differentes", "",
			float64(sf.distinctConcepts), 7),
		badge("shows_connaisseur", "Connaisseur", "Entendre 15 emissions differentes", "",
			float64(sf.distinctConcepts), 15),

		// Fidelity: distinct evenings on a single recurring programme.
		badge("shows_fidele_bronze", "Fidele (bronze)", "Suivre une meme emission 5 soirees", "",
			float64(sf.maxEvenings), 5),
		badge("shows_fidele_argent", "Fidele (argent)", "Suivre une meme emission 20 soirees", "",
			float64(sf.maxEvenings), 20),
		badge("shows_fidele_or", "Fidele (or)", "Suivre une meme emission 50 soirees", "",
			float64(sf.maxEvenings), 50),

		// Consecutive-evening streak on one programme (a standing rendez-vous).
		badge("shows_assidu", "Assidu", "3 jours d'affilee au meme rendez-vous", "",
			float64(sf.maxConceptStreak), 3),
		badge("shows_rituel", "Rituel", "7 jours d'affilee au meme rendez-vous", "",
			float64(sf.maxConceptStreak), 7),

		// Cumulative time spent inside programmes.
		badge("shows_temps_bronze", "Auditeur (bronze)", "Cumuler 5h d'ecoute en emission", "",
			inShowHours, 5),
		badge("shows_temps_argent", "Auditeur (argent)", "Cumuler 20h d'ecoute en emission", "",
			inShowHours, 20),
		badge("shows_temps_or", "Auditeur (or)", "Cumuler 50h d'ecoute en emission", "",
			inShowHours, 50),

		// Tracks heard within programmes.
		badge("shows_titres_bronze", "Oreille attentive (bronze)", "Entendre 50 titres en emission", "",
			float64(sf.inShowTracks), 50),
		badge("shows_titres_argent", "Oreille attentive (argent)", "Entendre 200 titres en emission", "",
			float64(sf.inShowTracks), 200),
		badge("shows_titres_or", "Oreille attentive (or)", "Entendre 500 titres en emission", "",
			float64(sf.inShowTracks), 500),

		// Share of listening spent in programmes rather than the rotation.
		badge("shows_part_quart", "Un quart en emission", "Passer 25% de ton ecoute en emission", "",
			inShowShare, 25),
		badge("shows_part_moitie", "Moitie en emission", "Passer 50% de ton ecoute en emission", "",
			inShowShare, 50),
		badge("shows_part_gros", "Surtout en emission", "Passer 75% de ton ecoute en emission", "",
			inShowShare, 75),

		// Programme boundaries witnessed while listening (show_change events).
		badge("shows_traversee_bronze", "Traversee (bronze)", "Traverser 10 changements d'emission", "",
			float64(sf.showChanges), 10),
		badge("shows_traversee_or", "Traversee (or)", "Traverser 50 changements d'emission", "",
			float64(sf.showChanges), 50),

		// In-programme listening packed into a single evening.
		badge("shows_marathon", "Marathon d'emissions", "2h d'emissions dans une meme soiree", "",
			maxEveningHours, 2),
		badge("shows_veillee", "Veillee", "4h d'emissions dans une meme soiree", "",
			maxEveningHours, 4),

		// Two different programmes in one evening.
		badge("shows_double_programme", "Double programme", "Entendre 2 emissions dans une soiree", "",
			b2f(sf.twoConceptsEvening), 1),

		// The Sunday-evening listener.
		badge("shows_dimanche", "L'auditeur du dimanche soir", "Ecouter une emission un dimanche soir", "",
			b2f(sf.sundayEvening), 1),

		// A programme heard in the deep night.
		badge("shows_nocturne", "Emission nocturne", "30 min d'emission entre minuit et 5h", "",
			float64(sf.nightInShowSec), 1800),

		// Discovering the calendar feature (a stats-derived, events-based badge).
		badge("shows_calendrier", "Lecteur du calendrier", "Ouvrir le calendrier des emissions", "",
			b2f(sf.calendarDiscovered), 1),
	)
	return out
}

// b2f maps a boolean condition to the 0/1 progress a one-shot badge grades on.
func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// longestStreak returns the length of the longest run of consecutive calendar
// days present in the active-day set.
func longestStreak(days map[string]bool) int {
	if len(days) == 0 {
		return 0
	}
	list := make([]time.Time, 0, len(days))
	for d := range days {
		t, err := time.Parse("2006-01-02", d)
		if err == nil {
			list = append(list, t)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Before(list[j]) })
	best, run := 1, 1
	for i := 1; i < len(list); i++ {
		if list[i].Sub(list[i-1]) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
	}
	return best
}

// maxStationsInADay returns the largest number of distinct stations heard
// within a single calendar day (session start day as the bucket key).
func maxStationsInADay(ss []session) int {
	byDay := map[string]map[string]bool{}
	for _, s := range ss {
		day := s.start.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = map[string]bool{}
		}
		for st := range s.stations {
			byDay[day][st] = true
		}
	}
	best := 0
	for _, set := range byDay {
		if len(set) > best {
			best = len(set)
		}
	}
	return best
}
