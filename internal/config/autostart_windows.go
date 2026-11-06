//go:build windows

package config

// AutostartSupported reports whether "launch at login" is wired on this OS.
// Windows has no implementation yet, so the menu item stays hidden.
const AutostartSupported = false

// SetAutostart is a no-op on Windows.
func SetAutostart(bool) error { return nil }
