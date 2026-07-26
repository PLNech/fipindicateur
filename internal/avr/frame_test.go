package avr

import (
	"bytes"
	"errors"
	"testing"
)

// frame builds a synthetic amp response frame with an arbitrary payload,
// so tests control the trailer bytes exactly.
func frame(payload string) []byte {
	buf := make([]byte, headerSize+len(payload))
	copy(buf, "ISCP")
	buf[7] = headerSize
	buf[11] = byte(len(payload))
	buf[12] = frameVersion
	copy(buf[headerSize:], payload)
	return buf
}

func TestEncodeFrameBytes(t *testing.T) {
	got := EncodeFrame("PWRQSTN")
	want := []byte{
		'I', 'S', 'C', 'P',
		0x00, 0x00, 0x00, 0x10, // header size 16
		0x00, 0x00, 0x00, 0x0A, // data size: len("!1PWRQSTN\r") = 10
		0x01, 0x00, 0x00, 0x00, // version + reserved
		'!', '1', 'P', 'W', 'R', 'Q', 'S', 'T', 'N', '\r',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFrame(PWRQSTN)\n got %x\nwant %x", got, want)
	}
}

func TestDecodeFrameTrailers(t *testing.T) {
	// Real amps end responses with any mix of EOF/CR/LF.
	for _, trailer := range []string{"\x1a\r\n", "\x1a", "\r\n", "\r", "\n", "\x1a\r", ""} {
		msg, rest, err := DecodeFrame(frame("!1PWR01" + trailer))
		if err != nil {
			t.Fatalf("trailer %q: %v", trailer, err)
		}
		if msg != "PWR01" {
			t.Errorf("trailer %q: msg = %q, want PWR01", trailer, msg)
		}
		if len(rest) != 0 {
			t.Errorf("trailer %q: %d leftover bytes", trailer, len(rest))
		}
	}
}

func TestDecodeFrameRoundTrip(t *testing.T) {
	for _, cmd := range []string{"ZPW01", "MVL3E", "SLZ2B", "ZVLQSTN"} {
		msg, rest, err := DecodeFrame(EncodeFrame(cmd))
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		if msg != cmd || len(rest) != 0 {
			t.Errorf("%s: got msg=%q rest=%d bytes", cmd, msg, len(rest))
		}
	}
}

func TestDecodeFrameStream(t *testing.T) {
	// Two frames back to back: decode must return the tail as rest.
	buf := append(frame("!1PWR01\x1a\r\n"), frame("!1ZPW00\x1a\r\n")...)
	msg, rest, err := DecodeFrame(buf)
	if err != nil || msg != "PWR01" {
		t.Fatalf("first frame: msg=%q err=%v", msg, err)
	}
	msg, rest, err = DecodeFrame(rest)
	if err != nil || msg != "ZPW00" {
		t.Fatalf("second frame: msg=%q err=%v", msg, err)
	}
	if len(rest) != 0 {
		t.Fatalf("leftover %d bytes", len(rest))
	}
}

func TestDecodeFrameShort(t *testing.T) {
	full := frame("!1PWR01\x1a\r\n")
	for _, n := range []int{0, 3, headerSize - 1, headerSize, len(full) - 1} {
		if _, _, err := DecodeFrame(full[:n]); !errors.Is(err, ErrShortFrame) {
			t.Errorf("truncated at %d: err = %v, want ErrShortFrame", n, err)
		}
	}
}

func TestDecodeFrameBadMagic(t *testing.T) {
	buf := frame("!1PWR01\r")
	copy(buf, "JUNK")
	if _, _, err := DecodeFrame(buf); err == nil || errors.Is(err, ErrShortFrame) {
		t.Fatalf("bad magic: err = %v, want a hard error", err)
	}
}

func TestParseVolume(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"3E", 62, true},             // typical main level
		{"00", 0, true},              // silence
		{"64", 100, true},            // top of the catalog scale
		{"0A", 10, true},             // leading zero
		{"N/A", VolumeUnknown, true}, // zone in standby
		{"ZZ", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, err := parseVolume(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("parseVolume(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("parseVolume(%q): expected error", c.in)
		}
	}
}

func TestValidSelector(t *testing.T) {
	for _, ok := range []string{"2B", "05", "00", "FF"} {
		if !validSelector(ok) {
			t.Errorf("validSelector(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"2b", "B", "2BX", "", "G1", "N/A"} {
		if validSelector(bad) {
			t.Errorf("validSelector(%q) = true, want false", bad)
		}
	}
}
