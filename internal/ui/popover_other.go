//go:build !(darwin && swiftui)

package ui

import (
	"github.com/PLNech/fipindicateur/internal/popover"
	"github.com/PLNech/fipindicateur/internal/vu"
)

// The SwiftUI popover exists only on darwin behind the swiftui build tag
// (popover_darwin.go). Everywhere else the seams the shared code calls are
// no-ops and the tray menu stays the whole UI.

func (a *App) wirePopover() {}

func (a *App) syncPopover() {}

func (a *App) popoverShown() bool { return false }

func popoverBars(vu.Heights) {}

// carton: no card on this build, the system banner stays.
func (a *App) carton(popover.Carton) bool { return false }

// hasPopover: no card on this build, the menu carries every setting.
const hasPopover = false

func (a *App) openSettings() {}
