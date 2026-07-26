package cast

import (
	"encoding/binary"
	"testing"
)

// pktBuilder assembles a synthetic DNS response, tracking offsets so the test
// can plant real compression pointers (the part of the parser that bites).
type pktBuilder struct{ b []byte }

func (p *pktBuilder) off() int { return len(p.b) }

func (p *pktBuilder) raw(bs ...byte) { p.b = append(p.b, bs...) }

func (p *pktBuilder) u16(v uint16) {
	p.b = binary.BigEndian.AppendUint16(p.b, v)
}

// labels writes plain (uncompressed) labels WITHOUT the root terminator.
func (p *pktBuilder) labels(ls ...string) {
	for _, l := range ls {
		p.b = append(p.b, byte(len(l)))
		p.b = append(p.b, l...)
	}
}

// ptr writes a compression pointer to a prior offset.
func (p *pktBuilder) ptr(off int) {
	p.u16(0xc000 | uint16(off))
}

// rrHeader writes type, class IN, TTL and returns the position of the
// 2-byte RDLENGTH placeholder for patchRdlen.
func (p *pktBuilder) rrHeader(typ uint16) int {
	p.u16(typ)
	p.u16(0x8001) // class IN, cache-flush bit set (as real responders do)
	p.raw(0, 0, 0, 120)
	pos := p.off()
	p.u16(0)
	return pos
}

func (p *pktBuilder) patchRdlen(pos int) {
	binary.BigEndian.PutUint16(p.b[pos:], uint16(p.off()-pos-2))
}

// buildCastResponse fabricates a captured-style answer for one device
// ("Salon" / fn=Salon TV / salon-tv.local:8009 / 192.168.1.42), spreading
// PTR+TXT over the answer section and SRV+A over additionals, with the
// instance and service names compressed via pointers.
func buildCastResponse(t *testing.T) []byte {
	t.Helper()
	p := &pktBuilder{}
	p.u16(0)      // ID
	p.u16(0x8400) // flags: response, authoritative
	p.u16(0)      // QDCOUNT
	p.u16(2)      // ANCOUNT: PTR + TXT
	p.u16(0)      // NSCOUNT
	p.u16(2)      // ARCOUNT: SRV + A

	// Answer 1: PTR _googlecast._tcp.local -> Salon._googlecast._tcp.local
	svcOff := p.off()
	p.labels("_googlecast", "_tcp")
	localOff := p.off()
	p.labels("local")
	p.raw(0)
	rdpos := p.rrHeader(typePTR)
	instOff := p.off()
	p.labels("Salon")
	p.ptr(svcOff) // instance name compressed onto the service name
	p.patchRdlen(rdpos)

	// Answer 2: TXT for the instance (owner is a pointer), fn= among noise.
	p.ptr(instOff)
	rdpos = p.rrHeader(typeTXT)
	p.labels("id=0123456789abcdef", "fn=Salon TV", "md=Chromecast")
	p.patchRdlen(rdpos)

	// Additional 1: SRV for the instance, target compressed onto "local".
	p.ptr(instOff)
	rdpos = p.rrHeader(typeSRV)
	p.u16(0)    // priority
	p.u16(0)    // weight
	p.u16(8009) // port
	p.labels("salon-tv")
	p.ptr(localOff)
	p.patchRdlen(rdpos)

	// Additional 2: A record for the SRV target, written uncompressed.
	p.labels("salon-tv", "local")
	p.raw(0)
	rdpos = p.rrHeader(typeA)
	p.raw(192, 168, 1, 42)
	p.patchRdlen(rdpos)

	return p.b
}

func TestParseCastResponse(t *testing.T) {
	devs := parseCastResponse(buildCastResponse(t))
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d: %+v", len(devs), devs)
	}
	d, ok := devs["salon._googlecast._tcp.local"]
	if !ok {
		t.Fatalf("instance name not found (names are lowercased); got %+v", devs)
	}
	if d.Name != "Salon TV" {
		t.Errorf("Name = %q, want %q (from the fn= TXT key)", d.Name, "Salon TV")
	}
	if d.Addr != "192.168.1.42" {
		t.Errorf("Addr = %q, want 192.168.1.42", d.Addr)
	}
	if d.Port != 8009 {
		t.Errorf("Port = %d, want 8009", d.Port)
	}
}

func TestParseCastResponseNoFnFallsBackToLabel(t *testing.T) {
	p := &pktBuilder{}
	p.u16(0)
	p.u16(0x8400)
	p.u16(0)
	p.u16(2) // AN: SRV + A
	p.u16(0)
	p.u16(0)

	p.labels("Cuisine", "_googlecast", "_tcp", "local")
	p.raw(0)
	rdpos := p.rrHeader(typeSRV)
	p.u16(0)
	p.u16(0)
	p.u16(8009)
	p.labels("cuisine", "local")
	p.raw(0)
	p.patchRdlen(rdpos)

	p.labels("cuisine", "local")
	p.raw(0)
	rdpos = p.rrHeader(typeA)
	p.raw(10, 0, 0, 7)
	p.patchRdlen(rdpos)

	devs := parseCastResponse(p.b)
	d, ok := devs["cuisine._googlecast._tcp.local"]
	if !ok {
		t.Fatalf("instance not parsed: %+v", devs)
	}
	if d.Name != "cuisine" || d.Addr != "10.0.0.7" || d.Port != 8009 {
		t.Fatalf("got %+v, want label-fallback name cuisine @10.0.0.7:8009", d)
	}
}

func TestParseCastResponseMalformedNeverPanics(t *testing.T) {
	good := buildCastResponse(t)
	cases := [][]byte{
		nil,
		{},
		good[:8],  // shorter than a header
		good[:40], // truncated inside the first record's fixed fields
		{0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0xc0, 0x0c}, // owner pointer to itself
	}
	for i, pkt := range cases {
		if devs := parseCastResponse(pkt); len(devs) != 0 {
			t.Errorf("case %d: expected no devices from a malformed packet, got %+v", i, devs)
		}
	}
}

func TestParseNamePointerLoopGuard(t *testing.T) {
	// A name that is a pointer to itself must fail, not spin.
	pkt := make([]byte, 14)
	pkt[12], pkt[13] = 0xc0, 0x0c
	if _, _, ok := parseName(pkt, 12); ok {
		t.Fatal("self-referential pointer should not parse")
	}
}

func TestBuildPTRQuery(t *testing.T) {
	q := buildPTRQuery(castService)
	if len(q) < 12+len(castService)+2+4 {
		t.Fatalf("query too short: %d bytes", len(q))
	}
	if q[4] != 0 || q[5] != 1 {
		t.Fatalf("QDCOUNT = %d, want 1", int(q[4])<<8|int(q[5]))
	}
	// The question name must parse back to the service.
	name, off, ok := parseName(q, 12)
	if !ok || name != castService {
		t.Fatalf("question name = %q ok=%v, want %q", name, ok, castService)
	}
	if typ := binary.BigEndian.Uint16(q[off:]); typ != typePTR {
		t.Fatalf("QTYPE = %d, want PTR (%d)", typ, typePTR)
	}
	// QU bit (RFC 6762 section 5.4) must be set on class IN: real devices
	// ignore the legacy multicast-response form of this query.
	class := binary.BigEndian.Uint16(q[off+2:])
	if class&0x8000 == 0 {
		t.Fatalf("QCLASS = %#04x, QU bit (0x8000) not set", class)
	}
	if class&0x7fff != 1 {
		t.Fatalf("QCLASS = %#04x, want class IN (1) under the QU bit", class)
	}
}
