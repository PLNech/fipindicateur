//go:build darwin

package ui

// The darwin volume gestures behind the shared builder: the inline NSSlider
// hosted as the Volume submenu's first entry (internal/tray owns the menu, so
// the view is a plain NSMenuItem.view) and the status item's scroll wheel.
// ui_menu.go carries no darwin branch: it calls the seam functions below,
// whose no-ops live in volumeui_other.go.

import "github.com/PLNech/fipindicateur/internal/tray"

// volSlider is the inline volume slider (nil until addVolumeSlider builds
// it). Package-level like the Linux sni handle: the App is a singleton.
var volSlider *tray.Slider

// addVolumeSlider inserts the native slider as the Volume submenu's first
// entry. Its writes land on the same chokepoint as the preset checkboxes
// (setActiveVolume, which records at source); the drag ticks are coalesced
// for the events log by recordVolume, the one volume coalescer, so only the
// write is per tick. The menu-open hook re-reads the active sink on every
// open: the slider must show the device's level while casting, not the local
// one of build time.
func (a *App) addVolumeSlider() {
	tray.OnMenuOpen(a.applyVolumeUI)
	v, known := a.activeVolume()
	volSlider = a.mVolume.InsertSlider(v, a.setActiveVolume)
	volSlider.SetState(v, known)
}

// setTraySliderVolume mirrors the active sink onto the slider. known=false
// (casting to a device that has not reported a level) grays it out at the
// neutral midpoint instead of displaying an invented level, the same rule
// that keeps a wheel notch from becoming a master-volume step. Nil-safe
// before the menu exists.
func setTraySliderVolume(pct int, known bool) {
	volSlider.SetState(pct, known)
}

// wireTrayScroll hands the tray's scroll steps to App.scrollVolume: the tray
// already folds the raw wheel/trackpad deltas and emits them in
// scrollVolume's own sign convention (positive = down), so the seam is a
// pass-through.
func (a *App) wireTrayScroll() {
	tray.SetScrollHook(func(delta int) { a.scrollVolume(delta, "vertical") })
}
