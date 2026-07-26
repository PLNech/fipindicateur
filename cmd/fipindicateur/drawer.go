package main

import (
	"fmt"
	"os"

	"github.com/PLNech/fipindicateur/internal/drawer"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/version"
)

// runDrawer is the `drawer` dev subcommand: it opens « le panneau » standalone
// with a plausible mock state, so its design can be iterated on without a
// running tray, a stream or a Chromecast. Commands from the page are echoed
// to stdout and applied to the mock state (so the whole UI is clickable), and
// the process blocks until the window closes (Escape or the ✕ button).
func runDrawer() int {
	if !drawer.Available {
		fmt.Fprintln(os.Stderr, "le panneau n'est disponible que sous Linux")
		return 1
	}

	chips := make([]drawer.Station, len(stations.All))
	for i, s := range stations.All {
		chips[i] = drawer.Station{Key: s.Key, Display: s.Display, Color: s.Color}
	}
	state := drawer.State{
		Dark:    drawer.DarkPreferred(),
		Station: "groove",
		Playing: true,
		Volume:  64,
		Devices: []string{"Ampli du salon", "Enceinte cuisine"},
		Track: drawer.Track{
			Title:  "Un jour comme un autre (Bonnie & Clyde)",
			Artist: "Brigitte Bardot · Serge Gainsbourg",
		},
		Stations: chips,
		Settings: drawer.Settings{
			Stats:              true,
			HiFi:               true,
			Notifications:      true,
			PlayOnStart:        false,
			Autostart:          false,
			AutostartSupported: true,
		},
		History: []drawer.HistoryEntry{
			{Title: "Un jour comme un autre (Bonnie & Clyde)", Artist: "Brigitte Bardot · Serge Gainsbourg"},
			{Title: "Alright", Artist: "Kendrick Lamar"},
			{Title: "Água de Beber", Artist: "Astrud Gilberto · Antônio Carlos Jobim"},
			{Title: "Le sud", Artist: "Nino Ferrer"},
			{Title: "Pull Up", Artist: "Koffee"},
		},
		Version: version.String(),
	}

	done := make(chan struct{})
	var d *drawer.Drawer
	d = drawer.New(func(c drawer.Command) {
		fmt.Printf("commande: %+v\n", c)
		// Apply plausible mutations so every control visibly reacts.
		switch c.Action {
		case "toggle_play":
			if state.Cast.Active {
				state.Cast.Playing = !state.Cast.Playing
			} else {
				state.Playing = !state.Playing
			}
		case "volume":
			if state.Cast.Active {
				state.Cast.Volume = c.Value
			} else {
				state.Volume = c.Value
			}
		case "toggle_mute":
			if state.Cast.Active {
				state.Cast.Muted = !state.Cast.Muted
			} else {
				state.Muted = !state.Muted
			}
		case "station":
			state.Station = c.Key
		case "output":
			if c.Value >= 0 && c.Value < len(state.Devices) {
				// Mock a Pioneer-style device: master control, its own level.
				state.Cast = drawer.Cast{
					Active: true, DeviceName: state.Devices[c.Value], Playing: true,
					Volume: 34, VolumeKnown: true, ControlType: "master",
				}
			} else {
				state.Cast = drawer.Cast{}
			}
		case "rescan":
			state.Scanning = !state.Scanning // toggle so the affordance is visible
		case "toggle_stats":
			state.Settings.Stats = !state.Settings.Stats
		case "toggle_hifi":
			state.Settings.HiFi = !state.Settings.HiFi
		case "toggle_notif":
			state.Settings.Notifications = !state.Settings.Notifications
		case "toggle_play_on_start":
			state.Settings.PlayOnStart = !state.Settings.PlayOnStart
		case "toggle_autostart":
			state.Settings.Autostart = !state.Settings.Autostart
			// like/dislike, stats_view/_folder/_clear, prefs_clear, update_check,
			// open_fip/open_github and quit are echoed above: enough to verify the
			// wiring without a running tray.
		}
		d.Push(state)
	}, func() { close(done) })

	if err := d.Show(state); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	<-done
	return 0
}
