//go:build darwin && swiftui

package popover

import (
	"unsafe"

	"github.com/PLNech/fipindicateur/internal/popover/fipui"
)

// SetHandler installs the receiver of the popover's actions (id, arg). It is
// called on the AppKit main thread and must not block.
func SetHandler(fn func(id, arg string)) { fipui.SetHandler(fn) }

// Push sends the state to render. Safe from any goroutine.
func Push(s Snapshot) bool { return fipui.PushState(s.JSON()) }

// Toggle shows the card (top right of the screen holding anchor, the status
// item's button) or closes it.
func Toggle(anchor unsafe.Pointer) { fipui.Toggle(anchor) }

// Close puts the card away if it is up.
func Close() { fipui.Close() }

// SetBars feeds the card's meter one frame: the tray glyph's four bar
// heights, each 0..1.
func SetBars(b [4]float64) { fipui.SetBars(b) }

// Shown reports whether the card is up. Safe from any goroutine.
func Shown() bool { return fipui.Shown() }

// ShowCarton drops a carton under the menu bar (ignored while the card is up).
func ShowCarton(c Carton) { fipui.ShowCarton(c.JSON()) }

// ShowSettings turns the card to its Réglages face, showing it if needed.
func ShowSettings() { fipui.ShowSettings() }
