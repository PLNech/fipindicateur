package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AutostartSupported reports whether "launch at login" is wired on this OS.
// Only Linux has an implementation (XDG autostart .desktop); the menu item is
// hidden elsewhere.
const AutostartSupported = true

// autostartPath returns ~/.config/autostart/fipindicateur.desktop.
func autostartPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "autostart")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "fipindicateur.desktop"), nil
}

// SetAutostart writes or removes the XDG autostart entry pointing at the
// currently running executable.
func SetAutostart(enabled bool) error {
	p, err := autostartPath()
	if err != nil {
		return err
	}
	if !enabled {
		err := os.Remove(p)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=le fipindicateur
Comment=Listen to FIP webradios
Exec=%s
Icon=fipindicateur
Terminal=false
Categories=AudioVideo;Audio;Player;
X-GNOME-Autostart-enabled=true
`, autostartExec(exe))
	return os.WriteFile(p, []byte(entry), 0o644)
}

// autostartExec resolves the binary path to bake into the autostart entry.
// Autostart fires at login and must survive reboot, so it must NEVER point at a
// transient build path: a dev run (`go run`, `make run`, a binary in /tmp) that
// happens to toggle the setting would leave an Exec= that no longer exists next
// boot (the 2026-07-09 freeze had Exec=/tmp/fipindicateur). We therefore prefer
// the user-level installed binary (~/.local/bin/fipindicateur, what `make
// install` writes), and only fall back to the running executable when it is a
// real, non-ephemeral path.
func autostartExec(exe string) string {
	if p := installedBinary(); p != "" {
		return p
	}
	if exe != "" && !isEphemeralPath(exe) {
		return exe
	}
	// Last resort: the canonical install location even if absent, so a later
	// `make install` makes the entry valid, rather than baking a temp path.
	return canonicalInstallPath()
}

// canonicalInstallPath is the documented user-level install target
// (~/.local/bin/fipindicateur, PREFIX=$HOME/.local in the Makefile).
func canonicalInstallPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "fipindicateur" // rely on PATH as an absolute last resort
	}
	return filepath.Join(home, ".local", "bin", "fipindicateur")
}

// installedBinary returns the canonical install path if an executable exists
// there, else "".
func installedBinary() string {
	p := canonicalInstallPath()
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
		return p
	}
	return ""
}

// isEphemeralPath reports whether a path is a throwaway build/run location that
// must not be persisted into an autostart entry.
func isEphemeralPath(p string) bool {
	if p == "" {
		return true
	}
	prefixes := []string{"/tmp/", "/var/tmp/", "/dev/", "/proc/", "/run/user/"}
	for _, pre := range prefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	// `go run` and `go test` binaries live under a go-build cache directory.
	return strings.Contains(p, "/go-build") ||
		strings.Contains(p, "/go/pkg/mod/") ||
		strings.Contains(p, "T/go-build")
}
