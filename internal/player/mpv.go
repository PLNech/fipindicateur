// Package player wraps libmpv via cgo to play FIP's icecast streams.
//
// Derived from the fip-player project (WTFPL):
// https://github.com/DucNg/fip-player (player/mpv.go). Changes: pkg-config
// linkage, live-radio pause semantics (stop instead of the pause property),
// and media-title observation for ICY metadata fallback.
package player

// #cgo pkg-config: mpv
// #include <mpv/client.h>
// #include <stdlib.h>
//
// /* helper functions for building C string arrays */
// char** makeCharArray(int size) {
//     return calloc(sizeof(char*), size);
// }
// void setArrayString(char** a, int i, char* s) {
//     a[i] = s;
// }
import "C"

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"unsafe"
)

// property observe reply ids
const (
	propMediaTitle C.uint64_t = 1
	propCoreIdle   C.uint64_t = 2
)

// MPV is a libmpv-backed player for a single live stream.
type MPV struct {
	handle *C.mpv_handle

	mu       sync.Mutex
	running  bool
	playing  bool
	url      string
	coreIdle bool

	exit chan struct{}

	// TitleChanged is invoked (best-effort) when mpv's media-title property
	// changes; used for ICY metadata fallback. Set before Initialize.
	TitleChanged func(title string)
}

// Initialize creates and configures the libmpv handle.
func (m *MPV) Initialize() error {
	m.mu.Lock()
	if m.handle != nil || m.running {
		m.mu.Unlock()
		return fmt.Errorf("player: already initialized")
	}
	m.exit = make(chan struct{})
	m.running = true
	m.mu.Unlock()

	m.handle = C.mpv_create()
	if m.handle == nil {
		return fmt.Errorf("player: mpv_create failed")
	}

	m.setOptionFlag("resume-playback", false)
	m.setOptionInt("volume", 100)
	m.setOptionInt("volume-max", 100)

	// Audio only.
	m.setOptionFlag("video", false)
	m.setOptionString("vo", "null")
	m.setOptionString("vid", "no")

	m.setOptionInt("cache-secs", 3)

	// astats filter: exposes per-window audio levels as filter metadata, used
	// by the animated tray icon. Labeled @astats so the property path is
	// stable. Negligible DSP cost (it runs on Android radios).
	m.setOptionString("af", "@astats:lavfi=[astats=metadata=1:reset=6]")

	m.setOptionFlag("terminal", false)
	m.setOptionFlag("input-terminal", false)
	m.setOptionFlag("quiet", true)

	if err := m.check(C.mpv_initialize(m.handle)); err != nil {
		return err
	}

	m.observe("media-title", propMediaTitle, C.MPV_FORMAT_STRING)
	m.observe("core-idle", propCoreIdle, C.MPV_FORMAT_FLAG)

	go m.eventLoop()
	return nil
}

// Play loads the given URL and (re)joins the live edge. For live radio this is
// also how we "resume" from a stopped state: a fresh loadfile, not the pause
// property (which would replay a stale buffer).
func (m *MPV) Play(url string) {
	m.mu.Lock()
	m.url = url
	m.playing = true
	m.mu.Unlock()
	m.command([]string{"loadfile", url})
}

// Stop halts playback entirely (mpv "stop"). This is our pause: it drops the
// buffer so a later Play() rejoins live rather than replaying stale audio.
func (m *MPV) Stop() {
	m.mu.Lock()
	m.playing = false
	m.mu.Unlock()
	m.command([]string{"stop"})
}

// SetVolume sets the playback volume in percent (clamped to 0..100). Safe to
// call before the first loadfile: mpv volume is a global property.
func (m *MPV) SetVolume(pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	cn := C.CString("volume")
	defer C.free(unsafe.Pointer(cn))
	if err := m.check(C.mpv_set_property_async(m.handle, 1, cn, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&pct))); err != nil {
		log.Printf("player: set volume: %v", err)
	}
}

// SetMute sets the mute state (deterministic, unlike a toggle).
func (m *MPV) SetMute(mute bool) {
	v := "no"
	if mute {
		v = "yes"
	}
	m.setPropertyString("mute", v)
}

// setPropertyString sets a string property asynchronously.
func (m *MPV) setPropertyString(name, value string) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	cv := C.CString(value)
	defer C.free(unsafe.Pointer(cv))
	if err := m.check(C.mpv_set_property_async(m.handle, 1, cn, C.MPV_FORMAT_STRING, unsafe.Pointer(&cv))); err != nil {
		log.Printf("player: set %s: %v", name, err)
	}
}

// IsPlaying reports whether the player is in the playing state.
func (m *MPV) IsPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.playing
}

// CoreIdle reports mpv's core-idle property; false means audio is flowing.
func (m *MPV) CoreIdle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.coreIdle
}

// URL returns the currently loaded stream URL.
func (m *MPV) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.url
}

// RMSLevelDB returns the current overall RMS level in dB (typically -60..0)
// from the astats filter, or ok=false when unavailable (stopped, buffering,
// filter missing).
//
// It fetches the WHOLE af-metadata/astats map and parses the JSON: asking
// libmpv for the sub-key path ("af-metadata/astats/lavfi.astats...") segfaults
// libmpv 2.2 (verified on Ubuntu 24.04). Never use sub-key access here.
func (m *MPV) RMSLevelDB() (float64, bool) {
	m.mu.Lock()
	handle := m.handle
	running := m.running
	m.mu.Unlock()
	if handle == nil || !running {
		return 0, false
	}

	cn := C.CString("af-metadata/astats")
	defer C.free(unsafe.Pointer(cn))
	cs := C.mpv_get_property_string(handle, cn)
	if cs == nil {
		return 0, false
	}
	defer C.mpv_free(unsafe.Pointer(cs))

	var meta map[string]string
	if err := json.Unmarshal([]byte(C.GoString(cs)), &meta); err != nil {
		return 0, false
	}
	raw, ok := meta["lavfi.astats.Overall.RMS_level"]
	if !ok {
		return 0, false
	}
	if raw == "-inf" { // digital silence
		return -60, true
	}
	db, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return db, true
}

// Close tears down libmpv cleanly.
func (m *MPV) Close() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	// Wake the event loop and wait for it to drain.
	C.mpv_wakeup(m.handle)
	<-m.exit

	C.mpv_terminate_destroy(m.handle)
	m.handle = nil
}

// --- option/command plumbing (ported) ---

func (m *MPV) setOptionFlag(key string, value bool) {
	v := C.int(0)
	if value {
		v = 1
	}
	m.setOption(key, C.MPV_FORMAT_FLAG, unsafe.Pointer(&v))
}

func (m *MPV) setOptionInt(key string, value int) {
	v := C.int64_t(value)
	m.setOption(key, C.MPV_FORMAT_INT64, unsafe.Pointer(&v))
}

func (m *MPV) setOptionString(key, value string) {
	cv := C.CString(value)
	defer C.free(unsafe.Pointer(cv))
	m.setOption(key, C.MPV_FORMAT_STRING, unsafe.Pointer(&cv))
}

func (m *MPV) setOption(key string, format C.mpv_format, value unsafe.Pointer) {
	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))
	if err := m.check(C.mpv_set_option(m.handle, ck, format, value)); err != nil {
		log.Printf("player: set option %s: %v", key, err)
	}
}

func (m *MPV) observe(name string, reply C.uint64_t, format C.mpv_format) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	C.mpv_observe_property(m.handle, reply, cn, format)
}

func (m *MPV) command(cmd []string) {
	// Log without leaking the stream URL.
	safe := make([]string, len(cmd))
	copy(safe, cmd)
	if len(cmd) > 1 && cmd[0] == "loadfile" {
		safe[1] = "<stream>"
	}
	log.Println("player: command", safe)

	arr := C.makeCharArray(C.int(len(cmd) + 1))
	if arr == nil {
		log.Println("player: calloc returned NULL")
		return
	}
	defer C.free(unsafe.Pointer(arr))
	for i, s := range cmd {
		cs := C.CString(s)
		C.setArrayString(arr, C.int(i), cs)
		defer C.free(unsafe.Pointer(cs))
	}
	if err := m.check(C.mpv_command_async(m.handle, 0, arr)); err != nil {
		log.Printf("player: command %v: %v", safe, err)
	}
}

func (m *MPV) eventLoop() {
	for {
		event := C.mpv_wait_event(m.handle, 1)

		m.mu.Lock()
		running := m.running
		m.mu.Unlock()
		if !running {
			close(m.exit)
			return
		}

		if event.event_id == C.MPV_EVENT_NONE {
			continue
		}

		switch event.event_id {
		case C.MPV_EVENT_END_FILE:
			log.Println("player: end-file")
		case C.MPV_EVENT_PLAYBACK_RESTART:
			log.Println("player: playback-restart")
		case C.MPV_EVENT_PROPERTY_CHANGE:
			m.onPropertyChange(event)
		}
	}
}

func (m *MPV) onPropertyChange(event *C.mpv_event) {
	prop := (*C.mpv_event_property)(event.data)
	switch event.reply_userdata {
	case propMediaTitle:
		if prop.format == C.MPV_FORMAT_STRING && prop.data != nil {
			cs := *(**C.char)(prop.data)
			if cs != nil {
				title := C.GoString(cs)
				if m.TitleChanged != nil && title != "" {
					m.TitleChanged(title)
				}
			}
		}
	case propCoreIdle:
		if prop.format == C.MPV_FORMAT_FLAG && prop.data != nil {
			idle := *(*C.int)(prop.data) != 0
			m.mu.Lock()
			m.coreIdle = idle
			m.mu.Unlock()
		}
	}
}

func (m *MPV) check(status C.int) error {
	if status < 0 {
		return fmt.Errorf("mpv API error: %s (%d)", C.GoString(C.mpv_error_string(status)), int(status))
	}
	return nil
}
