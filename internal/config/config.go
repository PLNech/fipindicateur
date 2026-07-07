// Package config persists user settings to ~/.config/fipindicateur/config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the persisted user state.
type Config struct {
	Station       string `json:"station"`       // station key
	HiFi          bool   `json:"hifi"`          // true = AAC 192k
	Notifications bool   `json:"notifications"` // desktop notifications
	Autostart     bool   `json:"autostart"`     // launch at login
}

// Default returns the initial config: FIP, midfi, notifications on.
func Default() Config {
	return Config{
		Station:       "fip",
		HiFi:          false,
		Notifications: true,
		Autostart:     false,
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
