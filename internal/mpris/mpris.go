//go:build linux

// Package mpris exposes the player over MPRIS2 D-Bus so desktop media keys and
// tools like playerctl can control it.
//
// Derived from the fip-player project (WTFPL):
// https://github.com/DucNg/fip-player (dbus/*.go). Adapted to our NowPlaying
// type and a Controller interface, and renamed to
// org.mpris.MediaPlayer2.fipindicateur.
package mpris

import (
	"errors"

	"github.com/PLNech/fipindicateur/internal/metadata"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const wellKnownName = "org.mpris.MediaPlayer2.fipindicateur"

// ErrAlreadyRunning means another instance owns the MPRIS name. The
// well-known name doubles as the single-instance lock: a second launch (e.g.
// from GNOME activities) must exit cleanly instead of spawning a second tray
// icon. D-Bus releases the name automatically if its owner dies, so the lock
// cannot go stale.
var ErrAlreadyRunning = errors.New("mpris: name already taken (another instance is running)")

// Controller receives playback commands issued over D-Bus.
type Controller interface {
	Play()
	Pause() // full stop for live radio
	Toggle()
	// SetVolumeFrac applies an externally requested volume in 0..1 (the MPRIS
	// Volume unit) and reflects it in the app UI.
	SetVolumeFrac(v float64)
}

// Instance owns the D-Bus connection and exported properties.
type Instance struct {
	props *prop.Properties
	conn  *dbus.Conn
	ctrl  Controller
}

// Connect exports the MPRIS interfaces and claims the well-known name.
func Connect(ctrl Controller) (*Instance, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	ins := &Instance{conn: conn, ctrl: ctrl}
	mp2 := &mediaPlayer2{ins: ins}

	if err := conn.Export(mp2, "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2.Player"); err != nil {
		return nil, err
	}
	if err := conn.Export(mp2, "/org/mpris/MediaPlayer2", "org.mpris.MediaPlayer2"); err != nil {
		return nil, err
	}
	if err := conn.Export(introspect.NewIntrospectable(introspectNode()), "/org/mpris/MediaPlayer2", "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}

	ins.props, err = prop.Export(conn, "/org/mpris/MediaPlayer2", map[string]map[string]*prop.Prop{
		"org.mpris.MediaPlayer2":        mp2.rootProps(),
		"org.mpris.MediaPlayer2.Player": mp2.playerProps(),
	})
	if err != nil {
		return nil, err
	}

	// No ReplaceExisting: failing to own the name IS the single-instance
	// signal, not something to override.
	reply, err := conn.RequestName(wellKnownName, 0)
	if err != nil {
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		_ = conn.Close()
		return nil, ErrAlreadyRunning
	}
	return ins, nil
}

// SetPlaybackStatus updates the PlaybackStatus property. SetMust is used
// (not Set) because the property is read-only to D-Bus clients but must be
// writable internally; SetMust bypasses the writability check and emits the
// PropertiesChanged signal.
func (ins *Instance) SetPlaybackStatus(playing bool) {
	status := "Paused"
	if playing {
		status = "Playing"
	}
	ins.props.SetMust("org.mpris.MediaPlayer2.Player", "PlaybackStatus", status)
}

// UpdateMetadata publishes the current track (SetMust: see SetPlaybackStatus).
func (ins *Instance) UpdateMetadata(np metadata.NowPlaying) {
	ins.props.SetMust("org.mpris.MediaPlayer2.Player", "Metadata", metadataMap(np))
}

// SetVolume publishes the current volume (0..1) to MPRIS clients. The app
// guards against echo loops by ignoring no-op volume applications.
func (ins *Instance) SetVolume(v float64) {
	ins.props.SetMust("org.mpris.MediaPlayer2.Player", "Volume", v)
}

// Close releases the D-Bus connection.
func (ins *Instance) Close() {
	if ins.conn != nil {
		_ = ins.conn.Close()
	}
}

func metadataMap(np metadata.NowPlaying) map[string]dbus.Variant {
	m := map[string]dbus.Variant{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/CurrentTrack")),
		"xesam:title":   dbus.MakeVariant(np.Title),
		"xesam:artist":  dbus.MakeVariant([]string{np.Artist}),
	}
	if np.Album != "" {
		m["xesam:album"] = dbus.MakeVariant(np.Album)
	}
	if np.Artist != "" {
		m["xesam:albumArtist"] = dbus.MakeVariant([]string{np.Artist})
	}
	if np.CoverURL != "" {
		m["mpris:artUrl"] = dbus.MakeVariant(np.CoverURL)
	}
	if np.Link != "" {
		m["xesam:url"] = dbus.MakeVariant(np.Link)
	}
	if !np.Start.IsZero() {
		m["xesam:contentCreated"] = dbus.MakeVariant(np.Start.Format("2006-01-02T15:04:05Z07:00"))
	}
	if !np.Start.IsZero() && !np.End.IsZero() {
		// mpris:length in microseconds.
		m["mpris:length"] = dbus.MakeVariant(np.End.Sub(np.Start).Microseconds())
	}
	return m
}

// --- MediaPlayer2 object ---

type mediaPlayer2 struct {
	ins *Instance
}

func (m *mediaPlayer2) rootProps() map[string]*prop.Prop {
	return map[string]*prop.Prop{
		"CanQuit":             ro(false),
		"CanRaise":            ro(false),
		"HasTrackList":        ro(false),
		"Identity":            ro("fipindicateur"),
		"SupportedUriSchemes": ro([]string{}),
		"SupportedMimeTypes":  ro([]string{}),
	}
}

func (m *mediaPlayer2) playerProps() map[string]*prop.Prop {
	return map[string]*prop.Prop{
		"PlaybackStatus": ro("Playing"),
		"Rate":           roCB(1.0, notImplemented),
		"Metadata":       ro(map[string]dbus.Variant{}),
		"Volume":         roCB(1.0, m.onVolumeChange),
		"Position":       ro(int64(0)),
		"MinimumRate":    ro(1.0),
		"MaximumRate":    ro(1.0),
		"CanGoNext":      ro(false),
		"CanGoPrevious":  ro(false),
		"CanPlay":        ro(true),
		"CanPause":       ro(true),
		"CanSeek":        ro(false),
		"CanControl":     ro(true),
	}
}

func (m *mediaPlayer2) Play() *dbus.Error {
	m.ins.ctrl.Play()
	return nil
}

func (m *mediaPlayer2) Pause() *dbus.Error {
	m.ins.ctrl.Pause()
	return nil
}

func (m *mediaPlayer2) Stop() *dbus.Error {
	m.ins.ctrl.Pause()
	return nil
}

func (m *mediaPlayer2) PlayPause() *dbus.Error {
	m.ins.ctrl.Toggle()
	return nil
}

// onVolumeChange handles an external write to the Volume property
// (e.g. playerctl volume 0.5) and forwards it to the app.
func (m *mediaPlayer2) onVolumeChange(c *prop.Change) *dbus.Error {
	v, ok := c.Value.(float64)
	if !ok {
		return dbus.MakeFailedError(errors.New("volume must be a double"))
	}
	m.ins.ctrl.SetVolumeFrac(v)
	return nil
}

// Root interface no-ops (CanQuit/CanRaise are false).
func (m *mediaPlayer2) Quit() *dbus.Error  { return nil }
func (m *mediaPlayer2) Raise() *dbus.Error { return nil }

func ro(v interface{}) *prop.Prop {
	return &prop.Prop{Value: v, Writable: false, Emit: prop.EmitTrue}
}

func roCB(v interface{}, cb func(*prop.Change) *dbus.Error) *prop.Prop {
	return &prop.Prop{Value: v, Writable: true, Emit: prop.EmitTrue, Callback: cb}
}

func notImplemented(*prop.Change) *dbus.Error {
	return dbus.MakeFailedError(errors.New("not implemented"))
}
