//go:build windows

package ui

// encodeTrayIcon wraps the PNG the icon library produces in an ICO container,
// which is what the Windows systray backend (LoadImageW) expects.
func encodeTrayIcon(b []byte) []byte {
	return pngToICO(b)
}
