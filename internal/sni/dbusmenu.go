//go:build linux

package sni

// A dynamic com.canonical.dbusmenu server: the whole tray UI lives in this
// tree (sections, separators, checkmarks, radio marks, disabled and hidden
// entries), and every mutation bumps the revision and signals LayoutUpdated
// so the shell refetches. The old one-entry hack (« Ouvrir le panneau »,
// whose opening was the GNOME single-click signal) is gone: the menu IS the
// UI now, and a single left click opens it because the GNOME AppIndicator
// extension toggles the shell popup for any item whose menu has entries.
//
// GNOME quirks that shaped this layer (field-tested, see the sni.go header):
//   - the extension delivers Event(id, "clicked") for an activated entry;
//   - AboutToShow(0) is called once at attach time, so it must NOT be treated
//     as a click (it would fire at registration);
//   - a menu whose visible entries are all separators never opens, so the
//     tree always carries real, labelled entries.
//
// A GNOME double click still lands on the item's Activate method (the
// extension cancels the pending menu-open), which the app wires as its
// primary action.

import (
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	menuPath  = "/StatusNotifierMenu" // the Menu property points here
	menuIface = "com.canonical.dbusmenu"
)

// layoutEmitDelay coalesces LayoutUpdated signals: building a menu fires one
// add per entry, and a signal per add would make an attached shell refetch
// the whole layout once per entry. The delay is well under human perception
// and only groups mutations, never delays a click.
const layoutEmitDelay = 30 * time.Millisecond

// Menu is a live com.canonical.dbusmenu tree. Entries are added by the app
// after the item is registered; every mutation is published via LayoutUpdated.
// All methods are safe from any goroutine.
type Menu struct {
	conn *dbus.Conn // nil in tests: signals are skipped

	mu       sync.Mutex
	root     *Entry
	byID     map[int32]*Entry
	nextID   int32
	revision uint32
	emitPend bool // a coalesced LayoutUpdated signal is scheduled
}

// Entry is one node of the Menu tree. The zero Entry is not usable: build the
// tree through Add/AddSeparator/AddCheckbox. ClickedCh receives one value per
// activation the host reports (Event(id, "clicked")); it is buffered, so a
// slow consumer drops rather than blocking the D-Bus method handler.
type Entry struct {
	ClickedCh chan struct{}

	menu     *Menu
	id       int32
	sep      bool // separator: no label, never clickable
	label    string
	enabled  bool
	visible  bool
	toggle   bool // carries a toggle-state (checkbox or radio)
	checked  bool
	radio    bool // mutually-exclusive mark instead of a plain checkmark
	iconData []byte
	children []*Entry
}

// newMenu builds an empty menu (a root with no children). conn is where
// LayoutUpdated signals are emitted; nil skips signalling (tests).
func newMenu(conn *dbus.Conn) *Menu {
	root := &Entry{id: 0, enabled: true, visible: true}
	return &Menu{
		conn:     conn,
		root:     root,
		byID:     map[int32]*Entry{0: root},
		nextID:   1,
		revision: 1,
	}
}

// Add appends a top-level labelled entry.
func (m *Menu) Add(label string) *Entry {
	return m.append(m.root, &Entry{label: label, enabled: true, visible: true})
}

// AddSeparator appends a top-level separator.
func (m *Menu) AddSeparator() *Entry {
	return m.append(m.root, &Entry{sep: true, enabled: true, visible: true})
}

// AddCheckbox appends a top-level entry carrying a toggle-state (see
// Entry.AddCheckbox).
func (m *Menu) AddCheckbox(label string, checked, radio bool) *Entry {
	return m.append(m.root, &Entry{label: label, enabled: true, visible: true, toggle: true, checked: checked, radio: radio})
}

// Add appends a labelled child entry.
func (e *Entry) Add(label string) *Entry {
	return e.menu.append(e, &Entry{label: label, enabled: true, visible: true})
}

// AddCheckbox appends a child entry carrying a toggle-state: a checkmark, or
// a radio mark when radio (the mutually-exclusive rendering for station,
// volume-preset and device groups).
func (e *Entry) AddCheckbox(label string, checked, radio bool) *Entry {
	return e.menu.append(e, &Entry{label: label, enabled: true, visible: true, toggle: true, checked: checked, radio: radio})
}

// AddSeparator appends a child separator.
func (e *Entry) AddSeparator() *Entry {
	return e.menu.append(e, &Entry{sep: true, enabled: true, visible: true})
}

// append attaches a prepared entry under parent, publishing the new layout.
func (m *Menu) append(parent, e *Entry) *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.menu = m
	e.id = m.nextID
	m.nextID++
	e.ClickedCh = make(chan struct{}, 8)
	m.byID[e.id] = e
	parent.children = append(parent.children, e)
	m.changedLocked()
	return e
}

// --- entry mutators (each publishes the new layout) ---

// SetLabel replaces the entry's text.
func (e *Entry) SetLabel(label string) {
	e.menu.mu.Lock()
	defer e.menu.mu.Unlock()
	e.label = label
	e.menu.changedLocked()
}

// Enable makes the entry activatable.
func (e *Entry) Enable() { e.setEnabled(true) }

// Disable greys the entry out (hosts then refuse to activate it).
func (e *Entry) Disable() { e.setEnabled(false) }

func (e *Entry) setEnabled(on bool) {
	e.menu.mu.Lock()
	defer e.menu.mu.Unlock()
	e.enabled = on
	e.menu.changedLocked()
}

// Check sets the toggle-state on (a no-op publishing nothing on entries
// without a toggle, which never show a mark anyway).
func (e *Entry) Check() { e.setChecked(true) }

// Uncheck clears the toggle-state.
func (e *Entry) Uncheck() { e.setChecked(false) }

func (e *Entry) setChecked(on bool) {
	e.menu.mu.Lock()
	defer e.menu.mu.Unlock()
	if !e.toggle || e.checked == on {
		return
	}
	e.checked = on
	e.menu.changedLocked()
}

// Show marks the entry visible to the shell.
func (e *Entry) Show() { e.setVisible(true) }

// Hide removes the entry from the rendered menu without unlinking it, so it
// can come back (pre-allocated slots).
func (e *Entry) Hide() { e.setVisible(false) }

func (e *Entry) setVisible(on bool) {
	e.menu.mu.Lock()
	defer e.menu.mu.Unlock()
	e.visible = on
	e.menu.changedLocked()
}

// SetIconData gives the entry an inline icon (PNG bytes, the dbusmenu
// "icon-data" property). Nil clears it.
func (e *Entry) SetIconData(png []byte) {
	e.menu.mu.Lock()
	defer e.menu.mu.Unlock()
	e.iconData = png
	e.menu.changedLocked()
}

// changedLocked records a layout mutation and schedules one coalesced
// LayoutUpdated signal (see layoutEmitDelay). Caller holds m.mu.
func (m *Menu) changedLocked() {
	m.revision++
	if m.emitPend || m.conn == nil {
		return
	}
	m.emitPend = true
	time.AfterFunc(layoutEmitDelay, m.emitLayoutUpdated)
}

// emitLayoutUpdated publishes the current revision. Runs on a timer goroutine.
func (m *Menu) emitLayoutUpdated() {
	m.mu.Lock()
	rev := m.revision
	m.emitPend = false
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return
	}
	if err := conn.Emit(menuPath, menuIface+".LayoutUpdated", rev, int32(0)); err != nil {
		// Best-effort like every signal here: a lost update means a stale
		// menu until the next mutation, never a broken app.
		return
	}
}

// --- layout serialization (wire format ia{sv}av) ---

// menuLayoutItem is the dbusmenu layout node wire format.
type menuLayoutItem struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// menuItemProperties is one (id, properties) pair of GetGroupProperties'
// a(ia{sv}) reply.
type menuItemProperties struct {
	ID    int32
	Props map[string]dbus.Variant
}

// menuEvent is one entry of EventGroup's a(isvu) argument.
type menuEvent struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}

// propsLocked renders an entry's property set (caller holds m.mu). The root
// carries children-display only, exactly like the one-entry menu did.
func (e *Entry) propsLocked() map[string]dbus.Variant {
	if e.id == 0 {
		return map[string]dbus.Variant{
			"children-display": dbus.MakeVariant("submenu"),
		}
	}
	p := map[string]dbus.Variant{
		"label":   dbus.MakeVariant(e.label),
		"enabled": dbus.MakeVariant(e.enabled),
		"visible": dbus.MakeVariant(e.visible),
	}
	if e.sep {
		p["type"] = dbus.MakeVariant("separator")
	}
	if len(e.children) > 0 {
		p["children-display"] = dbus.MakeVariant("submenu")
	}
	if e.toggle {
		if e.radio {
			p["toggle-type"] = dbus.MakeVariant("radio")
		} else {
			p["toggle-type"] = dbus.MakeVariant("checkmark")
		}
		if e.checked {
			p["toggle-state"] = dbus.MakeVariant(int32(1))
		} else {
			p["toggle-state"] = dbus.MakeVariant(int32(0))
		}
	}
	if len(e.iconData) > 0 {
		p["icon-data"] = dbus.MakeVariant(e.iconData)
	}
	return p
}

// itemLocked serializes an entry and its subtree (caller holds m.mu). depth
// follows GetLayout's recursionDepth: -1 everything, 0 the entry alone, n the
// entry plus n-1 levels of children.
func (m *Menu) itemLocked(e *Entry, depth int32) menuLayoutItem {
	var children []dbus.Variant
	if depth != 0 {
		d := depth
		if d > 0 {
			d--
		}
		for _, c := range e.children {
			children = append(children, dbus.MakeVariant(m.itemLocked(c, d)))
		}
	}
	if children == nil {
		children = []dbus.Variant{}
	}
	return menuLayoutItem{ID: e.id, Props: e.propsLocked(), Children: children}
}

// --- the exported com.canonical.dbusmenu method surface ---

// menuMethods is the exported com.canonical.dbusmenu method surface.
type menuMethods struct{ it *Item }

func (m *menuMethods) GetLayout(parentID, recursionDepth int32, propertyNames []string) (uint32, menuLayoutItem, *dbus.Error) {
	mnu := m.it.menu
	mnu.mu.Lock()
	defer mnu.mu.Unlock()
	e, ok := mnu.byID[parentID]
	if !ok {
		return 0, menuLayoutItem{}, dbus.NewError("com.canonical.dbusmenu.Error.UnknownItem", nil)
	}
	return mnu.revision, mnu.itemLocked(e, recursionDepth), nil
}

func (m *menuMethods) GetGroupProperties(ids []int32, propertyNames []string) ([]menuItemProperties, *dbus.Error) {
	mnu := m.it.menu
	mnu.mu.Lock()
	defer mnu.mu.Unlock()
	out := []menuItemProperties{}
	for _, id := range ids {
		if e, ok := mnu.byID[id]; ok {
			out = append(out, menuItemProperties{ID: id, Props: e.propsLocked()})
		}
	}
	return out, nil
}

func (m *menuMethods) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	mnu := m.it.menu
	mnu.mu.Lock()
	defer mnu.mu.Unlock()
	e, ok := mnu.byID[id]
	if !ok {
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.UnknownItem", nil)
	}
	props := e.propsLocked()
	v, ok := props[name]
	if !ok {
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.UnknownProperty", nil)
	}
	return v, nil
}

// Event receives the host's item events. "clicked" on a visible, enabled,
// labelled entry is the user activating it; everything else (opened, closed,
// hovered, shortcut noise) is ignored.
func (m *menuMethods) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	m.it.menu.click(id, eventID)
	return nil
}

func (m *menuMethods) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	for _, e := range events {
		m.it.menu.click(e.ID, e.EventID)
	}
	return []int32{}, nil
}

// click dispatches one activation if the event means one. Pure gating (no
// bus I/O), so it is unit-testable.
func (m *Menu) click(id int32, eventID string) {
	if eventID != "clicked" {
		return
	}
	m.mu.Lock()
	e, ok := m.byID[id]
	activatable := ok && e.enabled && e.visible && !e.sep
	var ch chan struct{}
	if activatable {
		ch = e.ClickedCh
	}
	m.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default: // consumer behind: drop, never block the D-Bus handler
	}
}

// AboutToShow always answers "no update needed": mutations are published the
// moment they happen (LayoutUpdated), so a client asking on open is already
// current. Answering true here would ask for a refetch loop.
func (m *menuMethods) AboutToShow(id int32) (bool, *dbus.Error) {
	return false, nil
}

func (m *menuMethods) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}

// menuIntrospect describes the dbusmenu surface for `gdbus introspect` and
// picky hosts (the GNOME extension uses its own bundled XML, but KDE and
// debugging tools introspect).
var menuIntrospect = introspect.Interface{
	Name: menuIface,
	Methods: []introspect.Method{
		{Name: "GetLayout", Args: []introspect.Arg{
			{Name: "parentId", Type: "i", Direction: "in"},
			{Name: "recursionDepth", Type: "i", Direction: "in"},
			{Name: "propertyNames", Type: "as", Direction: "in"},
			{Name: "revision", Type: "u", Direction: "out"},
			{Name: "layout", Type: "(ia{sv}av)", Direction: "out"},
		}},
		{Name: "GetGroupProperties", Args: []introspect.Arg{
			{Name: "ids", Type: "ai", Direction: "in"},
			{Name: "propertyNames", Type: "as", Direction: "in"},
			{Name: "properties", Type: "a(ia{sv})", Direction: "out"},
		}},
		{Name: "GetProperty", Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "name", Type: "s", Direction: "in"},
			{Name: "value", Type: "v", Direction: "out"},
		}},
		{Name: "Event", Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "eventId", Type: "s", Direction: "in"},
			{Name: "data", Type: "v", Direction: "in"},
			{Name: "timestamp", Type: "u", Direction: "in"},
		}},
		{Name: "EventGroup", Args: []introspect.Arg{
			{Name: "events", Type: "a(isvu)", Direction: "in"},
			{Name: "idErrors", Type: "ai", Direction: "out"},
		}},
		{Name: "AboutToShow", Args: []introspect.Arg{
			{Name: "id", Type: "i", Direction: "in"},
			{Name: "needUpdate", Type: "b", Direction: "out"},
		}},
		{Name: "AboutToShowGroup", Args: []introspect.Arg{
			{Name: "ids", Type: "ai", Direction: "in"},
			{Name: "updatesNeeded", Type: "ai", Direction: "out"},
			{Name: "idErrors", Type: "ai", Direction: "out"},
		}},
	},
	Signals: []introspect.Signal{
		{Name: "ItemsPropertiesUpdated", Args: []introspect.Arg{
			{Name: "updatedProps", Type: "a(ia{sv})"},
			{Name: "removedProps", Type: "a(ias)"},
		}},
		{Name: "LayoutUpdated", Args: []introspect.Arg{
			{Name: "revision", Type: "u"},
			{Name: "parent", Type: "i"},
		}},
		{Name: "ItemActivationRequested", Args: []introspect.Arg{
			{Name: "id", Type: "i"},
			{Name: "timestamp", Type: "u"},
		}},
	},
	Properties: []introspect.Property{
		{Name: "Version", Type: "u", Access: "read"},
		{Name: "TextDirection", Type: "s", Access: "read"},
		{Name: "Status", Type: "s", Access: "read"},
		{Name: "IconThemePath", Type: "as", Access: "read"},
	},
}

// exportMenu publishes the menu at menuPath on the item's connection.
func exportMenu(conn *dbus.Conn, it *Item) error {
	if err := conn.Export(&menuMethods{it: it}, menuPath, menuIface); err != nil {
		return err
	}
	if _, err := prop.Export(conn, menuPath, map[string]map[string]*prop.Prop{
		menuIface: {
			"Version":       {Value: uint32(3), Emit: prop.EmitTrue},
			"TextDirection": {Value: "ltr", Emit: prop.EmitTrue},
			"Status":        {Value: "normal", Emit: prop.EmitTrue},
			"IconThemePath": {Value: []string{}, Emit: prop.EmitTrue},
		},
	}); err != nil {
		return err
	}
	node := introspect.Node{
		Name: menuPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			menuIntrospect,
		},
	}
	return conn.Export(introspect.NewIntrospectable(&node), menuPath, "org.freedesktop.DBus.Introspectable")
}
