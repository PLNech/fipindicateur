package nowplay

// Handler receives remote commands (play/pause/toggle from the OS media
// surface). Nil fields are treated as unsupported for that command: the
// command still answers Success so the system does not fall through to
// another player, but nothing runs.
type Handler struct {
	Play   func()
	Pause  func()
	Toggle func()
	// Next and Prev zap the webradio list: a live stream has no track
	// queue, so the hardware pair is repurposed as station prev/next.
	Next func()
	Prev func()
}

// Info is the track shown in the OS media surface (Control Center and the
// lock screen on macOS).
type Info struct {
	Title  string
	Artist string
	Album  string
}
