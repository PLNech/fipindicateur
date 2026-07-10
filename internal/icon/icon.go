// Package icon embeds the tray icons and selects a variant for the current
// desktop color scheme. The glyph is our own broadcast/radio-waves mark, not
// the FIP logo (trademark). Regenerate with: go run internal/icon/gen/main.go
package icon

import _ "embed"

//go:embed icon_light_44.png
var light []byte

//go:embed icon_dark_44.png
var dark []byte

//go:embed icon_light_dim_44.png
var lightDim []byte

//go:embed icon_dark_dim_44.png
var darkDim []byte

// darkPanel reports whether the tray/panel is likely dark (so we should draw a
// light-ink glyph). It is best-effort and platform-specific: see panel_linux.go
// (gsettings), panel_darwin.go (AppleInterfaceStyle), and panel_other.go (the
// safe default). The result is cached once by panelIsDark (see bars.go), so no
// process is spawned per frame.

// Active returns the icon bytes for the current scheme and play state.
// On a dark panel we use the light (near-white) ink so the glyph is visible.
// The scheme probe is cached (see bars.go): no process spawn per call.
func Active(paused bool) []byte {
	onDark := panelIsDark()
	switch {
	case onDark && paused:
		return lightDim
	case onDark:
		return light
	case paused:
		return darkDim
	default:
		return dark
	}
}
