//go:build darwin

package icon

import "testing"

// TestHaloFollowsPanel pins the owner's decision (M4 milestone): on darwin
// the white rim is drawn exactly when the panel is dark, so the decision
// boundary IS the panel state. This is the unit proof that the dark-mode
// path now triggers: before the fix the rim was build-time false here.
func TestHaloFollowsPanel(t *testing.T) {
	for _, dark := range []bool{false, true} {
		if got := haloOnPanel(dark); got != dark {
			t.Errorf("haloOnPanel(%v) = %v, want %v", dark, got, dark)
		}
	}
}

// TestDarkPanelProbeAnswers runs the appearance probe the way the app does:
// it must answer with a bool without an NSApplication (a bare test host has
// none), falling back to the legacy defaults read when no appearance
// resolves. The value is the host's live resolution, printed for the smoke
// report rather than asserted: a CI host's appearance is whatever it is.
func TestDarkPanelProbeAnswers(t *testing.T) {
	d := darkPanel()
	t.Logf("darkPanel() = %v (host appearance resolution)", d)

	// The cached view must track a re-probe of the same state: no flip
	// happened under the test, so RefreshPanelDark reports false and the
	// cached answer stays the freshly probed one.
	first := panelIsDark()
	if RefreshPanelDark() {
		t.Error("RefreshPanelDark reported a flip with no appearance change")
	}
	if got := panelIsDark(); got != first {
		t.Errorf("cached panel state drifted: %v then %v", first, got)
	}
	if got := darkPanel(); got != first {
		t.Errorf("cached %v vs fresh probe %v (appearance flipped mid-test?)", first, got)
	}
}

// TestPanelDarkResolvesDrawingAppearance is the concrete demonstration that
// the dark-mode path now triggers: inside the DarkAqua drawing appearance
// the probe answers dark, inside Aqua it answers light, on a host whose
// system appearance never changes. The old probe (a `defaults read -g
// AppleInterfaceStyle` grep) could not even be exercised this way: it read a
// global preference, not the drawing context, and under the Auto appearance
// that key is absent, so it answered light forever and the glyph vanished on
// the bar that goes dark every evening.
func TestPanelDarkResolvesDrawingAppearance(t *testing.T) {
	if got := probeWithinAppearance(false); got != 0 {
		t.Errorf("inside the Aqua drawing appearance the probe answered %d, want 0 (light)", got)
	}
	if got := probeWithinAppearance(true); got != 1 {
		t.Errorf("inside the DarkAqua drawing appearance the probe answered %d, want 1 (dark)", got)
	}
}
