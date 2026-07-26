// Package drawer renders « le panneau », the quick-control popup: an
// undecorated webkit2gtk window at the top-right of the screen showing our own
// HTML/CSS/JS (embedded, self-contained, system fonts). Linux-only: the real
// implementation builds on linux && cgo against the system webkit2gtk-4.1 and
// gtk+-3.0 (hand-rolled cgo, like internal/player does for libmpv); every
// other platform gets a no-op stub so darwin/windows keep compiling and keep
// their native volume submenu.
//
// The page is dumb on purpose: Go pushes a full State JSON on every relevant
// change (and on each show) via window.fipState(json); the page renders from
// state only, no polling. User actions come back as JSON Commands through a
// webkit script message handler named "fip". The same HTML UI is meant to be
// served to phones later, which is why it owns its whole look.
package drawer

import (
	_ "embed"
	"os/exec"
	"strings"
)

// pageHTML is the whole UI, embedded so the binary stays self-contained (no
// files to install, no CDN, system fonts only).
//
//go:embed page.html
var pageHTML string

// DarkPreferred reads the desktop's color-scheme preference (GNOME gsettings).
// Called at each Show so a theme flip is honoured on the next open; it is a
// short exec, never on a hot path. False when gsettings is absent (non-GNOME,
// non-Linux): the page then falls back to its prefers-color-scheme media query.
func DarkPreferred() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "dark")
}

// Command is one user action posted by the page (JS to Go). Actions:
//
//	toggle_play          play/pause the active sink
//	volume               Value = target percent 0..100
//	toggle_mute          flip mute on the active sink
//	station              Key = station key to zap to
//	output               Value = device index from State.Devices, -1 = local
//	audio_device         Key = local sink name from State.AudioDevices
//	rescan               run a fresh Chromecast discovery
//	like / dislike       taste verdict on the current track (prefs.jsonl)
//	open_wiki            open the current track's artist page
//	open_link            open the current track's Radio France music link
//	open_history         Value = index into State.History, open that artist
//	toggle_stats         flip the opt-in listening statistics
//	stats_view           build and open the listening report
//	stats_folder         open the data folder in the file manager
//	stats_clear          delete events.jsonl (the page already confirmed)
//	prefs_clear          delete prefs.jsonl (the page already confirmed)
//	toggle_hifi          flip stream quality (AAC 192k)
//	crossfade            Value = crossfade seconds 0..10 (0 = hard cut)
//	toggle_notif         flip track notifications
//	toggle_show_notif    flip the émission-start notifications
//	toggle_show_calendar flip the upcoming-programmes section
//	toggle_anim          flip the animated (VU) tray icon
//	toggle_hist_file     flip the local track-history file
//	toggle_update_startup flip the quiet startup update check
//	toggle_play_on_start flip playback at launch
//	toggle_autostart     flip launch at login (XDG)
//	update_check         on-demand update check against GitHub releases
//	restart              relaunch the app (loads the last installed binary)
//	open_fip             open FIP on radiofrance.fr
//	open_github          open the project page
//	quit                 quit the app
//
// The two _clear actions arrive only after the page's own two-click confirm
// (first click arms with a visual warning, the second one posts), mirroring
// the tray menu's arm-then-confirm semantics.
//
// The page also posts internal actions (ready, height, hide) that the drawer
// consumes itself; they never reach the app's command handler.
type Command struct {
	Action string `json:"action"`
	Value  int    `json:"value"`
	Key    string `json:"key"`
}

// Station is one chip of the station strip: key to zap, display name, and the
// official webradio brand color (hex) from internal/stations.
type Station struct {
	Key     string `json:"key"`
	Display string `json:"display"`
	Color   string `json:"color"`
}

// Track is the now-playing identity when known. Artwork is a URL already
// present in the metadata plumbing (livemeta's visual); empty means the page
// shows a station-color block instead. HasLink reports whether Radio France
// provided a music link for this track (the "Écouter ailleurs" affordance).
type Track struct {
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Artwork string `json:"artwork"`
	HasLink bool   `json:"hasLink"`
}

// AudioDevice is one local output sink for the « Sur cet appareil » section:
// mpv's device name (the command key) and a display label.
type AudioDevice struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// Upcoming is one scheduled programme for the « À venir » section of the
// history view. Display only, like the menu's Calendrier.
type Upcoming struct {
	Time  string `json:"time"` // local "15:04"; empty when unscheduled
	Title string `json:"title"`
}

// Cast is the casting side of the state. Volume/Muted are the DEVICE's level
// as reported by RECEIVER_STATUS (never a value we invented); VolumeKnown is
// false until the device reported once. ControlType "master" means the slider
// drives an amplifier's master volume: the page labels it accordingly.
type Cast struct {
	Active      bool   `json:"active"`
	DeviceName  string `json:"deviceName"`
	Playing     bool   `json:"playing"`
	Volume      int    `json:"volume"`
	Muted       bool   `json:"muted"`
	VolumeKnown bool   `json:"volumeKnown"`
	ControlType string `json:"controlType"`
}

// Settings mirrors the tray menu's toggles for the panel's settings view. The
// page only displays them; flipping one posts the matching toggle_* command,
// which lands on the same App handler the menu item uses (one wiring, two
// skins), so each records its telemetry kind exactly once.
type Settings struct {
	Stats              bool `json:"stats"`             // listening statistics opt-in
	HiFi               bool `json:"hifi"`              // AAC 192k stream
	Notifications      bool `json:"notifications"`     // track notifications
	ShowNotifications  bool `json:"showNotifications"` // émission-start notifications
	ShowCalendar       bool `json:"showCalendar"`      // upcoming programmes (À venir)
	AnimatedIcon       bool `json:"animatedIcon"`      // VU bars in the tray icon
	HistoryFile        bool `json:"historyFile"`       // local track log (history.jsonl)
	UpdateStartup      bool `json:"updateStartup"`     // quiet update check at launch
	PlayOnStart        bool `json:"playOnStart"`       // start the stream at launch
	Autostart          bool `json:"autostart"`         // launch at login (XDG)
	AutostartSupported bool `json:"autostartSupported"`
	CrossfadeSecs      int  `json:"crossfadeSecs"` // station-zap fade, 0..10 s
}

// HistoryEntry is one row of the panel's history view: the same recent-tracks
// ring the tray's Historique submenu shows. Display only; identity stays in
// the page and never reaches the events log.
type HistoryEntry struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
}

// State is the full UI state, pushed as one JSON document. The page renders
// exclusively from it.
type State struct {
	Dark         bool           `json:"dark"`
	Station      string         `json:"station"` // active station key
	Playing      bool           `json:"playing"` // local player state
	Volume       int            `json:"volume"`  // local volume percent
	Muted        bool           `json:"muted"`
	Cast         Cast           `json:"cast"`
	Devices      []string       `json:"devices"` // friendly names from the last scan
	Scanning     bool           `json:"scanning"`
	AudioDevices []AudioDevice  `json:"audioDevices"` // local sinks (Sur cet appareil)
	AudioDevice  string         `json:"audioDevice"`  // selected sink name ("auto" = automatic)
	Track        Track          `json:"track"`
	Show         string         `json:"show"` // programme on air, when known
	Stations     []Station      `json:"stations"`
	Settings     Settings       `json:"settings"`
	History      []HistoryEntry `json:"history"`  // most recent first
	Upcoming     []Upcoming     `json:"upcoming"` // scheduled programmes (À venir)
	Version      string         `json:"version"`
}
