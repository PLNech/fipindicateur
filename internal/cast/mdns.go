package cast

import (
	"bytes"
	"encoding/binary"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device is a Chromecast target found on the local network.
type Device struct {
	Name string // friendly name (TXT fn=), or the mDNS instance label
	Addr string // IPv4 address, dotted quad
	Port int    // CASTV2 TLS port (8009 in practice)
}

// castService is the mDNS service Chromecasts register under (no trailing
// dot: parseName renders names without one).
const castService = "_googlecast._tcp.local"

// DNS record types we care about.
const (
	typeA   = 1
	typePTR = 12
	typeTXT = 16
	typeSRV = 33
)

// Discover looks for Chromecast devices: one mDNS PTR question for
// _googlecast._tcp.local sent to the mDNS group on port 5353, answers
// collected for the window.
//
// The question sets the QU (unicast-response) bit and leaves from an
// ephemeral port, so responders that honour QU reply straight to that socket.
// Per RFC 6762 section 5.4, QU is a request, not a guarantee: responders may
// still answer multicast (real devices do), so a second socket joins the mDNS
// group on port 5353 to hear those. Go sets SO_REUSEADDR on the multicast
// listen, so it coexists with a running avahi-daemon; if that listen fails on
// an exotic setup we degrade gracefully to the unicast socket alone.
// Best-effort by contract: any network trouble yields an empty list, never an
// error (the tray must not care why the network said nothing).
func Discover(window time.Duration) []Device {
	uni, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil
	}
	defer uni.Close()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	multi, merr := net.ListenMulticastUDP("udp4", nil, group)
	if merr == nil {
		defer multi.Close()
	}

	query := buildPTRQuery(castService)
	if _, err := uni.WriteToUDP(query, group); err != nil {
		return nil
	}
	// mDNS over UDP is lossy: ask a second time mid-window, from the same
	// ephemeral socket (its source port is where QU unicast replies land).
	again := time.AfterFunc(window/2, func() { _, _ = uni.WriteToUDP(query, group) })
	defer again.Stop()

	// Read replies from both sockets until the window elapses, funnelling
	// packets into one channel so record sets merge across sources.
	deadline := time.Now().Add(window)
	pkts := make(chan []byte, 16)
	var wg sync.WaitGroup
	collect := func(c *net.UDPConn) {
		defer wg.Done()
		_ = c.SetReadDeadline(deadline)
		buf := make([]byte, 9000) // an mDNS packet fits a jumbo frame
		for {
			n, _, err := c.ReadFromUDP(buf)
			if err != nil {
				return // window elapsed (or socket trouble): done collecting
			}
			pkts <- append([]byte(nil), buf[:n]...)
		}
	}
	wg.Add(1)
	go collect(uni)
	if merr == nil {
		wg.Add(1)
		go collect(multi)
	}
	go func() { wg.Wait(); close(pkts) }()

	found := map[string]Device{} // keyed by instance name; packets may split a record set
	for pkt := range pkts {
		for inst, d := range parseCastResponse(pkt) {
			found[inst] = mergeDevice(found[inst], d)
		}
	}

	insts := make([]string, 0, len(found))
	for inst, d := range found {
		if d.Addr != "" && d.Port != 0 { // else not dialable (no A or no SRV seen)
			insts = append(insts, inst)
		}
	}
	sort.Strings(insts) // stable menu order across scans
	devs := make([]Device, 0, len(insts))
	for _, inst := range insts {
		devs = append(devs, found[inst])
	}
	return devs
}

// mergeDevice fills dst's missing fields from src (record sets can arrive
// split across packets).
func mergeDevice(dst, src Device) Device {
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Addr == "" {
		dst.Addr = src.Addr
	}
	if dst.Port == 0 {
		dst.Port = src.Port
	}
	return dst
}

// buildPTRQuery encodes a one-question DNS query (ID 0, no flags) asking for
// PTR records of the given service. The QU bit (top bit of QCLASS, RFC 6762
// §5.4) requests a unicast response back to our ephemeral source port; real
// devices ignore the legacy multicast-response form of this query entirely.
func buildPTRQuery(service string) []byte {
	b := make([]byte, 12)
	b[5] = 1 // QDCOUNT = 1
	for _, label := range strings.Split(service, ".") {
		if label == "" {
			continue
		}
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0x00)       // root
	b = append(b, 0x00, 0x0c) // QTYPE = PTR
	b = append(b, 0x80, 0x01) // QCLASS = IN, QU (unicast response requested)
	return b
}

// parseCastResponse extracts googlecast SRV/TXT/A record sets from one DNS
// response packet, returning instance name to (possibly partial) Device.
// Malformed packets yield nil: discovery reads whatever the network delivers
// and must never panic on it.
func parseCastResponse(pkt []byte) map[string]Device {
	if len(pkt) < 12 {
		return nil
	}
	qd := int(binary.BigEndian.Uint16(pkt[4:6]))
	rrCount := int(binary.BigEndian.Uint16(pkt[6:8])) + // answers
		int(binary.BigEndian.Uint16(pkt[8:10])) + // authority
		int(binary.BigEndian.Uint16(pkt[10:12])) // additional
	off := 12
	for i := 0; i < qd; i++ { // skip questions
		_, n, ok := parseName(pkt, off)
		if !ok || n+4 > len(pkt) {
			return nil
		}
		off = n + 4 // QTYPE + QCLASS
	}

	srvPort := map[string]int{}    // instance -> port
	srvHost := map[string]string{} // instance -> target host
	txtName := map[string]string{} // instance -> friendly name (fn=)
	hostIP := map[string]string{}  // host -> IPv4

	for i := 0; i < rrCount; i++ {
		name, n, ok := parseName(pkt, off)
		if !ok || n+10 > len(pkt) {
			break
		}
		off = n
		typ := binary.BigEndian.Uint16(pkt[off:])
		rdlen := int(binary.BigEndian.Uint16(pkt[off+8:]))
		off += 10
		if off+rdlen > len(pkt) {
			break
		}
		rd := pkt[off : off+rdlen]
		switch typ {
		case typeSRV:
			if strings.HasSuffix(name, "."+castService) && len(rd) >= 6 {
				// priority(2) weight(2) port(2) then the target name, which may
				// be compressed, so it is parsed against the whole packet.
				srvPort[name] = int(binary.BigEndian.Uint16(rd[4:6]))
				if target, _, ok := parseName(pkt, off+6); ok {
					srvHost[name] = target
				}
			}
		case typeTXT:
			if strings.HasSuffix(name, "."+castService) {
				if fn := txtValue(rd, "fn="); fn != "" {
					txtName[name] = fn
				}
			}
		case typeA:
			if len(rd) == 4 {
				hostIP[name] = net.IP(rd).String()
			}
		}
		off += rdlen
	}

	out := map[string]Device{}
	for inst, port := range srvPort {
		d := Device{Port: port, Addr: hostIP[srvHost[inst]]}
		if fn := txtName[inst]; fn != "" {
			d.Name = fn
		} else {
			d.Name = instanceLabel(inst)
		}
		out[inst] = d
	}
	// A TXT-only packet still contributes the friendly name.
	for inst, fn := range txtName {
		if _, ok := out[inst]; !ok {
			out[inst] = Device{Name: fn}
		}
	}
	return out
}

// txtValue scans a TXT rdata (length-prefixed strings) for a key prefix and
// returns the remainder of the first match.
func txtValue(rd []byte, prefix string) string {
	for i := 0; i < len(rd); {
		l := int(rd[i])
		i++
		if i+l > len(rd) {
			return ""
		}
		if s := string(rd[i : i+l]); strings.HasPrefix(s, prefix) {
			return s[len(prefix):]
		}
		i += l
	}
	return ""
}

// instanceLabel is the first label of an instance name, the fallback display
// name when no fn= TXT key was seen.
func instanceLabel(inst string) string {
	if i := strings.Index(inst, "."); i >= 0 {
		return inst[:i]
	}
	return inst
}

// parseName decodes a possibly-compressed DNS name starting at off. It
// returns the dotted name (lowercased: mDNS names are case-insensitive) and
// the offset just past the name at its original position (a compression
// pointer occupies two bytes there, wherever it points).
func parseName(pkt []byte, off int) (string, int, bool) {
	var sb strings.Builder
	end := -1 // offset after the name at the original position
	jumps := 0
	for {
		if off >= len(pkt) {
			return "", 0, false
		}
		b := pkt[off]
		switch {
		case b == 0:
			if end < 0 {
				end = off + 1
			}
			return sb.String(), end, true
		case b&0xc0 == 0xc0: // compression pointer
			if off+1 >= len(pkt) {
				return "", 0, false
			}
			if jumps++; jumps > 32 { // guard against pointer loops
				return "", 0, false
			}
			if end < 0 {
				end = off + 2
			}
			off = int(b&0x3f)<<8 | int(pkt[off+1])
		case b&0xc0 == 0: // plain label
			l := int(b)
			if off+1+l > len(pkt) {
				return "", 0, false
			}
			if sb.Len() > 0 {
				sb.WriteByte('.')
			}
			sb.Write(bytes.ToLower(pkt[off+1 : off+1+l]))
			off += 1 + l
		default: // 0x40/0x80 label types are obsolete and unexpected here
			return "", 0, false
		}
	}
}
