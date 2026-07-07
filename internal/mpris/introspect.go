//go:build linux

package mpris

import (
	introspect "github.com/godbus/dbus/v5/introspect"
)

// introspectNode returns the MPRIS2 introspection tree.
// Derived from fip-player (WTFPL), renamed node to "fipindicateur".
func introspectNode() *introspect.Node {
	return &introspect.Node{
		Name: "fipindicateur",
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name: "org.mpris.MediaPlayer2",
				Properties: []introspect.Property{
					{Name: "CanQuit", Type: "b", Access: "read"},
					{Name: "CanRaise", Type: "b", Access: "read"},
					{Name: "HasTrackList", Type: "b", Access: "read"},
					{Name: "Identity", Type: "s", Access: "read"},
					{Name: "SupportedUriSchemes", Type: "as", Access: "read"},
					{Name: "SupportedMimeTypes", Type: "as", Access: "read"},
				},
				Methods: []introspect.Method{
					{Name: "Raise"},
					{Name: "Quit"},
				},
			},
			{
				Name: "org.mpris.MediaPlayer2.Player",
				Properties: []introspect.Property{
					{Name: "PlaybackStatus", Type: "s", Access: "read"},
					{Name: "Rate", Type: "d", Access: "readwrite"},
					{Name: "Metadata", Type: "a{sv}", Access: "read"},
					{Name: "Volume", Type: "d", Access: "readwrite"},
					{Name: "Position", Type: "x", Access: "read"},
					{Name: "MinimumRate", Type: "d", Access: "read"},
					{Name: "MaximumRate", Type: "d", Access: "read"},
					{Name: "CanGoNext", Type: "b", Access: "read"},
					{Name: "CanGoPrevious", Type: "b", Access: "read"},
					{Name: "CanPlay", Type: "b", Access: "read"},
					{Name: "CanPause", Type: "b", Access: "read"},
					{Name: "CanSeek", Type: "b", Access: "read"},
					{Name: "CanControl", Type: "b", Access: "read"},
				},
				Methods: []introspect.Method{
					{Name: "Pause"},
					{Name: "PlayPause"},
					{Name: "Stop"},
					{Name: "Play"},
				},
			},
		},
	}
}
