//go:build !linux

// Package mpris is a no-op on non-Linux platforms (MPRIS is a freedesktop
// D-Bus spec). The API mirrors the Linux build so callers stay portable.
package mpris

import (
	"errors"

	"github.com/PLNech/fipindicateur/internal/metadata"
)

// ErrAlreadyRunning mirrors the Linux build (never returned off Linux).
var ErrAlreadyRunning = errors.New("mpris: name already taken (another instance is running)")

// Controller receives playback commands (unused off Linux).
type Controller interface {
	Play()
	Pause()
	Toggle()
	SetVolumeFrac(v float64)
}

// Instance is a no-op MPRIS instance.
type Instance struct{}

// Connect is a no-op that returns a stub instance.
func Connect(Controller) (*Instance, error) { return &Instance{}, nil }

// SetPlaybackStatus is a no-op.
func (*Instance) SetPlaybackStatus(bool) {}

// UpdateMetadata is a no-op.
func (*Instance) UpdateMetadata(metadata.NowPlaying) {}

// SetVolume is a no-op.
func (*Instance) SetVolume(float64) {}

// Close is a no-op.
func (*Instance) Close() {}
