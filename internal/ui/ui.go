// Package ui builds the system-tray menu and wires together the player,
// metadata, MPRIS and notifications.
package ui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/stats"
	"github.com/PLNech/fipindicateur/internal/update"
	"github.com/PLNech/fipindicateur/internal/version"
	"github.com/PLNech/fipindicateur/internal/wiki"
)

const (
	repoURL      = "https://github.com/PLNech/fipindicateur"
	fipURL       = "https://www.radiofrance.fr/fip"
	historySlots = 10
	// calendarSlots bounds how many upcoming programmes the tray lists. A single
	// poll returns two to three days ahead, far more than a menu should show.
	calendarSlots = 12
)

// App holds the running application state.
type App struct {
	cfg     config.Config
	player  *player.Fader
	meta    *metadata.Manager
	mpris   *mpris.Instance
	notif   *notify.Notifier
	current stations.Station

	mu       sync.Mutex
	now      metadata.NowPlaying
	history  []metadata.NowPlaying
	upcoming []metadata.Show // upcoming programmes, for the calendar submenu

	// lastShowConcept is the conceptUuid of the programme we last notified for,
	// so a show is announced once at its start and not re-announced for every
	// track it plays. Guarded by a.mu.
	lastShowConcept string

	watchCancel context.CancelFunc

	histPath  string // resolved once; empty if unresolvable
	prefsPath string // resolved once; empty if unresolvable
	anim      animator
	wiki      *wiki.Resolver
	rec       *events.Recorder

	// iconMu guards lastIcon, the last bytes handed to systray.SetIcon. The
	// static-icon path (setPlayingUI) and the animator goroutine both set the
	// tray icon; the chokepoint serializes them and skips redundant identical
	// pushes so the SNI is not churned needlessly.
	iconMu   sync.Mutex
	lastIcon []byte

	// nowThrottle coalesces "now playing" label pushes to the tray so a burst
	// of metadata updates cannot hammer the appindicator extension.
	nowThrottle *throttle

	statsClearArmed bool // two-click confirm state for "Effacer les statistiques"
	prefsClearArmed bool // two-click confirm state for "Effacer mes goûts"

	// menu items
	mNow           *systray.MenuItem
	mShow          *systray.MenuItem // current programme (émission) on air; hidden when none
	mVoirWiki      *systray.MenuItem
	mVoirLink      *systray.MenuItem
	mLike          *systray.MenuItem
	mDislike       *systray.MenuItem
	mPlay          *systray.MenuItem
	stationMI      map[string]*systray.MenuItem
	histMI         []*systray.MenuItem
	mCalendar      *systray.MenuItem   // calendar submenu container; hidden when disabled
	calMI          []*systray.MenuItem // pre-allocated calendar slots
	mHiFi          *systray.MenuItem
	mNotif         *systray.MenuItem
	mShowNotif     *systray.MenuItem
	mShowCalendar  *systray.MenuItem
	mAuto          *systray.MenuItem
	mHistFile      *systray.MenuItem
	mAnim          *systray.MenuItem
	audioMI        map[string]*systray.MenuItem // audio-output items, keyed by device name ("auto" = automatic)
	mStats         *systray.MenuItem
	mStatsClear    *systray.MenuItem
	mPrefsClear    *systray.MenuItem
	mUpdateStartup *systray.MenuItem
	mVolume        *systray.MenuItem
	mMute          *systray.MenuItem
	volMI          map[int]*systray.MenuItem
	mCrossfade     *systray.MenuItem
	crossfadeMI    map[int]*systray.MenuItem // preset checkboxes; nil in zenity-slider mode

	// dialogOpen guards against launching two zenity dialogs at once (the volume
	// slider and the crossfade slider share it). A click while one is open is
	// ignored. Guarded by a.mu.
	dialogOpen bool
}

// volumePresets are the quick-pick volume levels in the tray menu.
var volumePresets = []int{10, 25, 50, 75, 100}

// crossfadePresets are the quick-pick crossfade durations (seconds) for the
// fallback submenu shown when zenity is unavailable. 0 = hard cut.
var crossfadePresets = []int{0, 2, 4, 6}

// nowLabelMinInterval is the floor between two "now playing" label pushes to
// the tray. livemeta polls are naturally minutes apart, but an ICY burst (or a
// rapid station zap) can arrive faster; coalescing to ~1.5s keeps the SNI calm
// without a human noticing a label lag.
const nowLabelMinInterval = 1500 * time.Millisecond

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

	// Set a valid icon as the very first thing, before building the menu: the
	// StatusNotifierItem is registered by the systray runtime the instant it is
	// ready, and GNOME reads the icon pixmap immediately. Handing it real bytes
	// up front minimises the window where the SNI has a null/empty pixmap (the
	// cogl "data != NULL" assertion). setIcon guarantees the bytes are non-empty.
	a.applyIcon()

	// The tray "now playing" label goes through a dedupe+debounce throttle so a
	// metadata burst cannot churn the SNI. SetTitle and SetTooltip carry the
	// same string, so one throttle drives both.
	a.nowThrottle = newThrottle(nowLabelMinInterval, func(label string) {
		a.mNow.SetTitle(label)
		a.mNow.SetTooltip(label)
	})

	a.player = &player.Fader{
		// A live station zap crossfades over this duration (0 = hard cut).
		Crossfade:    time.Duration(a.cfg.CrossfadeSecs) * time.Second,
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
	// Restore the persisted audio sink. SetAudioDevice maps ""->"auto", so an
	// unconditional call is harmless when no device was ever chosen.
	a.player.SetAudioDevice(a.cfg.AudioDevice)

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
	// (icon already set at the top of OnReady, before the menu, to shrink the
	// null-pixmap window at SNI registration.)
	a.rec.Record(events.Event{Kind: events.KindAppStart, Station: a.current.Key})
	a.startStation(a.current, true)

	// Opt-in: one quiet update check at launch. Off by default.
	if a.cfg.UpdateStartup {
		go a.runUpdateCheck(true)
	}
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

	// The programme (émission) currently on air. Display-only (disabled), hidden
	// until a show is playing. Shows exist only on the main antenna.
	a.mShow = systray.AddMenuItem("", "Émission en cours sur l'antenne")
	a.mShow.Disable()
	a.mShow.Hide()

	voir := systray.AddMenuItem("Voir…", "Liens pour ce titre")
	a.mVoirWiki = voir.AddSubMenuItem("Wikipédia (artiste)", "Chercher l'artiste sur fr.wikipedia.org")
	a.on(a.mVoirWiki, events.KindOpenWiki, a.openNow)
	a.mVoirLink = voir.AddSubMenuItem("Écouter ailleurs (lien FIP)", "Lien musique fourni par Radio France")
	a.mVoirLink.Disable()
	a.on(a.mVoirLink, events.KindOpenLink, a.openNowLink)

	// Taste signals: an explicit verdict on the current track. Unlike the events
	// log, prefs has no opt-in gate: the click itself is the consent (see
	// internal/prefs). The a.on chokepoint records the behaviour (KindLike/
	// KindDislike, station only, no track identity); the handler snapshots the
	// track into prefs.jsonl. Both are no-ops when nothing is playing.
	a.mLike = systray.AddMenuItem("J'aime ce morceau", "Mémoriser que vous aimez ce titre (prefs.jsonl)")
	a.mLike.Disable() // enabled once a track is known (see onNowPlaying)
	a.on(a.mLike, events.KindLike, func() { a.recordTaste(prefs.Like) })
	a.mDislike = systray.AddMenuItem("Pas pour moi", "Mémoriser que ce titre n'est pas pour vous (prefs.jsonl)")
	a.mDislike.Disable()
	a.on(a.mDislike, events.KindDislike, func() { a.recordTaste(prefs.Dislike) })

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
	// A real slider, when zenity is present (Linux/GNOME). Absent on macOS/Windows
	// and minimal installs, so the item is only added when the binary exists (no
	// dead item). The resulting volume change is the measurable action, recorded
	// once at source on OK inside runVolumeSlider, so this click carries no Kind.
	if zenityAvailable() {
		slider := a.mVolume.AddSubMenuItem("Régler au curseur…", "Curseur de volume (zenity)")
		a.on(slider, "", a.openVolumeSlider)
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

	// Calendrier: the upcoming programmes on the antenna (station 7 only). The
	// slots are display-only (no click telemetry needed); the whole submenu is
	// hidden when the calendar setting is off or nothing is scheduled.
	a.mCalendar = systray.AddMenuItem("Calendrier", "Prochaines émissions sur l'antenne")
	a.calMI = make([]*systray.MenuItem, calendarSlots)
	for i := 0; i < calendarSlots; i++ {
		it := a.mCalendar.AddSubMenuItem("", "")
		it.Disable()
		it.Hide()
		a.calMI[i] = it
	}
	// Start hidden: refreshCalendarMenu reveals it once programmes are scheduled
	// and the setting is on (so the webradios never show an empty submenu).
	a.mCalendar.Hide()

	// Réglages
	settings := systray.AddMenuItem("Réglages", "Options")
	a.mHiFi = settings.AddSubMenuItemCheckbox("Haute qualité (AAC 192k)", "", a.cfg.HiFi)
	a.on(a.mHiFi, "", a.toggleHiFi)
	// Fondu enchaîné (crossfade on a live station zap). With zenity a single item
	// opens a 0..10s slider; without it, a preset-checkbox submenu is the fallback
	// (macOS / minimal installs). Either way setCrossfade records KindCrossfade at
	// source, so the a.on click carries no Kind.
	if zenityAvailable() {
		a.mCrossfade = settings.AddSubMenuItem(crossfadeTitle(a.cfg.CrossfadeSecs, true), "Durée du fondu entre stations (curseur zenity)")
		a.on(a.mCrossfade, "", a.openCrossfadeSlider)
	} else {
		a.mCrossfade = settings.AddSubMenuItem(crossfadeTitle(a.cfg.CrossfadeSecs, false), "Durée du fondu entre stations")
		a.crossfadeMI = map[int]*systray.MenuItem{}
		for _, secs := range crossfadePresets {
			it := a.mCrossfade.AddSubMenuItemCheckbox(crossfadePresetLabel(secs), "", secs == a.cfg.CrossfadeSecs)
			a.crossfadeMI[secs] = it
			s := secs
			a.on(it, "", func() { a.setCrossfade(s) }) // setCrossfade records KindCrossfade at source
		}
	}
	a.mNotif = settings.AddSubMenuItemCheckbox("Notifications", "", a.cfg.Notifications)
	a.on(a.mNotif, "", a.toggleNotif)
	a.mShowNotif = settings.AddSubMenuItemCheckbox("Notifications d'émission", "Prévenir au début d'une émission sur l'antenne", a.cfg.ShowNotifications)
	a.on(a.mShowNotif, "", a.toggleShowNotif)
	a.mShowCalendar = settings.AddSubMenuItemCheckbox("Afficher le calendrier", "Lister les prochaines émissions dans le menu", a.cfg.ShowCalendar)
	a.on(a.mShowCalendar, "", a.toggleShowCalendar)
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

	// Sortie audio: pick the output sink through mpv's audio-device property,
	// so no pavucontrol (Linux) or macOS audio panel is needed. mpv enumerates
	// the devices cross-platform; the list already carries an "auto" entry.
	audio := settings.AddSubMenuItem("Sortie audio", "Choisir la sortie audio")
	a.audioMI = map[string]*systray.MenuItem{}
	cur := a.cfg.AudioDevice
	if cur == "" {
		cur = "auto" // empty config means mpv's automatic default
	}
	if devs, ok := a.player.AudioDeviceList(); ok && len(devs) > 0 {
		for _, dev := range devs {
			label := dev.Description
			if dev.Name == "auto" {
				label = "Automatique" // friendlier than mpv's "Autodetect device"
			} else if label == "" {
				label = dev.Name // fall back to the raw name when unlabeled
			}
			it := audio.AddSubMenuItemCheckbox(label, dev.Name, dev.Name == cur)
			a.audioMI[dev.Name] = it
			name := dev.Name
			a.on(it, events.KindAudioDevice, func() { a.setAudioDevice(name) })
		}
	} else {
		// Enumeration failed or is empty: keep a single Automatique entry so the
		// submenu is never blank and the user can still reset to the default.
		it := audio.AddSubMenuItemCheckbox("Automatique", "auto", cur == "auto")
		a.audioMI["auto"] = it
		a.on(it, events.KindAudioDevice, func() { a.setAudioDevice("auto") })
	}

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
	// Taste verdicts (J'aime / Pas pour moi) persist to a separate file,
	// prefs.jsonl, with its own consent. It gets its own delete affordance so
	// the "see / edit / delete" promise covers every local log.
	a.mPrefsClear = statsMenu.AddSubMenuItem("Effacer mes goûts…", "Supprimer prefs.jsonl (vos J'aime / Pas pour moi)")
	a.on(a.mPrefsClear, "", a.clearPrefsConfirm)

	systray.AddSeparator()
	about := systray.AddMenuItem("À propos", "Ouvrir la page du projet")
	a.on(about, events.KindOpenAbout, func() { open.URL(repoURL) })
	ver := systray.AddMenuItem("le fipindicateur "+version.String(), "Version installée")
	ver.Disable()
	// Mises à jour: "Vérifier maintenant" is the on-demand check; the checkbox
	// is the opt-in startup check. Both off + never clicking = never checks.
	maj := systray.AddMenuItem("Mises à jour", "Vérifier les nouvelles versions")
	checkNow := maj.AddSubMenuItem("Vérifier maintenant", "Comparer avec la dernière release sur GitHub")
	a.on(checkNow, events.KindUpdateCheck, a.checkUpdates)
	a.mUpdateStartup = maj.AddSubMenuItemCheckbox("Vérifier au démarrage", "Un contrôle discret au lancement (sinon jamais)", a.cfg.UpdateStartup)
	a.on(a.mUpdateStartup, "", a.toggleUpdateStartup)
	relancer := systray.AddMenuItem("Relancer", "Redémarrer le fipindicateur (recharge la dernière version installée)")
	a.on(relancer, events.KindRestart, a.restart)
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

	// Crossfade the animated glyph's ink toward this station's legible brand
	// color. Color only while music plays: paused/stopped falls back to the
	// static neutral icon (Active) via setPlayingUI. The gsettings panel probe
	// is cached (icon.PanelIsDark), never per frame.
	if play {
		a.anim.setTintTarget(icon.Legible(s.Color, icon.PanelIsDark()))
	}

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

// showConcept is the stable identity of a show, or "" for no show.
func showConcept(s *metadata.Show) string {
	if s == nil {
		return ""
	}
	return s.ConceptUUID
}

// onNowPlaying handles a metadata update.
func (a *App) onNowPlaying(np metadata.NowPlaying) {
	if np.Empty() {
		return
	}
	a.mu.Lock()
	changed := np.Artist != a.now.Artist || np.Title != a.now.Title
	showChanged := showConcept(np.Show) != showConcept(a.now.Show)
	a.now = np
	a.upcoming = np.UpcomingShows
	if changed {
		a.pushHistoryLocked(np)
	}
	a.mu.Unlock()

	// Nothing tray-visible changes when neither the track nor the show moved:
	// the watcher re-polls the SAME track many times over its (minutes-long)
	// life, so pushing the label, the "Voir…" state and the MPRIS metadata on
	// every poll churned the SNI for no reason. Gate every push on a change.
	if !changed && !showChanged {
		return
	}

	if changed {
		label := np.Title
		if np.Artist != "" {
			label = np.Artist + " · " + np.Title
		}
		log.Printf("now playing [%s]: %s", a.current.Key, label)

		// Dedupe + debounce the label push (SetTitle + SetTooltip) so a burst
		// cannot hammer the appindicator extension.
		a.nowThrottle.update(label)

		if np.Link != "" {
			a.mVoirLink.Enable()
		} else {
			a.mVoirLink.Disable()
		}

		// A track is now known: allow an explicit taste verdict on it. Once
		// enabled they stay enabled, so you can always like/dislike what is on air.
		a.mLike.Enable()
		a.mDislike.Enable()

		if a.mpris != nil {
			a.mpris.UpdateMetadata(np)
		}
		a.refreshHistoryMenu()
	}

	if showChanged {
		a.refreshShowMenu()
		a.refreshCalendarMenu()
	}

	// A notification and a history-log line both mean "you heard this". The
	// watcher keeps polling FIP while paused or stopped, so gate both on actual
	// playback: when the stream is not playing you are not listening, so we stay
	// silent and log nothing. The menu still updates above, so resuming shows
	// what is on air.
	if !a.player.IsPlaying() {
		return
	}

	// A programme starting takes precedence over the track banner for this tick:
	// both share the one replace-in-place notification, so firing the track one
	// too would instantly clobber the show announcement.
	notifiedShow := false
	if a.cfg.ShowNotifications {
		a.mu.Lock()
		cur := showConcept(np.Show)
		fresh := np.Show != nil && cur != "" && cur != a.lastShowConcept
		a.lastShowConcept = cur
		a.mu.Unlock()
		if fresh {
			a.notifyShow(*np.Show)
			notifiedShow = true
		}
	}
	if changed && a.cfg.Notifications && !notifiedShow {
		a.notify(np)
	}
	if changed && a.cfg.HistoryFile {
		a.appendHistFile(np)
	}
}

// notifyShow announces a programme starting on the antenna. Best-effort, like
// the track notification, and reusing the same replace-in-place channel.
func (a *App) notifyShow(s metadata.Show) {
	summary := s.Title
	if summary == "" {
		summary = "Nouvelle émission"
	}
	body := "En ce moment sur l'antenne"
	if s.Description != "" {
		body = s.Description
	}
	a.notif.Notify(summary, body, "", a.cfg.NotifTimeoutMs)
}

// refreshShowMenu updates the "émission en cours" item from the current show.
func (a *App) refreshShowMenu() {
	a.mu.Lock()
	show := a.now.Show
	a.mu.Unlock()
	if show == nil || show.Title == "" {
		a.mShow.Hide()
		return
	}
	a.mShow.SetTitle("Émission : " + show.Title)
	a.mShow.Show()
}

// refreshCalendarMenu fills the calendar slots from the upcoming programmes and
// manages the container's visibility. A no-op when the calendar is disabled.
// The whole submenu hides when nothing is scheduled (e.g. on the webradios), so
// there is never an empty "Calendrier" entry.
func (a *App) refreshCalendarMenu() {
	if !a.cfg.ShowCalendar {
		return
	}
	a.mu.Lock()
	up := make([]metadata.Show, len(a.upcoming))
	copy(up, a.upcoming)
	a.mu.Unlock()

	if len(up) == 0 {
		a.mCalendar.Hide()
		return
	}
	a.mCalendar.Show()
	for i, it := range a.calMI {
		if i < len(up) {
			s := up[i]
			label := s.Title
			if !s.Start.IsZero() {
				label = s.Start.Local().Format("15:04") + " · " + s.Title
			}
			it.SetTitle(label)
			it.Show()
		} else {
			it.Hide()
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
		Link:    np.Link,
		Cover:   np.CoverURL,
	})
	if err != nil {
		log.Printf("ui: history file append: %v", err)
	}
}

// recordTaste appends an explicit like/dislike verdict for the current track to
// prefs.jsonl. It snapshots the now-playing metadata and current station, then
// writes best-effort (a failed taste write never affects playback). A no-op
// when no track is known (the menu items are disabled until one is, but we
// guard anyway). A subtle notification confirms the verdict when notifications
// are on. The behaviour event (KindLike/KindDislike, station only) is already
// recorded by a.on; this adds the track identity the verdict is about.
func (a *App) recordTaste(verdict string) {
	a.mu.Lock()
	np := a.now
	a.mu.Unlock()
	if np.Empty() {
		return
	}
	if a.prefsPath == "" {
		p, err := prefs.DefaultPath()
		if err != nil {
			log.Printf("ui: prefs path: %v", err)
			return
		}
		a.prefsPath = p
	}
	err := prefs.Append(a.prefsPath, prefs.Entry{
		Verdict: verdict,
		Station: a.current.Key,
		Artist:  np.Artist,
		Title:   np.Title,
		Album:   np.Album,
		Year:    np.Year,
		Label:   np.Label,
		Link:    np.Link,
	})
	if err != nil {
		log.Printf("ui: prefs append: %v", err)
		return
	}
	if a.cfg.Notifications && a.notif != nil {
		summary := "C'est noté"
		if verdict == prefs.Dislike {
			summary = "Noté : pas pour vous"
		}
		body := np.Title
		if np.Artist != "" {
			body = np.Artist + " · " + np.Title
		}
		a.notif.Notify(summary, body, "", a.cfg.NotifTimeoutMs)
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
		// Only paint the static glyph when the animator will not run: when it
		// will, repainting here flashed the neutral mark on every zap before
		// the first tinted frame landed.
		if !a.anim.willRun() {
			a.applyIconState(false)
		}
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
	// Reload the stream at the new quality if playing. Stop first so this is a
	// hard cut, not a crossfade: it is the same station at a different bitrate,
	// and fading identical content against itself would phase/echo. Stopping
	// also drops the current URL so the reload starts from a not-playing handle,
	// which the Fader's "only a live zap crossfades" rule relies on.
	if a.player.IsPlaying() {
		a.player.Stop()
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

func (a *App) toggleShowNotif() {
	a.cfg.ShowNotifications = !a.cfg.ShowNotifications
	if a.cfg.ShowNotifications {
		a.mShowNotif.Check()
	} else {
		a.mShowNotif.Uncheck()
	}
	a.rec.Record(events.Event{Kind: events.KindShowNotif, Value: b2i(a.cfg.ShowNotifications)})
	a.save()
}

func (a *App) toggleShowCalendar() {
	a.cfg.ShowCalendar = !a.cfg.ShowCalendar
	if a.cfg.ShowCalendar {
		a.mShowCalendar.Check()
		a.mCalendar.Show()
		a.refreshCalendarMenu()
	} else {
		a.mShowCalendar.Uncheck()
		a.mCalendar.Hide()
	}
	a.rec.Record(events.Event{Kind: events.KindShowCalendar, Value: b2i(a.cfg.ShowCalendar)})
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

// --- zenity dialogs (volume slider, crossfade slider) ---

// zenityAvailable reports whether the zenity binary is on PATH. GNOME/most Linux
// desktops ship it; macOS, Windows and minimal installs do not, so the slider
// items are only added when it exists (no dead menu entry).
func zenityAvailable() bool {
	_, err := exec.LookPath("zenity")
	return err == nil
}

// acquireDialog claims the single zenity dialog slot, returning true on success.
// A false result means a dialog is already open and the caller must do nothing.
// Pair every true with a releaseDialog (deferred).
func (a *App) acquireDialog() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dialogOpen {
		return false
	}
	a.dialogOpen = true
	return true
}

func (a *App) releaseDialog() {
	a.mu.Lock()
	a.dialogOpen = false
	a.mu.Unlock()
}

// openVolumeSlider launches the zenity volume slider off the tray goroutine.
// A second click while one is open is ignored (single dialog at a time).
func (a *App) openVolumeSlider() {
	if !a.acquireDialog() {
		log.Printf("ui: a zenity dialog is already open, ignoring volume slider")
		return
	}
	start := a.cfg.Volume
	go func() {
		defer a.releaseDialog()
		a.runVolumeSlider(start)
	}()
}

// runVolumeSlider runs `zenity --scale --print-partial` and applies each live
// drag value to the player immediately, WITHOUT recording an event or saving
// config (no telemetry/disk spam while dragging). On OK (exit 0) it commits:
// one KindVolume event at the final value plus one config save. On Cancel/Esc/
// close (non-zero exit) it reverts to the volume that was current when the
// dialog opened (apply + UI sync, no event, no save). External pavucontrol
// changes during the drag flow through onExternalVolume; last writer wins.
func (a *App) runVolumeSlider(start int) {
	cmd := exec.Command("zenity", "--scale",
		"--title", "FIP · Volume",
		"--text", "Volume de lecture",
		"--min-value", "0",
		"--max-value", "100",
		"--step", "1",
		"--value", strconv.Itoa(start),
		"--print-partial",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("ui: volume slider: stdout pipe: %v", err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("ui: volume slider: start: %v", err)
		return
	}

	last := start
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		v, perr := strconv.Atoi(strings.TrimSpace(sc.Text()))
		if perr != nil {
			continue
		}
		v = clampPct(v)
		last = v
		a.applyVolumeLive(v) // player + menu + MPRIS, no event, no save
	}

	if err := cmd.Wait(); err == nil {
		// OK: state is already applied live; record exactly one event and persist.
		a.cfg.Volume = clampPct(last)
		a.rec.Record(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: a.cfg.Volume})
		a.save()
		a.applyVolumeUI()
	} else {
		// Cancel/Esc/close: revert to the pre-dialog level, silently.
		a.applyVolumeLive(start)
	}
}

// applyVolumeLive applies a volume to the player, MPRIS and the menu WITHOUT
// recording an event or saving config. It sets a.cfg.Volume BEFORE SetVolume so
// the ao-volume observer echo is swallowed by onExternalVolume's equal-value
// guard (the same trick setVolume uses). Used for slider drag ticks and revert.
func (a *App) applyVolumeLive(pct int) {
	pct = clampPct(pct)
	a.cfg.Volume = pct
	a.player.SetVolume(float64(pct))
	if a.mpris != nil {
		a.mpris.SetVolume(float64(pct) / 100)
	}
	a.applyVolumeUI()
}

// --- crossfade duration ---

// crossfadeTitle is the label for the "Fondu enchaîné" menu item: it always
// reflects the current value ("(désactivé)" at 0), and appends an ellipsis in
// slider mode to hint that a click opens a dialog.
func crossfadeTitle(secs int, slider bool) string {
	var s string
	if secs == 0 {
		s = "Fondu enchaîné (désactivé)"
	} else {
		s = fmt.Sprintf("Fondu enchaîné (%d s)", secs)
	}
	if slider {
		s += "…"
	}
	return s
}

// crossfadePresetLabel labels a preset checkbox in the zenity-less fallback.
func crossfadePresetLabel(secs int) string {
	if secs == 0 {
		return "Désactivé"
	}
	return fmt.Sprintf("%d s", secs)
}

// applyCrossfadeUI syncs the crossfade item title (and the preset checkmarks in
// fallback mode) with the current config.
func (a *App) applyCrossfadeUI() {
	if a.mCrossfade == nil {
		return
	}
	a.mCrossfade.SetTitle(crossfadeTitle(a.cfg.CrossfadeSecs, a.crossfadeMI == nil))
	for secs, it := range a.crossfadeMI {
		if secs == a.cfg.CrossfadeSecs {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
}

// setCrossfade persists a crossfade duration, applies it LIVE to the player
// (takes effect on the next zap), records one KindCrossfade event at source
// (Value = seconds, 0 = off) and refreshes the menu. Clamped to [0,10] to match
// config.Load.
func (a *App) setCrossfade(secs int) {
	if secs < 0 {
		secs = 0
	}
	if secs > 10 {
		secs = 10
	}
	a.cfg.CrossfadeSecs = secs
	a.rec.Record(events.Event{Kind: events.KindCrossfade, Station: a.current.Key, Value: secs})
	a.save()
	a.player.SetCrossfade(time.Duration(secs) * time.Second)
	a.applyCrossfadeUI()
}

// openCrossfadeSlider launches the zenity crossfade-duration slider off the tray
// goroutine, sharing the single-dialog guard with the volume slider.
func (a *App) openCrossfadeSlider() {
	if !a.acquireDialog() {
		log.Printf("ui: a zenity dialog is already open, ignoring crossfade slider")
		return
	}
	start := a.cfg.CrossfadeSecs
	go func() {
		defer a.releaseDialog()
		a.runCrossfadeSlider(start)
	}()
}

// runCrossfadeSlider runs `zenity --scale` over 0..10s. On OK (exit 0) it commits
// the chosen value via setCrossfade (which records the single KindCrossfade
// event). On Cancel/Esc/close (non-zero exit) it does nothing. No live-apply:
// crossfade only takes effect at the next zap, so a per-tick apply would be
// pointless here.
func (a *App) runCrossfadeSlider(start int) {
	out, err := exec.Command("zenity", "--scale",
		"--title", "FIP · Fondu enchaîné",
		"--text", "Durée du fondu entre stations (secondes, 0 = coupure sèche)",
		"--min-value", "0",
		"--max-value", "10",
		"--step", "1",
		"--value", strconv.Itoa(start),
	).Output()
	if err != nil {
		return // non-zero exit = Cancel/Esc/close: do nothing
	}
	v, perr := strconv.Atoi(strings.TrimSpace(string(out)))
	if perr != nil {
		log.Printf("ui: crossfade slider: unparsable value %q: %v", strings.TrimSpace(string(out)), perr)
		return
	}
	a.setCrossfade(v)
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

// setAudioDevice switches the mpv audio output device. The click event is
// already recorded by a.on (KindAudioDevice), so we do not record again here.
// mpv reinits the audio output live, so no stream reload. "auto" is stored as
// an empty config value (mpv's default).
func (a *App) setAudioDevice(name string) {
	a.cfg.AudioDevice = name
	if name == "auto" {
		a.cfg.AudioDevice = "" // store auto as empty, matching the config default
	}
	a.save()
	a.player.SetAudioDevice(name)
	// Refresh checkmarks so only the chosen sink is ticked.
	for n, it := range a.audioMI {
		if n == name {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
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

// clearPrefsConfirm deletes prefs.jsonl (the taste verdicts) with the same
// two-click confirmation as clearStatsConfirm: first click arms and relabels, a
// second click within a short window deletes. It removes only prefs.jsonl,
// never events.jsonl or history.jsonl. The KindPrefsClear behaviour event is
// recorded at source on the confirming click only (not on the arming click, so
// a click the user backs out of logs nothing); it lands in events.jsonl, a
// different file, so it never resurrects the taste log we just deleted.
func (a *App) clearPrefsConfirm() {
	a.mu.Lock()
	armed := a.prefsClearArmed
	a.prefsClearArmed = !armed // arm on first click, disarm on the confirming click
	a.mu.Unlock()

	if !armed {
		a.mPrefsClear.SetTitle("Confirmer l'effacement ?")
		go func() {
			time.Sleep(5 * time.Second)
			a.mu.Lock()
			still := a.prefsClearArmed
			a.prefsClearArmed = false
			a.mu.Unlock()
			if still {
				a.mPrefsClear.SetTitle("Effacer mes goûts…")
			}
		}()
		return
	}

	a.mPrefsClear.SetTitle("Effacer mes goûts…")

	if a.prefsPath == "" {
		p, err := prefs.DefaultPath()
		if err != nil {
			log.Printf("ui: prefs path: %v", err)
			return
		}
		a.prefsPath = p
	}
	if err := prefs.Clear(a.prefsPath); err != nil {
		log.Printf("ui: prefs clear: %v", err)
		return
	}
	a.rec.Record(events.Event{Kind: events.KindPrefsClear, Station: a.current.Key})
	// The like/dislike menu items carry no cached verdict state (they only toggle
	// enabled once a track is known), so there is nothing on them to reset after
	// a delete. They stay enabled: you can still record a fresh verdict.
	if a.cfg.Notifications && a.notif != nil {
		a.notif.Notify("Goûts effacés", "Le journal prefs.jsonl a été supprimé.", "", a.cfg.NotifTimeoutMs)
	}
}

// --- restart & updates ---

// restart relaunches the app so a freshly installed binary takes over. It
// starts a detached helper that waits for this instance to exit (freeing the
// MPRIS single-instance name and mpv) then execs the current executable path,
// picking up whatever `make install` last wrote there.
func (a *App) restart() {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("ui: restart: cannot resolve executable: %v", err)
		return
	}
	cmd := relaunchCmd(exe)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		log.Printf("ui: restart: relaunch failed: %v", err)
		return
	}
	systray.Quit() // clean teardown: records app_stop, closes mpv and D-Bus
}

// checkUpdates runs an on-demand update check off the UI goroutine.
func (a *App) checkUpdates() { go a.runUpdateCheck(false) }

// runUpdateCheck queries GitHub Releases and notifies the result. On an
// explicit check it always reports (and opens the release page when newer); on
// the startup check it stays quiet unless a newer release actually exists, and
// never steals focus with a browser tab.
func (a *App) runUpdateCheck(startup bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	res, err := update.Check(ctx, version.String())
	if err != nil {
		log.Printf("ui: update check: %v", err)
		if !startup {
			a.notifyUpdate("Vérification impossible", "Impossible de contacter GitHub pour le moment.")
		}
		return
	}
	switch {
	case res.Newer:
		a.notifyUpdate("Mise à jour disponible", fmt.Sprintf("%s est disponible (vous avez %s).", res.Latest, res.Current))
		if !startup {
			open.URL(res.URL)
		}
	case res.Dev:
		if !startup {
			a.notifyUpdate("Build de développement", fmt.Sprintf("Version %s. Dernière release : %s. Mettez à jour avec git pull puis make install.", res.Current, res.Latest))
			open.URL(res.URL)
		}
	default:
		if !startup {
			a.notifyUpdate("À jour", fmt.Sprintf("Vous utilisez la dernière version (%s).", res.Current))
		}
	}
}

func (a *App) notifyUpdate(summary, body string) {
	if a.notif != nil {
		a.notif.Notify(summary, body, "", a.cfg.NotifTimeoutMs)
		return
	}
	log.Printf("ui: update: %s - %s", summary, body)
}

func (a *App) toggleUpdateStartup() {
	a.cfg.UpdateStartup = !a.cfg.UpdateStartup
	if a.cfg.UpdateStartup {
		a.mUpdateStartup.Check()
	} else {
		a.mUpdateStartup.Uncheck()
	}
	a.rec.Record(events.Event{Kind: events.KindUpdateStartup, Value: b2i(a.cfg.UpdateStartup)})
	a.save()
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

func (a *App) applyIcon() { a.setIcon(icon.Rest(false, color.NRGBA{})) }

// applyIconState paints the frozen bars glyph. While playing it carries the
// current station's legible brand tint, so the FIP colors stay on the tray
// even when the animated icon is off; paused/stopped falls back to neutral
// theme ink (color only while music plays, matching the animator's fade-out).
func (a *App) applyIconState(paused bool) {
	var tint color.NRGBA
	if !paused {
		tint = icon.Legible(a.current.Color, icon.PanelIsDark())
	}
	a.setIcon(icon.Rest(paused, tint))
}

// setIcon is the single chokepoint for the tray icon. It (1) refuses empty
// bytes, which would register a null pixmap on the StatusNotifierItem and trip
// GNOME's cogl "data != NULL" assertion, and (2) skips a push when the bytes
// are identical to the last set, so the static-icon path and the animator (two
// goroutines) never churn the SNI with a redundant redraw. The icon library
// never returns nil in practice; the guard is defence against a future
// regression handing us an empty asset.
func (a *App) setIcon(b []byte) {
	if len(b) == 0 {
		log.Printf("ui: refusing to set an empty tray icon (would register a null pixmap)")
		return
	}
	a.iconMu.Lock()
	if bytes.Equal(b, a.lastIcon) {
		a.iconMu.Unlock()
		return
	}
	// Copy so a caller reusing its buffer cannot mutate our dedupe baseline.
	a.lastIcon = append(a.lastIcon[:0:0], b...)
	a.iconMu.Unlock()
	setTrayIcon(b)
}

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
