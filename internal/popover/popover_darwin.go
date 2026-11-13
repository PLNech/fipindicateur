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

// Toggle shows the popover under anchor (the status item's button) or closes
// it.
func Toggle(anchor unsafe.Pointer) { fipui.Toggle(anchor) }
