package cast

import (
	"bytes"
	"strings"
	"testing"
)

func TestCastMessageRoundTrip(t *testing.T) {
	in := castMessage{
		source:      "sender-0",
		destination: "receiver-0",
		namespace:   nsReceiver,
		payload:     `{"type":"LAUNCH","appId":"CC1AD845","requestId":1}`,
	}
	out, err := decodeCastMessage(encodeCastMessage(in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}

func TestCastMessageRoundTripEmptyFields(t *testing.T) {
	in := castMessage{namespace: nsHeartbeat}
	out, err := decodeCastMessage(encodeCastMessage(in))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: %+v vs %+v", in, out)
	}
}

// TestDecodeSkipsUnknownFields interleaves every protobuf wire type the
// decoder must skip (varint, fixed64, length-delimited, fixed32) between the
// known fields, as a future receiver revision could.
func TestDecodeSkipsUnknownFields(t *testing.T) {
	known := encodeCastMessage(castMessage{
		source:      "sender-0",
		destination: "receiver-0",
		namespace:   nsMedia,
		payload:     `{"type":"PONG"}`,
	})
	var b []byte
	b = append(b, 0x38, 0xac, 0x02)             // field 7, varint 300 (multi-byte)
	b = append(b, known[:2]...)                 // protocol_version
	b = append(b, 0x41, 1, 2, 3, 4, 5, 6, 7, 8) // field 8, fixed64
	b = append(b, known[2:]...)                 // the rest of the known fields
	b = append(b, 0x4d, 0xde, 0xad, 0xbe, 0xef) // field 9, fixed32
	b = append(b, 0x52, 0x03, 'a', 'b', 'c')    // field 10, length-delimited
	b = append(b, 0x58, 0x00)                   // field 11, varint 0
	m, err := decodeCastMessage(b)
	if err != nil {
		t.Fatalf("decode with unknown fields: %v", err)
	}
	if m.source != "sender-0" || m.destination != "receiver-0" || m.namespace != nsMedia || m.payload != `{"type":"PONG"}` {
		t.Fatalf("known fields lost while skipping unknowns: %+v", m)
	}
}

func TestDecodeMalformed(t *testing.T) {
	cases := map[string][]byte{
		"truncated varint":        {0x08},
		"truncated fixed64":       {0x09, 1, 2, 3},
		"truncated fixed32":       {0x0d, 1, 2},
		"length beyond buffer":    {0x12, 0x20, 'a', 'b'},
		"wire type 3 start-group": {0x0b},
	}
	for name, data := range cases {
		if _, err := decodeCastMessage(data); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestFrameRoundTrip(t *testing.T) {
	in := castMessage{source: senderID, destination: receiverID, namespace: nsConnection, payload: `{"type":"CONNECT"}`}
	var buf bytes.Buffer
	if err := writeFrame(&buf, in); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	out, err := readFrame(&buf)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if out != in {
		t.Fatalf("frame round trip mismatch: %+v vs %+v", in, out)
	}
}

func TestReadFrameRejectsOversizedLength(t *testing.T) {
	// A corrupt 4-byte prefix claiming a huge frame must error out instead of
	// allocating and blocking on gigabytes that will never come.
	_, err := readFrame(strings.NewReader("\xff\xff\xff\xff"))
	if err == nil {
		t.Fatal("expected an error for an oversized frame length")
	}
}
