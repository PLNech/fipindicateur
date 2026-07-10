//go:build darwin

package icon

import (
	"os/exec"
	"strings"
)

// darkPanel reports whether the macOS menu bar is dark. `defaults read -g
// AppleInterfaceStyle` prints "Dark" and exits 0 when dark mode is on; in light
// mode the key is absent so the command exits non-zero. A dark menu bar means
// we want the light-ink glyph, so a successful "Dark" read maps to true.
//
// Colored (tinted) tray glyphs work on macOS because the app calls
// systray.SetIcon, not SetTemplateIcon: a template icon would be flattened to a
// monochrome mask, discarding the station brand tint.
func darkPanel() bool {
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Dark")
}
