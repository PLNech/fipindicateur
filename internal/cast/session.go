package cast

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Namespaces and well-known ids of the CASTV2 protocol.
const (
	nsConnection = "urn:x-cast:com.google.cast.tp.connection"
	nsHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	nsReceiver   = "urn:x-cast:com.google.cast.receiver"
	nsMedia      = "urn:x-cast:com.google.cast.media"

	senderID   = "sender-0"
	receiverID = "receiver-0"

	// defaultReceiverApp is Google's Default Media Receiver, which plays
	// plain audio URLs natively: casting FIP is LOADing the icecast URL
	// into it.
	defaultReceiverApp = "CC1AD845"
)

const (
	dialTimeout = 5 * time.Second
	// launchTimeout bounds the wait for the RECEIVER_STATUS that answers
	// LAUNCH. Deliberately generous: AV receivers cold-boot their cast module
	// on LAUNCH (a Pioneer VSX-933 measured 8.2s to answer on a warm launch;
	// cold exceeds 10s). PINGs keep being answered during the wait.
	launchTimeout = 30 * time.Second
	writeTimeout  = 5 * time.Second
	// readIdleTimeout bounds the read loop: the device pings every ~5s, so a
	// silent 30s means the link is dead even if TCP has not noticed yet.
	readIdleTimeout = 30 * time.Second
	// heartbeatEvery is our own keepalive PING cadence.
	heartbeatEvery = 5 * time.Second
)

// Session is a live connection to a Chromecast running the Default Media
// Receiver. All exported methods are safe to call from the UI goroutine:
// writes are serialized and time-bounded, reads happen on an internal
// goroutine, and nothing here can block the tray for more than a bounded
// network timeout.
type Session struct {
	dev  Device
	conn *tls.Conn

	wmu   sync.Mutex // serializes frames on the wire (UI calls vs heartbeat)
	reqID int64      // monotonically increasing requestId (atomic)

	sessionID   string // receiver session id, needed to STOP the app
	transportID string // the media channel peer for this session

	// Device-side state gleaned from the read loop. The volume block feeds the
	// panel's cast-aware slider; the media session id scopes PLAY/PAUSE. All
	// guarded by smu (the read loop writes, UI goroutines read).
	smu            sync.Mutex
	vol            VolumeStatus
	volKnown       bool
	mediaSessionID int
	playerState    string

	onError  func(error) // surfaced at most once, on unexpected connection death
	onStatus func()      // optional: fired after any device status update (read-loop goroutine)
	closed   chan struct{}
	once     sync.Once
}

// Dial connects to the device, launches the Default Media Receiver and joins
// its session. onError (optional) is called at most once, from an internal
// goroutine, if the connection later dies unexpectedly; a deliberate
// Stop/Close never triggers it. onStatus (optional) is called from the read
// goroutine after each device status update (volume, media state), so a UI
// can refresh what it displays; it must not block.
func Dial(dev Device, onError func(error), onStatus func()) (*Session, error) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	// Chromecasts present self-signed device certificates: there is no CA to
	// verify against, so skipping verification is the protocol's normal mode.
	conn, err := tls.DialWithDialer(dialer, "tcp",
		net.JoinHostPort(dev.Addr, strconv.Itoa(dev.Port)),
		&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	s := &Session{dev: dev, conn: conn, onError: onError, onStatus: onStatus, closed: make(chan struct{})}
	if err := s.handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	go s.readLoop()
	go s.heartbeatLoop()
	return s, nil
}

// Device returns the target this session is casting to.
func (s *Session) Device() Device { return s.dev }

// handshake runs the session bring-up synchronously: CONNECT to the platform
// receiver, LAUNCH the Default Media Receiver, wait for the RECEIVER_STATUS
// carrying the session's transportId, then CONNECT to that transport. Each
// send is bounded by writeTimeout; the status wait is bounded by the more
// generous launchTimeout (slow AV receivers, see the constant).
func (s *Session) handshake() error {
	if err := s.send(nsConnection, receiverID, `{"type":"CONNECT"}`); err != nil {
		return err
	}
	if err := s.send(nsReceiver, receiverID, launchPayload(s.nextReq())); err != nil {
		return err
	}
	_ = s.conn.SetReadDeadline(time.Now().Add(launchTimeout))
	defer s.conn.SetReadDeadline(time.Time{})
	for {
		m, err := readFrame(s.conn)
		if err != nil {
			return fmt.Errorf("cast: waiting for receiver status: %w", err)
		}
		switch m.namespace {
		case nsHeartbeat:
			// Answer pings even mid-handshake, or the device drops us.
			if payloadType(m.payload) == "PING" {
				if err := s.send(nsHeartbeat, receiverID, `{"type":"PONG"}`); err != nil {
					return err
				}
			}
		case nsReceiver:
			switch payloadType(m.payload) {
			case "LAUNCH_ERROR":
				return errors.New("cast: receiver refused the launch")
			case "RECEIVER_STATUS":
				// The LAUNCH answer usually carries the device's current
				// volume: read it now so the UI can display the real level.
				// We never SET a volume on start; the device's level rules.
				s.storeReceiverVolume(m.payload)
				sess, transport, ok := findDefaultReceiver(m.payload)
				if !ok {
					continue // a status for something else; keep waiting
				}
				s.sessionID, s.transportID = sess, transport
				// Join the application's own channel before media commands.
				return s.send(nsConnection, s.transportID, `{"type":"CONNECT"}`)
			}
		}
	}
}

// Load starts (or replaces, on a station zap) a live stream on the device.
// Fire-and-forget: a transport-level failure surfaces via onError, a media
// error on a healthy connection is the device's problem to display.
func (s *Session) Load(url, contentType, title string) error {
	return s.send(nsMedia, s.transportID, loadPayload(s.nextReq(), url, contentType, title))
}

// ReceiverVolume returns the device-side volume as last reported by the
// device (RECEIVER_STATUS). ok is false until the first report arrives.
func (s *Session) ReceiverVolume() (VolumeStatus, bool) {
	s.smu.Lock()
	defer s.smu.Unlock()
	return s.vol, s.volKnown
}

// SetVolume sets the device's volume level (0..1), quantized to the device's
// stepInterval. On a controlType "master" device (an AV receiver) this drives
// the amplifier's MASTER volume: the caller sends only user-chosen levels,
// never a default. The device answers with a RECEIVER_STATUS that refreshes
// ReceiverVolume via the read loop.
func (s *Session) SetVolume(level float64) error {
	s.smu.Lock()
	step := s.vol.StepInterval
	s.smu.Unlock()
	return s.send(nsReceiver, receiverID, setVolumeLevelPayload(s.nextReq(), quantizeLevel(level, step)))
}

// SetMuted mutes or unmutes the device (the level is preserved device-side).
func (s *Session) SetMuted(muted bool) error {
	return s.send(nsReceiver, receiverID, setVolumeMutedPayload(s.nextReq(), muted))
}

// MediaSessionID returns the media session learned from MEDIA_STATUS, or 0
// when none was seen yet (LOAD not yet answered).
func (s *Session) MediaSessionID() int {
	s.smu.Lock()
	defer s.smu.Unlock()
	return s.mediaSessionID
}

// MediaPlaying reports whether the device says its media is playing. Before
// the first MEDIA_STATUS it assumes true: LOADs are sent with autoplay.
func (s *Session) MediaPlaying() bool {
	s.smu.Lock()
	defer s.smu.Unlock()
	switch s.playerState {
	case "PAUSED", "IDLE":
		return false
	default: // "", "PLAYING", "BUFFERING": sound is (about to be) on
		return true
	}
}

// PauseMedia pauses the media on the device (the cast session stays up).
// Needs a media session id from MEDIA_STATUS; errors until one arrived.
func (s *Session) PauseMedia() error {
	return s.mediaCommand("PAUSE")
}

// PlayMedia resumes the paused media on the device.
func (s *Session) PlayMedia() error {
	return s.mediaCommand("PLAY")
}

func (s *Session) mediaCommand(typ string) error {
	s.smu.Lock()
	id := s.mediaSessionID
	s.smu.Unlock()
	if id == 0 {
		return errors.New("cast: no media session yet (LOAD not answered)")
	}
	return s.send(nsMedia, s.transportID, mediaCommandPayload(s.nextReq(), typ, id))
}

// storeReceiverVolume records the volume block of a RECEIVER_STATUS, if it
// carries one, and reports whether anything was stored.
func (s *Session) storeReceiverVolume(payload string) bool {
	v, ok := parseReceiverVolume(payload)
	if !ok {
		return false
	}
	s.smu.Lock()
	s.vol = v
	s.volKnown = true
	s.smu.Unlock()
	return true
}

// notifyStatus fires the optional status callback (read-loop goroutine).
func (s *Session) notifyStatus() {
	if s.onStatus != nil {
		s.onStatus()
	}
}

// Stop quits the receiver app on the device and closes the connection. All
// of it best-effort: the device may already be unreachable, and the caller's
// state reset must not depend on the goodbye being heard.
func (s *Session) Stop() {
	_ = s.send(nsReceiver, receiverID, stopPayload(s.nextReq(), s.sessionID))
	_ = s.send(nsConnection, s.transportID, `{"type":"CLOSE"}`)
	_ = s.send(nsConnection, receiverID, `{"type":"CLOSE"}`)
	s.Close()
}

// Close tears the connection down without touching the device (the receiver
// app keeps playing). Safe to call multiple times; never triggers onError.
func (s *Session) Close() { s.shutdown(nil) }

// shutdown closes exactly once; a non-nil err marks the death unexpected and
// is handed to onError on its own goroutine (never the caller's).
func (s *Session) shutdown(err error) {
	s.once.Do(func() {
		close(s.closed)
		_ = s.conn.Close()
		if err != nil && s.onError != nil {
			go s.onError(err)
		}
	})
}

// send writes one frame, serialized and time-bounded so a wedged device can
// never block a UI-originated call for long.
func (s *Session) send(namespace, destination, payload string) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return writeFrame(s.conn, castMessage{
		source:      senderID,
		destination: destination,
		namespace:   namespace,
		payload:     payload,
	})
}

func (s *Session) nextReq() int { return int(atomic.AddInt64(&s.reqID, 1)) }

// readLoop consumes frames until the connection dies: it answers heartbeat
// PINGs, detects the peer closing a channel, and keeps the device-side state
// fresh (RECEIVER_STATUS volume, MEDIA_STATUS session id + player state) so
// the panel can display and drive the device.
func (s *Session) readLoop() {
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(readIdleTimeout))
		m, err := readFrame(s.conn)
		if err != nil {
			s.shutdown(fmt.Errorf("cast: connection lost: %w", err))
			return
		}
		switch m.namespace {
		case nsHeartbeat:
			if payloadType(m.payload) == "PING" {
				dest := m.source
				if dest == "" || dest == "*" {
					dest = receiverID
				}
				_ = s.send(nsHeartbeat, dest, `{"type":"PONG"}`)
			}
		case nsReceiver:
			// Volume changes (ours, another sender's, or the amp's own knob)
			// broadcast as RECEIVER_STATUS: mirror them.
			if payloadType(m.payload) == "RECEIVER_STATUS" && s.storeReceiverVolume(m.payload) {
				s.notifyStatus()
			}
		case nsMedia:
			if payloadType(m.payload) == "MEDIA_STATUS" {
				if id, state, ok := parseMediaStatus(m.payload); ok {
					s.smu.Lock()
					s.mediaSessionID = id
					s.playerState = state
					s.smu.Unlock()
					s.notifyStatus()
				}
			}
		case nsConnection:
			// The platform or the app closed our virtual connection: either
			// way this session cannot cast anymore.
			if payloadType(m.payload) == "CLOSE" {
				s.shutdown(errors.New("cast: receiver closed the session"))
				return
			}
		}
	}
}

// heartbeatLoop sends our own PING every heartbeatEvery, so the device knows
// the sender is alive (and a dead link is noticed by the write failing).
func (s *Session) heartbeatLoop() {
	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-s.closed:
			return
		case <-t.C:
			if err := s.send(nsHeartbeat, receiverID, `{"type":"PING"}`); err != nil {
				s.shutdown(fmt.Errorf("cast: heartbeat: %w", err))
				return
			}
		}
	}
}
