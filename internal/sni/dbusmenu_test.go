//go:build linux

package sni

import "testing"

// TestMenuOpenEvent pins the single-click contract with the GNOME AppIndicator
// extension: the root menu OPENING (Event(0, "opened"), sent exactly when the
// shell popup shows) and the one entry's activation (Event(1, "clicked")) both
// mean "open the panel"; everything else (hover, close, AboutToShow-time
// noise) must not.
func TestMenuOpenEvent(t *testing.T) {
	cases := []struct {
		id      int32
		eventID string
		want    bool
	}{
		{0, "opened", true},           // shell popup opened: the single click
		{menuItemID, "clicked", true}, // the entry activated (a separator cannot be, kept as a belt)
		{0, "closed", false},          // popup dismissed: not an open request
		{0, "clicked", false},         // root is never clicked as an entry
		{menuItemID, "opened", false},
		{menuItemID, "hovered", false},
		{2, "clicked", false}, // unknown id
	}
	for _, c := range cases {
		if got := menuOpenEvent(c.id, c.eventID); got != c.want {
			t.Errorf("menuOpenEvent(%d, %q) = %v, want %v", c.id, c.eventID, got, c.want)
		}
	}
}

// TestMenuLayout checks the static layout: a root marked as submenu holding
// exactly one visible, enabled entry, which is a separator (no label to read:
// the popup the shell insists on showing says nothing rather than duplicating
// the panel). The GNOME extension only reacts to a single left click when the
// menu has at least one entry, so an empty layout would silently reintroduce
// issue #18.
func TestMenuLayout(t *testing.T) {
	root := menuRoot()
	if root.ID != 0 {
		t.Fatalf("root id = %d, want 0", root.ID)
	}
	if got := root.Props["children-display"].Value(); got != "submenu" {
		t.Errorf("root children-display = %v, want submenu", got)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root has %d children, want 1 (an empty menu makes GNOME ignore single clicks)", len(root.Children))
	}
	entry, ok := root.Children[0].Value().(menuLayoutItem)
	if !ok {
		t.Fatalf("root child is %T, want menuLayoutItem", root.Children[0].Value())
	}
	if entry.ID != menuItemID {
		t.Errorf("entry id = %d, want %d", entry.ID, menuItemID)
	}
	if got := entry.Props["type"].Value(); got != "separator" {
		t.Errorf("entry type = %v, want separator (a labelled entry duplicates the panel)", got)
	}
	if _, labelled := entry.Props["label"]; labelled {
		t.Error("entry carries a label: the popup should say nothing, the panel speaks")
	}
	for _, p := range []string{"enabled", "visible"} {
		if got := entry.Props[p].Value(); got != true {
			t.Errorf("entry %s = %v, want true", p, got)
		}
	}
}
