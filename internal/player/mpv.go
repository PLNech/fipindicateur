// Package player wraps libmpv via cgo to play FIP's icecast streams.
//
// Derived from the fip-player project (WTFPL):
// https://github.com/DucNg/fip-player (player/mpv.go). Changes: per-platform
// cgo linkage (see cgo_pkgconfig.go / cgo_windows.go), live-radio pause
// semantics (stop instead of the pause property),
// and media-title observation for ICY metadata fallback.
package player

// #include <mpv/client.h>
// #include <stdlib.h>
// #include <locale.h>
//
// /* Read-only: libmpv refuses mpv_create unless LC_NUMERIC is "C". Never write
//    the locale from here: this runs on arbitrary goroutines, and another
//    toolkit in the process may be reading it. This only names the culprit in
//    the log. */
// static const char *lcNumeric(void) { return setlocale(LC_NUMERIC, NULL); }
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
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// property observe reply ids
const (
	propMediaTitle C.uint64_t = 1
	propCoreIdle   C.uint64_t = 2
	propAoVolume   C.uint64_t = 3
	propAoMute     C.uint64_t = 4
)

// numericLocale reports the process LC_NUMERIC category. libmpv only creates a
// handle when it is "C", so a crossfade that cannot bring up its incoming
// stream logs this value: a GUI toolkit calling setlocale(LC_ALL, "") behind our
// back is the one cause worth naming out loud.
func numericLocale() string { return C.GoString(C.lcNumeric()) }

// AudioDevice is one entry of mpv's audio-device-list: Name is the identifier
// passed to the audio-device property, Description is the human-readable label.
type AudioDevice struct {
	Name        string
	Description string
}

// MPV is a libmpv-backed player for a single live stream.
type MPV struct {
	handle *C.mpv_handle

	mu       sync.Mutex
	running  bool
	playing  bool
	url      string
	coreIdle bool

	exit chan struct{}

	// initialVolume is the mpv INTERNAL `volume` option applied at Initialize
	// (distinct from ao-volume, the PulseAudio per-app knob). nil means the
	// historical default of 100. The Fader creates an incoming crossfade handle
	// with 0 here so it starts silent and can be ramped up with the equal-power
	// curve while the outgoing handle ramps down.
	initialVolume *int

	// amp is the loudness preamp level baked into the af chain (0 = off,
	// 1 = doux, 2 = costaud, see afChain). Set before Initialize; SetAmp
	// applies a change to the live handle.
	amp int

	// silent selects the null output. Set before Initialize.
	silent bool

	// Callbacks, all best-effort and invoked from the event goroutine.
	// Set them before Initialize.

	// TitleChanged fires when mpv's media-title property changes; used for
	// ICY metadata fallback.
	TitleChanged func(title string)
	// VolumeChanged fires when the PulseAudio stream volume (ao-volume)
	// changes, including external changes from pavucontrol/GNOME.
	VolumeChanged func(pct float64)
	// MuteChanged fires when the stream mute state (ao-mute) changes.
	MuteChanged func(mute bool)
	// StreamEnded fires when the loaded stream died under us: an END_FILE
	// whose reason is EOF or ERROR. A user-driven stop (a new loadfile, Stop,
	// teardown) never fires it. This is the player admitting silence: the
	// playing flag was write-only before, so a dead icecast stream left the
	// UI claiming playback over a frozen tray glyph. Set before Initialize.
	StreamEnded func()
	// PlaybackRestarted fires when playback (re)starts, i.e. when the audio
	// output exists again. On a sink that remembers the per-app level
	// (SinkRestoresVolume) this is the moment to READ ao-volume/ao-mute and
	// sync the UI, never to write: PulseAudio's stream-restore is the source
	// of truth, and overwriting it once stomped a live pavucontrol adjustment
	// during a call. On a sink that forgets the level (CoreAudio), the UI
	// writes the saved level back, because the reopened output comes up at
	// the driver default.
	PlaybackRestarted func()
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
	vol := 100
	if m.initialVolume != nil {
		vol = *m.initialVolume
	}
	m.setOptionInt("volume", vol)
	m.setOptionInt("volume-max", 100)

	// Audio only.
	m.setOptionFlag("video", false)
	m.setOptionString("vo", "null")
	m.setOptionString("vid", "no")

	m.setOptionInt("cache-secs", 3)

	// The af chain: the optional amplification stage, then the astats filter,
	// which exposes per-window audio levels as filter metadata, used by the
	// animated tray icon. Labeled @astats so the property path is stable.
	// Negligible DSP cost (it runs on Android radios).
	//
	// astats measures ONLY the overall RMS level, the single number the VU
	// glyph reads: the default measures ~79 stats per window (parsed 6x/second
	// by RMSLevelDB) and dumps every one of them to the log when a handle is
	// destroyed, which a crossfade now does on every zap. It sits AFTER the
	// preamp stage so the icon dances to what the listener actually hears.
	// Four band-pass meters follow it. Each publishes one mean magnitude, which
	// the tray reads as one bar (bass, low-mid, presence, air). A full bin
	// dump would be hundreds of keys six times a second; four numbers is the
	// whole reading. The filters measure only: the samples that reach the
	// speakers are unchanged.
	m.setOptionString("af", afChain(m.amp))

	m.setOptionFlag("terminal", false)
	m.setOptionFlag("input-terminal", false)
	m.setOptionFlag("quiet", true)
	if m.silent {
		// Measurement only. No output device is opened, so a test can read
		// the bands without touching a speaker.
		m.setOptionString("ao", "null")
	}

	if err := m.check(C.mpv_initialize(m.handle)); err != nil {
		return err
	}

	m.observe("media-title", propMediaTitle, C.MPV_FORMAT_STRING)
	m.observe("core-idle", propCoreIdle, C.MPV_FORMAT_FLAG)
	m.observe("ao-volume", propAoVolume, C.MPV_FORMAT_DOUBLE)
	m.observe("ao-mute", propAoMute, C.MPV_FORMAT_FLAG)

	// Seed the cached core-idle with the truth at this instant: nothing is
	// loaded, so the core IS idle. The zero value (false = audio flowing) is
	// a lie a reader could act on in the window before the observer's initial
	// event lands (the animator's astats-failure counter trusts this cache).
	m.mu.Lock()
	m.coreIdle = true
	m.mu.Unlock()

	go m.eventLoop()
	return nil
}

// ampFilter returns the lavfi preamp stage for an amplification level, inserted
// at the head of the af graph (before the astats tap). Measured on the FIP
// streams (ffmpeg ebur128, 30 s of fip-hifi.aac, 2026-09-26): integrated
// loudness -18.5 LUFS, true peak -8.5 dBFS, LRA 2.4 LU. The stream is already
// broadcast-compressed (that LRA), so a static gain beats a dynamic normalizer,
// which would only add latency to a constant gain: +4 dB reaches streaming
// parity (platforms normalize around -14 LUFS) with headroom to spare, +8 dB
// rides the rest of the peak headroom and alimiter holds a true-peak-safe
// ceiling (level=disabled: without it alimiter re-normalizes the output toward
// 0 dBFS and the ceiling disappears). An empty stage leaves the chain unchanged.
func ampFilter(level int) string {
	switch level {
	case 1:
		return "volume=4dB,"
	case 2:
		return "volume=8dB,alimiter=limit=0.84:level=disabled,"
	default:
		return ""
	}
}

// afChain assembles the full af option value: the optional preamp, then the
// labeled astats tap (see Initialize). Pure function, unit-tested.
func afChain(amp int) string {
	// One flat graph. A side chain does not publish its numbers through this
	// player, and a bandpass in series would change what the speakers hear.
	// astats keeps the overall level. aspectralstats adds three numbers
	// (where the energy sits, how wide it is, where it rolls off). The tray
	// paints four bars from those. The samples are unchanged.
	return "@astats:lavfi=[" + ampFilter(amp) +
		"astats=metadata=1:reset=6:measure_perchannel=none:measure_overall=RMS_level," +
		"aspectralstats=win_size=2048:overlap=0.5:measure=centroid+spread+rolloff]"
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

// SetVolume sets the PulseAudio stream volume in percent (ao-volume): the
// SAME per-app knob pavucontrol and GNOME show, so there is one volume, not
// two stacked ones. Only call this for EXPLICIT user intent (menu preset,
// MPRIS write): PulseAudio owns volume persistence. ao-volume only exists
// while the audio output is open; returns false when not applied.
func (m *MPV) SetVolume(pct float64) bool {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return m.setPropDouble("ao-volume", pct)
}

// SetMute sets the stream mute state (ao-mute, the pavucontrol mute), falling
// back to mpv's internal mute when the AO is not open yet.
func (m *MPV) SetMute(mute bool) {
	if m.setPropFlag("ao-mute", mute) {
		return
	}
	if !m.setPropFlag("mute", mute) {
		log.Printf("player: set mute failed")
	}
}

// SetAmp swaps the amplification stage live: rewriting the af property re-chains
// the filters without a restart, stopped or playing. The next Initialize bakes
// the level in through afChain either way.
func (m *MPV) SetAmp(level int) {
	m.mu.Lock()
	m.amp = level
	handle := m.handle
	m.mu.Unlock()
	if handle == nil {
		return
	}
	m.setPropString("af", afChain(level))
}

// Volume reads the current PulseAudio stream volume percent (ao-volume);
// ok=false while the audio output is closed.
func (m *MPV) Volume() (float64, bool) {
	cn := C.CString("ao-volume")
	defer C.free(unsafe.Pointer(cn))
	var v C.double
	if C.mpv_get_property(m.handle, cn, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&v)) < 0 {
		return 0, false
	}
	return float64(v), true
}

// Mute reads the current stream mute state (ao-mute); ok=false while the
// audio output is closed.
func (m *MPV) Mute() (bool, bool) {
	cn := C.CString("ao-mute")
	defer C.free(unsafe.Pointer(cn))
	var v C.int
	if C.mpv_get_property(m.handle, cn, C.MPV_FORMAT_FLAG, unsafe.Pointer(&v)) < 0 {
		return false, false
	}
	return v != 0, true
}

// setInternalVolume synchronously sets mpv's INTERNAL `volume` property (0..100),
// the multiplier applied before the audio output. This is the ONLY knob the
// crossfade touches: never ao-volume/ao-mute, which are the PulseAudio per-app
// level synced with pavucontrol and persisted by stream-restore. The internal
// volume scales inside whatever level PulseAudio restored for the stream.
// Returns false when the property is unavailable.
func (m *MPV) setInternalVolume(pct float64) bool {
	return m.setPropDouble("volume", pct)
}

// getPropDouble synchronously reads a double property; ok=false if unavailable.
func (m *MPV) getPropDouble(name string) (float64, bool) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	var v C.double
	if C.mpv_get_property(m.handle, cn, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&v)) < 0 {
		return 0, false
	}
	return float64(v), true
}

// setPropDouble synchronously sets a double property; false if unavailable.
func (m *MPV) setPropDouble(name string, v float64) bool {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return C.mpv_set_property(m.handle, cn, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&v)) >= 0
}

// setPropString synchronously sets a string property; false if unavailable.
// MPV_FORMAT_STRING takes a pointer to a *char (a **char), mirroring how
// setOptionString passes its value: the value arg is &cv, not cv.
func (m *MPV) setPropString(name, value string) bool {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	cv := C.CString(value)
	defer C.free(unsafe.Pointer(cv))
	return C.mpv_set_property(m.handle, cn, C.MPV_FORMAT_STRING, unsafe.Pointer(&cv)) >= 0
}

// setPropFlag synchronously sets a boolean property; false if unavailable.
func (m *MPV) setPropFlag(name string, v bool) bool {
	cv := C.int(0)
	if v {
		cv = 1
	}
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return C.mpv_set_property(m.handle, cn, C.MPV_FORMAT_FLAG, unsafe.Pointer(&cv)) >= 0
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
	db, reason := m.rmsLevelDB()
	return db, reason == ""
}

// AudioLevelDiag names why RMSLevelDB is unavailable right now, or "" when the
// level reads fine. Used for the single log line the animated icon prints
// before giving up on a run: "unavailable" alone sent us hunting the wrong
// suspect once (a locale bug that turned out to be the crossfade's, not this).
func (m *MPV) AudioLevelDiag() string {
	_, reason := m.rmsLevelDB()
	return reason
}

// rmsLevelDB is the shared read: it returns the level, or a human reason (in
// French, it lands in the app log) naming the step that failed.
func (m *MPV) rmsLevelDB() (float64, string) {
	m.mu.Lock()
	handle := m.handle
	running := m.running
	m.mu.Unlock()
	if handle == nil || !running {
		return 0, "aucun flux (handle mpv absent ou arrêté)"
	}

	cn := C.CString("af-metadata/astats")
	defer C.free(unsafe.Pointer(cn))
	cs := C.mpv_get_property_string(handle, cn)
	if cs == nil {
		return 0, "propriété af-metadata/astats indisponible (filtre astats absent ou aucun audio en vol)"
	}
	defer C.mpv_free(unsafe.Pointer(cs))

	return parseRMSLevel(C.GoString(cs))
}

// Silence disconnects the output device. Call it before Initialize.
// Measurement still runs. Used by tests that must never open a speaker.
func (m *MPV) Silence() { m.silent = true }

// SpectrumBands returns four band levels in 0..1, low to high, folded from
// the mean magnitude of the FFT bins. ok is false when the filter has not
// published yet; the caller then falls back to the overall level.
func (m *MPV) SpectrumBands() (vu.Bands, bool) {
	m.mu.Lock()
	handle := m.handle
	running := m.running
	m.mu.Unlock()
	if handle == nil || !running {
		return vu.Bands{}, false
	}
	cn := C.CString("af-metadata/astats")
	defer C.free(unsafe.Pointer(cn))
	cs := C.mpv_get_property_string(handle, cn)
	if cs == nil {
		return vu.Bands{}, false
	}
	defer C.mpv_free(unsafe.Pointer(cs))
	return parseSpectrumShape(C.GoString(cs))
}

// filterMean reads lavfi.<label>.1.mean from one labeled filter's metadata.
func (m *MPV) filterMean(handle *C.mpv_handle, label string) (float64, bool) {
	cn := C.CString("af-metadata/" + label)
	defer C.free(unsafe.Pointer(cn))
	cs := C.mpv_get_property_string(handle, cn)
	if cs == nil {
		return 0, false
	}
	defer C.mpv_free(unsafe.Pointer(cs))
	return parseFilterMean(C.GoString(cs))
}

// parseRMSLevel extracts the overall RMS level from the JSON map libmpv returns
// for af-metadata/astats. Split from the cgo fetch so it is testable and
// benchmarkable without a libmpv handle.
func parseRMSLevel(js string) (float64, string) {
	var meta map[string]string
	if err := json.Unmarshal([]byte(js), &meta); err != nil {
		return 0, fmt.Sprintf("af-metadata/astats illisible: %v", err)
	}
	raw, ok := meta["lavfi.astats.Overall.RMS_level"]
	if !ok {
		return 0, fmt.Sprintf("lavfi.astats.Overall.RMS_level absent (%d clés présentes)", len(meta))
	}
	if raw == "-inf" { // digital silence
		return -60, ""
	}
	db, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Sprintf("niveau %q non numérique: %v", raw, err)
	}
	return db, ""
}

// parseSpectrumShape turns the three published spectrum numbers into four
// bar levels. centroid is where the energy sits (Hz), spread is how wide it
// is, rolloff is the frequency under which most of the energy falls. A tone
// lights one bar; a wide mix lights several. Missing keys are a miss.
func parseSpectrumShape(js string) (vu.Bands, bool) {
	var meta map[string]string
	if err := json.Unmarshal([]byte(js), &meta); err != nil {
		return vu.Bands{}, false
	}
	centroid, okC := parseMetaFloat(meta, "lavfi.aspectralstats.1.centroid")
	spread, okS := parseMetaFloat(meta, "lavfi.aspectralstats.1.spread")
	rolloff, okR := parseMetaFloat(meta, "lavfi.aspectralstats.1.rolloff")
	if !okC || !okS || !okR {
		return vu.Bands{}, false
	}
	return shapeToBands(centroid, spread, rolloff), true
}

func parseMetaFloat(meta map[string]string, key string) (float64, bool) {
	raw, ok := meta[key]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// shapeToBands paints four bars from centroid, spread and rolloff. The bar
// centers sit an octave-ish apart. Width comes from the spread, so a pure
// tone is one bar and a full mix is several.
func shapeToBands(centroid, spread, rolloff float64) vu.Bands {
	centers := [vu.Bars]float64{80, 300, 1200, 5000}
	width := math.Max(spread, 40)
	var b vu.Bands
	var sum float64
	for i, c := range centers {
		d := (math.Log2(math.Max(centroid, 20)) - math.Log2(c)) * 1200 // cents
		b[i] = math.Exp(-(d * d) / (2 * width * width))
		sum += b[i]
	}
	// rolloff below the centroid means the energy is piled on the low side,
	// so the lower bars keep more of the weight.
	if rolloff > 0 && rolloff < centroid {
		b[0] *= 1.3
		b[1] *= 1.1
	}
	if sum == 0 {
		return b
	}
	peak := 0.0
	for i := 0; i < vu.Bars; i++ {
		if b[i] > peak {
			peak = b[i]
		}
	}
	if peak > 0 {
		for i := 0; i < vu.Bars; i++ {
			b[i] /= peak
		}
	}
	return b
}

// parseFilterMean reads the single mean magnitude a band meter publishes.
// The key is lavfi.aspectralstats.1.mean (channel 1). A missing key is a miss.
func parseFilterMean(js string) (float64, bool) {
	var meta map[string]string
	if err := json.Unmarshal([]byte(js), &meta); err != nil {
		return 0, false
	}
	raw, ok := meta["lavfi.aspectralstats.1.mean"]
	if !ok {
		return 0, false
	}
	mag, err := strconv.ParseFloat(raw, 64)
	if err != nil || mag < 0 || math.IsNaN(mag) {
		return 0, false
	}
	return mag, true
}

// bandsFromMeans maps four mean magnitudes to 0..1 bar levels. When a band
// has not reported yet its level stays 0 and the caller still gets a reading
// from the bands that have.
func bandsFromMeans(means [vu.Bars]float64, complete bool) vu.Bands {
	_ = complete
	var b vu.Bands
	for i := 0; i < vu.Bars; i++ {
		b[i] = bandLevel(means[i])
	}
	return b
}

// bandLevel maps a mean bin magnitude to 0..1. The filter publishes a linear
// magnitude; a log curve keeps a quiet band visible next to a loud one,
// which is what a four-bar glyph needs at 16 points.
func bandLevel(mean float64) float64 {
	if mean <= 1e-6 {
		return 0
	}
	db := 20 * math.Log10(mean)
	return vu.LevelFromDB(db)
}

// AudioDeviceList enumerates the output devices mpv sees on this platform
// (PulseAudio/ALSA on Linux, CoreAudio on macOS), so the user can pick a sink
// without pavucontrol or the OS audio panel. The list is read as a JSON string
// (same fetch/free/unmarshal shape as RMSLevelDB) and is available right after
// mpv_initialize, before any audio output is open: enumeration needs no
// playback. The first entry is normally {"name":"auto","description":
// "Autodetect device"}. ok=false on any failure.
func (m *MPV) AudioDeviceList() ([]AudioDevice, bool) {
	m.mu.Lock()
	handle := m.handle
	running := m.running
	m.mu.Unlock()
	if handle == nil || !running {
		return nil, false
	}

	cn := C.CString("audio-device-list")
	defer C.free(unsafe.Pointer(cn))
	cs := C.mpv_get_property_string(handle, cn)
	if cs == nil {
		return nil, false
	}
	defer C.mpv_free(unsafe.Pointer(cs))

	var raw []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(C.GoString(cs)), &raw); err != nil {
		return nil, false
	}
	devs := make([]AudioDevice, len(raw))
	for i, d := range raw {
		devs[i] = AudioDevice{Name: d.Name, Description: d.Description}
	}
	return devs, true
}

// SetAudioDevice switches the output sink via the audio-device property: the
// SAME knob mpv and PulseAudio expose, so there is one selection, not two.
// mpv reinits the audio output live when this changes, so no stream reload is
// needed. An empty name means mpv's "auto" (system default). Returns whether
// the set succeeded.
func (m *MPV) SetAudioDevice(name string) bool {
	if name == "" {
		name = "auto"
	}
	return m.setPropString("audio-device", name)
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
			ef := (*C.mpv_event_end_file)(event.data)
			reason := int(ef.reason)
			log.Printf("player: end-file (reason %d)", reason)
			if endFileReports(reason) {
				// The stream died under us (eof on a live icecast relay, or
				// an error). Clear the playing flag BEFORE notifying: the
				// callback drives the UI, and a Play click racing it must
				// see a stopped player, not the corpse's stale flag.
				m.mu.Lock()
				m.playing = false
				cb := m.StreamEnded
				m.mu.Unlock()
				if cb != nil {
					cb()
				}
			}
		case C.MPV_EVENT_PLAYBACK_RESTART:
			log.Println("player: playback-restart")
			if m.PlaybackRestarted != nil {
				m.PlaybackRestarted()
			}
		case C.MPV_EVENT_PROPERTY_CHANGE:
			m.onPropertyChange(event)
		}
	}
}

// endFileReports tells whether an END_FILE reason describes a stream that
// died under us (eof or error), as opposed to one we dismissed ourselves:
// stop (a new loadfile replaced the file, or MPV.Stop) and quit (teardown).
// Only the dead ones clear the playing flag and fire StreamEnded; firing on
// our own stops would make every Stop look like a crash and every quality
// reload like a failure. Pure function, unit-tested.
func endFileReports(reason int) bool {
	return reason == int(C.MPV_END_FILE_REASON_EOF) || reason == int(C.MPV_END_FILE_REASON_ERROR)
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
	case propAoVolume:
		if prop.format == C.MPV_FORMAT_DOUBLE && prop.data != nil && m.VolumeChanged != nil {
			m.VolumeChanged(float64(*(*C.double)(prop.data)))
		}
	case propAoMute:
		if prop.format == C.MPV_FORMAT_FLAG && prop.data != nil && m.MuteChanged != nil {
			m.MuteChanged(*(*C.int)(prop.data) != 0)
		}
	}
}

func (m *MPV) check(status C.int) error {
	if status < 0 {
		return fmt.Errorf("mpv API error: %s (%d)", C.GoString(C.mpv_error_string(status)), int(status))
	}
	return nil
}
