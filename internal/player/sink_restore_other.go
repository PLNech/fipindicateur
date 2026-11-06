//go:build !linux

package player

// SinkRestoresVolume is false where the audio server does not remember the
// per-app level. CoreAudio (macOS) and WASAPI (Windows) reopen a stream at
// the driver default, which is 100. The app writes its saved level back on
// playback restart; see ui.reapplySavedVolume.
func SinkRestoresVolume() bool { return false }
