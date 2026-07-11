// Package events records user actions to a local append-only JSONL log, one
// JSON object per line. Opt-in (default off), local-only, no network ever.
//
// Events describe behaviour (what you did: play, pause, zap between stations,
// change volume), not track identity (that lives in internal/histlog). The
// derived analytics (session durations, listening hours, and the station
// transition Markov graph) are computed from this log by internal/stats.
//
// Design mirrors histlog: versioned JSONL, zero new dependencies, greppable,
// best-effort writes that must never affect playback. The Recorder's writes
// are async and non-blocking, so a menu click never waits on disk.
package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SchemaVersion is the current value of the "v" field.
const SchemaVersion = 1

// Kind is the action vocabulary. Every user action maps to one kind; this is
// the "measurable by design" contract (see internal/ui: clickable menu items
// are wired through App.on, which records automatically).
type Kind string

const (
	KindAppStart      Kind = "app_start"
	KindAppStop       Kind = "app_stop"
	KindPlay          Kind = "play"
	KindPause         Kind = "pause"
	KindLike          Kind = "like"           // explicit taste: liked the current track
	KindDislike       Kind = "dislike"        // explicit taste: disliked the current track
	KindStationChange Kind = "station_change" // From, To set
	KindAudioDevice   Kind = "audio_device"   // user picked an audio output device
	KindVolume        Kind = "volume"         // Value = percent
	KindMute          Kind = "mute"           // Value = 1 muted, 0 unmuted
	KindHiFi          Kind = "hifi"           // Value = 1 on, 0 off
	KindCrossfade     Kind = "crossfade"      // Value = seconds, 0 = off
	KindNotif         Kind = "notif_toggle"   // Value = 1 on, 0 off
	KindAnim          Kind = "anim_toggle"    // Value = 1 on, 0 off
	KindAutostart     Kind = "autostart"      // Value = 1 on, 0 off
	KindOpenWiki      Kind = "open_wiki"
	KindOpenLink      Kind = "open_link"
	KindOpenHistory   Kind = "open_history"
	KindOpenFip       Kind = "open_fip"
	KindOpenAbout     Kind = "open_about"
	KindStatsView     Kind = "stats_view"
	KindStatsToggle   Kind = "stats_toggle" // Value = 1 on, 0 off
	KindStatsClear    Kind = "stats_clear"
	KindRestart       Kind = "restart"
	KindUpdateCheck   Kind = "update_check"
	KindUpdateStartup Kind = "update_startup_toggle" // Value = 1 on, 0 off
	KindQuit          Kind = "quit"
)

// Event is one log line. V versions the schema. Optional fields are omitted
// when empty so the log stays lean and greppable.
type Event struct {
	V       int       `json:"v"`
	TS      time.Time `json:"ts"`
	Kind    Kind      `json:"kind"`
	Station string    `json:"station,omitempty"`
	From    string    `json:"from,omitempty"`
	To      string    `json:"to,omitempty"`
	Value   int       `json:"value,omitempty"`
}

// DefaultPath returns ~/.local/share/fipindicateur/events.jsonl (honoring
// XDG_DATA_HOME), creating the directory if needed. This is a separate file
// from history.jsonl: separate consent, separate purpose.
func DefaultPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "events.jsonl"), nil
}

// DataDir returns ~/.local/share/fipindicateur (honoring XDG_DATA_HOME),
// creating it if needed. Shared with histlog's location by convention.
func DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "fipindicateur")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Append writes one event as a JSON line to the given file, creating it if
// absent. V is forced to SchemaVersion and TS to now if unset. This is the
// low-level primitive; most callers use a Recorder.
func Append(path string, e Event) error {
	e.V = SchemaVersion
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// Load reads all events from a JSONL file. A missing file is not an error (it
// means "no data yet"): it returns an empty slice. Malformed lines are skipped
// so a partially-written tail never fails the whole read.
func Load(path string) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Event
		if json.Unmarshal(line, &e) != nil {
			continue // tolerate a torn final line
		}
		out = append(out, e)
	}
	return out, nil
}

// splitLines splits on '\n' without allocating a scanner, tolerating a missing
// trailing newline.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// bufferSize bounds the pending-write channel. Human click rates never come
// close; the bound only exists so Record can drop rather than block if disk IO
// ever stalls badly.
const bufferSize = 256

// Recorder is the async, opt-in sink. When disabled, Record is a cheap no-op.
// When enabled, events are queued to a background goroutine that appends them,
// so callers (menu handlers, playback callbacks) never block on disk.
type Recorder struct {
	mu      sync.Mutex
	enabled bool
	path    string // resolved lazily on first enable
	ch      chan Event
	done    chan struct{}
	started bool
}

// NewRecorder returns a Recorder. If enabled, the background writer starts
// immediately. Path resolution is deferred and best-effort: if it fails, the
// recorder silently degrades to a no-op (analytics must never break the app).
func NewRecorder(enabled bool) *Recorder {
	r := &Recorder{}
	if enabled {
		r.SetEnabled(true)
	}
	return r
}

// SetEnabled turns recording on or off at runtime (the Réglages toggle). Off
// stops the writer and drops the queue; on (re)starts it.
func (r *Recorder) SetEnabled(on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if on == r.enabled {
		return
	}
	r.enabled = on
	if on {
		r.startLocked()
	} else {
		r.stopLocked()
	}
}

// Enabled reports whether recording is currently on.
func (r *Recorder) Enabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

func (r *Recorder) startLocked() {
	if r.started {
		return
	}
	if r.path == "" {
		p, err := DefaultPath()
		if err != nil {
			// Can't resolve a path: stay disabled-in-practice. Record no-ops.
			r.enabled = false
			return
		}
		r.path = p
	}
	r.ch = make(chan Event, bufferSize)
	r.done = make(chan struct{})
	r.started = true
	go r.writeLoop(r.ch, r.done)
}

func (r *Recorder) stopLocked() {
	if !r.started {
		return
	}
	close(r.ch)
	<-r.done
	r.started = false
	r.ch = nil
	r.done = nil
}

// writeLoop drains queued events to disk until the channel is closed. Errors
// are swallowed: a failed analytics write must never surface to the user.
func (r *Recorder) writeLoop(ch chan Event, done chan struct{}) {
	defer close(done)
	for e := range ch {
		_ = Append(r.path, e)
	}
}

// Record queues an event. No-op when disabled. Non-blocking: if the buffer is
// full (pathological disk stall), the event is dropped rather than blocking a
// menu click. TS/V are filled by Append.
func (r *Recorder) Record(e Event) {
	r.mu.Lock()
	ch := r.ch
	enabled := r.enabled && r.started
	r.mu.Unlock()
	if !enabled {
		return
	}
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	select {
	case ch <- e:
	default: // buffer full: drop, never block the caller
	}
}

// Close flushes and stops the writer. Safe to call multiple times.
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
	r.enabled = false
}

// Clear deletes the events log (the "Effacer les statistiques" data right).
// It removes only events.jsonl, never history.jsonl. A missing file is not an
// error. The writer is briefly stopped and restarted if it was running, so we
// never race a delete against an in-flight append.
func (r *Recorder) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	wasRunning := r.started
	if wasRunning {
		r.stopLocked()
	}
	path := r.path
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		err = nil
	}
	if wasRunning {
		r.startLocked()
	}
	return err
}
