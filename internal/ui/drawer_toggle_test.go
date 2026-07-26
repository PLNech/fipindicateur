package ui

import "testing"

// TestDecideDrawerToggle pins the tray Activate decision table: hidden opens,
// visible hides, and the opening window ignores clicks (a Hide queued while
// the first Show is still initializing could outrun the pending present on
// the GTK thread and desync flag and window).
func TestDecideDrawerToggle(t *testing.T) {
	cases := []struct {
		name  string
		phase drawerPhase
		want  drawerToggleAction
	}{
		{"hidden opens", drawerHidden, drawerActOpen},
		{"opening ignores the impatient second click", drawerOpening, drawerActIgnore},
		{"visible hides", drawerVisible, drawerActHide},
	}
	for _, c := range cases {
		if got := decideDrawerToggle(c.phase); got != c.want {
			t.Errorf("%s: decideDrawerToggle(%d) = %d, want %d", c.name, c.phase, got, c.want)
		}
	}
}

// TestDrawerToggleSequence walks the double-click and impatient-click
// scenarios through the phase machine as toggleDrawer drives it.
func TestDrawerToggleSequence(t *testing.T) {
	phase := drawerHidden

	// First click: open, phase moves to opening.
	if decideDrawerToggle(phase) != drawerActOpen {
		t.Fatal("first click should open")
	}
	phase = drawerOpening

	// Impatient second click while Show initializes: strictly a no-op.
	if got := decideDrawerToggle(phase); got != drawerActIgnore {
		t.Fatalf("second click during opening = %d, want ignore", got)
	}

	// Show returned: present queued, phase settles to visible.
	phase = drawerVisible
	if decideDrawerToggle(phase) != drawerActHide {
		t.Fatal("click on a visible panel should hide")
	}

	// onDrawerHidden fired (Escape/close/toggle): back to square one.
	phase = drawerHidden
	if decideDrawerToggle(phase) != drawerActOpen {
		t.Fatal("after hide, a click should open again")
	}
}
