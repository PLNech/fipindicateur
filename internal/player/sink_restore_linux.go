//go:build linux

package player

// SinkRestoresVolume reports whether the audio server remembers the per-app
// level across an output close and reopen. PulseAudio and PipeWire do
// (module-stream-restore), including a level changed in the mixer while we
// were away. Writing a cached level on playback restart would stomp that.
func SinkRestoresVolume() bool { return true }
