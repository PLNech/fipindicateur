//go:build linux

// Package notify shows desktop notifications on track changes via the
// org.freedesktop.Notifications D-Bus service. Best-effort: any failure is
// logged and ignored, never fatal.
package notify

import (
	"context"
	"log"

	"github.com/godbus/dbus/v5"
)

// Actions carries the banner-interaction callbacks (macOS: the like/dislike
// buttons and click-to-open). On Linux the freedesktop spec has actions too,
// but they need an ActionInvoked signal listener; until one is wired the
// callbacks are accepted and unused, so the call sites stay platform-neutral.
type Actions struct {
	Like    func()
	Dislike func()
	Open    func(link string)
	Denied  func()
}

// Notifier sends replace-in-place desktop notifications.
type Notifier struct {
	conn   *dbus.Conn
	lastID uint32
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
	return n
}

// Notify posts a notification, replacing the previous one. iconPath may be an
// empty string. summary is the title, body the artist/album line. timeoutMs is
// the expire hint in milliseconds: GNOME Shell ignores it (banners stay ~4s,
// missed ones land in the clock menu) but dunst, KDE and most other daemons
// honor it, so passing it keeps the app correct everywhere.
func (n *Notifier) Notify(summary, body, iconPath string, timeoutMs int) {
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
		[]string{},                // actions
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
