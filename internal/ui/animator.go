package ui

import (
	"image/color"
	"log"
	"math"
	"sync"
	"time"

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
	// A station zap crossfades the bar ink over tintDur, quantized to tintSteps
	// discrete values so the frame cache stays bounded and at most tintSteps
	// extra SetIcon calls happen per change (when VU heights alone would dedup).
	tintDur   = 10 * time.Second
	tintSteps = 16
)

type animator struct {
	app *App

	mu     sync.Mutex
	stopCh chan struct{}
	broken bool // astats unavailable; animation off for this run

	// Tint crossfade state, guarded by mu. tintSet stays false until the first
	// target is set, so the icon draws with theme ink (zero tint) until then.
	tintFrom  color.NRGBA
	tintTo    color.NRGBA
	tintStart time.Time
	tintSet   bool
}

// smoothstep eases 0..1 with a symmetric ease-in-out.
func smoothstep(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

// setTintTarget starts a crossfade toward c. A zap mid-transition starts from
// the currently displayed (interpolated) tint, not the old endpoint, so the
// motion never snaps. The first-ever target eases in from the neutral theme
// ink, turning app start into a fade rather than a jump.
func (an *animator) setTintTarget(c color.NRGBA) {
	an.mu.Lock()
	defer an.mu.Unlock()
	now := time.Now()
	if an.tintSet {
		an.tintFrom = an.currentTintLocked(now)
	} else {
		an.tintFrom = icon.ThemeInk(icon.PanelIsDark())
	}
	an.tintTo = c
	an.tintStart = now
	an.tintSet = true
}

// currentTintLocked is the un-quantized eased tint at now. Used only to seed a
// new crossfade's starting point; the displayed frame uses the quantized value.
func (an *animator) currentTintLocked(now time.Time) color.NRGBA {
	t := float64(now.Sub(an.tintStart)) / float64(tintDur)
	return icon.Lerp(an.tintFrom, an.tintTo, smoothstep(t))
}

// quantizedTint is the tint to draw this frame: zero (theme ink) before any
// target, else the eased crossfade snapped to one of tintSteps discrete values
// so the whole transition yields at most tintSteps distinct tints.
func (an *animator) quantizedTint(now time.Time) color.NRGBA {
	an.mu.Lock()
	defer an.mu.Unlock()
	if !an.tintSet {
		return color.NRGBA{}
	}
	t := float64(now.Sub(an.tintStart)) / float64(tintDur)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	// Snap the progress to tintSteps levels (0 .. tintSteps-1): tintSteps
	// distinct positions, hence at most tintSteps distinct tints start to end.
	step := math.Round(t * float64(tintSteps-1))
	qt := step / float64(tintSteps-1)
	return icon.Lerp(an.tintFrom, an.tintTo, smoothstep(qt))
}

// willRun reports whether start() would actually animate: used by the static
// icon path to avoid repainting a placeholder the very next frame replaces
// (the repaint read as a flash of the neutral glyph on every zap).
func (an *animator) willRun() bool {
	an.mu.Lock()
	defer an.mu.Unlock()
	return !an.broken && an.app.cfg.AnimatedIcon
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

	// The frame identity is the (heights, tint) tuple: a tint step advance
	// redraws, an unchanged tuple never does. Both members are comparable.
	type frameKey struct {
		h    vu.Heights
		tint color.NRGBA
	}
	var (
		prev  vu.Heights
		last  = frameKey{h: vu.Heights{255}} // sentinel: first real frame draws
		frame int
		errs  int
	)

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		// No measurable audio (buffering, stream restart, astats hiccup) is
		// rendered as silence: the bars decay gracefully to their stubs via
		// the envelope instead of freezing mid-pose, and the tint crossfade
		// keeps advancing underneath. Bounded: once decayed, the (heights,
		// tint) dedup stops the redraws.
		level := 0.0
		db, ok := an.app.player.RMSLevelDB()
		if ok {
			errs = 0
			level = vu.LevelFromDB(db)
		} else if an.app.player.CoreIdle() {
			// Core idle is not an astats failure: only count misses while
			// audio actually flows, otherwise a slow stream start would
			// falsely auto-disable the animation.
			errs = 0
		} else {
			errs++
			if errs >= animMaxErrs {
				an.mu.Lock()
				an.broken = true
				an.stopCh = nil
				an.mu.Unlock()
				log.Printf("ui: astats levels unavailable, animated icon disabled for this run")
				an.app.applyIconState(false)
				return
			}
		}

		target := vu.Targets(level, frame)
		frame++
		h := vu.Envelope(prev, target)
		prev = h

		fk := frameKey{h: h, tint: an.quantizedTint(time.Now())}
		if fk != last {
			an.app.setIcon(icon.BarsIcon(fk.h, fk.tint))
			last = fk
		}
	}
}
