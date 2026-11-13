//go:build darwin && swiftui

package ui

// The SwiftUI popover's wiring (make swiftui): a left click on the status
// item opens it, the menu moves to the right click. The popover is a second
// SURFACE, not a second UI: it renders snapshots of the App's state and its
// controls come back as action ids into App.fire, the same registry the menu
// items delegate to (actions.go), so nothing it does can escape telemetry.

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"github.com/PLNech/fipindicateur/internal/popover"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/tray"
	"github.com/PLNech/fipindicateur/internal/version"
	"github.com/PLNech/fipindicateur/internal/vu"
)

type popoverAction struct{ id, arg string }

// Package-level like volSlider: the App is a singleton. Both stay nil until
// wirePopover runs, and a send on a nil channel under select/default is a
// no-op, so syncPopover is safe to call from anywhere at any time.
var (
	popoverActions chan popoverAction
	popoverDirty   chan struct{}
)

// popoverCover remembers the one cover the popover shows: the URL it was
// asked for and, once fetched, the cached PNG's path. The fetch is the
// banner's (notify.FetchCover, cached by URL), so a track that already
// notified costs a disk stat.
var popoverCover struct {
	sync.Mutex
	url, path string
}

// coverFor returns the cached path for url, or "" while it is unknown, in
// which case one fetch is started and the popover is marked stale when it
// lands. A URL asked twice is fetched once; a failed fetch is not retried
// until the track changes (the blank sleeve stays).
func (a *App) coverFor(url string) string {
	popoverCover.Lock()
	defer popoverCover.Unlock()
	if url == popoverCover.url {
		return popoverCover.path
	}
	popoverCover.url, popoverCover.path = url, ""
	if url == "" || a.notif == nil {
		return ""
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		path := a.notif.FetchCover(ctx, url)
		popoverCover.Lock()
		current := popoverCover.url == url // a zap may have moved on
		if current {
			popoverCover.path = path
		}
		popoverCover.Unlock()
		if current && path != "" {
			a.syncPopover()
		}
	}()
	return ""
}

// wirePopover connects the three directions: click to toggle, actions in,
// snapshots out. Called once from buildUI, after registerActions.
func (a *App) wirePopover() {
	popoverActions = make(chan popoverAction, 64)
	popoverDirty = make(chan struct{}, 1)

	// Actions arrive on the AppKit main thread, inside a control's callback:
	// the handler only queues. One worker keeps them in order; a full queue
	// drops (the same rule as the tray's click channels) rather than stall
	// the UI thread.
	popover.SetHandler(func(id, arg string) {
		select {
		case popoverActions <- popoverAction{id, arg}:
		default:
		}
	})
	go func() {
		for act := range popoverActions {
			a.fire(act.id, act.arg)
			a.syncPopover()
		}
	}()

	// One pusher: every state change only raises the dirty flag, so a burst
	// (a zap touches the station, the playback state and the track) costs one
	// snapshot, and no caller ever waits on Swift while holding a lock.
	go func() {
		for range popoverDirty {
			popover.Push(a.popoverSnapshot())
		}
	}()

	tray.SetPrimaryClick(func(anchor unsafe.Pointer) {
		a.syncPopover() // the fader must show the active sink's level now
		popover.Toggle(anchor)
		if a.player.IsPlaying() {
			a.anim.startForCard()
		}
	})
	a.syncPopover()
}

// syncPopover marks the popover's state stale. Non-blocking and lock-free:
// call it from every chokepoint that changes what the popover shows.
func (a *App) syncPopover() {
	select {
	case popoverDirty <- struct{}{}:
	default: // already dirty, or not wired yet
	}
}

// popoverSnapshot reads the App's state into the popover's contract.
func (a *App) popoverSnapshot() popover.Snapshot {
	cur := a.currentStation()
	a.mu.Lock()
	np := a.now
	a.mu.Unlock()

	s := popover.Snapshot{
		Playing: a.audible(),
		Station: cur.Key,
		Title:   np.Title,
		Artist:  np.Artist,
		Album:   np.Album,
		Label:   np.Label,
		Year:    np.Year,
		Cover:   a.coverFor(np.CoverURL),
		Verdict: a.tasteFor(np),
	}
	for _, st := range stations.Wheel() {
		s.Stations = append(s.Stations, popover.Station{Key: st.Key, Name: st.Display, Color: st.Color})
	}
	if np.Show != nil {
		s.Show = np.Show.Title
	}
	if !np.Start.IsZero() && !np.End.IsZero() {
		s.Start, s.End = float64(np.Start.Unix()), float64(np.End.Unix())
	}
	cfg := a.cfgSnapshot()
	s.Settings = popover.Settings{
		HiFi: cfg.HiFi, Crossfade: cfg.CrossfadeSecs, Amp: cfg.Amp,
		Notifications: cfg.Notifications, ShowNotifications: cfg.ShowNotifications,
		ShowCalendar: cfg.ShowCalendar, AnimatedIcon: cfg.AnimatedIcon,
		PlayOnStart: cfg.PlayOnStart, History: cfg.History, UpdateStartup: cfg.UpdateStartup,
		AudioDevice: cfg.AudioDevice, Version: version.String(),
	}
	if s.Settings.AudioDevice == "" {
		s.Settings.AudioDevice = "auto" // empty config means mpv's default
	}
	for _, dev := range menuAudioChoices(a.audioDevs, s.Settings.AudioDevice) {
		s.Settings.AudioDevices = append(s.Settings.AudioDevices, popover.AudioDevice{Name: dev.Name, Label: dev.Label})
	}
	// An unreported cast level stays at 0 with the fader drawn empty: never
	// an invented number (the same rule as the menu's slider).
	s.Volume, _ = a.activeVolume()
	if sess := a.castSession(); sess != nil {
		if v, ok := sess.ReceiverVolume(); ok {
			s.Muted = v.Muted
		}
	} else {
		s.Muted = a.cfgSnapshot().Mute
	}
	return s
}

// popoverShown reports whether the card is up. While it is, the banners that
// would only repeat it (new track, new show, zap, verdict) are skipped: they
// land on the very corner the card occupies.
func (a *App) popoverShown() bool { return popover.Shown() }

// popoverBars hands the card the frame the tray glyph is drawing: the same
// four heights, scaled to 0..1.
func popoverBars(h vu.Heights) {
	var b [vu.Bars]float64
	for i, v := range h {
		b[i] = float64(v) / vu.MaxHeight
	}
	popover.SetBars(b)
}

// hasPopover: the menu shrinks to a short list, the settings move to the
// back of the card.
const hasPopover = true

// openSettings turns the card to its Réglages face, showing it if needed.
func (a *App) openSettings() {
	a.syncPopover()
	popover.ShowSettings()
	if a.player.IsPlaying() {
		a.anim.startForCard()
	}
}
