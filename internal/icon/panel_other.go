//go:build !linux && !darwin

package icon

// darkPanel returns true on platforms without a scheme probe. A dark panel is
// the common default (GNOME-like top bars), and a light-ink glyph on a dark
// panel is the safe guess when we cannot ask the desktop.
func darkPanel() bool { return true }
