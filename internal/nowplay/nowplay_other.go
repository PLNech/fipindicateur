//go:build !darwin

// Package nowplay publishes the current track to the OS media surface so
// hardware media keys and the system Now Playing widget can drive the app.
//
// On Linux that surface is MPRIS (internal/mpris); this package is the macOS
// counterpart (MPNowPlayingInfoCenter + MPRemoteCommandCenter). Everywhere
// else it is a no-op, so the call sites in internal/ui stay unconditional.
package nowplay

// Start registers the process with the OS media surface and routes remote
// commands to the callbacks. A nil callback is treated as unsupported for
// that command. Off macOS this does nothing.
func Start(Handler) {}

// Stop unregisters and clears the published track. Off macOS this does nothing.
func Stop() {}

// SetTrack publishes the current track. An empty title and artist clear it.
// Off macOS this does nothing.
func SetTrack(Info) {}

// SetPlaying publishes whether audio is coming out. Off macOS this does nothing.
func SetPlaying(bool) {}
