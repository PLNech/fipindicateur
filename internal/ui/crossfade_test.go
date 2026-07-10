package ui

import "testing"

// TestCrossfadeTitle covers the "Fondu enchaîné" menu-item title: value display,
// the "(désactivé)" special case at 0, and the slider-mode ellipsis hint. A
// hand-edited config value (e.g. 5) that matches no preset still shows in title.
func TestCrossfadeTitle(t *testing.T) {
	cases := []struct {
		secs   int
		slider bool
		want   string
	}{
		{0, false, "Fondu enchaîné (désactivé)"},
		{0, true, "Fondu enchaîné (désactivé)…"},
		{4, false, "Fondu enchaîné (4 s)"},
		{4, true, "Fondu enchaîné (4 s)…"},
		{5, false, "Fondu enchaîné (5 s)"}, // non-preset value still shown
		{10, true, "Fondu enchaîné (10 s)…"},
	}
	for _, c := range cases {
		if got := crossfadeTitle(c.secs, c.slider); got != c.want {
			t.Errorf("crossfadeTitle(%d, %v) = %q, want %q", c.secs, c.slider, got, c.want)
		}
	}
}

// TestCrossfadePresetLabel covers the fallback-submenu checkbox labels.
func TestCrossfadePresetLabel(t *testing.T) {
	cases := map[int]string{0: "Désactivé", 2: "2 s", 4: "4 s", 6: "6 s"}
	for secs, want := range cases {
		if got := crossfadePresetLabel(secs); got != want {
			t.Errorf("crossfadePresetLabel(%d) = %q, want %q", secs, got, want)
		}
	}
}
