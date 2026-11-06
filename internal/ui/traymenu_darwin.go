//go:build darwin

package ui

// The darwin tray adapters: the hand-rolled internal/tray (NSStatusItem +
// NSMenu owned in-tree) behind the small vocabulary the shared builder in
// ui_menu.go speaks. Windows keeps fyne/systray unchanged
// (traymenu_systray.go); Linux builds its whole UI on internal/sni
// (ui_sni_linux.go). The item surface (ClickedCh plus the nil-safe
// mutators) is the contract App.on and the shared chokepoints consume.

import (
	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/tray"
)

// menuItem is the tray item type behind the shared menu fields.
type menuItem = tray.Item

func trayRun(onReady, onExit func()) { tray.Run(onReady, onExit) }

// trayQuit asks the tray loop to end (signal handler, CLI, menu item).
func trayQuit() { tray.Quit() }

// setTrayIconNative hands PNG bytes to the status item, sized from the PNG
// itself (22 pt at the 44 px @2x the icon package renders).
func setTrayIconNative(b []byte) { tray.SetIcon(b) }

func addMenuItem(title, tooltip string) *menuItem {
	return tray.AddMenuItem(title, tooltip)
}

func addSeparator() { tray.AddSeparator() }

// traySetTitle/traySetTooltip push the status item's title and tooltip.
func traySetTitle(s string)   { tray.SetTitle(s) }
func traySetTooltip(s string) { tray.SetTooltip(s) }

// wirePanelFlip tracks the live appearance: when the Auto appearance flips
// the menu bar, RefreshPanelDark re-probes (the boot probe alone would keep
// the launch-time answer forever) and the glyph re-inks.
func (a *App) wirePanelFlip() {
	tray.OnAppearanceChange(a.onPanelFlip)
}

func (a *App) onPanelFlip() {
	if !icon.RefreshPanelDark() {
		return
	}
	a.repaintIconState()
}
