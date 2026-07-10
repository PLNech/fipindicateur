package player

import (
	"log"
	"math"
	"sync"
	"time"
)

// Fader is a facade over one or two MPV handles that turns a live station zap
// into a gapless equal-power crossfade instead of a hard cut.
//
// It exposes the same surface ui.go used on a single *MPV (Play, Stop, volume,
// mute, device, level and lifecycle methods, plus the callback fields), so the
// UI is agnostic to how many handles exist underneath. At rest it owns one
// "current" handle. During a zap it briefly owns a second "outgoing" handle:
// the incoming stream is promoted to current immediately (the UI already shows
// the new station), starts silent on mpv's internal `volume`, and is ramped up
// while the outgoing is ramped down, then the outgoing is torn down.
//
// Concurrency: exactly one fade runs at a time, guarded by mu. A new Play/Stop/
// Close arriving mid-fade cancels the running fade (snapping the incoming to
// full volume and closing the outgoing) before proceeding, so rapid A->B->C
// zaps never overlap handles or leak goroutines.
type Fader struct {
	mu sync.Mutex

	current  *MPV   // the audible, promoted handle (never nil after Initialize until Close)
	outgoing *MPV   // during a fade, the handle being faded out; nil otherwise
	incoming *MPV   // during a fade, the handle whose PLAYBACK_RESTART we await
	device   string // last selected audio-device, replayed onto new handles

	active      *fade         // the running fade, nil when none
	fadeRestart chan struct{} // closed once when incoming's playback restarts

	// Crossfade is the fade duration for a live station zap. 0 disables it
	// (hard cut, the old behaviour). Set before Initialize.
	Crossfade time.Duration

	// Facade-level callbacks, mirroring MPV's. Each underlying handle forwards
	// to these only while it is the current handle, so an outgoing handle's
	// late events never reach the UI. Set before Initialize.
	TitleChanged      func(title string)
	VolumeChanged     func(pct float64)
	MuteChanged       func(mute bool)
	PlaybackRestarted func()
}

// fade is one in-flight crossfade. cancel is closed (once) to request early
// termination; done is closed by the fade goroutine when it has fully cleaned
// up (snapped the incoming to full and closed the outgoing).
type fade struct {
	cancel chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (fd *fade) requestCancel() { fd.once.Do(func() { close(fd.cancel) }) }

const (
	// fadeTick is the ramp granularity. 50ms is imperceptible as steps yet
	// cheap: an 80-tick 4s fade is two synchronous property sets per tick.
	fadeTick = 50 * time.Millisecond
	// fadeWaitTimeout bounds how long we wait for the incoming stream's audio
	// to start before giving up and hard-cutting to it.
	fadeWaitTimeout = 10 * time.Second
)

// equalPowerGains returns the incoming and outgoing INTERNAL volume levels
// (0..100) at normalized fade progress t in [0,1], on an equal-power (constant
// energy) curve: in = 100*sin(t*pi/2), out = 100*cos(t*pi/2). Because
// sin^2+cos^2 == 1, in^2 + out^2 == 100^2 for all t, so the summed acoustic
// power (and thus perceived loudness) stays roughly constant across the fade
// instead of dipping in the middle the way a linear crossfade does. At t=0 the
// incoming is silent and the outgoing full; at t=1 the reverse. Pure function,
// unit-tested without libmpv.
func equalPowerGains(t float64) (in, out float64) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	in = 100 * math.Sin(t*math.Pi/2)
	out = 100 * math.Cos(t*math.Pi/2)
	return in, out
}

// newHandle builds an MPV whose callbacks forward to the facade only while it
// is the current handle. Callbacks are wired here, BEFORE Initialize, because
// the event goroutine reads them unlocked. initialVolume is passed straight to
// the handle (nil => 100).
func (f *Fader) newHandle(initialVolume *int) *MPV {
	m := &MPV{initialVolume: initialVolume}
	m.TitleChanged = func(title string) {
		f.mu.Lock()
		fire := m == f.current
		cb := f.TitleChanged
		f.mu.Unlock()
		if fire && cb != nil {
			cb(title)
		}
	}
	m.VolumeChanged = func(pct float64) {
		f.mu.Lock()
		fire := m == f.current
		cb := f.VolumeChanged
		f.mu.Unlock()
		if fire && cb != nil {
			cb(pct)
		}
	}
	m.MuteChanged = func(mute bool) {
		f.mu.Lock()
		fire := m == f.current
		cb := f.MuteChanged
		f.mu.Unlock()
		if fire && cb != nil {
			cb(mute)
		}
	}
	m.PlaybackRestarted = func() { f.onHandlePlaybackRestart(m) }
	return m
}

// onHandlePlaybackRestart handles a PLAYBACK_RESTART from handle m: if a fade
// is waiting on m as its incoming stream, signal the fade goroutine (once);
// and if m is the current handle, forward to the facade callback so the UI
// syncs the stream's volume/mute.
func (f *Fader) onHandlePlaybackRestart(m *MPV) {
	f.mu.Lock()
	if m == f.incoming && f.fadeRestart != nil {
		close(f.fadeRestart)
		f.fadeRestart = nil
	}
	fire := m == f.current
	cb := f.PlaybackRestarted
	f.mu.Unlock()
	if fire && cb != nil {
		cb()
	}
}

// Initialize creates and starts the first (current) handle.
func (f *Fader) Initialize() error {
	m := f.newHandle(nil)
	if err := m.Initialize(); err != nil {
		return err
	}
	f.mu.Lock()
	f.current = m
	f.mu.Unlock()
	return nil
}

// SetCrossfade updates the live-zap crossfade duration at runtime. Guarded by
// mu because Play reads the Crossfade field under the same lock; the constructor
// still sets the field directly before Initialize (no lock needed there, as no
// other goroutine touches it yet).
func (f *Fader) SetCrossfade(d time.Duration) {
	f.mu.Lock()
	f.Crossfade = d
	f.mu.Unlock()
}

// Play loads url. Decision rule: if the current handle is playing AND url
// differs from what it is playing AND a crossfade is configured, do a crossfade
// switch; otherwise load plainly on the current handle (the exact old
// behaviour). Only a live station zap satisfies all three: initial start and
// resume begin from a stopped handle (not playing), and the quality-change path
// stops first (see ui.toggleHiFi), so neither crossfades.
//
// Any in-flight fade is cancelled first, so a Play arriving mid-fade takes over
// cleanly.
func (f *Fader) Play(url string) {
	f.cancelFade()

	f.mu.Lock()
	cur := f.current
	cross := f.Crossfade
	f.mu.Unlock()
	if cur == nil {
		return
	}

	if cross > 0 && cur.IsPlaying() && cur.URL() != url {
		f.startCrossfade(cur, url, cross)
		return
	}
	cur.Play(url)
}

// startCrossfade brings up a fresh silent handle on url, promotes it to current
// (demoting outgoing so its events stop reaching the UI), and spawns the fade
// goroutine. On a failure to initialize the incoming handle it falls back to a
// plain load on the outgoing handle (the old hard-cut behaviour).
func (f *Fader) startCrossfade(outgoing *MPV, url string, dur time.Duration) {
	f.mu.Lock()
	device := f.device
	f.mu.Unlock()

	zero := 0
	incoming := f.newHandle(&zero)
	if err := incoming.Initialize(); err != nil {
		log.Printf("player: crossfade: incoming init failed (%v), hard cut", err)
		outgoing.Play(url)
		return
	}
	// Match the currently selected sink before any audio flows.
	incoming.SetAudioDevice(device)

	fd := &fade{cancel: make(chan struct{}), done: make(chan struct{})}
	restart := make(chan struct{})

	f.mu.Lock()
	f.outgoing = outgoing
	f.incoming = incoming
	f.fadeRestart = restart
	f.current = incoming // promote: UI-facing events now come from the new station
	f.active = fd
	f.mu.Unlock()

	// Load AFTER promotion and after fadeRestart is armed, so the incoming's
	// PLAYBACK_RESTART cannot be missed by the fade goroutine.
	incoming.Play(url)

	go f.fadeLoop(fd, incoming, outgoing, restart, dur)
}

// fadeLoop waits for the incoming audio to flow, then ramps the equal-power
// crossfade over dur, and finally tears down the outgoing handle. It exits
// early on cancellation or on a wait timeout (hard cut to the incoming). It
// always snaps the incoming to full volume and closes the outgoing before
// returning, and always closes fd.done so a waiting cancelFade unblocks.
func (f *Fader) fadeLoop(fd *fade, incoming, outgoing *MPV, restart <-chan struct{}, dur time.Duration) {
	defer close(fd.done)

	finish := func() {
		incoming.setInternalVolume(100)
		outgoing.Close() // blocks until the outgoing event loop drains
		f.clearActive(fd)
	}

	// 1) Wait for the incoming stream to actually produce audio.
	select {
	case <-restart:
	case <-fd.cancel:
		finish()
		return
	case <-time.After(fadeWaitTimeout):
		log.Printf("player: crossfade: incoming did not start within %s, hard cut", fadeWaitTimeout)
		finish()
		return
	}

	// 2) Equal-power ramp.
	ticks := int(dur / fadeTick)
	if ticks < 1 {
		ticks = 1
	}
	ticker := time.NewTicker(fadeTick)
	defer ticker.Stop()
	for i := 1; i <= ticks; i++ {
		select {
		case <-fd.cancel:
			finish()
			return
		case <-ticker.C:
		}
		in, out := equalPowerGains(float64(i) / float64(ticks))
		incoming.setInternalVolume(in)
		outgoing.setInternalVolume(out)
	}

	// 3) Done: incoming at full, outgoing gone.
	finish()
}

// clearActive resets the fade bookkeeping if fd is still the active fade (a
// later fade may have already replaced it).
func (f *Fader) clearActive(fd *fade) {
	f.mu.Lock()
	if f.active == fd {
		f.active = nil
		f.outgoing = nil
		f.incoming = nil
		f.fadeRestart = nil
	}
	f.mu.Unlock()
}

// cancelFade requests cancellation of any running fade and blocks until its
// goroutine has finished cleaning up (incoming snapped to full, outgoing
// closed). Safe to call with no fade active and safe under concurrent callers.
func (f *Fader) cancelFade() {
	f.mu.Lock()
	fd := f.active
	f.mu.Unlock()
	if fd == nil {
		return
	}
	fd.requestCancel()
	<-fd.done
}

// Stop cancels any fade and stops the current handle.
func (f *Fader) Stop() {
	f.cancelFade()
	f.mu.Lock()
	cur := f.current
	f.mu.Unlock()
	if cur != nil {
		cur.Stop()
	}
}

// Close cancels any fade and tears down the current handle.
func (f *Fader) Close() {
	f.cancelFade()
	f.mu.Lock()
	cur := f.current
	f.current = nil
	f.mu.Unlock()
	if cur != nil {
		cur.Close()
	}
}

// --- delegating pass-throughs to the current handle ---

// SetVolume sets the current stream's PulseAudio volume (ao-volume). Unrelated
// to the internal crossfade volume.
func (f *Fader) SetVolume(pct float64) bool {
	cur := f.cur()
	if cur == nil {
		return false
	}
	return cur.SetVolume(pct)
}

// SetMute sets the current stream's mute state (ao-mute).
func (f *Fader) SetMute(mute bool) {
	if cur := f.cur(); cur != nil {
		cur.SetMute(mute)
	}
}

// Volume reads the current stream's PulseAudio volume.
func (f *Fader) Volume() (float64, bool) {
	cur := f.cur()
	if cur == nil {
		return 0, false
	}
	return cur.Volume()
}

// Mute reads the current stream's mute state.
func (f *Fader) Mute() (bool, bool) {
	cur := f.cur()
	if cur == nil {
		return false, false
	}
	return cur.Mute()
}

// IsPlaying reports the current handle's play state.
func (f *Fader) IsPlaying() bool {
	cur := f.cur()
	return cur != nil && cur.IsPlaying()
}

// CoreIdle reports the current handle's core-idle.
func (f *Fader) CoreIdle() bool {
	cur := f.cur()
	return cur != nil && cur.CoreIdle()
}

// URL returns the current handle's loaded URL.
func (f *Fader) URL() string {
	cur := f.cur()
	if cur == nil {
		return ""
	}
	return cur.URL()
}

// RMSLevelDB returns the current handle's RMS level (drives the VU icon, which
// thus follows the incoming station the moment a zap promotes it).
func (f *Fader) RMSLevelDB() (float64, bool) {
	cur := f.cur()
	if cur == nil {
		return 0, false
	}
	return cur.RMSLevelDB()
}

// AudioDeviceList enumerates output devices via the current handle.
func (f *Fader) AudioDeviceList() ([]AudioDevice, bool) {
	cur := f.cur()
	if cur == nil {
		return nil, false
	}
	return cur.AudioDeviceList()
}

// SetAudioDevice switches the current handle's output sink and remembers the
// selection so any handle created for a later crossfade inherits it.
func (f *Fader) SetAudioDevice(name string) bool {
	f.mu.Lock()
	f.device = name
	cur := f.current
	f.mu.Unlock()
	if cur == nil {
		return false
	}
	return cur.SetAudioDevice(name)
}

// cur returns the current handle under lock.
func (f *Fader) cur() *MPV {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}
