//go:build linux

// Package sni exposes one StatusNotifierItem on the session bus, hand-rolled
// over godbus: the tray icon without any menu. On Linux the drawer (« le
// panneau ») is the single UI, so the item deliberately publishes no DBusMenu
// (ItemIsMenu=false, Menu points at a dead path): the host forwards clicks and
// scrolls as method calls (Activate, ContextMenu, SecondaryActivate, Scroll)
// and the app decides what they mean.
//
// The wire conventions (premultiplied ARGB32 pixmaps, registration payload,
// property set, NewIcon/NewTitle/NewToolTip signals) mirror what fyne/systray
// negotiated with the GNOME AppIndicator extension, the proven pipeline this
// package replaces on Linux.
package sni

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	itemPath    = "/StatusNotifierItem"
	itemIface   = "org.kde.StatusNotifierItem"
	watcherName = "org.kde.StatusNotifierWatcher"
	watcherPath = "/StatusNotifierWatcher"

	// Registration retry window when no watcher owns the name at startup
	// (GNOME still loading the extension). Past it we log and give up for
	// this owner; a later watcher (re)start re-registers via NameOwnerChanged.
	registerRetries  = 10
	registerInterval = time.Second
)

// pixmap is the SNI icon wire format: width, height, then premultiplied ARGB32
// bytes in network byte order (dbus signature (iiay)).
type pixmap struct {
	W   int32
	H   int32
	Pix []byte
}

// toolTip is the SNI ToolTip wire format (dbus signature (sa(iiay)ss)):
// icon name, icon pixmaps, title, descriptive text.
type toolTip struct {
	IconName string
	Pixmaps  []pixmap
	Title    string
	Text     string
}

// Handlers are the app-side reactions to host-forwarded input. Each is called
// on a fresh goroutine (a D-Bus method handler must return promptly) and may
// be nil. Scroll receives the raw delta (sign only is reliable across hosts;
// on GNOME a wheel notch up arrives as a negative vertical delta) and the
// orientation ("vertical" or "horizontal").
type Handlers struct {
	Activate          func()
	SecondaryActivate func()
	ContextMenu       func()
	Scroll            func(delta int, orientation string)
}

// Item is one live StatusNotifierItem. All methods are safe from any
// goroutine.
type Item struct {
	conn  *dbus.Conn
	props *prop.Properties
	h     Handlers
}

// methods is the exported D-Bus method surface. Kept separate from Item so
// only these four names are reachable over the bus.
type methods struct{ it *Item }

// Activate is the host's primary action (left click on the icon).
func (m *methods) Activate(x, y int32) *dbus.Error {
	if f := m.it.h.Activate; f != nil {
		go f()
	}
	return nil
}

// SecondaryActivate is the host's middle-click.
func (m *methods) SecondaryActivate(x, y int32) *dbus.Error {
	if f := m.it.h.SecondaryActivate; f != nil {
		go f()
	}
	return nil
}

// ContextMenu is the host's right click. With ItemIsMenu=false and no
// DBusMenu, the host calls this instead of popping a menu.
func (m *methods) ContextMenu(x, y int32) *dbus.Error {
	if f := m.it.h.ContextMenu; f != nil {
		go f()
	}
	return nil
}

// Scroll is a wheel event over the icon.
func (m *methods) Scroll(delta int32, orientation string) *dbus.Error {
	if f := m.it.h.Scroll; f != nil {
		go f(int(delta), orientation)
	}
	return nil
}

// introspectInterface describes our item interface so `gdbus introspect` and
// picky hosts see the full method/property/signal surface.
var introspectInterface = introspect.Interface{
	Name: itemIface,
	Methods: []introspect.Method{
		{Name: "Activate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
		{Name: "SecondaryActivate", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
		{Name: "ContextMenu", Args: []introspect.Arg{{Name: "x", Type: "i", Direction: "in"}, {Name: "y", Type: "i", Direction: "in"}}},
		{Name: "Scroll", Args: []introspect.Arg{{Name: "delta", Type: "i", Direction: "in"}, {Name: "orientation", Type: "s", Direction: "in"}}},
	},
	Signals: []introspect.Signal{
		{Name: "NewTitle"},
		{Name: "NewIcon"},
		{Name: "NewToolTip"},
		{Name: "NewStatus", Args: []introspect.Arg{{Name: "status", Type: "s"}}},
	},
	Properties: []introspect.Property{
		{Name: "Category", Type: "s", Access: "read"},
		{Name: "Id", Type: "s", Access: "read"},
		{Name: "Title", Type: "s", Access: "read"},
		{Name: "Status", Type: "s", Access: "read"},
		{Name: "WindowId", Type: "i", Access: "read"},
		{Name: "IconName", Type: "s", Access: "read"},
		{Name: "IconPixmap", Type: "a(iiay)", Access: "read"},
		{Name: "IconThemePath", Type: "s", Access: "read"},
		{Name: "OverlayIconName", Type: "s", Access: "read"},
		{Name: "OverlayIconPixmap", Type: "a(iiay)", Access: "read"},
		{Name: "AttentionIconName", Type: "s", Access: "read"},
		{Name: "AttentionIconPixmap", Type: "a(iiay)", Access: "read"},
		{Name: "AttentionMovieName", Type: "s", Access: "read"},
		{Name: "IconAccessibleDesc", Type: "s", Access: "read"},
		{Name: "AttentionAccessibleDesc", Type: "s", Access: "read"},
		{Name: "XAyatanaLabel", Type: "s", Access: "read"},
		{Name: "XAyatanaLabelGuide", Type: "s", Access: "read"},
		{Name: "XAyatanaOrderingIndex", Type: "u", Access: "read"},
		{Name: "ToolTip", Type: "(sa(iiay)ss)", Access: "read"},
		{Name: "ItemIsMenu", Type: "b", Access: "read"},
		{Name: "Menu", Type: "o", Access: "read"},
	},
}

// New connects to the session bus, exports the item and registers it with the
// StatusNotifierWatcher. title seeds Title and the tooltip; iconPNG is the
// initial icon (see SetIconPNG). The item then re-registers itself whenever
// the watcher changes owner (a GNOME shell reload), for the life of the
// process.
func New(id, title string, iconPNG []byte, h Handlers) (*Item, error) {
	// A private connection: the well-known item name and its lifetime belong
	// to this item alone (MPRIS owns its own connection the same way).
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	it := &Item{conn: conn, h: h}

	if err := conn.Export(&methods{it: it}, itemPath, itemIface); err != nil {
		conn.Close()
		return nil, err
	}

	pm, err := pngToPixmap(iconPNG)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sni: initial icon: %w", err)
	}
	// The full property surface of the extension's StatusNotifierItem.xml is
	// exported, including the attention/overlay slots this app never uses:
	// hosts refresh properties in batches on every New* signal, and godbus
	// answers a missing property with a nonstandard PropertyNotFound error
	// that the GNOME extension logs as a hard failure (which can abort the
	// very refresh that would have picked up the new icon).
	props, err := prop.Export(conn, itemPath, map[string]map[string]*prop.Prop{
		itemIface: {
			"Category":            {Value: "ApplicationStatus", Emit: prop.EmitTrue},
			"Id":                  {Value: id, Emit: prop.EmitTrue},
			"Title":               {Value: title, Emit: prop.EmitTrue},
			"Status":              {Value: "Active", Emit: prop.EmitTrue},
			"WindowId":            {Value: int32(0), Emit: prop.EmitTrue},
			"IconName":            {Value: "", Emit: prop.EmitTrue},
			"IconPixmap":          {Value: []pixmap{pm}, Emit: prop.EmitTrue},
			"IconThemePath":       {Value: "", Emit: prop.EmitTrue},
			"OverlayIconName":     {Value: "", Emit: prop.EmitTrue},
			"OverlayIconPixmap":   {Value: []pixmap{}, Emit: prop.EmitTrue},
			"AttentionIconName":   {Value: "", Emit: prop.EmitTrue},
			"AttentionIconPixmap": {Value: []pixmap{}, Emit: prop.EmitTrue},
			"AttentionMovieName":  {Value: "", Emit: prop.EmitTrue},
			// Accessible descriptions: the GNOME extension refreshes
			// IconAccessibleDesc alongside IconPixmap on every NewIcon, and a
			// missing property rejects that whole refresh batch, silently
			// dropping the icon update (observed live: the item stayed
			// invisible until this answered).
			"IconAccessibleDesc":      {Value: title, Emit: prop.EmitTrue},
			"AttentionAccessibleDesc": {Value: "", Emit: prop.EmitTrue},
			// The ayatana label extras: refreshed by the GNOME extension on
			// every batch, so they must answer (empty) rather than error.
			"XAyatanaLabel":         {Value: "", Emit: prop.EmitTrue},
			"XAyatanaLabelGuide":    {Value: "", Emit: prop.EmitTrue},
			"XAyatanaOrderingIndex": {Value: uint32(0), Emit: prop.EmitTrue},
			"ToolTip":               {Value: toolTip{Title: title}, Emit: prop.EmitTrue},
			// The whole point of this package: no menu. ItemIsMenu=false tells
			// spec-following hosts (KDE) to deliver Activate/ContextMenu
			// instead of demanding a DBusMenu. The Menu path deliberately
			// points at a path nothing is exported on: it must NOT be the
			// magic "/NO_DBUSMENU", because the GNOME AppIndicator extension
			// maps that to "no menu path" and then refuses to show the item
			// at all (its readiness check requires Id + a menu path). With a
			// dead path the item shows, its menu stays permanently empty, and
			// clicks fall through to the SNI methods.
			"ItemIsMenu": {Value: false, Emit: prop.EmitTrue},
			"Menu":       {Value: dbus.ObjectPath("/StatusNotifierMenu"), Emit: prop.EmitTrue},
		},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	it.props = props

	node := introspect.Node{
		Name: itemPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			introspectInterface,
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(&node), itemPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		conn.Close()
		return nil, err
	}

	// The per-process well-known name the SNI convention expects. Losing the
	// race is not fatal (the watcher tracks us by unique name + object path,
	// which is also what we register with, like fyne/systray did).
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if _, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
		log.Printf("sni: request name %s: %v (continuing on unique name)", name, err)
	}

	go it.stayRegistered()
	return it, nil
}

// register announces the item to the watcher. The payload is our object path:
// the watcher pairs it with the sender's unique name (the convention the
// GNOME extension and fyne/systray both use).
func (it *Item) register() error {
	obj := it.conn.Object(watcherName, watcherPath)
	return obj.Call(watcherName+".RegisterStatusNotifierItem", 0, itemPath).Err
}

// stayRegistered performs the initial registration (with a short retry window
// for a watcher still starting up) and then re-registers every time the
// watcher name changes owner, i.e. after a GNOME shell reload. Runs for the
// life of the connection; the signal channel closes with it.
func (it *Item) stayRegistered() {
	registered := false
	for i := 0; i < registerRetries; i++ {
		if err := it.register(); err == nil {
			registered = true
			break
		}
		time.Sleep(registerInterval)
	}
	if !registered {
		// Same failure mode as a tray without a watcher always had: the app
		// runs on, just without an icon, until a watcher appears.
		log.Printf("sni: no StatusNotifierWatcher answered after %v; icon hidden until one appears", registerRetries*registerInterval)
	}

	if err := it.conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, watcherName),
	); err != nil {
		log.Printf("sni: watch watcher: %v (no re-registration after a shell reload)", err)
		return
	}
	sc := make(chan *dbus.Signal, 16)
	it.conn.Signal(sc)
	for sig := range sc {
		if len(sig.Body) < 3 {
			continue
		}
		if owner, ok := sig.Body[2].(string); ok && owner != "" {
			if err := it.register(); err != nil {
				log.Printf("sni: re-register: %v", err)
			}
		}
	}
}

// SetIconPNG replaces the icon from PNG bytes and signals NewIcon, so the VU
// animation keeps living in the tray exactly as before.
func (it *Item) SetIconPNG(b []byte) {
	pm, err := pngToPixmap(b)
	if err != nil {
		log.Printf("sni: icon: %v", err)
		return
	}
	it.props.SetMust(itemIface, "IconPixmap", []pixmap{pm})
	if err := it.conn.Emit(itemPath, itemIface+".NewIcon"); err != nil {
		log.Printf("sni: NewIcon: %v", err)
	}
}

// SetLabel updates Title and the tooltip text (the now-playing label) and
// signals the host.
func (it *Item) SetLabel(title, text string) {
	it.props.SetMust(itemIface, "Title", title)
	it.props.SetMust(itemIface, "ToolTip", toolTip{Title: title, Text: text})
	if err := it.conn.Emit(itemPath, itemIface+".NewTitle"); err != nil {
		log.Printf("sni: NewTitle: %v", err)
	}
	if err := it.conn.Emit(itemPath, itemIface+".NewToolTip"); err != nil {
		log.Printf("sni: NewToolTip: %v", err)
	}
}

// Close drops the connection; the bus releases our names and the watcher
// removes the item.
func (it *Item) Close() {
	_ = it.conn.Close()
}

// pngToPixmap decodes PNG bytes into the SNI pixmap: premultiplied ARGB32 in
// network byte order, the exact bytes fyne/systray fed the same hosts
// (image/color's RGBA() is premultiplied; the high byte of each 16-bit
// channel is the 8-bit value).
func pngToPixmap(b []byte) (pixmap, error) {
	if len(b) == 0 {
		return pixmap{}, fmt.Errorf("empty icon bytes")
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return pixmap{}, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	pix := make([]byte, 4*w*h)
	i := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			pix[i] = byte(a >> 8)
			pix[i+1] = byte(r >> 8)
			pix[i+2] = byte(g >> 8)
			pix[i+3] = byte(bl >> 8)
			i += 4
		}
	}
	return pixmap{W: int32(w), H: int32(h), Pix: pix}, nil
}
