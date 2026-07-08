package ui

import (
	"os"
	"strings"
	"testing"
)

// TestSingleOnClickCallSite enforces the "measurable by design" invariant: the
// only place a menu click is looped is inside App.on, which records the event.
// If this count is not 1, a clickable item was wired with a raw `go a.onClick`
// and bypassed telemetry. Route it through a.on(item, kind, fn) instead.
func TestSingleOnClickCallSite(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	n := strings.Count(string(src), "go a.onClick(")
	if n != 1 {
		t.Fatalf("measurable-by-design: `go a.onClick(` must appear exactly once (inside App.on); found %d. Wire clickable items via a.on(item, kind, fn).", n)
	}
	if !strings.Contains(string(src), "func (a *App) on(mi *systray.MenuItem, kind events.Kind, fn func())") {
		t.Fatal("the App.on chokepoint helper is missing or changed signature")
	}
}

// TestActionKindsWired checks that the discrete action kinds are actually
// referenced in the UI wiring, so a new Kind constant is not left dangling.
// State-dependent kinds (play/pause/volume/mute/station_change) are recorded at
// source; open/lifecycle kinds flow through a.on. All should appear in ui.go.
func TestActionKindsWired(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, kind := range []string{
		"KindAppStart", "KindAppStop", "KindPlay", "KindPause",
		"KindStationChange", "KindAudioDevice", "KindVolume", "KindMute", "KindHiFi",
		"KindNotif", "KindAnim", "KindAutostart", "KindOpenWiki",
		"KindOpenLink", "KindOpenHistory", "KindOpenFip", "KindOpenAbout",
		"KindStatsView", "KindStatsToggle", "KindQuit",
	} {
		if !strings.Contains(s, "events."+kind) {
			t.Errorf("action kind %s is defined but not wired in ui.go", kind)
		}
	}
}
