// Package popover is the Go half of the macOS SwiftUI popover: the state it
// renders (Snapshot, pure and built on every platform) and, behind the
// swiftui build tag on darwin, the calls into the linked Swift object
// (internal/popover/fipui).
//
// The direction of authority is one way: Go owns the state and pushes whole
// snapshots; Swift renders the last one and reports intents by action id.
package popover

import "encoding/json"

// Station is one stop on the popover's dial.
type Station struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"` // #rrggbb, the station's identity colour
}

// Snapshot is everything the popover shows, pushed whole on every change.
// The Swift struct of the same name (fipui/swift/Bridge.swift) mirrors it
// field for field; a field Swift does not know is ignored, a field Go omits
// keeps its Swift default.
type Snapshot struct {
	Playing  bool      `json:"playing"`
	Station  string    `json:"station"`
	Stations []Station `json:"stations"`
	Title    string    `json:"title"`
	Artist   string    `json:"artist"`
	Album    string    `json:"album"`
	Year     int       `json:"year"`
	Label    string    `json:"label"`
	Show     string    `json:"show"`
	Cover    string    `json:"cover"` // path of the cached cover PNG, "" when none yet
	Start    float64   `json:"start"` // unix seconds, 0 when unknown
	End      float64   `json:"end"`
	Volume   int       `json:"volume"` // 0..100, the active sink's
	Muted    bool      `json:"muted"`
	Verdict  int       `json:"verdict"` // -1 pas pour moi, 0 none, 1 j'aime
	Settings Settings  `json:"settings"`
}

// Carton is one banner of the card build: the carton d'antenne, a small
// card that drops under the menu bar for a few seconds and goes away by
// itself. It carries everything it shows (Swift does no network and keeps no
// state for it), so it can never render the previous track.
type Carton struct {
	Kind     string `json:"kind"` // track, show, zap, verdict-up, verdict-down
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Detail   string `json:"detail"`
	Cover    string `json:"cover"`   // path of a cached cover PNG, "" for the glyph tile
	Station  string `json:"station"` // display name
	Color    string `json:"color"`   // #rrggbb
}

// JSON encodes the carton for the bridge.
func (c Carton) JSON() []byte {
	b, _ := json.Marshal(c)
	return b
}

// AudioDevice is one output Réglages offers.
type AudioDevice struct {
	Name  string `json:"name"` // mpv's device name, "auto" for the default
	Label string `json:"label"`
}

// Settings is what the back of the card (Réglages) shows. It only displays: every change
// goes back as an action and comes round again in the next snapshot.
type Settings struct {
	HiFi              bool          `json:"hifi"`
	Crossfade         int           `json:"crossfade"` // seconds, 0 = off
	Amp               int           `json:"amp"`       // 0 off, 1 doux, 2 costaud
	AudioDevice       string        `json:"audio_device"`
	AudioDevices      []AudioDevice `json:"audio_devices"`
	Notifications     bool          `json:"notifications"`
	ShowNotifications bool          `json:"show_notifications"`
	ShowCalendar      bool          `json:"show_calendar"`
	AnimatedIcon      bool          `json:"animated_icon"`
	PlayOnStart       bool          `json:"play_on_start"`
	History           bool          `json:"history"`
	UpdateStartup     bool          `json:"update_startup"`
	Version           string        `json:"version"`
}

// JSON encodes the snapshot for the bridge. A nil slice is sent as an empty
// list: Swift decodes a non-optional array and rejects null.
func (s Snapshot) JSON() []byte {
	if s.Stations == nil {
		s.Stations = []Station{}
	}
	if s.Settings.AudioDevices == nil {
		s.Settings.AudioDevices = []AudioDevice{}
	}
	b, _ := json.Marshal(s) // plain fields only: cannot fail
	return b
}
