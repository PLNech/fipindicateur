// Package ui builds the system-tray menu and wires together the player,
// metadata, MPRIS and notifications.
package ui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/metadata"
	"github.com/PLNech/fipindicateur/internal/mpris"
	"github.com/PLNech/fipindicateur/internal/notify"
	"github.com/PLNech/fipindicateur/internal/open"
	"github.com/PLNech/fipindicateur/internal/player"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/stats"
	"github.com/PLNech/fipindicateur/internal/version"
	"github.com/PLNech/fipindicateur/internal/wiki"
)

const (
	repoURL      = "https://github.com/PLNech/fipindicateur"
	fipURL       = "https://www.radiofrance.fr/fip"
	historySlots = 10
)

// App holds the running application state.
type App struct {
	cfg     config.Config
	player  *player.MPV
	meta    *metadata.Manager
	mpris   *mpris.Instance
	notif   *notify.Notifier
	current stations.Station

	mu      sync.Mutex
	now     metadata.NowPlaying
	history []metadata.NowPlaying

	watchCancel context.CancelFunc

	histPath string // resolved once; empty if unresolvable
	anim     animator
	wiki     *wiki.Resolver
	rec      *events.Recorder

	statsClearArmed bool // two-click confirm state for "Effacer les statistiques"

	// menu items
	mNow        *systray.MenuItem
	mVoirWiki   *systray.MenuItem
	mVoirLink   *systray.MenuItem
	mPlay       *systray.MenuItem
	stationMI   map[string]*systray.MenuItem
	histMI      []*systray.MenuItem
	mHiFi       *systray.MenuItem
	mNotif      *systray.MenuItem
	mAuto       *systray.MenuItem
	mHistFile   *systray.MenuItem
	mAnim       *systray.MenuItem
	mStats      *systray.MenuItem
	mStatsClear *systray.MenuItem
	mVolume     *systray.MenuItem
	mMute       *systray.MenuItem
	volMI       map[int]*systray.MenuItem
}

// volumePresets are the quick-pick volume levels in the tray menu.
var volumePresets = []int{10, 25, 50, 75, 100}

// New returns an App with loaded config.
func New() *App {
	cfg := config.Load()
	return &App{
		cfg:       cfg,
		meta:      metadata.NewManager(),
		wiki:      wiki.NewResolver(),
		rec:       events.NewRecorder(cfg.Stats),
		stationMI: map[string]*systray.MenuItem{},
	}
}

// OnReady is the systray onReady callback: it builds everything and starts
// playing the last station.
func (a *App) OnReady() {
	a.current = stations.ByKey(a.cfg.Station)

	a.player = &player.MPV{
		TitleChanged: a.meta.PushTitle,
		// ao-volume/ao-mute observers: external pavucontrol/GNOME changes
		// flow back into the menu and MPRIS.
		VolumeChanged: a.onExternalVolume,
		MuteChanged:   a.onExternalMute,
		// On restart we READ the stream state and sync the UI. PulseAudio
		// owns volume persistence; we never restore a stored level onto it.
		PlaybackRestarted: a.onPlaybackRestart,
	}
	if err := a.player.Initialize(); err != nil {
		log.Fatalf("ui: player init: %v", err)
	}

	if ins, err := mpris.Connect(a); err != nil {
		if errors.Is(err, mpris.ErrAlreadyRunning) {
			// Single-instance guard: an activities/menu launch while the app
			// runs must not spawn a second tray icon.
			log.Printf("another instance is already running, exiting")
			a.player.Close()
			os.Exit(0)
		}
		log.Printf("ui: mpris unavailable: %v", err)
	} else {
		a.mpris = ins
	}

	a.notif = notify.New()
	a.anim.app = a

	a.buildMenu()
	if a.mpris != nil {
		a.mpris.SetVolume(float64(a.cfg.Volume) / 100)
	}
	a.applyIcon()
	a.rec.Record(events.Event{Kind: events.KindAppStart, Station: a.current.Key})
	a.startStation(a.current, true)
}

// OnExit tears everything down.
func (a *App) OnExit() {
	a.rec.Record(events.Event{Kind: events.KindAppStop, Station: a.current.Key})
	a.rec.Close() // flushes the queued app_stop before we return
	a.anim.stop()
	if a.watchCancel != nil {
		a.watchCancel()
	}
	if a.player != nil {
		a.player.Close()
	}
	if a.mpris != nil {
		a.mpris.Close()
	}
	if a.notif != nil {
		a.notif.Close()
	}
}

func (a *App) buildMenu() {
	a.mNow = systray.AddMenuItem("FIP", "Titre en cours : cliquer pour ouvrir Wikipédia")
	a.on(a.mNow, events.KindOpenWiki, a.openNow)

	voir := systray.AddMenuItem("Voir…", "Liens pour ce titre")
	a.mVoirWiki = voir.AddSubMenuItem("Wikipédia (artiste)", "Chercher l'artiste sur fr.wikipedia.org")
	a.on(a.mVoirWiki, events.KindOpenWiki, a.openNow)
	a.mVoirLink = voir.AddSubMenuItem("Écouter ailleurs (lien FIP)", "Lien musique fourni par Radio France")
	a.mVoirLink.Disable()
	a.on(a.mVoirLink, events.KindOpenLink, a.openNowLink)

	systray.AddSeparator()
	a.mPlay = systray.AddMenuItem("⏸ Pause", "Lecture / pause")
	// Play/pause is state-dependent: setPlayingUI records the resulting
	// play/pause event (so media keys and MPRIS are captured too), hence "".
	a.on(a.mPlay, "", a.togglePlay)

	// Volume
	a.mVolume = systray.AddMenuItem(volumeLabel(a.cfg.Volume), "Volume de lecture")
	a.mMute = a.mVolume.AddSubMenuItemCheckbox("Muet", "Couper le son", a.cfg.Mute)
	a.on(a.mMute, "", a.toggleMute) // toggleMute records the resulting state
	a.volMI = map[int]*systray.MenuItem{}
	for _, pct := range volumePresets {
		it := a.mVolume.AddSubMenuItemCheckbox(fmt.Sprintf("%d %%", pct), "", pct == a.cfg.Volume)
		a.volMI[pct] = it
		p := pct
		a.on(it, "", func() { a.setVolume(p) }) // setVolume records the level
	}

	// Radios
	radios := systray.AddMenuItem("Radios", "Choisir une webradio")
	for _, s := range stations.All {
		it := radios.AddSubMenuItemCheckbox(s.Display, s.Slug, s.Key == a.current.Key)
		a.stationMI[s.Key] = it
		key := s.Key
		a.on(it, "", func() { a.setStation(key) }) // startStation records the from->to transition
	}
	fipItem := radios.AddSubMenuItem("FIP sur radiofrance.fr", fipURL)
	a.on(fipItem, events.KindOpenFip, func() { open.URL(fipURL) })

	// Historique
	hist := systray.AddMenuItem("Historique", "Titres récents")
	a.histMI = make([]*systray.MenuItem, historySlots)
	for i := 0; i < historySlots; i++ {
		it := hist.AddSubMenuItem("", "")
		it.Hide()
		a.histMI[i] = it
		idx := i
		a.on(it, events.KindOpenHistory, func() { a.openHistory(idx) })
	}

	// Réglages
	settings := systray.AddMenuItem("Réglages", "Options")
	a.mHiFi = settings.AddSubMenuItemCheckbox("Haute qualité (AAC 192k)", "", a.cfg.HiFi)
	a.on(a.mHiFi, "", a.toggleHiFi)
	a.mNotif = settings.AddSubMenuItemCheckbox("Notifications", "", a.cfg.Notifications)
	a.on(a.mNotif, "", a.toggleNotif)
	// Launch at login is XDG-only (writes ~/.config/autostart/*.desktop); hide
	// it where config.SetAutostart is a no-op (macOS and other non-Linux).
	if config.AutostartSupported {
		a.mAuto = settings.AddSubMenuItemCheckbox("Lancer au démarrage", "", a.cfg.Autostart)
		a.on(a.mAuto, "", a.toggleAutostart)
	}
	a.mHistFile = settings.AddSubMenuItemCheckbox("Historique local (fichier)", "Journal des titres dans ~/.local/share/fipindicateur/history.jsonl", a.cfg.HistoryFile)
	a.on(a.mHistFile, "", a.toggleHistFile)
	a.mAnim = settings.AddSubMenuItemCheckbox("Icône animée", "Barres qui suivent le niveau audio", a.cfg.AnimatedIcon)
	a.on(a.mAnim, "", a.toggleAnim)

	// Statistiques d'écoute: opt-in (default off), local-only. The toggle
	// gates the recorder; the submenu lets you see, locate and delete the data.
	a.mStats = settings.AddSubMenuItemCheckbox("Statistiques d'écoute (local)", "Journal d'actions local pour vos statistiques (events.jsonl)", a.cfg.Stats)
	a.on(a.mStats, "", a.toggleStats)
	statsMenu := settings.AddSubMenuItem("Statistiques", "Voir et gérer vos statistiques d'écoute")
	mStatsView := statsMenu.AddSubMenuItem("Voir le rapport", "Ouvrir le rapport d'écoute dans le navigateur")
	a.on(mStatsView, events.KindStatsView, a.viewStats)
	mStatsFolder := statsMenu.AddSubMenuItem("Ouvrir le dossier de données", "Dossier ~/.local/share/fipindicateur")
	a.on(mStatsFolder, "", a.openDataDir)
	a.mStatsClear = statsMenu.AddSubMenuItem("Effacer les statistiques…", "Supprimer events.jsonl (l'historique des titres n'est pas touché)")
	a.on(a.mStatsClear, "", a.clearStatsConfirm)

	systray.AddSeparator()
	about := systray.AddMenuItem("À propos", "Ouvrir la page du projet")
	a.on(about, events.KindOpenAbout, func() { open.URL(repoURL) })
	ver := systray.AddMenuItem("le fipindicateur "+version.String(), "Version installée")
	ver.Disable()
	quit := systray.AddMenuItem("Quitter", "Fermer le fipindicateur")
	a.on(quit, events.KindQuit, func() { systray.Quit() })

	systray.SetTitle("")
	systray.SetTooltip("le fipindicateur")
}

// on wires a menu item's click to fn and, for a fixed-kind action, records the
// event automatically. This is the "measurable by design" chokepoint: every
// clickable item goes through here, so an action cannot be added without a
// telemetry decision. A kind of "" means the handler records its own event at
// source (state-dependent actions like play/pause, volume, station change).
//
// Invariant (enforced by TestSingleOnClickCallSite): the only onClick loop in
// this package lives below. Add clickable items via a.on, never onClick.
func (a *App) on(mi *systray.MenuItem, kind events.Kind, fn func()) {
	go a.onClick(mi.ClickedCh, func() {
		if kind != "" {
			a.rec.Record(events.Event{Kind: kind, Station: a.current.Key})
		}
		fn()
	})
}

// onClick loops over a menu item's click channel, running fn each time.
func (a *App) onClick(ch <-chan struct{}, fn func()) {
	for range ch {
		fn()
	}
}

// startStation switches to a station: stop, load new URL, restart metadata.
// A change of station (from != to) is recorded as the Markov transition edge;
// the initial start (from == to) is not a transition.
func (a *App) startStation(s stations.Station, play bool) {
	if a.watchCancel != nil {
		a.watchCancel()
	}
	if a.current.Key != s.Key {
		a.rec.Record(events.Event{Kind: events.KindStationChange, From: a.current.Key, To: s.Key})
	}
	a.current = s

	url := s.StreamURL(a.quality())
	if play {
		a.player.Play(url)
	} else {
		a.player.Stop()
	}
	a.setPlayingUI(play)

	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	updates := a.meta.Watch(ctx, s)
	go func() {
		for np := range updates {
			a.onNowPlaying(np)
		}
	}()
}

func (a *App) quality() stations.Quality {
	if a.cfg.HiFi {
		return stations.Hifi
	}
	return stations.Midfi
}

// onNowPlaying handles a metadata update.
func (a *App) onNowPlaying(np metadata.NowPlaying) {
	if np.Empty() {
		return
	}
	a.mu.Lock()
	changed := np.Artist != a.now.Artist || np.Title != a.now.Title
	a.now = np
	if changed {
		a.pushHistoryLocked(np)
	}
	a.mu.Unlock()

	label := np.Title
	if np.Artist != "" {
		label = np.Artist + " · " + np.Title
	}
	if changed {
		log.Printf("now playing [%s]: %s", a.current.Key, label)
	}
	a.mNow.SetTitle(label)
	a.mNow.SetTooltip(label)

	if np.Link != "" {
		a.mVoirLink.Enable()
	} else {
		a.mVoirLink.Disable()
	}

	if a.mpris != nil {
		a.mpris.UpdateMetadata(np)
	}
	if changed {
		a.refreshHistoryMenu()
		if a.cfg.Notifications {
			a.notify(np)
		}
		if a.cfg.HistoryFile {
			a.appendHistFile(np)
		}
	}
}

// appendHistFile writes the track to the local jsonl log. Best-effort: any
// error is logged once and never affects playback.
func (a *App) appendHistFile(np metadata.NowPlaying) {
	if a.histPath == "" {
		p, err := histlog.DefaultPath()
		if err != nil {
			log.Printf("ui: history file path: %v", err)
			return
		}
		a.histPath = p
	}
	err := histlog.Append(a.histPath, histlog.Entry{
		Station: a.current.Key,
		Artist:  np.Artist,
		Title:   np.Title,
		Album:   np.Album,
		Year:    np.Year,
		Label:   np.Label,
	})
	if err != nil {
		log.Printf("ui: history file append: %v", err)
	}
}

func (a *App) notify(np metadata.NowPlaying) {
	summary := np.Title
	body := np.Artist
	if np.Album != "" {
		extra := np.Album
		if np.Year > 0 {
			extra = fmt.Sprintf("%s (%d)", np.Album, np.Year)
		}
		body = np.Artist + " · " + extra
		if np.Label != "" {
			body += " · " + np.Label
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	iconPath := a.notif.FetchCover(ctx, np.CoverURL)
	a.notif.Notify(summary, body, iconPath, a.cfg.NotifTimeoutMs)
}

// pushHistoryLocked prepends np to the history ring (caller holds a.mu).
func (a *App) pushHistoryLocked(np metadata.NowPlaying) {
	a.history = append([]metadata.NowPlaying{np}, a.history...)
	if len(a.history) > historySlots {
		a.history = a.history[:historySlots]
	}
}

// refreshHistoryMenu updates the pre-allocated hidden slots.
func (a *App) refreshHistoryMenu() {
	a.mu.Lock()
	hist := make([]metadata.NowPlaying, len(a.history))
	copy(hist, a.history)
	a.mu.Unlock()

	for i, it := range a.histMI {
		if i < len(hist) {
			np := hist[i]
			label := np.Title
			if np.Artist != "" {
				label = np.Artist + " · " + np.Title
			}
			it.SetTitle(label)
			it.Show()
		} else {
			it.Hide()
		}
	}
}

// --- click handlers ---

func (a *App) togglePlay() {
	if a.player.IsPlaying() {
		a.player.Stop()
		a.setPlayingUI(false)
	} else {
		a.player.Play(a.current.StreamURL(a.quality()))
		a.setPlayingUI(true)
	}
}

func (a *App) setPlayingUI(playing bool) {
	// The single chokepoint for playback state: menu toggle, MPRIS, media keys
	// and station switches all pass through here, so recording play/pause here
	// captures every source. Redundant same-state events are deduplicated in
	// the stats derivation.
	if playing {
		a.rec.Record(events.Event{Kind: events.KindPlay, Station: a.current.Key})
	} else {
		a.rec.Record(events.Event{Kind: events.KindPause, Station: a.current.Key})
	}
	if playing {
		a.mPlay.SetTitle("⏸ Pause")
	} else {
		a.mPlay.SetTitle("▶ Play")
	}
	if playing {
		// Static icon first (in case animation is off or breaks), then the
		// animator takes over within one frame if enabled.
		a.applyIconState(false)
		a.anim.start()
	} else {
		a.anim.stop()
		a.applyIconState(true)
	}
	if a.mpris != nil {
		a.mpris.SetPlaybackStatus(playing)
	}
}

// b2i maps a toggle's resulting state to the event Value field (1 on, 0 off).
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (a *App) toggleAnim() {
	a.cfg.AnimatedIcon = !a.cfg.AnimatedIcon
	if a.cfg.AnimatedIcon {
		a.mAnim.Check()
		if a.player.IsPlaying() {
			a.anim.start()
		}
	} else {
		a.mAnim.Uncheck()
		a.anim.stop()
		a.applyIconState(!a.player.IsPlaying())
	}
	a.rec.Record(events.Event{Kind: events.KindAnim, Value: b2i(a.cfg.AnimatedIcon)})
	a.save()
}

func (a *App) setStation(key string) {
	s := stations.ByKey(key)
	if s.Key == a.current.Key && a.player.IsPlaying() {
		return
	}
	for k, it := range a.stationMI {
		if k == key {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
	a.cfg.Station = key
	a.save()
	a.startStation(s, true)
}

func (a *App) toggleHiFi() {
	a.cfg.HiFi = !a.cfg.HiFi
	if a.cfg.HiFi {
		a.mHiFi.Check()
	} else {
		a.mHiFi.Uncheck()
	}
	a.rec.Record(events.Event{Kind: events.KindHiFi, Value: b2i(a.cfg.HiFi)})
	a.save()
	// Reload the stream at the new quality if playing.
	if a.player.IsPlaying() {
		a.player.Play(a.current.StreamURL(a.quality()))
	}
}

func (a *App) toggleNotif() {
	a.cfg.Notifications = !a.cfg.Notifications
	if a.cfg.Notifications {
		a.mNotif.Check()
	} else {
		a.mNotif.Uncheck()
	}
	a.rec.Record(events.Event{Kind: events.KindNotif, Value: b2i(a.cfg.Notifications)})
	a.save()
}

// --- volume ---

func volumeLabel(pct int) string {
	return fmt.Sprintf("Volume (%d %%)", pct)
}

// applyVolumeUI syncs the volume submenu (title, preset checkmarks, mute)
// with the current config.
func (a *App) applyVolumeUI() {
	a.mVolume.SetTitle(volumeLabel(a.cfg.Volume))
	for pct, it := range a.volMI {
		if pct == a.cfg.Volume {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
	if a.cfg.Mute {
		a.mMute.Check()
	} else {
		a.mMute.Uncheck()
	}
}

// setVolume applies a menu-selected volume preset.
func (a *App) setVolume(pct int) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if pct != a.cfg.Volume {
		a.cfg.Volume = pct
		a.rec.Record(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: pct})
		a.save()
		// ao-volume applies only while the AO is open; when it is not (e.g.
		// paused), the persisted value is applied on the next playback
		// restart. Either way the menu reflects the chosen level now.
		a.player.SetVolume(float64(pct))
		if a.mpris != nil {
			a.mpris.SetVolume(float64(pct) / 100)
		}
	}
	a.applyVolumeUI()
}

func (a *App) toggleMute() {
	a.cfg.Mute = !a.cfg.Mute
	a.rec.Record(events.Event{Kind: events.KindMute, Station: a.current.Key, Value: b2i(a.cfg.Mute)})
	a.save()
	a.player.SetMute(a.cfg.Mute)
	a.applyVolumeUI()
}

// onPlaybackRestart READS the PulseAudio stream state and syncs the UI to it.
// PulseAudio (module-stream-restore) is the single source of truth for the
// per-app volume: it remembers the level across app restarts, including any
// live pavucontrol adjustment. We never write a stored volume onto the
// stream here; an earlier version did and stomped the user's duck during a
// call. Config only caches the last-known level for pre-playback display.
func (a *App) onPlaybackRestart() {
	if v, ok := a.player.Volume(); ok {
		a.onExternalVolume(v)
	}
	if mu, ok := a.player.Mute(); ok {
		a.onExternalMute(mu)
	}
}

// onExternalVolume handles an ao-volume observer event: the PulseAudio stream
// volume changed, possibly from pavucontrol/GNOME (or as the echo of our own
// set, which the equal-value guard swallows). Syncs config, menu and MPRIS.
func (a *App) onExternalVolume(v float64) {
	pct := clampPct(int(math.Round(v)))
	if pct == a.cfg.Volume {
		return
	}
	a.cfg.Volume = pct
	a.rec.Record(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: pct})
	a.save()
	a.applyVolumeUI()
	if a.mpris != nil {
		a.mpris.SetVolume(float64(pct) / 100)
	}
}

// onExternalMute handles an ao-mute observer event (pavucontrol mute button).
func (a *App) onExternalMute(mute bool) {
	if mute == a.cfg.Mute {
		return
	}
	a.cfg.Mute = mute
	a.rec.Record(events.Event{Kind: events.KindMute, Station: a.current.Key, Value: b2i(mute)})
	a.save()
	a.applyVolumeUI()
}

// SetVolumeFrac implements mpris.Controller: an external client (playerctl,
// GNOME) wrote the Volume property. Reflect it in player, config and menu.
// The equal-value early return breaks any publish/callback echo loop.
func (a *App) SetVolumeFrac(v float64) {
	pct := clampPct(int(math.Round(v * 100)))
	if pct == a.cfg.Volume {
		return
	}
	a.cfg.Volume = pct
	a.rec.Record(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: pct})
	a.save()
	a.player.SetVolume(float64(pct))
	a.applyVolumeUI()
}

func clampPct(pct int) int {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func (a *App) toggleHistFile() {
	a.cfg.HistoryFile = !a.cfg.HistoryFile
	if a.cfg.HistoryFile {
		a.mHistFile.Check()
	} else {
		a.mHistFile.Uncheck()
	}
	a.save()
}

func (a *App) toggleAutostart() {
	a.cfg.Autostart = !a.cfg.Autostart
	if a.cfg.Autostart {
		a.mAuto.Check()
	} else {
		a.mAuto.Uncheck()
	}
	if err := config.SetAutostart(a.cfg.Autostart); err != nil {
		log.Printf("ui: autostart: %v", err)
	}
	a.rec.Record(events.Event{Kind: events.KindAutostart, Value: b2i(a.cfg.Autostart)})
	a.save()
}

// --- statistics (opt-in listening analytics) ---

// toggleStats flips the opt-in and starts/stops the recorder at runtime. The
// stats_toggle event is recorded while recording is still active on the edge
// that keeps it (on the way on: after enabling; on the way off: before
// disabling), so the toggle itself always lands in the log.
func (a *App) toggleStats() {
	a.cfg.Stats = !a.cfg.Stats
	if a.cfg.Stats {
		a.mStats.Check()
		a.rec.SetEnabled(true)
		a.rec.Record(events.Event{Kind: events.KindStatsToggle, Station: a.current.Key, Value: 1})
	} else {
		a.rec.Record(events.Event{Kind: events.KindStatsToggle, Station: a.current.Key, Value: 0})
		a.mStats.Uncheck()
		a.rec.SetEnabled(false)
	}
	a.save()
}

// viewStats builds the local report and opens it in the browser. It runs in a
// goroutine so the tray never blocks on report generation or the short-lived
// HTTP server. Works whether or not recording is currently enabled, as long as
// a log exists (you can view, then delete).
func (a *App) viewStats() {
	go func() {
		html, _, err := stats.Generate(time.Now())
		if err != nil {
			log.Printf("ui: stats generate: %v", err)
			return
		}
		if err := stats.ServeAndOpen(html); err != nil {
			log.Printf("ui: stats serve: %v", err)
		}
	}()
}

// openDataDir opens the data folder (events.jsonl, history.jsonl) in the file
// manager. file:// on a directory is handled by xdg-open (nautilus etc.).
func (a *App) openDataDir() {
	dir, err := events.DataDir()
	if err != nil {
		log.Printf("ui: data dir: %v", err)
		return
	}
	open.URL("file://" + dir)
}

// clearStatsConfirm deletes events.jsonl with a two-click confirmation (a tray
// menu has no dialog). First click arms and relabels; a second click within a
// short window deletes. It removes only events.jsonl, never history.jsonl.
func (a *App) clearStatsConfirm() {
	a.mu.Lock()
	armed := a.statsClearArmed
	a.statsClearArmed = !armed // arm on first click, disarm on the confirming click
	a.mu.Unlock()

	if !armed {
		a.mStatsClear.SetTitle("Confirmer l'effacement ?")
		go func() {
			time.Sleep(5 * time.Second)
			a.mu.Lock()
			still := a.statsClearArmed
			a.statsClearArmed = false
			a.mu.Unlock()
			if still {
				a.mStatsClear.SetTitle("Effacer les statistiques…")
			}
		}()
		return
	}

	a.mStatsClear.SetTitle("Effacer les statistiques…")
	if err := a.rec.Clear(); err != nil {
		log.Printf("ui: stats clear: %v", err)
		return
	}
	if a.notif != nil {
		a.notif.Notify("Statistiques effacées", "Le journal events.jsonl a été supprimé.", "", a.cfg.NotifTimeoutMs)
	}
}

func (a *App) openNow() {
	a.mu.Lock()
	np := a.now
	a.mu.Unlock()
	a.openTrack(np)
}

func (a *App) openHistory(i int) {
	a.mu.Lock()
	var np metadata.NowPlaying
	if i < len(a.history) {
		np = a.history[i]
	}
	a.mu.Unlock()
	a.openTrack(np)
}

// openTrack opens the primary link for a track: the artist's Wikipedia
// article, resolved via opensearch on fr.wp then en.wp, falling back to the
// fr.wp search page (never a dead end). Resolution uses the cleaned primary
// artist (highlightedArtists or the credit cut at the first separator) and
// runs in a goroutine so the menu never blocks on the network. DuckDuckGo is
// the fallback when no artist is known at all. The metadata Link (often Apple
// Music) stays available as the secondary "Voir…" item.
func (a *App) openTrack(np metadata.NowPlaying) {
	if np.Empty() {
		return
	}
	artist := np.PrimaryArtist
	if artist == "" {
		artist = np.Artist
	}
	if artist == "" {
		open.URL(open.Search(np.Title))
		return
	}
	go func() {
		open.URL(a.wiki.ArtistURL(context.Background(), artist))
	}()
}

// openNowLink opens the current track's Radio France music link, if any.
func (a *App) openNowLink() {
	a.mu.Lock()
	link := a.now.Link
	a.mu.Unlock()
	open.URL(link)
}

func (a *App) applyIcon()                 { systray.SetIcon(icon.Active(false)) }
func (a *App) applyIconState(paused bool) { systray.SetIcon(icon.Active(paused)) }

func (a *App) save() {
	if err := a.cfg.Save(); err != nil {
		log.Printf("ui: save config: %v", err)
	}
}

// --- mpris.Controller ---

// Play resumes playback (rejoins live).
func (a *App) Play() {
	a.player.Play(a.current.StreamURL(a.quality()))
	a.setPlayingUI(true)
}

// Pause stops playback (full stop for live radio).
func (a *App) Pause() {
	a.player.Stop()
	a.setPlayingUI(false)
}

// Toggle flips play/pause.
func (a *App) Toggle() { a.togglePlay() }
