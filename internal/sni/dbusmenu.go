//go:build linux

package sni

// A minimal com.canonical.dbusmenu server: one item, « Ouvrir le panneau ».
//
// Why it exists (issue #18): the GNOME AppIndicator extension delivers
// Activate only on a DOUBLE left click; a single left click only toggles the
// item's DBusMenu, and only when that menu has at least one entry. Since
// v0.5.0 this app exported no menu at all, so a single click did nothing.
// Exporting this one-entry menu gives the single click something to do, and
// the app treats the menu being OPENED as the click itself:
//
//   - The extension sends Event(0, "opened") exactly when the shell popup
//     opens (indicatorStatusIcon.js waits out the double-click window, then
//     menu.toggle() -> _onMenuOpenStateChanged(true) -> handleEvent("opened")).
//     That is our single-click signal: the panel opens right away.
//   - AboutToShow is deliberately NOT a click signal: the extension calls
//     AboutToShow(0) once at attachToMenu time ("Dropbox requires us to call
//     AboutToShow(0) first"), i.e. at every (re)registration, which would
//     pop the panel at app start.
//   - Event(1, "clicked") (the user activating the visible entry) also opens
//     the panel: harmless when the "opened" event already did (the open is
//     idempotent app-side), and the working path on hosts that only report
//     item activation.
//
// A GNOME double click still lands on Activate (the extension cancels the
// pending menu-open), and KDE keeps its spec behaviour (ItemIsMenu=false:
// left click = Activate; the menu only shows on right click).

import (
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	menuPath   = "/StatusNotifierMenu" // the Menu property already points here
	menuIface  = "com.canonical.dbusmenu"
	menuItemID = 1 // the one entry; 0 is the root
	// menuRevision never changes: the layout is static, LayoutUpdated is
	// never emitted.
	menuRevision = uint32(1)
)

// menuOpenEvent reports whether a DBusMenu event means "the user asked for
// the panel": the root menu opening (the single click on GNOME) or the one
// entry being activated. Pure function, unit-tested.
func menuOpenEvent(id int32, eventID string) bool {
	return (id == 0 && eventID == "opened") || (id == menuItemID && eventID == "clicked")
}

// menuLayoutItem is the dbusmenu layout node wire format (ia{sv}av).
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

func menuRootProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"children-display": dbus.MakeVariant("submenu"),
	}
}

func menuEntryProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"label":   dbus.MakeVariant("Ouvrir le panneau"),
		"enabled": dbus.MakeVariant(true),
		"visible": dbus.MakeVariant(true),
	}
}

func menuEntry() menuLayoutItem {
	return menuLayoutItem{ID: menuItemID, Props: menuEntryProps(), Children: []dbus.Variant{}}
}

func menuRoot() menuLayoutItem {
	return menuLayoutItem{
		ID:       0,
		Props:    menuRootProps(),
		Children: []dbus.Variant{dbus.MakeVariant(menuEntry())},
	}
}

// menuMethods is the exported com.canonical.dbusmenu method surface.
type menuMethods struct{ it *Item }

func (m *menuMethods) GetLayout(parentID, recursionDepth int32, propertyNames []string) (uint32, menuLayoutItem, *dbus.Error) {
	if parentID == menuItemID {
		return menuRevision, menuEntry(), nil
	}
	return menuRevision, menuRoot(), nil
}

func (m *menuMethods) GetGroupProperties(ids []int32, propertyNames []string) ([]menuItemProperties, *dbus.Error) {
	if len(ids) == 0 {
		ids = []int32{0, menuItemID}
	}
	out := make([]menuItemProperties, 0, len(ids))
	for _, id := range ids {
		switch id {
		case 0:
			out = append(out, menuItemProperties{ID: 0, Props: menuRootProps()})
		case menuItemID:
			out = append(out, menuItemProperties{ID: menuItemID, Props: menuEntryProps()})
		}
	}
	return out, nil
}

func (m *menuMethods) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	var props map[string]dbus.Variant
	switch id {
	case 0:
		props = menuRootProps()
	case menuItemID:
		props = menuEntryProps()
	default:
		return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.UnknownItem", nil)
	}
	if v, ok := props[name]; ok {
		return v, nil
	}
	return dbus.Variant{}, dbus.NewError("com.canonical.dbusmenu.Error.UnknownProperty", nil)
}

// Event receives the host's item events; the "opened"/"clicked" pair is the
// single-click signal (see the package comment above).
func (m *menuMethods) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if menuOpenEvent(id, eventID) {
		if f := m.it.h.MenuOpen; f != nil {
			go f()
		}
	}
	return nil
}

func (m *menuMethods) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	for _, e := range events {
		if menuOpenEvent(e.ID, e.EventID) {
			if f := m.it.h.MenuOpen; f != nil {
				go f()
			}
		}
	}
	return []int32{}, nil
}

// AboutToShow always answers "no update needed": the layout is static, and
// treating it as a click would pop the panel at registration time (the GNOME
// extension calls AboutToShow(0) once when it attaches the menu).
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
