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
		{menuItemID, "clicked", true}, // « le FIPindicateur » activated
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
// exactly one visible, enabled, LABELLED entry. The GNOME extension only reacts
// to a single left click when the menu has at least one entry, and the shell
// only opens a menu that has something visible to show, so neither an empty
// layout nor a label-less separator will do: both silently reintroduce issue #18
// (field-tested 2026-07-28, the separator killed the single click).
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
	label, labelled := entry.Props["label"]
	if !labelled {
		t.Fatal("entry has no label: GNOME hides an edge separator and never opens an empty menu, killing the single click")
	}
	if got := label.Value(); got != "le FIPindicateur" {
		t.Errorf("entry label = %v, want « le FIPindicateur »", got)
	}
	for _, p := range []string{"enabled", "visible"} {
		if got := entry.Props[p].Value(); got != true {
			t.Errorf("entry %s = %v, want true", p, got)
		}
	}
}
