package ui

import (
	"log"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/vu"
)

// The animated tray icon: polls real audio levels from mpv's astats filter
// and renders a 4-bar VU glyph. CPU discipline, in order of importance:
//  1. low frame rate (6 fps, one ticker, only while playing);
//  2. quantized frame cache (icon.BarsIcon renders each state once);
//  3. skip SetIcon entirely when the quantized state is unchanged
//     (the dbus round-trip + shell redraw is the real cost, not our pixels).
const (
	animFPS      = 6
	animInterval = time.Second / animFPS
	// ~5s of consecutive astats failures while audio flows: assume the filter
	// this libmpv and auto-disable for the rest of the run.
	animMaxErrs = 5 * animFPS
)

type animator struct {
	app *App

	mu     sync.Mutex
	stopCh chan struct{}
	broken bool // astats unavailable; animation off for this run
}

// start launches the animation loop if enabled, not already running, and not
// auto-disabled. Idempotent.
func (an *animator) start() {
	an.mu.Lock()
	defer an.mu.Unlock()
	if an.stopCh != nil || an.broken || !an.app.cfg.AnimatedIcon {
		return
	}
	an.stopCh = make(chan struct{})
	go an.loop(an.stopCh)
}

// stop halts the animation loop. Idempotent. The caller decides which static
// icon to show afterwards.
func (an *animator) stop() {
	an.mu.Lock()
	defer an.mu.Unlock()
	if an.stopCh == nil {
		return
	}
	close(an.stopCh)
	an.stopCh = nil
}

func (an *animator) loop(stop chan struct{}) {
	ticker := time.NewTicker(animInterval)
	defer ticker.Stop()

	var (
		prev  vu.Heights
		last  = vu.Heights{255} // sentinel: first real frame always draws
		frame int
		errs  int
	)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		db, ok := an.app.player.RMSLevelDB()
		if !ok {
			// While the core is idle (buffering, stream restart) there is no
			// audio to measure: that is not an astats failure. Only count
			// misses while audio actually flows, otherwise a slow stream
			// start would falsely auto-disable the animation.
			if an.app.player.CoreIdle() {
				errs = 0
				continue
			}
			errs++
			if errs >= animMaxErrs {
				an.mu.Lock()
				an.broken = true
				an.stopCh = nil
				an.mu.Unlock()
				log.Printf("ui: astats levels unavailable, animated icon disabled for this run")
				systray.SetIcon(icon.Active(false))
				return
			}
			continue
		}
		errs = 0

		target := vu.Targets(vu.LevelFromDB(db), frame)
		frame++
		h := vu.Envelope(prev, target)
		prev = h

		if h != last {
			systray.SetIcon(icon.BarsIcon(h))
			last = h
		}
	}
}
