//go:build darwin && swiftui

package popover

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PLNech/fipindicateur/internal/popover/fipui"
)

// AppKit draws on the process main thread only: pin it before the test
// runner starts, and run the tests from another goroutine.
func TestMain(m *testing.M) {
	runtime.LockOSThread()
	done := make(chan int, 1)
	go func() { done <- m.Run() }()
	for {
		select {
		case code := <-done:
			os.Exit(code)
		case fn := <-onMain:
			fn()
		}
	}
}

var onMain = make(chan func())

// TestRenderCard draws the card offscreen in both appearances. It is the way
// to look at a layout change without clicking: set FIPUI_RENDER_DIR to keep
// the PNGs, and FIPUI_RENDER_COVER to a picture for the sleeve.
func TestRenderCard(t *testing.T) {
	dir := os.Getenv("FIPUI_RENDER_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	snap := Snapshot{
		Playing: true, Station: "jazz",
		Stations: []Station{
			{"fip", "FIP", "#e2007a"}, {"rock", "Rock", "#b61f24"}, {"jazz", "Jazz", "#0d5c8c"},
			{"groove", "Groove", "#c8102e"}, {"monde", "Monde", "#2e8b57"}, {"nouveautes", "Nouveautés", "#f0a500"},
			{"reggae", "Reggae", "#3a7d44"}, {"electro", "Electro", "#5b2a86"}, {"metal", "Metal", "#333333"},
		},
		Title: "Mystery of Love", Artist: "Dorothy Ashby", Album: "Afro-Harping", Year: 1968, Label: "Cadet",
		Show: "Club Jazzafip", Cover: os.Getenv("FIPUI_RENDER_COVER"),
		Start: 1, End: 2, Volume: 62, Verdict: 1,
	}
	for name, dark := range map[string]bool{"card-nuit.png": true, "card-papier.png": false} {
		path := filepath.Join(dir, name)
		ok := make(chan bool, 1)
		onMain <- func() { ok <- fipui.DebugRender(path, snap.JSON(), dark) }
		if !<-ok {
			t.Fatalf("no render for %s", name)
		}
		if fi, err := os.Stat(path); err != nil || fi.Size() < 2000 {
			t.Fatalf("%s: empty render (%v)", name, err)
		}
	}
}
