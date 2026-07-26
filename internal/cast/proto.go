// Package cast discovers Chromecast devices on the local network (mDNS) and
// drives Google's Default Media Receiver over the CASTV2 protocol, so the
// tray can hand the station's icecast URL to a speaker. Stdlib only, like the
// rest of the app: the wire protobuf has six simple fields and the mDNS
// exchange is one fixed question, neither justifies a dependency.
package cast

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// castMessage mirrors the CastMessage protobuf from cast_channel.proto,
// reduced to the fields the protocol actually exchanges. protocol_version
// (field 1) is always CASTV2_1_0 and payload_type (field 5) always STRING,
// so both are constants of the encoder and ignored by the decoder.
type castMessage struct {
	source      string // field 2, e.g. "sender-0"
	destination string // field 3, "receiver-0" or a session transportId
	namespace   string // field 4, urn:x-cast:...
	payload     string // field 6, a JSON document
}

var errMalformed = errors.New("cast: malformed protobuf message")

// maxFrameSize caps an incoming frame. Receiver status payloads are a few KB;
// anything bigger is a corrupt length prefix, not a message.
const maxFrameSize = 1 << 20

// encodeCastMessage serializes m to protobuf wire format.
func encodeCastMessage(m castMessage) []byte {
	b := make([]byte, 0, 32+len(m.source)+len(m.destination)+len(m.namespace)+len(m.payload))
	b = append(b, 0x08, 0x00) // field 1 varint: protocol_version = CASTV2_1_0
	b = appendStringField(b, 0x12, m.source)
	b = appendStringField(b, 0x1a, m.destination)
	b = appendStringField(b, 0x22, m.namespace)
	b = append(b, 0x28, 0x00) // field 5 varint: payload_type = STRING
	b = appendStringField(b, 0x32, m.payload)
	return b
}

func appendStringField(b []byte, tag byte, s string) []byte {
	b = append(b, tag)
	b = appendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

func appendUvarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

// decodeCastMessage parses the string fields we consume and skips any other
// field by wire type, so a receiver adding fields cannot break the parse.
func decodeCastMessage(data []byte) (castMessage, error) {
	var m castMessage
	i := 0
	for i < len(data) {
		tag, n := binary.Uvarint(data[i:])
		if n <= 0 {
			return m, errMalformed
		}
		i += n
		field, wire := tag>>3, tag&0x7
		switch wire {
		case 0: // varint
			_, vn := binary.Uvarint(data[i:])
			if vn <= 0 {
				return m, errMalformed
			}
			i += vn
		case 1: // fixed64
			if i+8 > len(data) {
				return m, errMalformed
			}
			i += 8
		case 5: // fixed32
			if i+4 > len(data) {
				return m, errMalformed
			}
			i += 4
		case 2: // length-delimited
			l, ln := binary.Uvarint(data[i:])
			if ln <= 0 || l > uint64(len(data)-i-ln) {
				return m, errMalformed
			}
			i += ln
			s := string(data[i : i+int(l)])
			i += int(l)
			switch field {
			case 2:
				m.source = s
			case 3:
				m.destination = s
			case 4:
				m.namespace = s
			case 6:
				m.payload = s
			}
		default:
			return m, errMalformed
		}
	}
	return m, nil
}

// writeFrame writes one length-prefixed CastMessage (4-byte big-endian length
// followed by the protobuf body).
func writeFrame(w io.Writer, m castMessage) error {
	body := encodeCastMessage(m)
	frame := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint32(frame, uint32(len(body)))
	_, err := w.Write(append(frame, body...))
	return err
}

// readFrame reads one length-prefixed CastMessage.
func readFrame(r io.Reader) (castMessage, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return castMessage{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameSize {
		return castMessage{}, fmt.Errorf("cast: frame length %d exceeds limit", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return castMessage{}, err
	}
	return decodeCastMessage(body)
}
