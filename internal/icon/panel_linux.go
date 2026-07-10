//go:build linux

package icon

import (
	"os/exec"
	"strings"
)

// darkPanel reports whether the GNOME top bar is likely dark (so we should draw
// a light-ink glyph). GNOME's default top bar is dark, so we assume dark unless
// the user explicitly prefers a light color scheme.
func darkPanel() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true
	}
	return !strings.Contains(string(out), "prefer-light")
}
