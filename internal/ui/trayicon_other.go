//go:build !windows

package ui

// encodeTrayIcon is a passthrough on Linux and macOS, where the systray backend
// consumes the PNG bytes directly.
func encodeTrayIcon(b []byte) []byte {
	return b
}
