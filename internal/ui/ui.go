// Package ui wires together the tray presence, the player, metadata, MPRIS
// and notifications. The user-facing skin is platform-split: on Linux a
// hand-rolled StatusNotifierItem plus « le panneau » (the drawer) are the
// whole UI (ui_sni_linux.go); elsewhere fyne/systray builds the classic tray
// menu (ui_menu.go). This file is the shared core: state, chokepoints,
// handlers and telemetry.
package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PLNech/fipindicateur/internal/cast"
	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/drawer"
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
	// castSlots bounds the "Diffuser sur…" device list. Pre-allocated because
	// systray cannot remove items; a home network rarely exceeds a handful of
	// Chromecast endpoints.
	castSlots = 8
	// castScanWindow is how long one mDNS discovery listens for answers.
	castScanWindow = 3 * time.Second
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

	// volRec coalesces KindVolume records: a slider drag (panel or
	// pavucontrol) is a stream of applied values, and the log wants the
	// gesture (first level, then the settled one), not every intermediate
	// step. Built lazily by recordVolume; flushed on exit.
	volRecOnce sync.Once
	volRec     *volCoalescer

	statsClearArmed bool // two-click confirm state for "Effacer les statistiques"
	prefsClearArmed bool // two-click confirm state for "Effacer mes goûts"

	// casting ("Diffuser sur…"): the live Chromecast session (nil = playing
	// locally) and the devices from the last mDNS scan. All guarded by a.mu.
	// castName is the active target's friendly name, used only for menu
	// checkmarks: device identity never reaches the events log.
	castSess     *cast.Session
	castName     string
	castDevices  []cast.Device
	castScanning bool // one discovery at a time

	// « le panneau »: the quick-control drawer (Linux only; a stub elsewhere).
	// Built lazily on first open (or by the startup prewarm), then resident.
	// drawerDark caches the desktop color-scheme probe, refreshed at each
	// open. drawerPhase tracks visibility so the tray Activate toggles (show
	// when hidden, hide when shown, and ignores clicks while the first Show is
	// still initializing; see decideDrawerToggle). It falls back to hidden
	// through onDrawerHidden whenever the panel hides (Escape, the page's
	// close button, or the toggle itself). Guarded by a.mu.
	drawer      *drawer.Drawer
	drawerDark  bool
	drawerPhase drawerPhase

	// audioDevs is mpv's output-device enumeration, cached once at startup:
	// the menu's "Sortie audio" submenu and the panel's « Sur cet appareil »
	// section both render from it.
	audioDevs []player.AudioDevice

	// menu items (menuItem is *systray.MenuItem off Linux; on Linux a nil-safe
	// no-op stand-in, since the drawer replaced the menu there).
	mNow           *menuItem
	mShow          *menuItem // current programme (émission) on air; hidden when none
	mVoirWiki      *menuItem
	mVoirLink      *menuItem
	mLike          *menuItem
	mDislike       *menuItem
	mPlay          *menuItem
	stationMI      map[string]*menuItem
	histMI         []*menuItem
	mCalendar      *menuItem   // calendar submenu container; hidden when disabled
	calMI          []*menuItem // pre-allocated calendar slots
	mHiFi          *menuItem
	mNotif         *menuItem
	mShowNotif     *menuItem
	mShowCalendar  *menuItem
	mAuto          *menuItem
	mPlayOnStart   *menuItem
	mHistFile      *menuItem
	mAnim          *menuItem
	audioMI        map[string]*menuItem // audio-output items, keyed by device name ("auto" = automatic)
	mStats         *menuItem
	mStatsClear    *menuItem
	mPrefsClear    *menuItem
	mUpdateStartup *menuItem
	mCastLocal     *menuItem   // "Cet ordinateur", checked when not casting
	mCastNone      *menuItem   // disabled placeholder when no device was found
	mCastScan      *menuItem   // "Rechercher les appareils"
	castMI         []*menuItem // pre-allocated device slots
	mVolume        *menuItem
	mMute          *menuItem
	volMI          map[int]*menuItem
	mCrossfade     *menuItem
	crossfadeMI    map[int]*menuItem // preset checkboxes; nil in zenity-slider mode

	// dialogOpen guards against launching two zenity dialogs at once (the volume
	// slider and the crossfade slider share it). A click while one is open is
	// ignored. Guarded by a.mu.
	dialogOpen bool
}

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
		stationMI: map[string]*menuItem{},
	}
}

// OnReady builds everything and starts playing the last station. Called by
// Run at the head of the platform main loop (systray's onReady callback off
// Linux; directly on Linux).
func (a *App) OnReady() {
	readyT0 := time.Now()
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
		// On Linux the label lives on the SNI itself (title + tooltip); the
		// menu-item calls above are nil-safe no-ops there, and this one is a
		// no-op elsewhere.
		a.setTrayNowLabel(label)
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
	// Enumerate the output devices once for both skins (menu submenu, panel
	// section). mpv's list is stable for a session; a failed enumeration
	// leaves the slice empty and the UIs fall back to a single Automatique.
	if devs, ok := a.player.AudioDeviceList(); ok {
		a.audioDevs = devs
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

	a.buildUI()
	if a.mpris != nil {
		a.mpris.SetVolume(float64(a.cfg.Volume) / 100)
	}
	// (icon already set at the top of OnReady, before the menu, to shrink the
	// null-pixmap window at SNI registration.)
	a.rec.Record(events.Event{Kind: events.KindAppStart, Station: a.current.Key})
	// Autostart is not autoplay: launch tunes the antenna (station selected,
	// icon, metadata polling) but only starts the stream when the user opted in
	// with PlayOnStart. When off, startStation(_, false) leaves playback paused
	// and the tray still indicates what is on air (metadata is API-driven, not
	// stream-driven), so a first press of play resumes promptly.
	a.startStation(a.current, a.cfg.PlayOnStart)

	// One background device scan at startup so "Diffuser sur…" is populated
	// by the time it is first opened. Runs entirely off the tray goroutine:
	// OnReady never waits on the network.
	go a.scanCast()

	// Control socket: lets `fip status`/`play`/`pause`/`station …` drive the
	// running app (and survive the tray dying). Best-effort; the app runs on if
	// it cannot bind. Unix-only (Windows is a no-op).
	a.startControlServer()

	// Opt-in: one quiet update check at launch. Off by default.
	if a.cfg.UpdateStartup {
		go a.runUpdateCheck(true)
	}

	timingf("app: OnReady terminé en %v (tray prêt, station lancée)", time.Since(readyT0))

	// Pre-warm « le panneau »: build the hidden webkit window a moment after
	// startup, so the first click only pays a present instead of the whole
	// gtk_init + WebKit spin-up. Delayed to never compete with startup and
	// playback; one-shot; never presents or steals focus. Headless failures
	// stay silent here (Prewarm swallows them) and the real open keeps its
	// error path. toggleDrawer works identically whether this won the race.
	if drawer.Available {
		go func() {
			time.Sleep(2 * time.Second)
			a.mu.Lock()
			if a.drawer == nil {
				a.drawer = drawer.New(a.onDrawerCommand, a.onDrawerHidden)
			}
			d := a.drawer
			a.mu.Unlock()
			d.Prewarm()
		}()
	}
}

// OnExit tears everything down.
func (a *App) OnExit() {
	// A live cast ends with the app: record the end of the behaviour (before
	// the recorder flushes) and tell the receiver goodbye, but never let a
	// wedged device hold the exit hostage.
	a.mu.Lock()
	sess := a.castSess
	a.castSess = nil
	a.mu.Unlock()
	if sess != nil {
		a.rec.Record(events.Event{Kind: events.KindCastStop, Station: a.current.Key})
		done := make(chan struct{})
		go func() { sess.Stop(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			sess.Close() // give up on the goodbye, just drop the connection
		}
	}
	a.flushVolumeRecord() // a drag's settled level must not die with the app
	a.rec.Record(events.Event{Kind: events.KindAppStop, Station: a.current.Key})
	a.rec.Close() // flushes the queued app_stop before we return
	a.stopControlServer()
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
	a.teardownUI()
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
	if sess := a.castSession(); sess != nil {
		// While casting, a zap re-LOADs the stream on the device instead of
		// starting it here: the local player stays paused so the audio never
		// doubles. The LOAD runs off the tray goroutine (a wedged device must
		// not block the menu); a failure surfaces through onCastError.
		play = false
		ctype := a.castContentType()
		go func() {
			if err := sess.Load(url, ctype, castTitle(s)); err != nil {
				a.onCastError(err)
			}
		}()
	}
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
	// A trackless update can still carry the programme on air: some émissions
	// (continuous mixes like "Fip Tape") expose no per-song grain in livemeta,
	// and the Radio France streams send no inline ICY titles either, so during
	// such shows the programme IS the whole signal. Only an update with neither
	// a track nor a show carries nothing.
	hasTrack := !np.Empty()
	if !hasTrack && np.Show == nil {
		return
	}
	a.mu.Lock()
	changed := hasTrack && (np.Artist != a.now.Artist || np.Title != a.now.Title)
	// trackCleared: a show-only update replaced a known track (the programme
	// took over the antenna), so the stale track must leave the label.
	trackCleared := !hasTrack && !a.now.Empty()
	newConcept := showConcept(np.Show)
	showChanged := newConcept != showConcept(a.now.Show)
	// pendingShow: the on-air concept differs from the last one witnessed
	// while listening (lastShowConcept). Unlike showChanged, a single missed
	// tick (a boundary crossed while paused, or racing playback at startup)
	// stays pending and is caught on a later poll, so the show_change event,
	// the notification and the show-start marker are never silently lost.
	pendingShow := newConcept != a.lastShowConcept
	a.now = np
	a.upcoming = np.UpcomingShows
	if changed {
		a.pushHistoryLocked(np)
	}
	a.mu.Unlock()

	// Nothing tray-visible changes when neither the track nor the show moved:
	// the watcher re-polls the SAME state many times over its (minutes-long)
	// life, so pushing the label, the "Voir…" state and the MPRIS metadata on
	// every poll churned the SNI for no reason. Gate every push on a change.
	if !changed && !showChanged && !trackCleared && !pendingShow {
		return
	}

	// Whatever moved (track, show, cleared label), the panel shows it too.
	a.pushDrawerState()

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
	} else if !hasTrack && np.Show != nil && (showChanged || trackCleared) {
		// No track is known during this programme: the show title becomes the
		// tray label, better than a stale track from before it started. Same
		// throttle as the track label, so the anti-churn contract holds.
		log.Printf("on air [%s]: %s (émission sans titrage)", a.current.Key, np.Show.Title)
		a.nowThrottle.update(np.Show.Title)
		a.mVoirLink.Disable() // no track, no track link
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

	// The programme boundary is witnessed against lastShowConcept, the last
	// concept seen while actually listening: a boundary that landed while
	// paused, or that raced playback at startup, is caught here on a later
	// poll instead of being lost with its single transition tick.
	a.mu.Lock()
	fresh := newConcept != a.lastShowConcept
	prevWitnessed := a.lastShowConcept
	a.lastShowConcept = newConcept
	a.mu.Unlock()

	// A programme starting takes precedence over the track banner for this tick:
	// both share the one replace-in-place notification, so firing the track one
	// too would instantly clobber the show announcement.
	notifiedShow := false
	if fresh && newConcept != "" && np.Show != nil && a.cfg.ShowNotifications {
		a.notifyShow(*np.Show)
		notifiedShow = true
	}
	if changed && a.cfg.Notifications && !notifiedShow {
		a.notify(np)
	}
	if changed && a.cfg.HistoryFile {
		a.appendHistFile(np)
	}
	// A trackless programme starting writes one show-start marker line to the
	// histlog: a bare show/show_concept tag with no artist and no title. It
	// carries the show's display name into the report (identity lives in the
	// histlog, never in the events log); the listening time itself is derived
	// from the show_change boundaries, so the marker is identity, not duration.
	if fresh && !hasTrack && np.Show != nil && a.cfg.HistoryFile {
		a.appendHistFile(np)
	}
	// The programme boundary, recorded like the station-change Markov edge but
	// keyed on the stable conceptUuid. Ambient (FIP drives it, not the user), so
	// it is recorded here at its source while listening; internal/stats crosses
	// these boundaries with the playback segments to accumulate time per show,
	// which is the only measure during émissions without a tracklist.
	if fresh {
		a.rec.Record(events.Event{Kind: events.KindShowChange, Station: a.current.Key, From: prevWitnessed, To: newConcept})
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
	entry := histlog.Entry{
		Station: a.current.Key,
		Artist:  np.Artist,
		Title:   np.Title,
		Album:   np.Album,
		Year:    np.Year,
		Label:   np.Label,
		Link:    np.Link,
		Cover:   np.CoverURL,
	}
	if np.Show != nil {
		entry.Show = np.Show.Title
		entry.ShowConcept = np.Show.ConceptUUID
	}
	err := histlog.Append(a.histPath, entry)
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
	// While casting, the play button means "bring the music back here": stop
	// the cast and resume locally, never start a second stream on top of the
	// speaker's.
	if a.castSession() != nil {
		a.stopCasting(true)
		return
	}
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
	a.pushDrawerState()
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
	if sess := a.castSession(); sess != nil {
		// Casting: re-LOAD at the new quality on the device; local playback
		// stays paused. Same off-goroutine rule as the startStation seam.
		url := a.current.StreamURL(a.quality())
		ctype := a.castContentType()
		title := castTitle(a.current)
		go func() {
			if err := sess.Load(url, ctype, title); err != nil {
				a.onCastError(err)
			}
		}()
	} else if a.player.IsPlaying() {
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

// recordVolume is the single chokepoint for KindVolume records (panel slider,
// menu preset, wheel, MPRIS, pavucontrol echo, cast volume). The change stays
// measurable by design; it is only coalesced per gesture (volCoalescer): a
// slider drag logs its first level immediately and its settled level on the
// trailing edge, not every intermediate step.
func (a *App) recordVolume(pct int) {
	a.volRecOnce.Do(func() {
		a.volRec = &volCoalescer{min: volumeRecordMin, record: a.rec.Record}
	})
	a.volRec.submit(events.Event{Kind: events.KindVolume, Station: a.current.Key, Value: pct})
}

// flushVolumeRecord empties any pending coalesced volume record (exit path,
// before the recorder closes).
func (a *App) flushVolumeRecord() {
	if a.volRec != nil {
		a.volRec.flush()
	}
}

// applyVolumeUI syncs the volume submenu (title, preset checkmarks, mute)
// with the current config, and mirrors the level into the panel. On Linux the
// submenu does not exist (the panel replaced it): only the push happens.
func (a *App) applyVolumeUI() {
	if a.mVolume != nil {
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
	a.pushDrawerState()
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
		a.recordVolume(pct)
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
	a.recordVolume(pct)
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
	a.recordVolume(pct)
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

// togglePlayOnStart flips whether launch also starts the stream. It only
// changes the next launch's behaviour; the current playback state is untouched.
func (a *App) togglePlayOnStart() {
	a.cfg.PlayOnStart = !a.cfg.PlayOnStart
	if a.cfg.PlayOnStart {
		a.mPlayOnStart.Check()
	} else {
		a.mPlayOnStart.Uncheck()
	}
	a.rec.Record(events.Event{Kind: events.KindPlayOnStart, Value: b2i(a.cfg.PlayOnStart)})
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

// --- casting (Diffuser sur…) ---

// castSession returns the live cast session, or nil when playing locally.
func (a *App) castSession() *cast.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.castSess
}

// castContentType is the MIME type of the current stream quality, for the
// device's LOAD request.
func (a *App) castContentType() string {
	if a.cfg.HiFi {
		return "audio/aacp" // 192k AAC
	}
	return "audio/mpeg" // 128k MP3
}

// castTitle is the label the device UI shows for a station's live stream.
func castTitle(s stations.Station) string {
	return "FIP · " + s.Display
}

// rescanCast is the "Rechercher les appareils" click: a fresh discovery, off
// the tray goroutine.
func (a *App) rescanCast() { go a.scanCast() }

// scanCast runs one mDNS discovery and refreshes the submenu. Blocking for
// the scan window, so it always runs off the tray goroutine; a scan already
// in flight is not doubled.
func (a *App) scanCast() {
	a.mu.Lock()
	if a.castScanning {
		a.mu.Unlock()
		return
	}
	a.castScanning = true
	a.mu.Unlock()
	a.pushDrawerState() // the panel shows "Recherche…" while the scan runs

	a.mCastScan.SetTitle("Recherche en cours…")
	devs := cast.Discover(castScanWindow)
	a.mCastScan.SetTitle("Rechercher les appareils")

	a.mu.Lock()
	a.castDevices = devs
	a.castScanning = false
	a.mu.Unlock()
	a.refreshCastMenu()
}

// refreshCastMenu syncs the device slots, the placeholder and the checkmarks
// with the last scan and the active target.
func (a *App) refreshCastMenu() {
	a.mu.Lock()
	devs := make([]cast.Device, len(a.castDevices))
	copy(devs, a.castDevices)
	active := a.castName
	casting := a.castSess != nil
	a.mu.Unlock()

	for i, it := range a.castMI {
		if i < len(devs) {
			it.SetTitle(devs[i].Name)
			if casting && devs[i].Name == active {
				it.Check()
			} else {
				it.Uncheck()
			}
			it.Show()
		} else {
			it.Uncheck()
			it.Hide()
		}
	}
	if len(devs) == 0 {
		a.mCastNone.Show()
	} else {
		a.mCastNone.Hide()
	}
	if casting {
		a.mCastLocal.Uncheck()
	} else {
		a.mCastLocal.Check()
	}
	a.pushDrawerState()
}

// castToDevice starts casting the current station to the i-th discovered
// device. The handshake and LOAD run off the tray goroutine.
func (a *App) castToDevice(i int) {
	a.mu.Lock()
	if i >= len(a.castDevices) {
		a.mu.Unlock()
		return
	}
	dev := a.castDevices[i]
	already := a.castSess != nil && a.castName == dev.Name
	a.mu.Unlock()
	if already {
		return // clicking the active target changes nothing
	}
	go a.startCast(dev)
}

// startCast dials the device, LOADs the current stream and, on success,
// pauses local playback and flips the menu state. Any failure surfaces as a
// notification and the menu resets to local: casting never crashes or blocks
// the tray. Runs off the tray goroutine (see castToDevice).
func (a *App) startCast(dev cast.Device) {
	station := a.current
	url := station.StreamURL(a.quality())
	ctype := a.castContentType()

	sess, err := cast.Dial(dev, a.onCastError, a.onCastStatus)
	if err != nil {
		a.castFailed(err)
		return
	}
	if err := sess.Load(url, ctype, castTitle(station)); err != nil {
		sess.Close()
		a.castFailed(err)
		return
	}

	a.mu.Lock()
	old := a.castSess
	a.castSess = sess
	a.castName = dev.Name
	a.mu.Unlock()
	if old != nil {
		// Device switch: quit the receiver on the previous target. Its
		// deliberate Stop never triggers onCastError.
		go old.Stop()
	}

	if old == nil {
		// Local playback pauses so the audio does not double; setPlayingUI
		// records that pause. The cast itself is the cast_start event,
		// recorded at source, behaviour only: no device name or address
		// ever reaches the events log.
		a.player.Stop()
		a.setPlayingUI(false)
		a.rec.Record(events.Event{Kind: events.KindCastStart, Station: station.Key})
	}
	a.refreshCastMenu()
	if a.cfg.Notifications && a.notif != nil {
		a.notif.Notify("Diffusion en cours", "La station joue sur « "+dev.Name+" ».", "", a.cfg.NotifTimeoutMs)
	}
}

// stopCasting ends the cast; when resume is true ("Cet ordinateur", the play
// button, an external play), playback comes back to the local player. No-op
// when not casting. Records cast_stop at source (behaviour only).
func (a *App) stopCasting(resume bool) {
	a.mu.Lock()
	sess := a.castSess
	a.castSess = nil
	a.castName = ""
	a.mu.Unlock()
	if sess == nil {
		a.refreshCastMenu() // keep "Cet ordinateur" checked on a redundant click
		return
	}
	go sess.Stop() // the network goodbye is best-effort, off the tray goroutine
	a.rec.Record(events.Event{Kind: events.KindCastStop, Station: a.current.Key})
	a.refreshCastMenu()
	if resume {
		a.player.Play(a.current.StreamURL(a.quality()))
		a.setPlayingUI(true)
	}
}

// castFailed reports a cast that could not start. The menu never left the
// local state, so refreshing it is enough of a reset.
func (a *App) castFailed(err error) {
	log.Printf("ui: cast: %v", err)
	a.refreshCastMenu()
	if a.notif != nil {
		a.notif.Notify("Diffusion impossible", "Impossible de diffuser sur cet appareil. L'appareil a peut-être besoin de quelques secondes : réessayez.", "", a.cfg.NotifTimeoutMs)
	}
}

// onCastError handles a live session dying unexpectedly (device rebooted,
// network drop, LOAD refused mid-session). The menu resets to local but
// playback stays paused: silently resuming sound here could blast at an
// unexpected moment. The casting behaviour did end, so cast_stop is recorded.
func (a *App) onCastError(err error) {
	a.mu.Lock()
	sess := a.castSess
	a.castSess = nil
	a.castName = ""
	a.mu.Unlock()
	if sess == nil {
		return // a deliberate stop raced the failure; already reset
	}
	log.Printf("ui: cast session lost: %v", err)
	sess.Close()
	a.rec.Record(events.Event{Kind: events.KindCastStop, Station: a.current.Key})
	a.refreshCastMenu()
	if a.notif != nil {
		a.notif.Notify("Diffusion interrompue", "La connexion avec l'appareil a été perdue.", "", a.cfg.NotifTimeoutMs)
	}
}

// --- le panneau (quick-control drawer) ---

// drawerPhase is the panel's visibility lifecycle, guarded by a.mu.
type drawerPhase int

const (
	drawerHidden  drawerPhase = iota
	drawerOpening             // Show dispatched, GTK still building/presenting
	drawerVisible
)

// drawerToggleAction is what one tray Activate should do given the phase.
type drawerToggleAction int

const (
	drawerActOpen drawerToggleAction = iota
	drawerActHide
	drawerActIgnore
)

// decideDrawerToggle maps the panel's phase to the toggle's effect. A
// double-click must not double-open; and while the first Show is still
// initializing (opening), a second click must be a no-op rather than a Hide,
// because a Hide queued at that moment can reach the GTK thread BEFORE the
// pending present (Show is still waiting in ensureStarted) and leave the flag
// and the window inconsistent. Pure function, unit-tested.
func decideDrawerToggle(p drawerPhase) drawerToggleAction {
	switch p {
	case drawerHidden:
		return drawerActOpen
	case drawerOpening:
		return drawerActIgnore
	default:
		return drawerActHide
	}
}

// timingsOn gates the FIP_TIMINGS=1 instrumentation logs (prefix "timing:",
// greppable), shared convention with internal/drawer.
var timingsOn = os.Getenv("FIP_TIMINGS") == "1"

func timingf(format string, args ...any) {
	if !timingsOn {
		return
	}
	log.Printf("timing: "+format, args...)
}

// toggleDrawer shows « le panneau » when hidden and hides it when shown; on
// Linux it is the tray icon's Activate (left click). The drawer is built
// lazily on first open and stays resident afterwards (hidden, instant to
// re-present). KindDrawerOpen is recorded at source, only on an actual open
// (hiding is not an open). The desktop color-scheme is re-probed at each
// open, so a theme flip lands on the next show.
func (a *App) toggleDrawer() {
	dark := drawer.DarkPreferred()
	a.mu.Lock()
	if a.drawer == nil {
		a.drawer = drawer.New(a.onDrawerCommand, a.onDrawerHidden)
	}
	d := a.drawer
	act := decideDrawerToggle(a.drawerPhase)
	if act == drawerActOpen {
		a.drawerPhase = drawerOpening // a double-click must not double-open
		a.drawerDark = dark
	}
	a.mu.Unlock()

	switch act {
	case drawerActIgnore:
		return // impatient second click during the first open: hold steady
	case drawerActHide:
		d.Hide() // onDrawerHidden resets the phase
		return
	}
	a.rec.Record(events.Event{Kind: events.KindDrawerOpen, Station: a.current.Key})
	// Show waits for the GTK thread to build the window on first open: keep
	// it off the tray goroutine.
	go a.showDrawer(d)
}

// showDrawer runs one Show and settles the phase: visible once the present
// is queued on the GTK thread (from then on a Hide queues after it, FIFO), or
// back to hidden if the drawer cannot start (headless, gtk_init failure).
func (a *App) showDrawer(d *drawer.Drawer) {
	t0 := time.Now()
	if err := d.Show(a.drawerState()); err != nil {
		log.Printf("ui: panneau: %v", err)
		a.onDrawerHidden()
		return
	}
	timingf("panneau: Show revenu %v après le clic (present en file)", time.Since(t0))
	a.mu.Lock()
	if a.drawerPhase == drawerOpening {
		a.drawerPhase = drawerVisible
	}
	a.mu.Unlock()
}

// openDrawer opens « le panneau » if it is not already open (or opening) and
// does nothing otherwise: the idempotent sibling of toggleDrawer for signals
// that can legitimately arrive while the panel is visible. On Linux it is the
// SNI DBusMenu's MenuOpen (the GNOME single-click path, which may fire twice
// for one interaction: menu opened, then its entry clicked). KindDrawerOpen
// is recorded once, only on an actual open, exactly like toggleDrawer.
func (a *App) openDrawer() {
	dark := drawer.DarkPreferred()
	a.mu.Lock()
	if a.drawer == nil {
		a.drawer = drawer.New(a.onDrawerCommand, a.onDrawerHidden)
	}
	d := a.drawer
	act := decideDrawerToggle(a.drawerPhase)
	if act == drawerActOpen {
		a.drawerPhase = drawerOpening
		a.drawerDark = dark
	}
	a.mu.Unlock()

	if act != drawerActOpen {
		return // already visible or opening: hold steady, record nothing
	}
	a.rec.Record(events.Event{Kind: events.KindDrawerOpen, Station: a.current.Key})
	go a.showDrawer(d)
}

// onDrawerHidden is the drawer's onHide callback: whatever hid the panel
// (Escape, the page's close button, the menu toggle), the menu entry must
// know it now opens again.
func (a *App) onDrawerHidden() {
	a.mu.Lock()
	a.drawerPhase = drawerHidden
	a.mu.Unlock()
}

// openDrawerSettings is the tray icon's ContextMenu (right click): the panel
// opens (or stays open) and lands on the Réglages view, the closest thing to
// the old right-click menu. KindDrawerOpen is recorded only on an actual
// open, like toggleDrawer.
func (a *App) openDrawerSettings() {
	dark := drawer.DarkPreferred()
	a.mu.Lock()
	if a.drawer == nil {
		a.drawer = drawer.New(a.onDrawerCommand, a.onDrawerHidden)
	}
	d := a.drawer
	act := decideDrawerToggle(a.drawerPhase)
	if act == drawerActOpen {
		a.drawerPhase = drawerOpening
		a.drawerDark = dark
	}
	a.mu.Unlock()

	// SetView is sticky until the page is ready, so ordering with Show is
	// safe on the very first open; when the panel is already open (or still
	// opening), it just lands on Réglages without a re-Show.
	d.SetView("settings")
	if act != drawerActOpen {
		return
	}
	a.rec.Record(events.Event{Kind: events.KindDrawerOpen, Station: a.current.Key})
	go a.showDrawer(d)
}

// scrollVolume is the tray icon's Scroll: a vertical wheel notch steps the
// ACTIVE sink's volume (the cast device while casting, mpv otherwise), like
// the panel's slider. On GNOME a wheel-up arrives as a negative vertical
// delta; only the sign is trusted (hosts scale deltas differently). The
// resulting change is recorded at its usual source (setVolume /
// setCastVolume), and an equal-value step (already at 0 or 100) records
// nothing.
func (a *App) scrollVolume(delta int, orientation string) {
	if orientation != "vertical" || delta == 0 {
		return
	}
	step := scrollVolumeStep
	if delta > 0 {
		step = -scrollVolumeStep
	}
	if sess := a.castSession(); sess != nil {
		if v, ok := sess.ReceiverVolume(); ok {
			a.setCastVolume(sess, clampPct(int(math.Round(v.Level*100))+step))
		}
		return
	}
	a.setVolume(clampPct(a.cfg.Volume + step))
}

// scrollVolumeStep is the volume change (percent) per wheel notch on the tray
// icon.
const scrollVolumeStep = 5

// drawerStations is the station strip data, straight from internal/stations
// (official brand colors included).
func drawerStations() []drawer.Station {
	out := make([]drawer.Station, len(stations.All))
	for i, s := range stations.All {
		out[i] = drawer.Station{Key: s.Key, Display: s.Display, Color: s.Color}
	}
	return out
}

// drawerState snapshots the full panel state. The page renders exclusively
// from this JSON, so everything the panel shows is assembled here.
func (a *App) drawerState() drawer.State {
	a.mu.Lock()
	np := a.now
	sess := a.castSess
	castName := a.castName
	devs := make([]string, len(a.castDevices))
	for i, d := range a.castDevices {
		devs[i] = d.Name
	}
	scanning := a.castScanning
	dark := a.drawerDark
	// The history view shows the same recent-tracks ring as the menu's
	// Historique submenu (refreshHistoryMenu): titles and artists, most
	// recent first. Clicking a row opens the artist page (open_history).
	hist := make([]drawer.HistoryEntry, len(a.history))
	for i, h := range a.history {
		hist[i] = drawer.HistoryEntry{Title: h.Title, Artist: h.Artist}
	}
	// Upcoming programmes for the « À venir » section (display only), same
	// data and cap as the menu's Calendrier, gated on the same setting.
	var upcoming []drawer.Upcoming
	if a.cfg.ShowCalendar {
		for i, s := range a.upcoming {
			if i >= calendarSlots {
				break
			}
			u := drawer.Upcoming{Title: s.Title}
			if !s.Start.IsZero() {
				u.Time = s.Start.Local().Format("15:04")
			}
			upcoming = append(upcoming, u)
		}
	}
	var showTitle string
	if np.Show != nil {
		showTitle = np.Show.Title
	}
	a.mu.Unlock()

	// Local output devices for « Sur cet appareil »: the cached mpv
	// enumeration, with the same labels the menu uses; a failed enumeration
	// degrades to the single Automatique entry. mpv lists every backend, so
	// the same physical sink appears once per backend and the alsa family
	// adds plugin pseudo-devices (rate converters, JACK bridges): where a
	// submenu could afford the full list, the panel keeps auto + the best
	// backend's sinks (pipewire, else pulse, else everything), deduplicated
	// by label. The selected device is always kept, so a sink chosen through
	// another backend still shows as active.
	backend := ""
	for _, want := range []string{"pipewire", "pulse"} {
		for _, dev := range a.audioDevs {
			if dev.Name == want || strings.HasPrefix(dev.Name, want+"/") {
				backend = want
				break
			}
		}
		if backend != "" {
			break
		}
	}
	audio := make([]drawer.AudioDevice, 0, len(a.audioDevs)+1)
	seen := map[string]bool{}
	for _, dev := range a.audioDevs {
		selected := dev.Name == a.cfg.AudioDevice
		inBackend := dev.Name == backend || strings.HasPrefix(dev.Name, backend+"/")
		if backend != "" && dev.Name != "auto" && !inBackend && !selected {
			continue
		}
		label := dev.Description
		if dev.Name == "auto" {
			label = "Automatique"
		} else if label == "" {
			label = dev.Name
		}
		if seen[label] && !selected {
			continue
		}
		seen[label] = true
		audio = append(audio, drawer.AudioDevice{Name: dev.Name, Label: label})
	}
	if len(audio) == 0 {
		audio = append(audio, drawer.AudioDevice{Name: "auto", Label: "Automatique"})
	}
	curAudio := a.cfg.AudioDevice
	if curAudio == "" {
		curAudio = "auto"
	}

	st := drawer.State{
		Dark:         dark,
		Station:      a.current.Key,
		Playing:      a.player != nil && a.player.IsPlaying(),
		Volume:       a.cfg.Volume,
		Muted:        a.cfg.Mute,
		Devices:      devs,
		Scanning:     scanning,
		AudioDevices: audio,
		AudioDevice:  curAudio,
		Track:        drawer.Track{Title: np.Title, Artist: np.Artist, Artwork: np.CoverURL, HasLink: np.Link != ""},
		Show:         showTitle,
		Stations:     drawerStations(),
		Settings: drawer.Settings{
			Stats:              a.cfg.Stats,
			HiFi:               a.cfg.HiFi,
			Notifications:      a.cfg.Notifications,
			ShowNotifications:  a.cfg.ShowNotifications,
			ShowCalendar:       a.cfg.ShowCalendar,
			AnimatedIcon:       a.cfg.AnimatedIcon,
			HistoryFile:        a.cfg.HistoryFile,
			UpdateStartup:      a.cfg.UpdateStartup,
			PlayOnStart:        a.cfg.PlayOnStart,
			Autostart:          a.cfg.Autostart,
			AutostartSupported: config.AutostartSupported,
			CrossfadeSecs:      a.cfg.CrossfadeSecs,
		},
		History:  hist,
		Upcoming: upcoming,
		Version:  version.String(),
	}
	if sess != nil {
		st.Cast = drawer.Cast{Active: true, DeviceName: castName, Playing: sess.MediaPlaying()}
		if v, ok := sess.ReceiverVolume(); ok {
			// The DEVICE's own level: displayed, never invented. On a
			// controlType "master" device this is the amp's master volume.
			st.Cast.Volume = clampPct(int(math.Round(v.Level * 100)))
			st.Cast.Muted = v.Muted
			st.Cast.VolumeKnown = true
			st.Cast.ControlType = v.ControlType
		}
	}
	return st
}

// pushDrawerState refreshes the panel's state (a no-op before the first
// open). Called from the same chokepoints that refresh the tray menu, so the
// two views can never disagree.
func (a *App) pushDrawerState() {
	a.mu.Lock()
	d := a.drawer
	a.mu.Unlock()
	if d == nil {
		return
	}
	d.Push(a.drawerState())
}

// onCastStatus mirrors a device-side status update (its volume knob turned,
// its media paused) into the panel. Called from the session's read goroutine.
func (a *App) onCastStatus() { a.pushDrawerState() }

// onDrawerCommand routes a panel action. Every branch lands on an EXISTING
// measurable chokepoint (togglePlay/setPlayingUI, setVolume, toggleMute,
// setStation, castToDevice/stopCasting) or records at source here
// (cast_pause/cast_resume, cast volume/mute): a panel click can never dodge
// the events log. Runs off the GTK thread (the drawer dispatches commands on
// a fresh goroutine, like the MPRIS handlers cross goroutines).
func (a *App) onDrawerCommand(c drawer.Command) {
	switch c.Action {
	case "toggle_play":
		// The button drives the ACTIVE sink: the device's media transport
		// while casting, the local player otherwise. The menu/MPRIS
		// semantics (play while casting = bring the music home) are
		// deliberately untouched.
		if sess := a.castSession(); sess != nil {
			if sess.MediaPlaying() {
				a.castPause(sess)
			} else {
				a.castResume(sess)
			}
		} else {
			a.togglePlay()
		}
	case "volume":
		if sess := a.castSession(); sess != nil {
			a.setCastVolume(sess, c.Value)
		} else {
			a.setVolume(c.Value)
		}
	case "toggle_mute":
		if sess := a.castSession(); sess != nil {
			a.toggleCastMuted(sess)
		} else {
			a.toggleMute()
		}
	case "station":
		if stations.Exists(c.Key) {
			a.setStation(c.Key) // startStation records the Markov edge
		}
	case "output":
		if c.Value < 0 {
			a.stopCasting(true) // records cast_stop at source; no-op when local
		} else {
			a.castToDevice(c.Value) // records cast_start at source on success
		}
	case "rescan":
		a.rescanCast()
	case "audio_device":
		// A local sink pick from « Sur cet appareil »: same kind the menu item
		// records via a.on. Picking a local output while casting also means
		// "play here": stop the cast (records cast_stop at source) and resume.
		a.rec.Record(events.Event{Kind: events.KindAudioDevice, Station: a.current.Key})
		if a.castSession() != nil {
			a.stopCasting(true)
		}
		a.setAudioDevice(c.Key)
	case "crossfade":
		a.setCrossfade(c.Value) // records KindCrossfade at source

	// Links out of the current track and the history rows: same fixed kinds
	// their menu twins record via a.on.
	case "open_wiki":
		a.rec.Record(events.Event{Kind: events.KindOpenWiki, Station: a.current.Key})
		a.openNow()
	case "open_link":
		a.rec.Record(events.Event{Kind: events.KindOpenLink, Station: a.current.Key})
		a.openNowLink()
	case "open_history":
		a.rec.Record(events.Event{Kind: events.KindOpenHistory, Station: a.current.Key})
		a.openHistory(c.Value)

	// Taste verdicts: the click IS the consent, exactly like the menu items.
	// The a.on chokepoint records the kind for the menu twin; here the record
	// happens before the same handler runs, so each verdict logs once
	// (behaviour only) and prefs.jsonl gets the track identity.
	case "like":
		a.rec.Record(events.Event{Kind: events.KindLike, Station: a.current.Key})
		a.recordTaste(prefs.Like)
	case "dislike":
		a.rec.Record(events.Event{Kind: events.KindDislike, Station: a.current.Key})
		a.recordTaste(prefs.Dislike)

	// Settings: every branch is the SAME handler func the menu checkbox uses,
	// which records its kind at source and keeps the menu checkmarks in sync
	// (two skins over one wiring).
	case "toggle_stats":
		a.toggleStats()
	case "toggle_hifi":
		a.toggleHiFi()
	case "toggle_notif":
		a.toggleNotif()
	case "toggle_show_notif":
		a.toggleShowNotif()
	case "toggle_show_calendar":
		a.toggleShowCalendar()
	case "toggle_anim":
		a.toggleAnim()
	case "toggle_hist_file":
		a.toggleHistFile()
	case "toggle_update_startup":
		a.toggleUpdateStartup()
	case "toggle_play_on_start":
		a.togglePlayOnStart()
	case "toggle_autostart":
		if config.AutostartSupported {
			a.toggleAutostart()
		}
	case "stats_view":
		a.rec.Record(events.Event{Kind: events.KindStatsView, Station: a.current.Key})
		a.viewStats()
	case "stats_folder":
		a.openDataDir() // like the menu item: plumbing, no kind
	case "stats_clear":
		// The page already ran its own two-click confirm (arm with a visual
		// warning, then confirm), so this lands on the confirmed action.
		a.clearStatsData()
	case "prefs_clear":
		a.clearPrefsData() // records KindPrefsClear at source
	case "update_check":
		a.rec.Record(events.Event{Kind: events.KindUpdateCheck, Station: a.current.Key})
		a.checkUpdates()
	case "restart":
		a.rec.Record(events.Event{Kind: events.KindRestart, Station: a.current.Key})
		a.restart()
		return // restart quits: no state push into teardown
	case "open_fip":
		a.rec.Record(events.Event{Kind: events.KindOpenFip, Station: a.current.Key})
		open.URL(fipURL)
	case "open_github":
		a.rec.Record(events.Event{Kind: events.KindOpenAbout, Station: a.current.Key})
		open.URL(repoURL)
	case "quit":
		a.rec.Record(events.Event{Kind: events.KindQuit, Station: a.current.Key})
		quitApp()
		return // no state push into teardown

	// The page's error trap: any JS exception or undeclared action lands here
	// so a broken render is loud in the app log, never a silent blank view.
	case "js_error":
		log.Printf("ui: panneau: erreur JS: %s", c.Key)
	default:
		log.Printf("ui: panneau: commande inconnue %q", c.Action)
	}
	a.pushDrawerState()
}

// castPause pauses the media ON the cast device (the session stays up), the
// panel's transport while casting. Recorded at source as cast_pause
// (behaviour only, never a device identity).
func (a *App) castPause(sess *cast.Session) {
	if err := sess.PauseMedia(); err != nil {
		log.Printf("ui: cast pause: %v", err)
		return
	}
	a.rec.Record(events.Event{Kind: events.KindCastPause, Station: a.current.Key})
}

// castResume resumes the paused media on the cast device (cast_resume).
func (a *App) castResume(sess *cast.Session) {
	if err := sess.PlayMedia(); err != nil {
		log.Printf("ui: cast resume: %v", err)
		return
	}
	a.rec.Record(events.Event{Kind: events.KindCastResume, Station: a.current.Key})
}

// setCastVolume drives the DEVICE's volume (quantized to its stepInterval by
// the session; on a "master" device this is the amplifier's master volume, so
// only user-chosen levels are ever sent). Recorded at source with the same
// kind and Value convention as local volume changes; the device identity
// never reaches the log. The device's RECEIVER_STATUS answer refreshes the
// panel via onCastStatus.
func (a *App) setCastVolume(sess *cast.Session, pct int) {
	pct = clampPct(pct)
	// Equal-value skip: a live slider drag repeats values (throttled page-side
	// to ~8/s, but still); when the device already reports this level there is
	// nothing to send, record or refresh. The comparison uses the last known
	// RECEIVER_STATUS, so a stale status errs on the side of sending.
	if v, ok := sess.ReceiverVolume(); ok && clampPct(int(math.Round(v.Level*100))) == pct {
		return
	}
	if err := sess.SetVolume(float64(pct) / 100); err != nil {
		log.Printf("ui: cast volume: %v", err)
		return
	}
	a.recordVolume(pct)
}

// toggleCastMuted flips the device-side mute (the level is preserved
// device-side). Same event convention as the local mute toggle.
func (a *App) toggleCastMuted(sess *cast.Session) {
	v, _ := sess.ReceiverVolume()
	muted := !v.Muted
	if err := sess.SetMuted(muted); err != nil {
		log.Printf("ui: cast mute: %v", err)
		return
	}
	a.rec.Record(events.Event{Kind: events.KindMute, Station: a.current.Key, Value: b2i(muted)})
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
	a.clearStatsData()
}

// clearStatsData is the CONFIRMED "Effacer les statistiques" action: it
// deletes only events.jsonl, never history.jsonl. Shared by the menu's
// two-click confirm and the panel's (the page arms and confirms on its side).
func (a *App) clearStatsData() {
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
	a.clearPrefsData()
}

// clearPrefsData is the CONFIRMED "Effacer mes goûts" action: it deletes only
// prefs.jsonl, never events.jsonl or history.jsonl, and records the single
// KindPrefsClear behaviour event at source (into events.jsonl, a different
// file, so it never resurrects the taste log just deleted). Shared by the
// menu's two-click confirm and the panel's.
func (a *App) clearPrefsData() {
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
	quitApp() // clean teardown: records app_stop, closes mpv and D-Bus
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
	// Every persisted setting is visible in the panel's settings view: saving
	// config is the one chokepoint all toggles pass through, so pushing here
	// keeps the two skins in sync whichever one was clicked (a cheap no-op
	// before the panel is first opened).
	a.pushDrawerState()
}

// --- mpris.Controller ---

// Play resumes playback (rejoins live). While casting, an external play
// (media key, playerctl, `fip play`) brings the music back to this machine,
// like the menu's play button: local audio must never double the speaker's.
func (a *App) Play() {
	if a.castSession() != nil {
		a.stopCasting(true)
		return
	}
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
