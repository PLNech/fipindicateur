// Package icon embeds the tray icons and selects a variant for the current
// desktop color scheme. The glyph is our own broadcast/radio-waves mark, not
// the FIP logo (trademark). Regenerate with: go run internal/icon/gen/main.go
package icon

import (
	_ "embed"
	"os/exec"
	"strings"
)

//go:embed icon_light_44.png
var light []byte

//go:embed icon_dark_44.png
var dark []byte

//go:embed icon_light_dim_44.png
var lightDim []byte

//go:embed icon_dark_dim_44.png
var darkDim []byte

// darkPanel reports whether the tray/panel is likely dark (so we should draw a
// light-ink glyph). Best-effort: GNOME's default top bar is dark, so we assume
// dark unless the user explicitly prefers a light color scheme.
func darkPanel() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true
	}
	return !strings.Contains(string(out), "prefer-light")
}

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
