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

// swiftActionIDs reads the raw values of the Swift `enum Action` that the
// popover's controls fire (the source, not the binary: the default build
// never compiles Swift). A case without an explicit raw value has its own
// name as raw value, the Swift rule for String enums.
func swiftActionIDs(t *testing.T, src string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?s)enum Action: String[^{]*\{(.*?)\n\}`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("Bridge.swift: `enum Action: String { ... }` not found; the guard reads the action ids from it")
	}
	one := regexp.MustCompile(`^(\w+)(?:\s*=\s*"([^"]*)")?$`)
	var ids []string
	for _, line := range strings.Split(m[1], "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		rest, ok := strings.CutPrefix(line, "case ")
		if !ok {
			t.Fatalf("Bridge.swift: enum Action holds something other than plain cases (%q); keep it a list of string cases so this guard can read it", line)
		}
		for _, c := range strings.Split(rest, ",") {
			p := one.FindStringSubmatch(strings.TrimSpace(c))
			if p == nil {
				t.Fatalf("Bridge.swift: unreadable Action case %q", c)
			}
			if p[2] != "" {
				ids = append(ids, p[2])
			} else {
				ids = append(ids, p[1])
			}
		}
	}
	return ids
}

// TestPopoverActionsRegistered carries the "measurable by design" invariant
// across the language boundary. The SwiftUI popover can only leave Swift
// through FipModel.send(Action), which lands in App.fire(id): every id the
// Swift enum names must therefore be a registered action, declared in
// registerActions with its telemetry decision. A control added in Swift with
// no Go handler fails here, on every platform, without a Swift toolchain.
func TestPopoverActionsRegistered(t *testing.T) {
	const dir = "../popover/fipui/swift"
	bridge, err := os.ReadFile(filepath.Join(dir, "Bridge.swift"))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.registerActions()
	ids := swiftActionIDs(t, string(bridge))
	if len(ids) == 0 {
		t.Fatal("Bridge.swift: enum Action is empty")
	}
	for _, id := range ids {
		if _, ok := a.actions[id]; !ok {
			t.Errorf("popover action %q (Bridge.swift, enum Action) has no handler: register it in registerActions (actions.go) with its events.Kind, or \"\" if it records at source", id)
		}
	}

	// The single exit: goFipAction is declared and called once, both inside
	// FipModel.send. A view calling it directly would bypass the enum.
	files, err := filepath.Glob(filepath.Join(dir, "*.swift"))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "//") { // prose may name it
				calls += strings.Count(line, "goFipAction(")
			}
		}
	}
	if calls != 2 {
		t.Fatalf("goFipAction( must appear exactly twice in the Swift sources (its declaration and the call in FipModel.send); found %d. Controls fire through model.send(Action).", calls)
	}
}

// The registry's own contract: an unknown id runs nothing and says so.
func TestFireUnknownAction(t *testing.T) {
	a := &App{}
	a.registerActions()
	if a.fire("no_such_action", "") {
		t.Fatal("fire reported an unknown action as run")
	}
}
