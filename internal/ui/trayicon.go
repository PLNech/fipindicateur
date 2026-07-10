package ui

import (
	"encoding/binary"

	"fyne.io/systray"
)

// setTrayIcon is the single, platform-neutral chokepoint for handing bytes to
// systray. It is the ONLY place `systray.SetIcon` is called in the tree (the
// guard test in guard_test.go enforces that). App.setIcon dedupes and refuses
// empty bytes before reaching here; this layer only adapts the byte format to
// what the platform's tray expects, via encodeTrayIcon.
func setTrayIcon(b []byte) {
	systray.SetIcon(encodeTrayIcon(b))
}

// pngToICO wraps a PNG image in a minimal single-image ICO container. Windows'
// systray hands icon bytes to LoadImageW, which wants an .ico; Vista and later
// accept a PNG embedded directly in the ICO (no BMP re-encode needed). The
// container is a 6-byte ICONDIR + one 16-byte ICONDIRENTRY + the raw PNG.
//
// The entry's width/height bytes are read from the PNG's IHDR (a byte of 0
// means 256, per the ICO spec). If the input is not a parseable PNG the dims
// fall back to 0 (interpreted as 256), which keeps a best-effort icon rather
// than crashing playback for a cosmetic asset.
func pngToICO(png []byte) []byte {
	w, h := pngDimensions(png)

	const (
		iconDirSize   = 6
		iconEntrySize = 16
		headerSize    = iconDirSize + iconEntrySize // 22, the PNG payload offset
	)

	out := make([]byte, headerSize+len(png))

	// ICONDIR: reserved=0, type=1 (icon), count=1. Multi-byte fields are LE.
	binary.LittleEndian.PutUint16(out[0:2], 0)
	binary.LittleEndian.PutUint16(out[2:4], 1)
	binary.LittleEndian.PutUint16(out[4:6], 1)

	// ICONDIRENTRY.
	out[6] = byteDim(w)                                           // bWidth (0 => 256)
	out[7] = byteDim(h)                                           // bHeight (0 => 256)
	out[8] = 0                                                    // bColorCount (0 for >=256 colors)
	out[9] = 0                                                    // bReserved
	binary.LittleEndian.PutUint16(out[10:12], 1)                  // wPlanes
	binary.LittleEndian.PutUint16(out[12:14], 32)                 // wBitCount
	binary.LittleEndian.PutUint32(out[14:18], uint32(len(png)))   // dwBytesInRes
	binary.LittleEndian.PutUint32(out[18:22], uint32(headerSize)) // dwImageOffset

	copy(out[headerSize:], png)
	return out
}

// pngDimensions extracts width and height from a PNG's IHDR chunk. Layout: an
// 8-byte signature, then the IHDR chunk whose 4-byte length and "IHDR" type
// precede the width (big-endian uint32 at offset 16) and height (offset 20).
// Returns 0,0 when the input is too short or lacks the PNG signature.
func pngDimensions(png []byte) (uint32, uint32) {
	const sig = "\x89PNG\r\n\x1a\n"
	if len(png) < 24 || string(png[:8]) != sig {
		return 0, 0
	}
	w := binary.BigEndian.Uint32(png[16:20])
	h := binary.BigEndian.Uint32(png[20:24])
	return w, h
}

// byteDim maps a pixel dimension to an ICO size byte: 256 (or anything that
// does not fit a byte) is encoded as 0, per the ICO spec.
func byteDim(d uint32) byte {
	if d >= 256 {
		return 0
	}
	return byte(d)
}
