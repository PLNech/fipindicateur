//go:build linux

package ui

// The Linux half of the UI: the tray menu IS the whole UI, built on the
// hand-rolled StatusNotifierItem (internal/sni) and its dynamic dbusmenu
// (see sni/dbusmenu.go):
//   left click     -> the shell opens the menu (ItemIsMenu=true on spec hosts;
//                     on GNOME the AppIndicator extension toggles the popup
//                     for any item whose menu has entries)
//   double click   -> Activate, the primary action: play/pause
//   middle click   -> SecondaryActivate, play/pause (same semantics)
//   vertical wheel -> volume steps on the active sink
// The drawer (« le panneau », internal/drawer) is abandoned: its code stays
// in the tree, unwired (drawerEnabled=false in ui.go). The menu structure
// mirrors ui_menu.go (macOS/Windows) item for item; every click is wired
// through the shared App.on chokepoint, exactly like there.
// The main loop is a plain quit-channel wait: D-Bus (SNI, MPRIS) runs on
// godbus goroutines, so nothing needs the main goroutine.

import (
	"fmt"
	"image/color"
	"log"
	"sync"

	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/open"
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/sni"
	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/version"
)

// protoBrand is the PROTO A brand: the first menu item names the prototype,
// and every tray icon byte is retinted toward this hue so the prototype is
// recognisable at a glance next to the production build. The hue is FIP's own
// design token (the antenna's official brand color, stations.go), so the
// prototype wears the house magenta rather than an invented tint.
var protoBrand = icon.ParseHex(stations.ByKey("fip").Color)

// protoBrandMix is how far each icon pixel is pulled toward protoBrand. High
// on purpose: at panel scale (44px bytes, ~22px on the bar) a shy mix reads
// as a dull wash, and the point of the pass is that this build cannot be
// mistaken for the production one. Both panel themes stay legible: the mix
// keeps the glyph's shape (bars) and alpha, only the ink moves.
const protoBrandMix = 0.9

// menuItem adapts a live dbusmenu entry to the shared menu-item surface used
// by the chokepoints in ui.go (setPlayingUI, applyVolumeUI, the toggles).
// ClickedCh comes from the embedded *sni.Entry (one post per activation the
// host reports). Every mutator is nil-safe: the chokepoints keep calling them
// on unbuilt fields (a bare App in the tests, a bus-less startup), exactly
// like the no-op stub this type replaced. dbusmenu has no tooltips, so
// SetTooltip is a no-op.
type menuItem struct{ *sni.Entry }

func (m *menuItem) live() bool { return m != nil && m.Entry != nil }

func (m *menuItem) SetTitle(s string) {
	if m.live() {
		m.Entry.SetLabel(s)
	}
}
func (m *menuItem) SetTooltip(string) {}
func (m *menuItem) Enable() {
	if m.live() {
		m.Entry.Enable()
	}
}
func (m *menuItem) Disable() {
	if m.live() {
		m.Entry.Disable()
	}
}
func (m *menuItem) Check() {
	if m.live() {
		m.Entry.Check()
	}
}
func (m *menuItem) Uncheck() {
	if m.live() {
		m.Entry.Uncheck()
	}
}
func (m *menuItem) Show() {
	if m.live() {
		m.Entry.Show()
	}
}
func (m *menuItem) Hide() {
	if m.live() {
		m.Entry.Hide()
	}
}

// mitem wraps a dbusmenu entry as the shared menu-item surface.
func mitem(e *sni.Entry) *menuItem { return &menuItem{Entry: e} }

// The process's one SNI item. setIcon (via setTrayIcon) can run before
// buildUI created the item (OnReady sets the icon first thing, to minimise
// the no-pixmap window): the bytes are stashed and used as the item's initial
// icon. Package-level like the old drawer's `current`: one app per process.
var (
	sniMu      sync.Mutex
	sniItem    *sni.Item
	sniPending []byte
)

// quit plumbing for Run/Quit.
var (
	quitCh   = make(chan struct{})
	quitOnce sync.Once
)

// Run is the Linux main loop: set up, wait for Quit, tear down. Signals and
// in-app quit both land on Quit, so OnExit (app_stop event, recorder flush,
// cast goodbye, control socket) runs exactly once on every exit path.
func Run(a *App) {
	a.OnReady()
	<-quitCh
	a.OnExit()
}

// Quit unblocks Run. Safe to call multiple times and from any goroutine.
func Quit() {
	quitOnce.Do(func() { close(quitCh) })
}

// quitApp is the in-app quit chokepoint (menu item, restart).
func quitApp() { Quit() }

// setTrayIcon hands PNG bytes to the StatusNotifierItem (which repacks them
// as an ARGB32 pixmap and signals NewIcon), after the PROTO A branding pass:
// every pixel is pulled toward the prototype hue, so this build cannot be
// mistaken for the production one. Before the item exists, the last bytes
// are kept as its initial icon.
func setTrayIcon(b []byte) {
	b = icon.Retint(b, protoBrand, protoBrandMix)
	sniMu.Lock()
	it := sniItem
	if it == nil {
		sniPending = append(sniPending[:0:0], b...)
	}
	sniMu.Unlock()
	if it != nil {
		it.SetIconPNG(b)
	}
}

// setTrayNowLabel pushes the now-playing label as the item's title and
// tooltip (what the mNow menu item also shows as its label).
func (a *App) setTrayNowLabel(label string) {
	sniMu.Lock()
	it := sniItem
	sniMu.Unlock()
	if it != nil {
		it.SetLabel("le fipindicateur", label)
	}
}

// buildUI registers the StatusNotifierItem, wires its raw input events onto
// the shared handlers, then builds the whole menu (the whole UI) on its
// dbusmenu. Best-effort like the old tray: if the session bus is unreachable
// the app runs on (MPRIS, control socket and media keys still work), just
// without an icon or a menu.
func (a *App) buildUI() {
	sniMu.Lock()
	initial := sniPending
	sniPending = nil
	sniMu.Unlock()
	if len(initial) == 0 {
		initial = icon.Rest(false, color.NRGBA{}) // OnReady sets one first; belt and braces
	}
	it, err := sni.New("fipindicateur", "le fipindicateur", icon.Retint(initial, protoBrand, protoBrandMix), sni.Handlers{
		// A single left click opens the menu: the shell pops the DBusMenu
		// itself (ItemIsMenu=true), so no handler is involved. A GNOME
		// double click still lands here (the extension cancels the pending
		// menu-open), so the primary action plays/pauses, like the middle
		// click.
		Activate:          a.togglePlay,
		SecondaryActivate: a.togglePlay,
		Scroll:            a.scrollVolume,
	})
	if err != nil {
		log.Printf("ui: statusnotifier: %v (pas d'icône; MPRIS/fip restent accessibles)", err)
		return
	}
	sniMu.Lock()
	sniItem = it
	sniMu.Unlock()
	a.buildMenu(it.Menu())
}

// teardownUI closes the SNI connection so the icon leaves the shell promptly.
func (a *App) teardownUI() {
	sniMu.Lock()
	it := sniItem
	sniItem = nil
	sniMu.Unlock()
	if it != nil {
		it.Close()
	}
}

// buildMenu builds the whole tray menu on the SNI's dbusmenu, mirroring
// ui_menu.go item for item (French copy included). Every clickable item is
// wired through App.on, the telemetry chokepoint, with exactly the same
// fixed-kind / record-at-source split as the macOS/Windows menu.
func (a *App) buildMenu(m *sni.Menu) {
	// PROTO A brand header: names the prototype, display-only (disabled),
	// carrying the violet bars glyph as its icon.
	brand := m.Add("PROTO A · SNI maison")
	brand.SetIconData(icon.BarsIcon(icon.RestPose(), protoBrand))
	brand.Disable()

	a.mNow = mitem(m.Add("FIP"))
	a.on(a.mNow, events.KindOpenWiki, a.openNow)

	// The programme (émission) currently on air. Display-only (disabled),
	// hidden until a show is playing. Shows exist only on the main antenna.
	a.mShow = mitem(m.Add(""))
	a.mShow.Disable()
	a.mShow.Hide()

	voir := m.Add("Voir…")
	a.mVoirWiki = mitem(voir.Add("Wikipédia (artiste)"))
	a.on(a.mVoirWiki, events.KindOpenWiki, a.openNow)
	a.mVoirLink = mitem(voir.Add("Écouter ailleurs (lien FIP)"))
	a.mVoirLink.Disable()
	a.on(a.mVoirLink, events.KindOpenLink, a.openNowLink)

	// Taste signals: an explicit verdict on the current track. Unlike the
	// events log, prefs has no opt-in gate: the click itself is the consent
	// (see internal/prefs). The a.on chokepoint records the behaviour
	// (KindLike/KindDislike, station only, no track identity); the handler
	// snapshots the track into prefs.jsonl. Both are no-ops when nothing is
	// playing.
	a.mLike = mitem(m.Add("J'aime ce morceau"))
	a.mLike.Disable() // enabled once a track is known (see onNowPlaying)
	a.on(a.mLike, events.KindLike, func() { a.recordTaste(prefs.Like) })
	a.mDislike = mitem(m.Add("Pas pour moi"))
	a.mDislike.Disable()
	a.on(a.mDislike, events.KindDislike, func() { a.recordTaste(prefs.Dislike) })

	m.AddSeparator()
	a.mPlay = mitem(m.Add("⏸ Pause"))
	// Play/pause is state-dependent: setPlayingUI records the resulting
	// play/pause event (so media keys and MPRIS are captured too), hence "".
	a.on(a.mPlay, "", a.togglePlay)

	// Volume submenu. Volume stays measurable through the same setters the
	// wheel drives, and every item drives the ACTIVE sink: the cast device
	// while casting, mpv otherwise (setActiveVolume / toggleActiveMute).
	// The submenu's own title says which one it is talking to
	// (applyVolumeUI).
	vol := m.Add(volumeLabel(a.cfg.Volume))
	a.mVolume = mitem(vol)
	a.mMute = mitem(vol.AddCheckbox("Muet", a.cfg.Mute, false))
	a.on(a.mMute, "", a.toggleActiveMute) // records the resulting state at source
	a.volMI = map[int]*menuItem{}
	for _, pct := range volumePresets {
		it := vol.AddCheckbox(fmt.Sprintf("%d %%", pct), pct == a.cfg.Volume, true)
		a.volMI[pct] = mitem(it)
		p := pct
		a.on(a.volMI[pct], "", func() { a.setActiveVolume(p) }) // records the level at source
	}
	// A real slider, when zenity is present. Absent on minimal installs, so
	// the item is only added when the binary exists (no dead item). The
	// resulting volume change is the measurable action, recorded once at
	// source on OK inside runVolumeSlider, so this click carries no Kind.
	if zenityAvailable() {
		slider := vol.Add("Régler au curseur…")
		a.on(mitem(slider), "", a.openVolumeSlider)
	}

	// Radios
	radios := m.Add("Radios")
	for _, s := range stations.All {
		it := radios.AddCheckbox(s.Display, s.Key == a.current.Key, true)
		a.stationMI[s.Key] = mitem(it)
		key := s.Key
		a.on(a.stationMI[s.Key], "", func() { a.setStation(key) }) // startStation records the from->to transition
	}
	fipItem := radios.Add("FIP sur radiofrance.fr")
	a.on(mitem(fipItem), events.KindOpenFip, func() { open.URL(fipURL) })

	// Diffuser sur…: cast the antenna to a Chromecast speaker. Devices are
	// discovered fresh each run, nothing persisted; the slots are
	// pre-allocated and hidden until a scan names them.
	castMenu := m.Add("Diffuser sur…")
	local := castMenu.AddCheckbox("Cet ordinateur", true, false)
	a.mCastLocal = mitem(local)
	// State-dependent: stopCasting records cast_stop at source only when a
	// cast was actually active, so a redundant click logs nothing.
	a.on(a.mCastLocal, "", func() { a.stopCasting(true) })
	a.castMI = make([]*menuItem, castSlots)
	for i := 0; i < castSlots; i++ {
		it := castMenu.AddCheckbox("", false, false)
		it.Hide()
		a.castMI[i] = mitem(it)
		idx := i
		// State-dependent: startCast records cast_start at source once the
		// device accepted the stream (a failed cast logs nothing).
		a.on(a.castMI[i], "", func() { a.castToDevice(idx) })
	}
	none := castMenu.Add("Aucun appareil trouvé")
	none.Disable()
	a.mCastNone = mitem(none)
	scan := castMenu.Add("Rechercher les appareils")
	// Discovery is ambient plumbing, not a listening behaviour: the
	// measurable actions here are cast_start/cast_stop, recorded at source.
	a.on(mitem(scan), "", a.rescanCast)

	// Historique
	hist := m.Add("Historique")
	a.histMI = make([]*menuItem, historySlots)
	for i := 0; i < historySlots; i++ {
		it := hist.Add("")
		it.Hide()
		a.histMI[i] = mitem(it)
		idx := i
		a.on(a.histMI[i], events.KindOpenHistory, func() { a.openHistory(idx) })
	}

	// Calendrier: the upcoming programmes on the antenna (station 7 only).
	// The slots are display-only; the whole submenu is hidden when the
	// calendar setting is off or nothing is scheduled.
	cal := m.Add("Calendrier")
	a.mCalendar = mitem(cal)
	a.calMI = make([]*menuItem, calendarSlots)
	for i := 0; i < calendarSlots; i++ {
		it := cal.Add("")
		it.Disable()
		it.Hide()
		a.calMI[i] = mitem(it)
	}
	// Start hidden: refreshCalendarMenu reveals it once programmes are
	// scheduled and the setting is on (so the webradios never show an empty
	// submenu).
	a.mCalendar.Hide()

	// Réglages
	settings := m.Add("Réglages")
	hifi := settings.AddCheckbox("Haute qualité (AAC 192k)", a.cfg.HiFi, false)
	a.mHiFi = mitem(hifi)
	a.on(a.mHiFi, "", a.toggleHiFi)
	// Fondu enchaîné (crossfade on a live station zap). With zenity a single
	// item opens a 0..10s slider; without it, a preset-checkbox submenu is
	// the fallback. Either way setCrossfade records KindCrossfade at source,
	// so the a.on click carries no Kind.
	if zenityAvailable() {
		xf := settings.Add(crossfadeTitle(a.cfg.CrossfadeSecs, true))
		a.mCrossfade = mitem(xf)
		a.on(a.mCrossfade, "", a.openCrossfadeSlider)
	} else {
		xf := settings.Add(crossfadeTitle(a.cfg.CrossfadeSecs, false))
		a.mCrossfade = mitem(xf)
		a.crossfadeMI = map[int]*menuItem{}
		for _, secs := range crossfadePresets {
			it := xf.AddCheckbox(crossfadePresetLabel(secs), secs == a.cfg.CrossfadeSecs, true)
			a.crossfadeMI[secs] = mitem(it)
			s := secs
			a.on(a.crossfadeMI[secs], "", func() { a.setCrossfade(s) }) // setCrossfade records KindCrossfade at source
		}
	}
	// Amplification (loudness preamp in the mpv filter chain): the FIP
	// streams sit ~4.5 dB below streaming-platform loudness (measured -18.5
	// LUFS vs ~-14). Radio presets, no slider: the loudness barely varies.
	amp := settings.Add("Amplification")
	a.mAmp = mitem(amp)
	a.ampMI = map[int]*menuItem{}
	for _, lvl := range ampPresets {
		it := amp.AddCheckbox(ampPresetLabel(lvl), lvl == a.cfg.Amp, true)
		a.ampMI[lvl] = mitem(it)
		l := lvl
		a.on(a.ampMI[lvl], "", func() { a.setAmp(l) }) // setAmp records KindAmp at source
	}
	notif := settings.AddCheckbox("Notifications", a.cfg.Notifications, false)
	a.mNotif = mitem(notif)
	a.on(a.mNotif, "", a.toggleNotif)
	showNotif := settings.AddCheckbox("Notifications d'émission", a.cfg.ShowNotifications, false)
	a.mShowNotif = mitem(showNotif)
	a.on(a.mShowNotif, "", a.toggleShowNotif)
	showCal := settings.AddCheckbox("Afficher le calendrier", a.cfg.ShowCalendar, false)
	a.mShowCalendar = mitem(showCal)
	a.on(a.mShowCalendar, "", a.toggleShowCalendar)
	// Launch at login is XDG-only (writes ~/.config/autostart/*.desktop);
	// hide it where config.SetAutostart is a no-op (macOS, other non-Linux).
	if config.AutostartSupported {
		auto := settings.AddCheckbox("Lancer au démarrage", a.cfg.Autostart, false)
		a.mAuto = mitem(auto)
		a.on(a.mAuto, "", a.toggleAutostart)
	}
	// Lecture au lancement: whether launching also starts the stream. Off
	// by default (autostart is not autoplay): the tray tunes and indicates
	// the antenna silently, and you press play when you want sound.
	pos := settings.AddCheckbox("Lecture au lancement", a.cfg.PlayOnStart, false)
	a.mPlayOnStart = mitem(pos)
	a.on(a.mPlayOnStart, "", a.togglePlayOnStart)
	histFile := settings.AddCheckbox("Historique local (fichier)", a.cfg.HistoryFile, false)
	a.mHistFile = mitem(histFile)
	a.on(a.mHistFile, "", a.toggleHistFile)
	anim := settings.AddCheckbox("Icône animée", a.cfg.AnimatedIcon, false)
	a.mAnim = mitem(anim)
	a.on(a.mAnim, "", a.toggleAnim)

	// Sortie audio: pick the output sink through mpv's audio-device
	// property, so no pavucontrol is needed. mpv enumerated the devices at
	// startup (a.audioDevs); the list already carries an "auto" entry.
	audio := settings.Add("Sortie audio")
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
			it := audio.AddCheckbox(label, dev.Name == cur, true)
			a.audioMI[dev.Name] = mitem(it)
			name := dev.Name
			a.on(a.audioMI[dev.Name], events.KindAudioDevice, func() { a.setAudioDevice(name) })
		}
	} else {
		// Enumeration failed or is empty: keep a single Automatique entry so
		// the submenu is never blank and the user can still reset to the
		// default.
		it := audio.AddCheckbox("Automatique", cur == "auto", true)
		a.audioMI["auto"] = mitem(it)
		a.on(a.audioMI["auto"], events.KindAudioDevice, func() { a.setAudioDevice("auto") })
	}

	// Statistiques d'écoute: opt-in (default off), local-only. The toggle
	// gates the recorder; the submenu lets you see, locate and delete the
	// data.
	statsToggle := settings.AddCheckbox("Statistiques d'écoute (local)", a.cfg.Stats, false)
	a.mStats = mitem(statsToggle)
	a.on(a.mStats, "", a.toggleStats)
	statsMenu := settings.Add("Statistiques")
	mStatsView := mitem(statsMenu.Add("Voir le rapport"))
	a.on(mStatsView, events.KindStatsView, a.viewStats)
	mStatsFolder := mitem(statsMenu.Add("Ouvrir le dossier de données"))
	a.on(mStatsFolder, "", a.openDataDir)
	a.mStatsClear = mitem(statsMenu.Add("Effacer les statistiques…"))
	a.on(a.mStatsClear, "", a.clearStatsConfirm)
	// Taste verdicts (J'aime / Pas pour moi) persist to a separate file,
	// prefs.jsonl, with its own consent. It gets its own delete affordance
	// so the "see / edit / delete" promise covers every local log.
	a.mPrefsClear = mitem(statsMenu.Add("Effacer mes goûts…"))
	a.on(a.mPrefsClear, "", a.clearPrefsConfirm)

	m.AddSeparator()
	about := m.Add("À propos")
	a.on(mitem(about), events.KindOpenAbout, func() { open.URL(repoURL) })
	ver := m.Add("le fipindicateur " + version.String())
	ver.Disable()
	// Mises à jour: "Vérifier maintenant" is the on-demand check; the
	// checkbox is the opt-in startup check. Both off + never clicking =
	// never checks.
	maj := m.Add("Mises à jour")
	checkNow := mitem(maj.Add("Vérifier maintenant"))
	a.on(checkNow, events.KindUpdateCheck, a.checkUpdates)
	majStartup := maj.AddCheckbox("Vérifier au démarrage", a.cfg.UpdateStartup, false)
	a.mUpdateStartup = mitem(majStartup)
	a.on(a.mUpdateStartup, "", a.toggleUpdateStartup)
	relancer := m.Add("Relancer")
	a.on(mitem(relancer), events.KindRestart, a.restart)
	quit := m.Add("Quitter")
	a.on(mitem(quit), events.KindQuit, quitApp)
}
