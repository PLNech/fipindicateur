// Package config persists user settings to ~/.config/fipindicateur/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the persisted user state.
type Config struct {
	Station        string `json:"station"`          // station key
	HiFi           bool   `json:"hifi"`             // true = AAC 192k
	Notifications  bool   `json:"notifications"`    // desktop notifications
	NotifTimeoutMs int    `json:"notif_timeout_ms"` // notification expire hint (GNOME ignores it; dunst/KDE honor it)
	Autostart      bool   `json:"autostart"`        // launch at login
	HistoryFile    bool   `json:"history_file"`     // append track changes to a local jsonl log
	Stats          bool   `json:"stats"`            // opt-in local listening analytics (events.jsonl)
	UpdateStartup  bool   `json:"update_startup"`   // check GitHub for a newer release at launch (opt-in)
	AnimatedIcon   bool   `json:"animated_icon"`    // audio-responsive VU tray icon
	// AudioDevice is the mpv audio-device name (empty = mpv "auto", i.e. the
	// system default output). Persisted so a chosen sink survives restarts.
	AudioDevice string `json:"audio_device"`
	// Volume/Mute cache the last-known PulseAudio stream state for
	// pre-playback DISPLAY only. PulseAudio (module-stream-restore) is the
	// single source of truth: these values are never written onto the audio
	// stream except on an explicit user action (menu preset, Muet, MPRIS).
	Volume int  `json:"volume"` // last-known stream volume percent (0..100)
	Mute   bool `json:"mute"`   // last-known stream mute state
}

// Default returns the initial config: FIP, midfi, notifications on.
func Default() Config {
	return Config{
		Station:        "fip",
		HiFi:           false,
		Notifications:  true,
		NotifTimeoutMs: 10000,
		Autostart:      false,
		HistoryFile:    false,
		Stats:          false,
		UpdateStartup:  false,
		AnimatedIcon:   true,
		Volume:         100,
		Mute:           false,
	}
}

// Dir returns the config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "fipindicateur")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config, returning defaults if the file is absent or invalid.
func Load() Config {
	c := Default()
	p, err := path()
	if err != nil {
		return c
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c) // tolerate partial/invalid: keep defaults
	if c.Station == "" {
		c.Station = "fip"
	}
	if c.NotifTimeoutMs <= 0 {
		c.NotifTimeoutMs = Default().NotifTimeoutMs
	}
	if c.Volume < 0 || c.Volume > 100 {
		c.Volume = Default().Volume
	}
	return c
}

// Save writes the config atomically.
func (c Config) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
