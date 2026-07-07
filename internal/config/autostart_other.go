//go:build !linux

package config

// AutostartSupported reports whether "launch at login" is wired on this OS.
// Non-Linux platforms have no XDG autostart, so the toggle is hidden and this
// is a no-op. A native macOS LaunchAgent could land here later.
const AutostartSupported = false

// SetAutostart is a no-op on platforms without XDG autostart. It exists so the
// cross-platform UI code compiles without OS build guards at the call site.
func SetAutostart(bool) error { return nil }
