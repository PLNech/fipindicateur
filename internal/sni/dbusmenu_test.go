//go:build linux

package sni

import "testing"

// freshMenu builds a menu the way New does (no bus, so no signals are
// emitted: the layer's signalling is skipped when conn is nil).
func freshMenu() *Menu { return newMenu(nil) }

// TestMenuTree pins the tree shape the shell renders: a root marked as a
// submenu holding labelled entries, nested submenus and separators. The GNOME
// extension only reacts to a single left click when the menu has entries, and
// a menu whose visible entries are all separators never opens (field-tested
// 2026-07-28), so a real, labelled entry must survive every refactor.
func TestMenuTree(t *testing.T) {
	m := freshMenu()
	brand := m.Add("PROTO A · SNI maison")
	brand.Disable()
	m.AddSeparator()
	radios := m.Add("Radios")
	radios.AddCheckbox("FIP", true, true)
	radios.AddCheckbox("FIP Jazz", false, true)
	radios.AddSeparator()
	radios.Add("FIP sur radiofrance.fr")

	root := m.itemLocked(m.root, -1)
	if root.ID != 0 {
		t.Fatalf("root id = %d, want 0", root.ID)
	}
	if got := root.Props["children-display"].Value(); got != "submenu" {
		t.Errorf("root children-display = %v, want submenu", got)
	}
	if len(root.Children) != 3 {
		t.Fatalf("root has %d children, want 3 (brand, separator, Radios)", len(root.Children))
	}
	brandItem, ok := root.Children[0].Value().(menuLayoutItem)
	if !ok {
		t.Fatalf("root child 0 is %T, want menuLayoutItem", root.Children[0].Value())
	}
	if label, _ := brandItem.Props["label"].Value().(string); label != "PROTO A · SNI maison" {
		t.Errorf("brand label = %q, want the proto branding", label)
	}
	if got := brandItem.Props["enabled"].Value(); got != false {
		t.Errorf("brand enabled = %v, want false (display-only header)", got)
	}
	if typ, ok := root.Children[1].Value().(menuLayoutItem); !ok || typ.Props["type"].Value() != "separator" {
		t.Errorf("root child 1 is not a separator")
	}
	radiosItem, ok := root.Children[2].Value().(menuLayoutItem)
	if !ok {
		t.Fatalf("root child 2 is %T, want menuLayoutItem", root.Children[2].Value())
	}
	if got := radiosItem.Props["children-display"].Value(); got != "submenu" {
		t.Errorf("Radios children-display = %v, want submenu", got)
	}
	if len(radiosItem.Children) != 4 {
		t.Fatalf("Radios has %d children, want 4", len(radiosItem.Children))
	}
	fipItem, _ := radiosItem.Children[0].Value().(menuLayoutItem)
	if got := fipItem.Props["toggle-type"].Value(); got != "radio" {
		t.Errorf("FIP toggle-type = %v, want radio", got)
	}
	if got := fipItem.Props["toggle-state"].Value(); got != int32(1) {
		t.Errorf("FIP toggle-state = %v, want 1", got)
	}
	jazzItem, _ := radiosItem.Children[1].Value().(menuLayoutItem)
	if got := jazzItem.Props["toggle-state"].Value(); got != int32(0) {
		t.Errorf("FIP Jazz toggle-state = %v, want 0", got)
	}
	linkItem, _ := radiosItem.Children[3].Value().(menuLayoutItem)
	if _, has := linkItem.Props["toggle-type"]; has {
		t.Error("a plain entry carries a toggle-type; only checkboxes may")
	}
}

// TestMenuRecursionDepth pins GetLayout's depth semantics: -1 the whole
// subtree, 0 the entry alone, n the entry plus n-1 levels below it.
func TestMenuRecursionDepth(t *testing.T) {
	m := freshMenu()
	sub := m.Add("Radios")
	sub.Add("FIP")
	sub.Add("FIP Jazz")

	if got := len(m.itemLocked(m.root, -1).Children); got != 1 {
		t.Errorf("depth -1: root exposes %d children, want 1", got)
	}
	if got := len(m.itemLocked(m.root, 0).Children); got != 0 {
		t.Errorf("depth 0: root exposes %d children, want 0 (entry alone)", got)
	}
	subItem := m.itemLocked(sub, 1)
	if len(subItem.Children) != 2 {
		t.Errorf("depth 1 on submenu: %d children, want 2", len(subItem.Children))
	}
	for _, c := range subItem.Children {
		leaf, _ := c.Value().(menuLayoutItem)
		if len(leaf.Children) != 0 {
			t.Errorf("depth 1 leaked a grandchild (id %d)", leaf.ID)
		}
	}
}

// TestMenuHiddenAndDisabled pins the two soft states the pre-allocated slots
// rely on: Hide keeps the entry in the tree but invisible; Disable keeps it
// visible but dead.
func TestMenuHiddenAndDisabled(t *testing.T) {
	m := freshMenu()
	e := m.Add("FIP")
	if got := e.propsLocked()["visible"].Value(); got != true {
		t.Errorf("visible = %v, want true by default", got)
	}
	e.Hide()
	if got := m.itemLocked(m.root, -1).Children[0].Value().(menuLayoutItem).Props["visible"].Value(); got != false {
		t.Errorf("visible after Hide = %v, want false", got)
	}
	e.Show()
	e.Disable()
	if got := m.itemLocked(m.root, -1).Children[0].Value().(menuLayoutItem).Props["enabled"].Value(); got != false {
		t.Errorf("enabled after Disable = %v, want false", got)
	}
}

// TestMenuCheckMutators pins the toggle-state mutators, including the no-op
// on plain (toggle-less) entries.
func TestMenuCheckMutators(t *testing.T) {
	m := freshMenu()
	plain := m.Add("FIP sur radiofrance.fr")
	plain.Check() // must be a no-op: plain entries never carry a mark
	if _, has := plain.propsLocked()["toggle-state"]; has {
		t.Error("plain entry gained a toggle-state after Check()")
	}
	box := m.AddCheckbox("Muet", false, false)
	box.Check()
	if got := box.propsLocked()["toggle-state"].Value(); got != int32(1) {
		t.Errorf("toggle-state after Check = %v, want 1", got)
	}
	if got := box.propsLocked()["toggle-type"].Value(); got != "checkmark" {
		t.Errorf("toggle-type = %v, want checkmark", got)
	}
	box.Uncheck()
	if got := box.propsLocked()["toggle-state"].Value(); got != int32(0) {
		t.Errorf("toggle-state after Uncheck = %v, want 0", got)
	}
}

// TestMenuIconData pins the inline-icon property (icon-data, PNG bytes).
func TestMenuIconData(t *testing.T) {
	m := freshMenu()
	e := m.Add("PROTO A · SNI maison")
	if _, has := e.propsLocked()["icon-data"]; has {
		t.Fatal("fresh entry carries icon-data")
	}
	e.SetIconData([]byte{0x89, 'P'})
	if got := e.propsLocked()["icon-data"].Value().([]byte); string(got) != "\x89P" {
		t.Errorf("icon-data = %v, want the PNG bytes back", got)
	}
}

// TestMenuClick pins the activation gate: Event(id, "clicked") on a visible,
// enabled, labelled entry posts to ClickedCh; disabled, hidden, separator and
// unknown entries, and any other event id, post nothing.
func TestMenuClick(t *testing.T) {
	m := freshMenu()
	e := m.Add("⏸ Pause")
	sep := m.AddSeparator()
	hidden := m.Add("Historique")
	hidden.Hide()

	m.click(e.id, "clicked")
	select {
	case <-e.ClickedCh:
	default:
		t.Fatal("click on an enabled entry did not post")
	}

	e.Disable()
	m.click(e.id, "clicked")
	if len(e.ClickedCh) != 0 {
		t.Error("click on a disabled entry posted")
	}
	e.Enable()

	m.click(hidden.id, "clicked")
	if len(hidden.ClickedCh) != 0 {
		t.Error("click on a hidden entry posted")
	}
	m.click(sep.id, "clicked")
	if len(sep.ClickedCh) != 0 {
		t.Error("click on a separator posted")
	}
	m.click(e.id, "hovered")
	m.click(e.id, "opened")
	m.click(999, "clicked")
	if len(e.ClickedCh) != 0 {
		t.Error("noise events posted")
	}
}

// TestMenuClickDrop pins the ClickedCh bound: a consumer that never drains
// must not wedge the D-Bus handler (the drop, not the block).
func TestMenuClickDrop(t *testing.T) {
	m := freshMenu()
	e := m.Add("Quitter")
	for i := 0; i < 20; i++ { // buffer is 8
		m.click(e.id, "clicked")
	}
	if len(e.ClickedCh) != 8 {
		t.Errorf("ClickedCh len = %d, want 8 (bounded, dropped)", len(e.ClickedCh))
	}
}

// TestMenuRevision pins the revision counter: every published mutation bumps
// it (LayoutUpdated carries it, and clients refetch when it moves), including
// for nested entries.
func TestMenuRevision(t *testing.T) {
	m := freshMenu()
	r0 := m.revision
	m.Add("FIP")
	if m.revision <= r0 {
		t.Error("Add did not bump the revision")
	}
	sub := m.Add("Radios")
	r1 := m.revision
	sub.Add("FIP Jazz")
	if m.revision <= r1 {
		t.Error("a nested Add did not bump the revision")
	}
}

// TestMenuRedundantMutators pins the no-change guards: setting a value the
// entry already holds leaves the revision alone (every bump makes the shell
// refetch the whole layout, cover PNGs included), a real change bumps once.
func TestMenuRedundantMutators(t *testing.T) {
	m := freshMenu()
	e := m.Add("Pause")
	e.SetIconData([]byte{1, 2, 3})
	cases := []struct {
		name         string
		same, change func()
	}{
		{"SetLabel", func() { e.SetLabel("Pause") }, func() { e.SetLabel("Lecture") }},
		{"Disable", func() { e.Enable() }, func() { e.Disable() }},
		{"Hide", func() { e.Show() }, func() { e.Hide() }},
		{"SetIconData", func() { e.SetIconData([]byte{1, 2, 3}) }, func() { e.SetIconData([]byte{4}) }},
	}
	for _, c := range cases {
		r := m.revision
		c.same()
		if m.revision != r {
			t.Errorf("%s: redundant call bumped the revision", c.name)
		}
		c.change()
		if m.revision != r+1 {
			t.Errorf("%s: real change moved the revision %d -> %d, want one bump", c.name, r, m.revision)
		}
	}
}

// TestMenuByUnknownID pins the error answers for ids outside the tree.
func TestMenuByUnknownID(t *testing.T) {
	m := freshMenu()
	if _, ok := m.byID[42]; ok {
		t.Fatal("unknown id found in the tree")
	}
	// The root is always present, id 0.
	if _, ok := m.byID[0]; !ok {
		t.Fatal("root id 0 missing")
	}
}
