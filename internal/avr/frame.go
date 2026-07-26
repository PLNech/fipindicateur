// Package avr speaks eISCP (Onkyo's Integra Serial Control Protocol over
// Ethernet) to an AV receiver on the LAN, TCP port 60128. Post-merger
// Pioneer amps (e.g. the VSX-933) answer Onkyo eISCP on that port.
//
// The package is a pure protocol layer: it never sends anything on its
// own, on load or otherwise. Every command on the wire is a deliberate
// caller decision; policy (like auto-mirroring zone 2 onto the main
// source) belongs to the UI layer, not here.
package avr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// eISCP frame layout: "ISCP" magic, header size (uint32 BE, always 16),
// data size (uint32 BE), version byte 0x01, three reserved zero bytes,
// then the ASCII data. Data is the "!1"-prefixed message plus a CR.
const (
	frameMagic   = "ISCP"
	headerSize   = 16
	frameVersion = 0x01

	// unitPrefix addresses the receiver unit. Zones are not selected
	// here but by the command prefix itself (PWR vs ZPW vs PW3).
	unitPrefix = "!1"

	// maxDataSize bounds the declared payload length so a corrupt
	// header cannot make a reader allocate or wait for gigabytes.
	maxDataSize = 64 * 1024
)

// ErrShortFrame reports a buffer that ends before the frame does. A
// stream reader should accumulate more bytes and retry.
var ErrShortFrame = errors.New("avr: incomplete eISCP frame")

// EncodeFrame wraps an ISCP message (e.g. "PWRQSTN" or "ZPW01") in an
// eISCP frame ready to write to the socket. The "!1" unit prefix and
// trailing CR are added here; callers pass the bare command.
func EncodeFrame(msg string) []byte {
	data := unitPrefix + msg + "\r"
	buf := make([]byte, headerSize+len(data))
	copy(buf, frameMagic)
	binary.BigEndian.PutUint32(buf[4:8], headerSize)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(data)))
	buf[12] = frameVersion
	// buf[13:16] stay zero (reserved).
	copy(buf[headerSize:], data)
	return buf
}

// DecodeFrame parses one eISCP frame from the start of buf. It returns
// the inner message with the "!1" unit prefix and any trailing
// EOF/CR/LF bytes stripped (devices variously end responses with 0x1A,
// 0x0D and/or 0x0A), plus the bytes remaining after the frame.
// ErrShortFrame means buf holds a truncated frame: read more and retry.
func DecodeFrame(buf []byte) (msg string, rest []byte, err error) {
	if len(buf) < headerSize {
		return "", buf, ErrShortFrame
	}
	if string(buf[:4]) != frameMagic {
		return "", buf, fmt.Errorf("avr: bad frame magic %q", buf[:4])
	}
	hdr := binary.BigEndian.Uint32(buf[4:8])
	if hdr < headerSize || hdr > maxDataSize {
		return "", buf, fmt.Errorf("avr: implausible header size %d", hdr)
	}
	size := binary.BigEndian.Uint32(buf[8:12])
	if size > maxDataSize {
		return "", buf, fmt.Errorf("avr: implausible data size %d", size)
	}
	total := int(hdr) + int(size)
	if len(buf) < total {
		return "", buf, ErrShortFrame
	}
	return trimMessage(buf[hdr:total]), buf[total:], nil
}

// trimMessage strips the trailing EOF/CR/LF variants and the leading
// "!1" unit prefix from a raw frame payload.
func trimMessage(data []byte) string {
	s := string(data)
	s = strings.TrimRight(s, "\x1a\r\n")
	s = strings.TrimPrefix(s, unitPrefix)
	return s
}
