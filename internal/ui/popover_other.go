//go:build !(darwin && swiftui)

package ui

// The SwiftUI popover exists only on darwin behind the swiftui build tag
// (popover_darwin.go). Everywhere else the seams the shared code calls are
// no-ops and the tray menu stays the whole UI.

func (a *App) wirePopover() {}

func (a *App) syncPopover() {}

func (a *App) popoverShown() bool { return false }

func popoverLevel(float64) {}
