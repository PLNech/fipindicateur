//go:build darwin && swiftui

package ui

// The SwiftUI popover's wiring (make swiftui): a left click on the status
// item opens it, the menu moves to the right click. The popover is a second
// SURFACE, not a second UI: it renders snapshots of the App's state and its
// controls come back as action ids into App.fire, the same registry the menu
// items delegate to (actions.go), so nothing it does can escape telemetry.

import (
	"unsafe"

	"github.com/PLNech/fipindicateur/internal/popover"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/tray"
)

type popoverAction struct{ id, arg string }

// Package-level like volSlider: the App is a singleton. Both stay nil until
// wirePopover runs, and a send on a nil channel under select/default is a
// no-op, so syncPopover is safe to call from anywhere at any time.
var (
	popoverActions chan popoverAction
	popoverDirty   chan struct{}
)

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

// popoverSnapshot reads the App's state into the popover's contract. The
// verdict and the VU level are not carried yet: the App keeps no latched
// taste verdict, and the level needs its own cadence.
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
