//go:build !linux

package ui

// The tray-menu half of the UI, for the platforms where the menu IS the UI
// (macOS, Windows): fyne/systray lifecycle, menu construction, the a.on click
// chokepoint and the zenity slider dialogs. On Linux the drawer replaces all
// of this (see ui_sni_linux.go); the shared core in ui.go touches menu items
// only through the nil-safe menuItem indirection.

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/open"
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/version"
)

// menuItem is the real systray item on the menu platforms. The Linux build
// substitutes a nil-safe no-op type, so shared chokepoints (setPlayingUI,
// applyVolumeUI, the toggles) update menu state without per-platform branches.
type menuItem = systray.MenuItem

// Run hands the process to the systray main loop; OnReady/OnExit carry the
// app lifecycle exactly as before.
func Run(a *App) {
	systray.Run(a.OnReady, a.OnExit)
}

// Quit asks the main loop to end (signal handler, CLI); systray then runs
// OnExit.
func Quit() {
	systray.Quit()
}

// quitApp is the in-app quit chokepoint (menu item, drawer command, restart).
func quitApp() { Quit() }

// setTrayIcon is the single, platform-neutral chokepoint for handing bytes to
// systray. It is the ONLY place `systray.SetIcon` is called in the tree (the
// guard test in guard_test.go enforces that). App.setIcon dedupes and refuses
// empty bytes before reaching here; this layer only adapts the byte format to
// what the platform's tray expects, via encodeTrayIcon.
func setTrayIcon(b []byte) {
	systray.SetIcon(encodeTrayIcon(b))
}

// setTrayNowLabel is a no-op here: the now-playing label lives on the mNow
// menu item, which the throttle updates directly.
func (a *App) setTrayNowLabel(string) {}

// teardownUI has nothing to release: systray closes its own connection when
// the main loop ends.
func (a *App) teardownUI() {}

// volumePresets are the quick-pick volume levels in the tray menu.
var volumePresets = []int{10, 25, 50, 75, 100}

// crossfadePresets are the quick-pick crossfade durations (seconds) for the
// fallback submenu shown when zenity is unavailable. 0 = hard cut.
var crossfadePresets = []int{0, 2, 4, 6}

// buildUI builds the tray menu (the whole UI on these platforms).
func (a *App) buildUI() { a.buildMenu() }

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

	// Volume submenu. Volume stays measurable through the same setters the
	// Linux panel drives.
	a.mVolume = systray.AddMenuItem(volumeLabel(a.cfg.Volume), "Volume de lecture")
	a.mMute = a.mVolume.AddSubMenuItemCheckbox("Muet", "Couper le son", a.cfg.Mute)
	a.on(a.mMute, "", a.toggleMute) // toggleMute records the resulting state
	a.volMI = map[int]*menuItem{}
	for _, pct := range volumePresets {
		it := a.mVolume.AddSubMenuItemCheckbox(fmt.Sprintf("%d %%", pct), "", pct == a.cfg.Volume)
		a.volMI[pct] = it
		p := pct
		a.on(it, "", func() { a.setVolume(p) }) // setVolume records the level
	}
	// A real slider, when zenity is present. Absent on macOS/Windows and
	// minimal installs, so the item is only added when the binary exists (no
	// dead item). The resulting volume change is the measurable action,
	// recorded once at source on OK inside runVolumeSlider, so this click
	// carries no Kind.
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

	// Diffuser sur…: cast the antenna to a Chromecast speaker. Devices are
	// discovered fresh each run, nothing persisted; the slots are
	// pre-allocated because systray cannot remove items once added.
	castMenu := systray.AddMenuItem("Diffuser sur…", "Diffuser la station sur un appareil Chromecast")
	a.mCastLocal = castMenu.AddSubMenuItemCheckbox("Cet ordinateur", "Lecture locale (arrête la diffusion)", true)
	// State-dependent: stopCasting records cast_stop at source only when a
	// cast was actually active, so a redundant click logs nothing.
	a.on(a.mCastLocal, "", func() { a.stopCasting(true) })
	a.castMI = make([]*menuItem, castSlots)
	for i := 0; i < castSlots; i++ {
		it := castMenu.AddSubMenuItemCheckbox("", "", false)
		it.Hide()
		a.castMI[i] = it
		idx := i
		// State-dependent: startCast records cast_start at source once the
		// device accepted the stream (a failed cast logs nothing).
		a.on(it, "", func() { a.castToDevice(idx) })
	}
	a.mCastNone = castMenu.AddSubMenuItem("Aucun appareil trouvé", "Aucun Chromecast détecté sur le réseau local")
	a.mCastNone.Disable()
	a.mCastScan = castMenu.AddSubMenuItem("Rechercher les appareils", "Chercher les Chromecast sur le réseau (mDNS)")
	// Discovery is ambient plumbing, not a listening behaviour: the
	// measurable actions here are cast_start/cast_stop, recorded at source.
	a.on(a.mCastScan, "", a.rescanCast)

	// Historique
	hist := systray.AddMenuItem("Historique", "Titres récents")
	a.histMI = make([]*menuItem, historySlots)
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
	a.calMI = make([]*menuItem, calendarSlots)
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
		a.crossfadeMI = map[int]*menuItem{}
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
	// Lecture au lancement: whether launching also starts the stream. Off by
	// default (autostart is not autoplay): the tray tunes and indicates the
	// antenna silently, and you press play when you want sound.
	a.mPlayOnStart = settings.AddSubMenuItemCheckbox("Lecture au lancement", "Démarrer la lecture au lancement (sinon l'antenne est en pause)", a.cfg.PlayOnStart)
	a.on(a.mPlayOnStart, "", a.togglePlayOnStart)
	a.mHistFile = settings.AddSubMenuItemCheckbox("Historique local (fichier)", "Journal des titres dans ~/.local/share/fipindicateur/history.jsonl", a.cfg.HistoryFile)
	a.on(a.mHistFile, "", a.toggleHistFile)
	a.mAnim = settings.AddSubMenuItemCheckbox("Icône animée", "Barres qui suivent le niveau audio", a.cfg.AnimatedIcon)
	a.on(a.mAnim, "", a.toggleAnim)

	// Sortie audio: pick the output sink through mpv's audio-device property,
	// so no pavucontrol or macOS audio panel is needed. mpv enumerated the
	// devices at startup (a.audioDevs, shared with the Linux panel); the list
	// already carries an "auto" entry.
	audio := settings.AddSubMenuItem("Sortie audio", "Choisir la sortie audio")
	a.audioMI = map[string]*menuItem{}
	cur := a.cfg.AudioDevice
	if cur == "" {
		cur = "auto" // empty config means mpv's automatic default
	}
	if len(a.audioDevs) > 0 {
		for _, dev := range a.audioDevs {
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
	a.on(quit, events.KindQuit, quitApp)

	systray.SetTitle("")
	systray.SetTooltip("le fipindicateur")
}

// on wires a menu item's click to fn and, for a fixed-kind action, records the
// event automatically. This is the "measurable by design" chokepoint: every
// clickable item goes through here, so an action cannot be added without a
// telemetry decision. A kind of "" means the handler records its own event at
// source (state-dependent actions like play/pause, volume, station change).
// (On Linux the drawer's onDrawerCommand plays this role.)
//
// Invariant (enforced by TestSingleOnClickCallSite): the only onClick loop in
// this package lives below. Add clickable items via a.on, never onClick.
func (a *App) on(mi *menuItem, kind events.Kind, fn func()) {
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

// --- zenity dialogs (volume slider, crossfade slider) ---

// zenityAvailable reports whether the zenity binary is on PATH. macOS,
// Windows and minimal installs usually lack it, so the slider items are only
// added when it exists (no dead menu entry).
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
