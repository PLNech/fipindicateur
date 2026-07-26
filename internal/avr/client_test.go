package avr

import (
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAmp is a loopback eISCP server with canned per-prefix replies and
// a record of every command it received. No external network is used.
type fakeAmp struct {
	t       *testing.T
	ln      net.Listener
	mu      sync.Mutex
	replies map[string]string // command prefix -> reply message
	seen    []string          // every message received, in order
	// noise, when set, is sent before every real reply to simulate the
	// unsolicited frames a live amp interleaves.
	noise string
}

func newFakeAmp(t *testing.T, replies map[string]string) *fakeAmp {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAmp{t: t, ln: ln, replies: replies}
	t.Cleanup(func() { ln.Close() })
	go f.serve()
	return f
}

func (f *fakeAmp) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeAmp) handle(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			return
		}
		buf = append(buf, tmp[:n]...)
		for {
			msg, rest, err := DecodeFrame(buf)
			if err != nil {
				break // short frame: keep reading
			}
			buf = rest
			f.mu.Lock()
			f.seen = append(f.seen, msg)
			noise := f.noise
			var reply string
			for prefix, r := range f.replies {
				if strings.HasPrefix(msg, prefix) {
					reply = r
				}
			}
			f.mu.Unlock()
			if noise != "" {
				conn.Write(EncodeFrame(noise + "\x1a"))
			}
			if reply != "" {
				// Real amps append EOF/CR/LF trailers; exercise that.
				conn.Write(EncodeFrame(reply + "\x1a"))
			}
		}
	}
}

func (f *fakeAmp) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func dialFake(t *testing.T, f *fakeAmp) *Client {
	t.Helper()
	c, err := Dial(f.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestQuery(t *testing.T) {
	f := newFakeAmp(t, map[string]string{"PWR": "PWR01", "MVL": "MVL3E"})
	c := dialFake(t, f)
	if v, err := c.Query("PWR"); err != nil || v != "01" {
		t.Fatalf("Query(PWR) = %q, %v; want 01", v, err)
	}
	if v, err := c.Query("MVL"); err != nil || v != "3E" {
		t.Fatalf("Query(MVL) = %q, %v; want 3E", v, err)
	}
	seen := f.received()
	if len(seen) != 2 || seen[0] != "PWRQSTN" || seen[1] != "MVLQSTN" {
		t.Fatalf("amp saw %v, want [PWRQSTN MVLQSTN]", seen)
	}
}

func TestQuerySkipsUnsolicitedFrames(t *testing.T) {
	f := newFakeAmp(t, map[string]string{"ZPW": "ZPW01"})
	f.noise = "NLSC-P" // a live amp chats about its network list state
	c := dialFake(t, f)
	if v, err := c.Query("ZPW"); err != nil || v != "01" {
		t.Fatalf("Query(ZPW) = %q, %v; want 01 past the noise frame", v, err)
	}
}

func TestStatus(t *testing.T) {
	// Main on NET at hex 3E; zone 2 in standby (volume reads N/A).
	f := newFakeAmp(t, map[string]string{
		"PWR": "PWR01", "MVL": "MVL3E", "SLI": "SLI2B",
		"ZPW": "ZPW00", "ZVL": "ZVLN/A", "SLZ": "SLZN/A",
	})
	c := dialFake(t, f)
	st, err := c.Status()
	if err != nil {
		t.Fatal(err)
	}
	want := State{
		Main:  ZoneState{Power: true, Volume: 0x3E, Source: SourceNet},
		Zone2: ZoneState{Power: false, Volume: VolumeUnknown, Source: ""},
	}
	if st != want {
		t.Fatalf("Status() = %+v, want %+v", st, want)
	}
}

func TestSetZone2(t *testing.T) {
	f := newFakeAmp(t, nil)
	c := dialFake(t, f)
	if err := c.SetZone2Power(true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetZone2Source(SourceNet); err != nil {
		t.Fatal(err)
	}
	if err := c.SetZone2Volume(0x28); err != nil {
		t.Fatal(err)
	}
	if err := c.SetZone2Power(false); err != nil {
		t.Fatal(err)
	}
	// Send returns after writing and each command rides its own
	// connection, so arrival order at the fake amp is not guaranteed:
	// compare as a set, after waiting for all four to land.
	want := []string{"ZPW01", "SLZ2B", "ZVL28", "ZPW00"}
	var seen []string
	for i := 0; i < 200; i++ {
		if seen = f.received(); len(seen) == len(want) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	sort.Strings(seen)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(seen, " ") != strings.Join(sorted, " ") {
		t.Fatalf("amp saw %v, want %v (any order)", seen, want)
	}
}

func TestSetZone2Bounds(t *testing.T) {
	c := &Client{addr: "127.0.0.1:1"} // never reached: validation first
	if err := c.SetZone2Volume(-1); err == nil {
		t.Error("SetZone2Volume(-1): expected range error")
	}
	if err := c.SetZone2Volume(0x65); err == nil {
		t.Error("SetZone2Volume(0x65): expected range error")
	}
	if err := c.SetZone2Source("nope"); err == nil {
		t.Error("SetZone2Source(nope): expected selector error")
	}
}
