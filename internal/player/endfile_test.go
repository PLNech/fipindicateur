package player

import "testing"

// mpv_end_file_reason values (mpv/client.h), pinned here as plain ints so the
// test needs no cgo preamble.
const (
	endFileEOF      = 0
	endFileStop     = 2
	endFileQuit     = 3
	endFileError    = 4
	endFileRedirect = 5
)

// TestEndFileReports pins the END_FILE reason filter: only a stream that died
// under us (eof, error) reports StreamEnded and clears the playing flag. Our
// own dismissals (a replacing loadfile, Stop, teardown) must stay silent, or
// every zap and pause would read as a crash.
func TestEndFileReports(t *testing.T) {
	cases := map[int]bool{
		endFileEOF:      true,  // relay closed the stream
		endFileError:    true,  // network death, 404, decode error
		endFileStop:     false, // our own Stop / loadfile replace
		endFileQuit:     false, // teardown
		endFileRedirect: false, // mpv redirected internally
		99:              false, // unknown future reason: stay calm
	}
	for reason, want := range cases {
		if got := endFileReports(reason); got != want {
			t.Errorf("endFileReports(reason %d) = %v, want %v", reason, got, want)
		}
	}
}
