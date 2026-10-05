package ui

// The zenity slider dialogs and the quick-pick presets they complement.
// Platform-neutral on purpose: zenity is a runtime check (LookPath), so the
// same code drives the fallback presets on macOS/Windows and the real sliders
// wherever the binary exists, Linux included (the tray menu is the UI there
// too, see ui_sni_linux.go).

import (
	"bufio"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/PLNech/fipindicateur/internal/events"
)

// volumePresets are the quick-pick volume levels in the tray menu.
var volumePresets = []int{10, 25, 50, 75, 100}

// crossfadePresets are the quick-pick crossfade durations (seconds) for the
// fallback submenu shown when zenity is unavailable. 0 = hard cut.
var crossfadePresets = []int{0, 2, 4, 6}

// zenityAvailable reports whether the zenity binary is on PATH. macOS,
// Windows and minimal installs usually lack it, so the slider items are only
// added when it exists (no dead menu entry).
func zenityAvailable() bool {
	_, err := exec.LookPath("zenity")
	return err == nil
}

// acquireDialog claims the single zenity dialog slot, returning true on success.
// A false result means a dialog is already open and the caller must do nothing.
// Pair every true with a releaseDialog (deferred).
func (a *App) acquireDialog() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dialogOpen {
		return false
	}
	a.dialogOpen = true
	return true
}

func (a *App) releaseDialog() {
	a.mu.Lock()
	a.dialogOpen = false
	a.mu.Unlock()
}

// openVolumeSlider launches the zenity volume slider off the tray goroutine.
// A second click while one is open is ignored (single dialog at a time). The
// dialog opens on the ACTIVE sink's level: the device's own while casting (so
// the handle does not jump), the local one otherwise.
func (a *App) openVolumeSlider() {
	if !a.acquireDialog() {
		log.Printf("ui: a zenity dialog is already open, ignoring volume slider")
		return
	}
	start, known := a.activeVolume()
	if !known {
		// Casting to a device that has not reported its level yet: the dialog
		// still opens, at mid-scale, because whatever the user releases on IS
		// a user-chosen level. Only invented ones are forbidden.
		start = 50
	}
	go func() {
		defer a.releaseDialog()
		a.runVolumeSlider(start)
	}()
}

// runVolumeSlider runs `zenity --scale --print-partial` and applies each live
// drag value to the player immediately, WITHOUT recording an event or saving
// config (no telemetry/disk spam while dragging). On OK (exit 0) it commits:
// one KindVolume event at the final value plus one config save. On Cancel/Esc/
// close (non-zero exit) it reverts to the volume that was current when the
// dialog opened (apply + UI sync, no event, no save). External pavucontrol
// changes during the drag flow through onExternalVolume; last writer wins.
//
// While casting, the drag ticks are NOT applied live: --print-partial emits one
// line per pixel of travel, and each tick would be a request on the wire to the
// receiver. The device gets exactly one level, on OK, through setCastVolume
// (which records it); Cancel then has nothing to revert, since nothing was ever
// sent. The local sink is left alone: it is paused while casting, so applying
// there would move a volume nobody can hear.
func (a *App) runVolumeSlider(start int) {
	cmd := exec.Command("zenity", "--scale",
		"--title", "FIP · Volume",
		"--text", "Volume de lecture",
		"--min-value", "0",
		"--max-value", "100",
		"--step", "1",
		"--value", strconv.Itoa(start),
		"--print-partial",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("ui: volume slider: stdout pipe: %v", err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("ui: volume slider: start: %v", err)
		return
	}

	last := start
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		v, perr := strconv.Atoi(strings.TrimSpace(sc.Text()))
		if perr != nil {
			continue
		}
		v = clampPct(v)
		last = v
		if a.castSession() == nil {
			a.applyVolumeLive(v) // player + menu + MPRIS, no event, no save
		}
	}

	ok := cmd.Wait() == nil
	if sess := a.castSession(); sess != nil {
		// Casting: one level on OK, nothing at all on Cancel.
		if ok {
			a.setCastVolume(sess, clampPct(last)) // records at source
		}
		return
	}
	if ok {
		// OK: state is already applied live; record exactly one event and persist.
		a.cfg.Volume = clampPct(last)
		a.rec.Record(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: a.cfg.Volume})
		a.save()
		a.applyVolumeUI()
	} else {
		// Cancel/Esc/close: revert to the pre-dialog level, silently.
		a.applyVolumeLive(start)
	}
}

// applyVolumeLive applies a volume to the player, MPRIS and the menu WITHOUT
// recording an event or saving config. It sets a.cfg.Volume BEFORE SetVolume so
// the ao-volume observer echo is swallowed by onExternalVolume's equal-value
// guard (the same trick setVolume uses). Used for slider drag ticks and revert.
func (a *App) applyVolumeLive(pct int) {
	pct = clampPct(pct)
	a.cfg.Volume = pct
	a.player.SetVolume(float64(pct))
	if a.mpris != nil {
		a.mpris.SetVolume(float64(pct) / 100)
	}
	a.applyVolumeUI()
}

// openCrossfadeSlider launches the zenity crossfade-duration slider off the tray
// goroutine, sharing the single-dialog guard with the volume slider.
func (a *App) openCrossfadeSlider() {
	if !a.acquireDialog() {
		log.Printf("ui: a zenity dialog is already open, ignoring crossfade slider")
		return
	}
	start := a.cfg.CrossfadeSecs
	go func() {
		defer a.releaseDialog()
		a.runCrossfadeSlider(start)
	}()
}

// runCrossfadeSlider runs `zenity --scale` over 0..10s. On OK (exit 0) it commits
// the chosen value via setCrossfade (which records the single KindCrossfade
// event). On Cancel/Esc/close (non-zero exit) it does nothing. No live-apply:
// crossfade only takes effect at the next zap, so a per-tick apply would be
// pointless here.
func (a *App) runCrossfadeSlider(start int) {
	out, err := exec.Command("zenity", "--scale",
		"--title", "FIP · Fondu enchaîné",
		"--text", "Durée du fondu entre stations (secondes, 0 = coupure sèche)",
		"--min-value", "0",
		"--max-value", "10",
		"--step", "1",
		"--value", strconv.Itoa(start),
	).Output()
	if err != nil {
		return // non-zero exit = Cancel/Esc/close: do nothing
	}
	v, perr := strconv.Atoi(strings.TrimSpace(string(out)))
	if perr != nil {
		log.Printf("ui: crossfade slider: unparsable value %q: %v", strings.TrimSpace(string(out)), perr)
		return
	}
	a.setCrossfade(v)
}
