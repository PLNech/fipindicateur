//go:build !darwin

package icon

// haloOnPanel reports whether the tray glyph carries the white rim. On Linux
// the tray downscales the 44px PNG, and the rim collapses to the thin edge
// the bars need against a near-black panel, whatever the theme (on a light
// panel the white-on-near-white rim stays invisible, so drawing it always
// costs nothing and saves a probe). The dark parameter is the darwin seam;
// here the answer does not depend on it.
func haloOnPanel(bool) bool { return true }
