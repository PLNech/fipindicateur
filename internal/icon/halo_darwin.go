//go:build darwin

package icon

// haloOnPanel reports whether the tray glyph carries the white rim: on darwin,
// exactly when the panel is dark (the owner's "outlined when dark" call).
//
// The rim exists for a panel that downscales a 44px PNG onto a near-black bar:
// there the white feather collapses to a thin edge and keeps the bars
// readable. The replaced systray backend drew the PNG at its native point
// size, where the feather stayed a thick white border and the rim only hurt
// (hence the old build-time false); the hand-rolled tray (internal/tray) draws
// the 44px bytes at 22pt, half the pixels, so the collapse happens exactly
// like on GNOME and the rim follows the live panel state instead of the
// build target.
func haloOnPanel(dark bool) bool { return dark }
