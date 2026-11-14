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
//  2. frames go straight to the tray's ARGB32 pixmap (icon.TrayFrame: memoized
//     bar shape, one colour pass, no PNG encode/decode, no second retint);
//  3. skip the draw entirely when the quantized state is unchanged, and the
//     SNI item drops a pixmap identical to the one the host holds (the dbus
//     round-trip + shell redraw is the real cost, not our pixels).
const (
	animFPS      = 6
	animInterval = time.Second / animFPS
	// ~5s of consecutive astats failures while audio flows: assume the filter
	// this libmpv and auto-disable for the rest of the run.
	animMaxErrs = 5 * animFPS
	// A station zap crossfades the bar ink over the AUDIO fondu's duration (see
	// App.tintDuration), quantized to tintSteps discrete values so the frame
	// cache stays bounded and at most tintSteps extra SetIcon calls happen per
	// change (when VU heights alone would dedup).
	tintSteps = 16
	// minTintDur floors the ink fade: with the fondu set to 0 (hard cut) the
	// colour still eases over a blink instead of snapping, and it keeps the
	// division in currentTintLocked safe.
	minTintDur = 300 * time.Millisecond
)

type animator struct {
	app *App

	mu        sync.Mutex
	stopCh    chan struct{}
	repaintCh chan struct{} // non-nil while running: a receive lifts the frame dedupe
	broken    bool          // astats unavailable; animation off for this run

	// Tint crossfade state, guarded by mu. tintSet stays false until the first
	// target is set, so the icon draws with theme ink (zero tint) until then.
	tintFrom  color.NRGBA
	tintTo    color.NRGBA
	tintStart time.Time
	tintDur   time.Duration // this transition's length, given by the caller
	tintSet   bool
}

// dur is the current transition's length, floored (caller holds mu).
func (an *animator) durLocked() time.Duration {
	if an.tintDur < minTintDur {
		return minTintDur
	}
	return an.tintDur
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

// setTintTarget starts a crossfade toward c over dur. A zap mid-transition
// starts from the currently displayed (interpolated) tint, not the old endpoint,
// so the motion never snaps. The first-ever target eases in from the neutral
// theme ink, turning app start into a fade rather than a jump.
//
// dur comes from the caller (App.tintDuration: the configured audio fondu), so
// the ink transition IS the audible transition, seen instead of heard.
func (an *animator) setTintTarget(c color.NRGBA, dur time.Duration) {
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
	an.tintDur = dur
	an.tintSet = true
}

// currentTintLocked is the un-quantized eased tint at now. Used only to seed a
// new crossfade's starting point; the displayed frame uses the quantized value.
func (an *animator) currentTintLocked(now time.Time) color.NRGBA {
	t := float64(now.Sub(an.tintStart)) / float64(an.durLocked())
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
	t := float64(now.Sub(an.tintStart)) / float64(an.durLocked())
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
// (the repaint read as a flash of the neutral glyph on every zap). The config
// is read before an.mu: the animator never holds an.mu while taking the
// config lock, so the two locks cannot order-invert against each other.
func (an *animator) willRun() bool {
	enabled := an.app.animatedIconEnabled()
	an.mu.Lock()
	defer an.mu.Unlock()
	return !an.broken && enabled
}

// unavailable reports whether the animation was auto-disabled for this run
// (astats proved unavailable while audio flowed). The animated-icon toggle
// uses it to stay honest: a checkbox that checks itself over a dead
// animation is a lie.
func (an *animator) unavailable() bool {
	an.mu.Lock()
	defer an.mu.Unlock()
	return an.broken
}

// start launches the animation loop if enabled, not already running, and not
// auto-disabled. Idempotent. The config read stays outside an.mu (see
// willRun).
func (an *animator) start() {
	enabled := an.app.animatedIconEnabled()
	an.launch(enabled || an.app.popoverShown())
}

// startForCard launches the loop for the card's meter even with the tray
// animation off: the card shows the same four bars, from the same frames.
// The loop retires by itself once neither reader wants them.
func (an *animator) startForCard() { an.launch(true) }

func (an *animator) launch(wanted bool) {
	an.mu.Lock()
	defer an.mu.Unlock()
	if an.stopCh != nil || an.broken || !wanted {
		return
	}
	an.stopCh = make(chan struct{})
	an.repaintCh = make(chan struct{}, 1)
	go an.loop(an.stopCh, an.repaintCh)
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
	an.repaintCh = nil
}

// repaint forces the next frame to draw even when the (heights, tint) tuple
// is unchanged: the pixels under the frame identity changed (the panel
// flipped, a cover just left the glyph), so the tuple alone can no longer
// tell whether the tray is current. Draws within one tick; a no-op while the
// loop is not running.
func (an *animator) repaint() {
	an.mu.Lock()
	ch := an.repaintCh
	an.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

// frameKey is the frame identity: the (heights, tint) tuple. A tint step
// advance redraws, an unchanged tuple never does. Both members are comparable.
type frameKey struct {
	h    vu.Heights
	tint color.NRGBA
}

func (an *animator) loop(stop, repaint <-chan struct{}) {
	ticker := time.NewTicker(animInterval)
	defer ticker.Stop()

	var (
		prev  vu.Heights
		last  = frameKey{h: vu.Heights{255}} // sentinel: first real frame draws
		frame int
		errs  int
		idle  int        // ticks with no reader at all (icon off, card down)
		sent  vu.Heights // the frame the card holds
	)
	sent[0] = 255 // sentinel: the first frame reaches the card

	for {
		select {
		case <-stop:
			return
		case <-repaint:
			// The pixels under the frame identity changed (panel flip, cover
			// cleared): lift the dedupe so the draw below really paints, and
			// do it now rather than one tick late.
			last = frameKey{h: vu.Heights{255}}
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
				// Retire the loop the same way stop() does (close then nil,
				// under the lock): a bare nil left a start() racing the exit
				// seeing a running loop that was already gone, and a request
				// silently swallowed.
				an.mu.Lock()
				an.broken = true
				close(an.stopCh)
				an.stopCh = nil
				an.repaintCh = nil
				an.mu.Unlock()
				log.Printf("ui: astats levels unavailable (%s), animated icon disabled for this run",
					an.app.player.AudioLevelDiag())
				an.app.applyIconState(false)
				return
			}
		}

		var target vu.Heights
		if bands, bok := an.app.player.SpectrumBands(); bok {
			// The shape says which bars; the overall level says how tall.
			// A quiet passage stays short, a shout fills the icon.
			for i := range bands {
				bands[i] *= level
			}
			target = vu.TargetsFromBands(bands)
		} else {
			target = vu.Targets(level, frame)
		}
		frame++
		h := vu.Envelope(prev, target)
		prev = h

		// Two readers of the one frame: the card's meter (when it is up) and
		// the tray glyph (when the animation is on). Both get the same
		// heights on the same tick, each only when its own copy is stale.
		card := an.app.popoverShown()
		if card && h != sent {
			popoverBars(h)
			sent = h
		} else if !card {
			sent[0] = 255
		}
		if !an.app.animatedIconEnabled() {
			// The loop only runs for the card here. Once the card is gone
			// for a couple of seconds (it opens asynchronously, hence the
			// grace), retire as stop() does.
			last = frameKey{h: vu.Heights{255}}
			if card {
				idle = 0
			} else if idle++; idle > 2*animFPS {
				an.mu.Lock()
				if an.stopCh != nil {
					close(an.stopCh)
					an.stopCh = nil
					an.repaintCh = nil
				}
				an.mu.Unlock()
				return
			}
			continue
		}
		idle = 0

		fk := frameKey{h: h, tint: an.quantizedTint(time.Now())}
		if fk != last {
			an.app.drawFrame(fk)
			last = fk
		}
	}
}
