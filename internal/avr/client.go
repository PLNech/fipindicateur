package avr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Command prefixes and catalog values, per the community eISCP catalog
// (github.com/miracle2k/onkyo-eiscp, commands/*.yaml). None of these
// has been verified against the household VSX-933 yet: live checks are
// pass 2, at a user-approved moment.
const (
	// DefaultPort is the eISCP TCP port every Onkyo/Pioneer amp listens on.
	DefaultPort = "60128"

	CmdMainPower   = "PWR" // main zone power: 00 standby, 01 on
	CmdMainVolume  = "MVL" // main zone volume: 2-digit uppercase hex level
	CmdMainSource  = "SLI" // main zone input selector
	CmdZone2Power  = "ZPW" // zone 2 power: 00 standby, 01 on
	CmdZone2Volume = "ZVL" // zone 2 volume: 2-digit uppercase hex level
	CmdZone2Source = "SLZ" // zone 2 input selector

	PowerOn      = "01"
	PowerStandby = "00"

	// SourceNet is the NETWORK/NET input selector value (SLI/SLZ "2B"
	// in the catalog). Chromecast built-in is expected to ride the
	// network input, so this is the likely cast source on the VSX-933.
	//
	// TODO(pass 2): NOT yet verified live. Before wiring the zone 2
	// mirror, observe the authoritative value with Query(CmdMainSource)
	// while the amp is casting, at a user-approved moment (the probe
	// harness under internal/avr/probe does exactly that). Catalog
	// candidates in the NET family: "2B" NETWORK/NET (most likely),
	// "28" INTERNET RADIO, "29"/"2A" USB front/rear.
	SourceNet = "2B"

	// queryArg turns a prefix into a status query, e.g. "PWRQSTN".
	queryArg = "QSTN"

	// notAvailable is what the amp answers when a value cannot be
	// reported, e.g. zone volume while the zone is in standby.
	notAvailable = "N/A"
)

// VolumeUnknown is the Volume value used when the amp reports "N/A"
// (typically a zone in standby).
const VolumeUnknown = -1

// maxVolume bounds deliberate absolute volume writes. The catalog caps
// the hex level scale at 100 (0x64) for the widest model range; the
// bound exists so a buggy caller cannot blast a zone.
const maxVolume = 0x64

// ZoneState is a snapshot of one zone.
type ZoneState struct {
	Power  bool   // true when the zone is powered on
	Volume int    // native device scale (2-digit hex on the wire); VolumeUnknown when the amp says N/A
	Source string // input selector value, e.g. "2B" (NET); "" when unknown
}

// State is a snapshot of the main zone and zone 2.
type State struct {
	Main  ZoneState
	Zone2 ZoneState
}

// Client talks eISCP to one amp. Each call opens a fresh, deadline-bound
// connection (the amp handles this fine and it avoids keep-alive
// bookkeeping); no call ever blocks past the configured timeout.
type Client struct {
	addr    string
	timeout time.Duration

	// Trace, when set, receives every raw frame written to or read
	// from the amp ("tx" or "rx"). Meant for probes and debugging.
	Trace func(dir string, raw []byte)
}

// Dial validates that an eISCP endpoint answers at addr (host or
// host:port; port defaults to 60128) and returns a client for it.
// Nothing is sent to the amp: connecting is the whole check.
func Dial(addr string) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, DefaultPort)
	}
	c := &Client{addr: addr, timeout: 3 * time.Second}
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("avr: dial %s: %w", c.addr, err)
	}
	conn.Close()
	return c, nil
}

// Send writes one command (e.g. "ZPW01") and returns once it is on the
// wire. It does not wait for the amp's echo; callers verify state
// changes with Query, which is more robust than matching echoes.
func (c *Client) Send(cmd string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.write(conn, cmd)
}

// Query sends "{prefix}QSTN" and returns the value of the first reply
// for that prefix (e.g. Query("PWR") -> "01"). Unsolicited frames for
// other prefixes are skipped. The whole exchange is deadline-bound.
func (c *Client) Query(prefix string) (string, error) {
	conn, err := c.connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := c.write(conn, prefix+queryArg); err != nil {
		return "", err
	}
	for {
		msg, err := c.readFrame(conn)
		if err != nil {
			return "", fmt.Errorf("avr: query %s: %w", prefix, err)
		}
		if strings.HasPrefix(msg, prefix) {
			return strings.TrimPrefix(msg, prefix), nil
		}
	}
}

// Status queries power, volume and source for the main zone and zone 2
// (six queries, one connection each, all deadline-bound).
func (c *Client) Status() (State, error) {
	var st State
	var err error
	if st.Main, err = c.zoneStatus(CmdMainPower, CmdMainVolume, CmdMainSource); err != nil {
		return st, err
	}
	if st.Zone2, err = c.zoneStatus(CmdZone2Power, CmdZone2Volume, CmdZone2Source); err != nil {
		return st, err
	}
	return st, nil
}

func (c *Client) zoneStatus(power, volume, source string) (ZoneState, error) {
	var z ZoneState
	pw, err := c.Query(power)
	if err != nil {
		return z, err
	}
	z.Power = pw == PowerOn
	vol, err := c.Query(volume)
	if err != nil {
		return z, err
	}
	if z.Volume, err = parseVolume(vol); err != nil {
		return z, err
	}
	src, err := c.Query(source)
	if err != nil {
		return z, err
	}
	if src != notAvailable {
		z.Source = src
	}
	return z, nil
}

// SetZone2Power switches zone 2 on or to standby. Main zone power is
// deliberately not exposed: this package only drives zone 2.
func (c *Client) SetZone2Power(on bool) error {
	v := PowerStandby
	if on {
		v = PowerOn
	}
	return c.Send(CmdZone2Power + v)
}

// SetZone2Volume sets the zone 2 absolute level on the native device
// scale (written as 2-digit uppercase hex). Levels outside [0, 0x64]
// are refused: volume changes are deliberate and bounded by design.
func (c *Client) SetZone2Volume(level int) error {
	if level < 0 || level > maxVolume {
		return fmt.Errorf("avr: zone2 volume %d out of range [0, %d]", level, maxVolume)
	}
	return c.Send(fmt.Sprintf("%s%02X", CmdZone2Volume, level))
}

// SetZone2Source selects the zone 2 input. Passing the value the main
// zone reports from Query(CmdMainSource) makes zone 2 mirror the main
// zone; whether and when to auto-mirror is UI-layer policy (a later
// pass), never decided here.
func (c *Client) SetZone2Source(value string) error {
	if !validSelector(value) {
		return fmt.Errorf("avr: invalid source selector %q", value)
	}
	return c.Send(CmdZone2Source + value)
}

// connect opens a fresh connection with an absolute deadline covering
// the whole call, so no exchange can outlive the timeout.
func (c *Client) connect() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("avr: dial %s: %w", c.addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *Client) write(conn net.Conn, cmd string) error {
	raw := EncodeFrame(cmd)
	if c.Trace != nil {
		c.Trace("tx", raw)
	}
	if _, err := conn.Write(raw); err != nil {
		return fmt.Errorf("avr: send %s: %w", cmd, err)
	}
	return nil
}

// readFrame reads exactly one eISCP frame off the connection and
// returns its trimmed message. The connection deadline bounds it.
func (c *Client) readFrame(conn net.Conn) (string, error) {
	hdr := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return "", err
	}
	msg, _, err := DecodeFrame(hdr)
	if err == nil {
		if c.Trace != nil {
			c.Trace("rx", hdr)
		}
		return msg, nil
	}
	if !errors.Is(err, ErrShortFrame) {
		return "", err
	}
	// Header parsed but data still on the wire: read the declared rest.
	declared := int(binary.BigEndian.Uint32(hdr[4:8]))
	size := int(binary.BigEndian.Uint32(hdr[8:12]))
	if declared < headerSize || declared > maxDataSize || size > maxDataSize {
		return "", fmt.Errorf("avr: implausible frame sizes hdr=%d data=%d", declared, size)
	}
	buf := make([]byte, declared+size)
	copy(buf, hdr)
	if _, err := io.ReadFull(conn, buf[headerSize:]); err != nil {
		return "", err
	}
	if c.Trace != nil {
		c.Trace("rx", buf)
	}
	msg, _, err = DecodeFrame(buf)
	return msg, err
}

// parseVolume decodes a volume reply: 2-digit hex level, or "N/A" when
// the zone cannot report one (VolumeUnknown).
func parseVolume(v string) (int, error) {
	if v == notAvailable {
		return VolumeUnknown, nil
	}
	n, err := strconv.ParseInt(v, 16, 32)
	if err != nil {
		return VolumeUnknown, fmt.Errorf("avr: bad volume value %q", v)
	}
	return int(n), nil
}

// validSelector accepts the 2-character hex selector values the SLI/SLZ
// catalog uses (e.g. "2B", "05").
func validSelector(v string) bool {
	if len(v) != 2 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
