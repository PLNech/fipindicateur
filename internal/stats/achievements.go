package stats

import (
	"sort"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

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

	return []Achievement{
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
