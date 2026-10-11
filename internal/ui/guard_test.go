package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSingleOnClickCallSite enforces the "measurable by design" invariant: the
// only place a menu click is looped is inside App.on, which records the event.
// If this count is not 1, a clickable item was wired with a raw `go a.onClick`
// and bypassed telemetry. Route it through a.on(item, kind, fn) instead.
//
// App.on lives in ui.go (shared): every platform builds its menu items on a
// click channel (systray's ClickedCh off Linux, the sni dbusmenu's activation
// channel on Linux) and wires them through the same chokepoint. The scan
// covers every non-test file so a raw onClick loop cannot hide elsewhere.
func TestSingleOnClickCallSite(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	var coreSrc string
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		total += strings.Count(string(src), "go a.onClick(")
		if f == "ui.go" {
			coreSrc = string(src)
		}
	}
	if total != 1 {
		t.Fatalf("measurable-by-design: `go a.onClick(` must appear exactly once (inside App.on in ui.go); found %d. Wire clickable items via a.on(item, kind, fn).", total)
	}
	if !strings.Contains(coreSrc, "func (a *App) on(mi *menuItem, kind events.Kind, fn func())") {
		t.Fatal("the App.on chokepoint helper is missing or changed signature")
	}
}

// legacyKinds are Kind constants no UI path emits any more but that must keep
// existing: old events.jsonl files still hold them and internal/stats derives
// listening time from them (cast_pause/cast_resume were emitted by the retired
// panel's transport). They are exempt from the wiring check; anything else
// declared in internal/events must be recorded by the ui package.
var legacyKinds = map[string]bool{
	"KindCastPause":  true,
	"KindCastResume": true,
}

// TestActionKindsWired checks that every action kind declared in
// internal/events is actually referenced in the UI wiring (any non-test file of the package: the shared
// core and both menu skins), so a new Kind
// constant is not left dangling. The list is read from events.go itself (a
// new constant is checked without editing this test). State-dependent kinds
// (play/pause/volume/mute/station_change) are recorded at source; open and
// lifecycle kinds flow through a.on. All should appear in the package, except the
// explicit legacyKinds that only old logs and the stats derivation still use.
func TestActionKindsWired(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(src)
	}
	s := all.String()
	decl, err := os.ReadFile(filepath.Join("..", "events", "events.go"))
	if err != nil {
		t.Fatal(err)
	}
	kinds := regexp.MustCompile(`(?m)^	Kind([A-Za-z]+)\s+Kind = `).FindAllStringSubmatch(string(decl), -1)
	if len(kinds) < 30 {
		t.Fatalf("found only %d Kind constants in events.go: the declaration pattern drifted", len(kinds))
	}
	for _, m := range kinds {
		kind := "Kind" + m[1]
		if legacyKinds[kind] {
			continue
		}
		if !strings.Contains(s, "events."+kind) {
			t.Errorf("action kind %s is defined but not wired in the ui package", kind)
		}
	}
	for kind := range legacyKinds {
		if strings.Contains(s, "events."+kind) {
			t.Errorf("%s is listed as legacy but the ui package records it: drop it from legacyKinds", kind)
		}
	}
}

// TestSingleSetIconCallSite enforces that the tray icon is set through exactly
// one chokepoint (setTrayIcon). App.setIcon refuses empty bytes (which register
// a null pixmap and trip GNOME's cogl "data != NULL" assertion) and dedupes
// redundant pushes, then hands off to setTrayIcon, which is the ONLY place a
// raw systray.SetIcon( is allowed (it adapts the byte format per platform via
// encodeTrayIcon: passthrough on Unix, PNG-in-ICO on Windows). If systray.SetIcon(
// appears more than once across the package, an icon path bypassed the guard:
// route it through a.setIcon / setTrayIcon instead.
func TestSingleSetIconCallSite(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		total += strings.Count(string(src), "systray.SetIcon(")
	}
	if total != 1 {
		t.Fatalf("guarded-icon invariant: `systray.SetIcon(` must appear exactly once (inside setTrayIcon); found %d. Route icon sets through a.setIcon.", total)
	}
}
