//go:build !linux

// Package mpris is a no-op on non-Linux platforms (MPRIS is a freedesktop
// D-Bus spec). The API mirrors the Linux build so callers stay portable.
package mpris

import "github.com/PLNech/fipindicateur/internal/metadata"

// Controller receives playback commands (unused off Linux).
type Controller interface {
	Play()
	Pause()
	Toggle()
}

// Instance is a no-op MPRIS instance.
type Instance struct{}

// Connect is a no-op that returns a stub instance.
func Connect(Controller) (*Instance, error) { return &Instance{}, nil }

// SetPlaybackStatus is a no-op.
func (*Instance) SetPlaybackStatus(bool) {}

// UpdateMetadata is a no-op.
func (*Instance) UpdateMetadata(metadata.NowPlaying) {}

// Close is a no-op.
func (*Instance) Close() {}
