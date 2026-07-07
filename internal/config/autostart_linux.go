package config

import (
	"fmt"
	"os"
	"path/filepath"
)

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
`, exe)
	return os.WriteFile(p, []byte(entry), 0o644)
}
