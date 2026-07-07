// Package ui builds the system-tray menu and wires together the player,
// metadata, MPRIS and notifications.
package ui

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/metadata"
	"github.com/PLNech/fipindicateur/internal/mpris"
	"github.com/PLNech/fipindicateur/internal/notify"
	"github.com/PLNech/fipindicateur/internal/open"
	"github.com/PLNech/fipindicateur/internal/player"
	"github.com/PLNech/fipindicateur/internal/stations"
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

	// menu items
	mNow      *systray.MenuItem
	mVoirWiki *systray.MenuItem
	mVoirLink *systray.MenuItem
	mPlay     *systray.MenuItem
	stationMI map[string]*systray.MenuItem
	histMI    []*systray.MenuItem
	mHiFi     *systray.MenuItem
	mNotif    *systray.MenuItem
	mAuto     *systray.MenuItem
	mHistFile *systray.MenuItem
	mAnim     *systray.MenuItem
	mVolume   *systray.MenuItem
	mMute     *systray.MenuItem
	volMI     map[int]*systray.MenuItem
}

// volumePresets are the quick-pick volume levels in the tray menu.
var volumePresets = []int{10, 25, 50, 75, 100}

// New returns an App with loaded config.
func New() *App {
	return &App{
		cfg:       config.Load(),
		meta:      metadata.NewManager(),
		stationMI: map[string]*systray.MenuItem{},
	}
}

// OnReady is the systray onReady callback: it builds everything and starts
// playing the last station.
func (a *App) OnReady() {
	a.current = stations.ByKey(a.cfg.Station)

	a.player = &player.MPV{TitleChanged: a.meta.PushTitle}
	if err := a.player.Initialize(); err != nil {
		log.Fatalf("ui: player init: %v", err)
	}
	// Apply persisted volume/mute before the first loadfile.
	a.player.SetVolume(float64(a.cfg.Volume))
	a.player.SetMute(a.cfg.Mute)

	if ins, err := mpris.Connect(a); err != nil {
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
	a.startStation(a.current, true)
}

// OnExit tears everything down.
func (a *App) OnExit() {
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
	go a.onClick(a.mNow.ClickedCh, a.openNow)

	voir := systray.AddMenuItem("Voir…", "Liens pour ce titre")
	a.mVoirWiki = voir.AddSubMenuItem("Wikipédia (artiste)", "Chercher l'artiste sur fr.wikipedia.org")
	go a.onClick(a.mVoirWiki.ClickedCh, a.openNow)
	a.mVoirLink = voir.AddSubMenuItem("Écouter ailleurs (lien FIP)", "Lien musique fourni par Radio France")
	a.mVoirLink.Disable()
	go a.onClick(a.mVoirLink.ClickedCh, a.openNowLink)

	systray.AddSeparator()
	a.mPlay = systray.AddMenuItem("⏸ Pause", "Lecture / pause")
	go a.onClick(a.mPlay.ClickedCh, a.togglePlay)

	// Volume
	a.mVolume = systray.AddMenuItem(volumeLabel(a.cfg.Volume), "Volume de lecture")
	a.mMute = a.mVolume.AddSubMenuItemCheckbox("Muet", "Couper le son", a.cfg.Mute)
	go a.onClick(a.mMute.ClickedCh, a.toggleMute)
	a.volMI = map[int]*systray.MenuItem{}
	for _, pct := range volumePresets {
		it := a.mVolume.AddSubMenuItemCheckbox(fmt.Sprintf("%d %%", pct), "", pct == a.cfg.Volume)
		a.volMI[pct] = it
		p := pct
		go a.onClick(it.ClickedCh, func() { a.setVolume(p) })
	}

	// Radios
	radios := systray.AddMenuItem("Radios", "Choisir une webradio")
	for _, s := range stations.All {
		it := radios.AddSubMenuItemCheckbox(s.Display, s.Slug, s.Key == a.current.Key)
		a.stationMI[s.Key] = it
		key := s.Key
		go a.onClick(it.ClickedCh, func() { a.setStation(key) })
	}
	fipItem := radios.AddSubMenuItem("FIP sur radiofrance.fr", fipURL)
	go a.onClick(fipItem.ClickedCh, func() { open.URL(fipURL) })

	// Historique
	hist := systray.AddMenuItem("Historique", "Titres récents")
	a.histMI = make([]*systray.MenuItem, historySlots)
	for i := 0; i < historySlots; i++ {
		it := hist.AddSubMenuItem("", "")
		it.Hide()
		a.histMI[i] = it
		idx := i
		go a.onClick(it.ClickedCh, func() { a.openHistory(idx) })
	}

	// Réglages
	settings := systray.AddMenuItem("Réglages", "Options")
	a.mHiFi = settings.AddSubMenuItemCheckbox("Haute qualité (AAC 192k)", "", a.cfg.HiFi)
	go a.onClick(a.mHiFi.ClickedCh, a.toggleHiFi)
	a.mNotif = settings.AddSubMenuItemCheckbox("Notifications", "", a.cfg.Notifications)
	go a.onClick(a.mNotif.ClickedCh, a.toggleNotif)
	// Launch at login is XDG-only (writes ~/.config/autostart/*.desktop); hide
	// it where config.SetAutostart is a no-op (macOS and other non-Linux).
	if config.AutostartSupported {
		a.mAuto = settings.AddSubMenuItemCheckbox("Lancer au démarrage", "", a.cfg.Autostart)
		go a.onClick(a.mAuto.ClickedCh, a.toggleAutostart)
	}
	a.mHistFile = settings.AddSubMenuItemCheckbox("Historique local (fichier)", "Journal des titres dans ~/.local/share/fipindicateur/history.jsonl", a.cfg.HistoryFile)
	go a.onClick(a.mHistFile.ClickedCh, a.toggleHistFile)
	a.mAnim = settings.AddSubMenuItemCheckbox("Icône animée", "Barres qui suivent le niveau audio", a.cfg.AnimatedIcon)
	go a.onClick(a.mAnim.ClickedCh, a.toggleAnim)

	systray.AddSeparator()
	about := systray.AddMenuItem("À propos", "Ouvrir la page du projet")
	go a.onClick(about.ClickedCh, func() { open.URL(repoURL) })
	quit := systray.AddMenuItem("Quitter", "Fermer le fipindicateur")
	go a.onClick(quit.ClickedCh, func() { systray.Quit() })

	systray.SetTitle("")
	systray.SetTooltip("le fipindicateur")
}

// onClick loops over a menu item's click channel, running fn each time.
func (a *App) onClick(ch <-chan struct{}, fn func()) {
	for range ch {
		fn()
	}
}

// startStation switches to a station: stop, load new URL, restart metadata.
func (a *App) startStation(s stations.Station, play bool) {
	if a.watchCancel != nil {
		a.watchCancel()
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
		a.save()
		a.player.SetVolume(float64(pct))
		if a.mpris != nil {
			a.mpris.SetVolume(float64(pct) / 100)
		}
	}
	a.applyVolumeUI()
}

func (a *App) toggleMute() {
	a.cfg.Mute = !a.cfg.Mute
	a.save()
	a.player.SetMute(a.cfg.Mute)
	a.applyVolumeUI()
}

// SetVolumeFrac implements mpris.Controller: an external client (playerctl,
// GNOME) wrote the Volume property. Reflect it in player, config and menu.
// The equal-value early return breaks any publish/callback echo loop.
func (a *App) SetVolumeFrac(v float64) {
	pct := int(math.Round(v * 100))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if pct == a.cfg.Volume {
		return
	}
	a.cfg.Volume = pct
	a.save()
	a.player.SetVolume(float64(pct))
	a.applyVolumeUI()
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

// openTrack opens the primary link for a track: Wikipedia search for the
// artist (fr.wp, FIP is French). DuckDuckGo is the fallback when the artist is
// unknown. The metadata Link (often Apple Music) stays available as the
// secondary "Voir…" item.
func (a *App) openTrack(np metadata.NowPlaying) {
	if np.Empty() {
		return
	}
	if np.Artist != "" {
		open.URL(open.WikipediaFr(np.Artist))
		return
	}
	open.URL(open.Search(np.Title))
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
