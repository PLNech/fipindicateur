package ui

// The action registry: the surface-neutral half of the "measurable by design"
// chokepoint. A tray menu item reaches its handler through App.on; any other
// surface (the macOS popover, tomorrow a Linux popup) reaches the same handler
// through App.fire(id, arg). Both go through recorded(), so an action is
// defined once, with its telemetry decision, and every surface inherits it.
//
// The ids are a contract with the popover: its Swift Action enum carries the
// same strings, and TestPopoverActionsRegistered fails the build when the
// Swift side names an id this file does not register.

import (
	"strconv"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/stations"
)

const (
	actPlayToggle  = "play_toggle"
	actLike        = "like"
	actDislike     = "dislike"
	actMuteToggle  = "mute_toggle"
	actVolume      = "volume"       // arg: the level, 0..100
	actStation     = "station"      // arg: a station key
	actStationNext = "station_next" // wheel order, like the media keys
	actStationPrev = "station_prev"
	actOpenWiki    = "open_wiki"
	actStatsView   = "stats_view"
)

// recorded wraps fn with the telemetry decision: a fixed kind is recorded
// before the handler runs; "" means the handler records at its own source
// (state-dependent actions: play/pause, volume, mute, station change).
func (a *App) recorded(kind events.Kind, fn func()) func() {
	return func() {
		if kind != "" {
			a.rec.Record(events.Event{Kind: kind, Station: a.currentStation().Key})
		}
		fn()
	}
}

// act registers a named action. Same contract as on: kind "" means recorded
// at source.
func (a *App) act(id string, kind events.Kind, fn func(arg string)) {
	if a.actions == nil {
		a.actions = map[string]func(string){}
	}
	a.actions[id] = func(arg string) {
		a.recorded(kind, func() { fn(arg) })()
	}
}

// fire runs a registered action; false means the id is unknown (nothing ran,
// nothing was recorded).
func (a *App) fire(id, arg string) bool {
	fn, ok := a.actions[id]
	if !ok {
		return false
	}
	fn(arg)
	return true
}

// firing adapts a registered action to a menu handler, so a menu item and the
// popover share one definition: a.on(item, "", a.firing(actLike)). The on
// kind stays "" because fire records.
func (a *App) firing(id string) func() {
	return func() { a.fire(id, "") }
}

// registerActions declares every action a non-menu surface may fire. It only
// builds closures, so it is safe on a bare App (the guard test does that).
func (a *App) registerActions() {
	a.act(actPlayToggle, "", func(string) { a.togglePlay() }) // setPlayingUI records
	a.act(actLike, events.KindLike, func(string) { a.recordTaste(prefs.Like) })
	a.act(actDislike, events.KindDislike, func(string) { a.recordTaste(prefs.Dislike) })
	a.act(actMuteToggle, "", func(string) { a.toggleActiveMute() }) // records at source
	a.act(actVolume, "", func(arg string) {
		// A malformed level is dropped, never guessed: on a cast receiver an
		// invented level is the amplifier's master volume.
		if pct, err := strconv.Atoi(arg); err == nil && pct >= 0 && pct <= 100 {
			a.setActiveVolume(pct) // records the level at source
		}
	})
	a.act(actStation, "", func(arg string) {
		// stations.ByKey falls back to FIP on an unknown key; an unknown key
		// from a surface must not zap anywhere.
		if stations.ByKey(arg).Key == arg {
			a.setStation(arg) // startStation records the transition
		}
	})
	a.act(actStationNext, "", func(string) { a.zapStation(+1) })
	a.act(actStationPrev, "", func(string) { a.zapStation(-1) })
	a.act(actOpenWiki, events.KindOpenWiki, func(string) { a.openNow() })
	a.act(actStatsView, events.KindStatsView, func(string) { a.viewStats() })
}
