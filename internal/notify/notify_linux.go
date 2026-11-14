//go:build linux

// Package notify shows desktop notifications on track changes via the
// org.freedesktop.Notifications D-Bus service. Best-effort: any failure is
// logged and ignored, never fatal.
package notify

import (
	"context"
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Actions carries the banner-interaction callbacks (macOS: the like/dislike
// buttons and click-to-open). On Linux, Default fires when the user clicks a
// banner posted by NotifyAction: the freedesktop ActionInvoked signal is
// listened for and routed back here, so a notice can open the thing it talks
// about.
type Actions struct {
	Like    func()
	Dislike func()
	Open    func(link string)
	Denied  func()
	Default func()
}

// Notifier sends replace-in-place desktop notifications.
type Notifier struct {
	conn   *dbus.Conn
	lastID uint32
	// mu guards defaultFn, the callback of the most recent NotifyAction
	// banner (replace-in-place: a new banner retires the old click).
	mu        sync.Mutex
	defaultFn func()
}

// New connects to the session bus. Returns a usable (possibly degraded)
// Notifier even on error, so callers need not nil-check.
func New(actions Actions) *Notifier {
	n := &Notifier{}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("notify: session bus: %v", err)
		return n
	}
	n.conn = conn
	// ActionInvoked carries the activation_key of a clicked banner action.
	// Best-effort: without the match, NotifyAction banners stay clickless,
	// which is the pre-listener behaviour anyway.
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("ActionInvoked"),
	); err != nil {
		log.Printf("notify: match ActionInvoked: %v", err)
		return n
	}
	go n.listenActions()
	return n
}

// listenActions drains the ActionInvoked signals and routes the "default"
// key (the banner body) to the pending NotifyAction callback.
func (n *Notifier) listenActions() {
	c := make(chan *dbus.Signal, 8)
	n.conn.Signal(c)
	for s := range c {
		if s.Name != "org.freedesktop.Notifications.ActionInvoked" || len(s.Body) < 2 {
			continue
		}
		var id uint32
		var key string
		if err := dbus.Store(s.Body, &id, &key); err != nil || key != "default" {
			continue
		}
		n.mu.Lock()
		fn := n.defaultFn
		n.mu.Unlock()
		if fn != nil {
			fn()
		}
	}
}

// Notify posts a notification, replacing the previous one. iconPath may be an
// empty string. summary is the title, body the artist/album line. timeoutMs is
// the expire hint in milliseconds: GNOME Shell ignores it (banners stay ~4s,
// missed ones land in the clock menu) but dunst, KDE and most other daemons
// honor it, so passing it keeps the app correct everywhere.
func (n *Notifier) Notify(summary, body, iconPath string, timeoutMs int) {
	n.post(summary, body, iconPath, timeoutMs, nil)
}

// NotifyAction posts a banner whose body is clickable: clicking routes to fn
// through the ActionInvoked listener. actionLabel names the action on daemons
// that render inline buttons; the body click works wherever the spec is
// honored. Replace-in-place: a newer banner retires the older callback.
func (n *Notifier) NotifyAction(summary, body, iconPath string, timeoutMs int, actionLabel string, fn func()) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.defaultFn = fn
	n.mu.Unlock()
	n.post(summary, body, iconPath, timeoutMs, []string{"default", actionLabel})
}

func (n *Notifier) post(summary, body, iconPath string, timeoutMs int, actionKeys []string) {
	if n == nil || n.conn == nil {
		return
	}
	if timeoutMs <= 0 {
		timeoutMs = 10000
	}
	obj := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		"fipindicateur",           // app_name
		n.lastID,                  // replaces_id
		iconPath,                  // app_icon
		summary,                   // summary
		body,                      // body
		actionKeys,                // actions
		map[string]dbus.Variant{}, // hints
		int32(timeoutMs),          // expire_timeout ms
	)
	if call.Err != nil {
		log.Printf("notify: %v", call.Err)
		return
	}
	var id uint32
	if err := call.Store(&id); err == nil {
		n.lastID = id
	}
}

// NotifyTrack posts the track banner. The platform has no subtitle field, so
// the artist leads the body line, ahead of album/label/station.
func (n *Notifier) NotifyTrack(title, subtitle, body, iconPath string, link string, timeoutMs int) {
	if subtitle != "" {
		if body != "" {
			body = subtitle + " · " + body
		} else {
			body = subtitle
		}
	}
	n.Notify(title, body, iconPath, timeoutMs)
}

// FetchCover downloads a cover image to a temp file and returns its path, or
// "" on any error. Caller passes the path to Notify as the icon.
func (n *Notifier) FetchCover(ctx context.Context, url string) string {
	if n == nil {
		return ""
	}
	return fetchCover(ctx, url)
}

// Close releases the connection.
func (n *Notifier) Close() {
	if n != nil && n.conn != nil {
		_ = n.conn.Close()
	}
}
