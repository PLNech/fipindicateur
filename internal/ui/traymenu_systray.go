//go:build !linux && !darwin

package ui

// The Windows tray adapters: fyne/systray, unchanged, behind the small
// vocabulary the shared builder in ui_menu.go speaks. macOS builds on the
// hand-rolled internal/tray (traymenu_darwin.go); Linux builds its whole UI
// on internal/sni (ui_sni_linux.go). The item surface (ClickedCh plus the
// mutators) is the contract App.on and the shared chokepoints consume.

import "fyne.io/systray"

// menuItem is the tray item type behind the shared menu fields.
type menuItem = systray.MenuItem

func trayRun(onReady, onExit func()) { systray.Run(onReady, onExit) }

// trayQuit asks the systray loop to end (signal handler, CLI, menu item).
func trayQuit() { systray.Quit() }

// setTrayIconNative is the ONLY place a raw systray icon-set call appears in
// the tree (the guard test counts the call sites); it adapts the byte format
// via encodeTrayIcon upstream.
func setTrayIconNative(b []byte) { systray.SetIcon(b) }

func addMenuItem(title, tooltip string) *menuItem {
	return systray.AddMenuItem(title, tooltip)
}

func addSeparator() { systray.AddSeparator() }

// traySetTitle/traySetTooltip push the status item's title and tooltip.
func traySetTitle(s string)   { systray.SetTitle(s) }
func traySetTooltip(s string) { systray.SetTooltip(s) }

// wirePanelFlip is a no-op here: the menu bar cannot flip under the glyph on
// Windows (no Auto appearance on the tray), the darwin tray owns the hook.
func (a *App) wirePanelFlip() {}
