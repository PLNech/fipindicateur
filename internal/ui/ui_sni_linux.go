//go:build linux

package ui

// The Linux half of the UI: the StatusNotifierItem (internal/sni, hand-rolled
// over godbus) is an icon plus input events, and « le panneau »
// (internal/drawer) is the single UI:
//   left click     -> open the drawer (on GNOME a single click opens the
//                     one-entry DBusMenu, whose opening IS the open signal;
//                     a double click, and KDE's plain click, land on Activate
//                     which toggles)
//   right click    -> open the drawer on the Réglages view
//   middle click   -> play/pause (togglePlay, same semantics as everywhere)
//   vertical wheel -> volume steps on the active sink
// The main loop is a plain quit-channel wait: GTK runs on the drawer's own
// dedicated goroutine (started lazily at first open) and D-Bus (SNI, MPRIS)
// on godbus goroutines, so nothing needs the main goroutine.

import (
	"image/color"
	"log"
	"sync"

	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/sni"
)

// menuItem is a nil-safe no-op stand-in for the systray item on Linux, where
// the drawer replaced the menu. The shared chokepoints in ui.go (setPlayingUI,
// applyVolumeUI, the toggles) keep calling these methods on their (nil)
// fields; state reaches the user through pushDrawerState instead.
type menuItem struct{}

func (m *menuItem) SetTitle(string)   {}
func (m *menuItem) SetTooltip(string) {}
func (m *menuItem) Enable()           {}
func (m *menuItem) Disable()          {}
func (m *menuItem) Check()            {}
func (m *menuItem) Uncheck()          {}
func (m *menuItem) Show()             {}
func (m *menuItem) Hide()             {}

// The process's one SNI item. setIcon (via setTrayIcon) can run before
// buildUI created the item (OnReady sets the icon first thing, to minimise
// the no-pixmap window): the bytes are stashed and used as the item's initial
// icon. Package-level like drawer's `current`: one app per process.
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

// quitApp is the in-app quit chokepoint (drawer command, restart).
func quitApp() { Quit() }

// setTrayIcon hands PNG bytes to the StatusNotifierItem (which repacks them
// as an ARGB32 pixmap and signals NewIcon). Before the item exists, the last
// bytes are kept as its initial icon.
func setTrayIcon(b []byte) {
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
// tooltip (what the mNow menu item showed elsewhere).
func (a *App) setTrayNowLabel(label string) {
	sniMu.Lock()
	it := sniItem
	sniMu.Unlock()
	if it != nil {
		it.SetLabel("le fipindicateur", label)
	}
}

// buildUI registers the StatusNotifierItem and wires its input events onto
// the shared handlers. Best-effort like the old tray: if the session bus is
// unreachable the app runs on (MPRIS, control socket and media keys still
// work), just without an icon.
func (a *App) buildUI() {
	sniMu.Lock()
	initial := sniPending
	sniPending = nil
	sniMu.Unlock()
	if len(initial) == 0 {
		initial = icon.Rest(false, color.NRGBA{}) // OnReady sets one first; belt and braces
	}
	it, err := sni.New("fipindicateur", "le fipindicateur", initial, sni.Handlers{
		Activate:          a.toggleDrawer,
		ContextMenu:       a.openDrawerSettings,
		SecondaryActivate: a.togglePlay,
		Scroll:            a.scrollVolume,
		// The GNOME single-click path: the extension only reacts to a single
		// left click by opening the item's DBusMenu, and that opening (or its
		// one entry being clicked) means "show the panel". Idempotent open,
		// never a toggle: both signals can fire for one interaction.
		MenuOpen: a.openDrawer,
	})
	if err != nil {
		log.Printf("ui: statusnotifier: %v (pas d'icône; le panneau reste accessible via MPRIS/fip)", err)
		return
	}
	sniMu.Lock()
	sniItem = it
	sniMu.Unlock()
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
