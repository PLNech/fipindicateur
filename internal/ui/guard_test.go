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
// The menu (and thus App.on) lives in ui_menu.go since the Linux UI moved to
// the drawer + SNI; there every user action routes through onDrawerCommand or
// the SNI handlers, which land on the same recording chokepoints. The scan
// covers every non-test file so a raw onClick loop cannot hide elsewhere.
func TestSingleOnClickCallSite(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	var menuSrc string
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		total += strings.Count(string(src), "go a.onClick(")
		if f == "ui_menu.go" {
			menuSrc = string(src)
		}
	}
	if total != 1 {
		t.Fatalf("measurable-by-design: `go a.onClick(` must appear exactly once (inside App.on in ui_menu.go); found %d. Wire clickable items via a.on(item, kind, fn).", total)
	}
	if !strings.Contains(menuSrc, "func (a *App) on(mi *menuItem, kind events.Kind, fn func())") {
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
		"KindLike", "KindDislike", "KindCastStart", "KindCastStop",
		"KindCastPause", "KindCastResume", "KindDrawerOpen",
		"KindStationChange", "KindShowChange", "KindAudioDevice", "KindVolume", "KindMute", "KindHiFi",
		"KindCrossfade",
		"KindNotif", "KindShowNotif", "KindShowCalendar", "KindAnim", "KindAutostart", "KindPlayOnStart", "KindOpenWiki",
		"KindOpenLink", "KindOpenHistory", "KindOpenFip", "KindOpenAbout",
		"KindStatsView", "KindStatsToggle", "KindPrefsClear", "KindRestart", "KindUpdateCheck",
		"KindUpdateStartup", "KindQuit",
	} {
		if !strings.Contains(s, "events."+kind) {
			t.Errorf("action kind %s is defined but not wired in ui.go", kind)
		}
	}
}

// TestDrawerActionsWired is the panel's sibling of TestActionKindsWired: the
// page declares its complete command vocabulary in one greppable ACTIONS
// manifest (page.html; send() refuses anything undeclared), and this test
// asserts, both ways, that the manifest and the Go router agree:
//   - every declared page action has a `case` in onDrawerCommand;
//   - every router case is declared by the page (js_error, the page's error
//     trap, is the one router-only route: it is posted directly, not via
//     send, and exists precisely to catch render bugs);
//   - every `action: '...'` literal in the page is declared in the manifest
//     or is drawer-internal plumbing.
//
// A button added without a handler, or a handler without a button, fails
// here; the live twin is `fipindicateur drawer --selftest`, which clicks
// every control in the real webkit.
func TestDrawerActionsWired(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "drawer", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(page)

	// The declared vocabulary: the ACTIONS manifest.
	m := regexp.MustCompile(`(?s)const ACTIONS = \[(.*?)\];`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("page.html: manifest `const ACTIONS = [...]` not found")
	}
	quoted := regexp.MustCompile(`'([a-z_]+)'`)
	declared := map[string]bool{}
	for _, g := range quoted.FindAllStringSubmatch(m[1], -1) {
		declared[g[1]] = true
	}
	if len(declared) == 0 {
		t.Fatal("page.html: empty ACTIONS manifest")
	}
	im := regexp.MustCompile(`(?s)const INTERNAL_ACTIONS = \[(.*?)\];`).FindStringSubmatch(src)
	if im == nil {
		t.Fatal("page.html: manifest `const INTERNAL_ACTIONS = [...]` not found")
	}
	internal := map[string]bool{}
	for _, g := range quoted.FindAllStringSubmatch(im[1], -1) {
		internal[g[1]] = true
	}

	// Every action literal used anywhere in the page must be declared.
	for _, g := range regexp.MustCompile(`action: '([a-z_]+)'`).FindAllStringSubmatch(src, -1) {
		if !declared[g[1]] && !internal[g[1]] {
			t.Errorf("page.html posts undeclared action %q (add it to ACTIONS)", g[1])
		}
	}
	for _, g := range regexp.MustCompile(`armThenSend\('[^']+', '([a-z_]+)'`).FindAllStringSubmatch(src, -1) {
		if !declared[g[1]] {
			t.Errorf("page.html arms undeclared action %q (add it to ACTIONS)", g[1])
		}
	}

	// The Go router's cases, extracted from onDrawerCommand only.
	uisrc, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?s)func \(a \*App\) onDrawerCommand\(.*?\n\}\n`).FindString(string(uisrc))
	if fn == "" {
		t.Fatal("ui.go: onDrawerCommand not found")
	}
	routed := map[string]bool{}
	for _, g := range regexp.MustCompile(`case "([a-z_]+)"`).FindAllStringSubmatch(fn, -1) {
		routed[g[1]] = true
	}

	for a := range declared {
		if !routed[a] {
			t.Errorf("page action %q has no case in onDrawerCommand (dead button)", a)
		}
	}
	for a := range routed {
		if !declared[a] && a != "js_error" {
			t.Errorf("onDrawerCommand case %q is not declared by the page (unreachable handler)", a)
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
