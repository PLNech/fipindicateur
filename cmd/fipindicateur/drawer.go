package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/PLNech/fipindicateur/internal/drawer"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/version"
)

// mockUpcoming is the harness's « À venir » fixture, restored when the mock's
// calendar toggle comes back on (mirroring the app's gating).
var mockUpcoming = []drawer.Upcoming{
	{Time: "20:00", Title: "Club Jazzafip"},
	{Time: "22:00", Title: "Fip Tape"},
}

// mockHistory is the harness's recent-tracks fixture; the stdin driver can
// clear and restore it to exercise the empty state.
var mockHistory = []drawer.HistoryEntry{
	{Title: "Un jour comme un autre (Bonnie & Clyde)", Artist: "Brigitte Bardot · Serge Gainsbourg"},
	{Title: "Alright", Artist: "Kendrick Lamar"},
	{Title: "Água de Beber", Artist: "Astrud Gilberto · Antônio Carlos Jobim"},
	{Title: "Le sud", Artist: "Nino Ferrer"},
	{Title: "Pull Up", Artist: "Koffee"},
}

// mockState builds the harness's base fixture state: plausible, complete and
// clearly branded as fake (the footer says maquette). Shared by the design
// harness (runDrawer) and the wiring selftest's scenarios.
func mockState() drawer.State {
	chips := make([]drawer.Station, len(stations.All))
	for i, s := range stations.All {
		chips[i] = drawer.Station{Key: s.Key, Display: s.Display, Color: s.Color}
	}
	return drawer.State{
		Dark:    drawer.DarkPreferred(),
		Station: "groove",
		Playing: true,
		Volume:  64,
		Devices: []string{"Ampli du salon", "Enceinte cuisine"},
		AudioDevices: []drawer.AudioDevice{
			{Name: "auto", Label: "Automatique"},
			{Name: "pulse/alsa_output.pci", Label: "Haut-parleurs internes"},
			{Name: "pulse/bluez_output.casque", Label: "Casque Bluetooth"},
		},
		AudioDevice: "auto",
		Track: drawer.Track{
			Title:   "Un jour comme un autre (Bonnie & Clyde)",
			Artist:  "Brigitte Bardot · Serge Gainsbourg",
			HasLink: true,
		},
		Stations: chips,
		Settings: drawer.Settings{
			Stats:              true,
			HiFi:               true,
			Notifications:      true,
			ShowNotifications:  false,
			ShowCalendar:       true,
			AnimatedIcon:       true,
			HistoryFile:        true,
			UpdateStartup:      false,
			PlayOnStart:        false,
			Autostart:          false,
			AutostartSupported: true,
			CrossfadeSecs:      4,
		},
		Upcoming: mockUpcoming,
		History:  mockHistory,
		// The footer brands the harness so a leftover mock window can never
		// pass for the real panel (its devices and tracks are fixtures).
		Version: "maquette · " + version.String(),
	}
}

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

	state := mockState()

	done := make(chan struct{})
	var d *drawer.Drawer
	// mu guards state: the page's commands and the stdin driver both mutate it.
	var mu sync.Mutex
	d = drawer.New(func(c drawer.Command) {
		mu.Lock()
		defer mu.Unlock()
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
		case "toggle_show_notif":
			state.Settings.ShowNotifications = !state.Settings.ShowNotifications
		case "toggle_show_calendar":
			// Mirror drawerState's gating: the « À venir » section only gets
			// data while the calendar setting is on, so the harness can
			// exercise the empty state too.
			state.Settings.ShowCalendar = !state.Settings.ShowCalendar
			if state.Settings.ShowCalendar {
				state.Upcoming = mockUpcoming
			} else {
				state.Upcoming = nil
			}
		case "toggle_anim":
			state.Settings.AnimatedIcon = !state.Settings.AnimatedIcon
		case "toggle_hist_file":
			state.Settings.HistoryFile = !state.Settings.HistoryFile
		case "toggle_update_startup":
			state.Settings.UpdateStartup = !state.Settings.UpdateStartup
		case "crossfade":
			state.Settings.CrossfadeSecs = c.Value
		case "audio_device":
			state.AudioDevice = c.Key
			state.Cast = drawer.Cast{} // picking a local sink brings playback home
			// like/dislike, open_wiki/_link/_history, stats_view/_folder/_clear,
			// prefs_clear, update_check, restart, open_fip/open_github and quit
			// are echoed above: enough to verify the wiring without a running tray.
		}
		d.Push(state)
	}, func() { close(done) })

	if err := d.Show(state); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Brand the harness window too (belt to the footer's suspenders): a
	// leftover mock must never be mistaken for the real panel.
	d.SetWindowTitle("le panneau · FIP (maquette)")

	// The stdin driver: lets design QA switch views and toggle fixture states
	// without a pointer (`echo view history | ...`, or a tail -f pipe for a
	// whole scripted session). One command per line; unknown lines are echoed.
	//
	//	view main|history|settings   switch the page view
	//	hist on|off                  fill or empty the recent-tracks fixture
	//	upcoming on|off              fill or empty the « À venir » fixture
	//	eval <js>                    run a script in the page (debug probe)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) < 2 {
				continue
			}
			if f[0] == "eval" {
				d.Eval(strings.TrimSpace(strings.TrimPrefix(sc.Text(), "eval")))
				continue
			}
			mu.Lock()
			switch f[0] {
			case "view":
				d.SetView(f[1])
			case "hist":
				if f[1] == "on" {
					state.History = mockHistory
				} else {
					state.History = nil
				}
			case "upcoming":
				if f[1] == "on" {
					state.Upcoming = mockUpcoming
				} else {
					state.Upcoming = nil
				}
			default:
				fmt.Printf("pilote: commande inconnue %q\n", f[0])
			}
			d.Push(state)
			mu.Unlock()
		}
	}()

	<-done
	return 0
}
