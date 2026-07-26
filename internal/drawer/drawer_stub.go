//go:build !linux || !cgo

package drawer

// The no-op stub keeps darwin/windows (and any cgo-less build) compiling
// without webkit2gtk. Available is false, so the tray keeps its native volume
// submenu and never offers the panel there.

import "errors"

// Available reports whether this build carries the real panel.
const Available = false

// Drawer is inert on this platform.
type Drawer struct{}

// New returns an inert drawer; the callbacks are never invoked.
func New(onCommand func(Command), onHide func()) *Drawer { return &Drawer{} }

// Show reports that the panel does not exist on this platform.
func (d *Drawer) Show(state State) error {
	return errors.New("drawer: le panneau n'est disponible que sous Linux")
}

// Push is a no-op.
func (d *Drawer) Push(state State) {}

// SetView is a no-op.
func (d *Drawer) SetView(view string) {}

// Eval is a no-op.
func (d *Drawer) Eval(script string) {}

// SetWindowTitle is a no-op.
func (d *Drawer) SetWindowTitle(title string) {}

// Hide is a no-op.
func (d *Drawer) Hide() {}
