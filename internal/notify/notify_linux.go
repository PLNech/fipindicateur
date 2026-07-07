//go:build linux

// Package notify shows desktop notifications on track changes via the
// org.freedesktop.Notifications D-Bus service. Best-effort: any failure is
// logged and ignored, never fatal.
package notify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
)

// Notifier sends replace-in-place desktop notifications.
type Notifier struct {
	conn   *dbus.Conn
	lastID uint32
	client *http.Client
}

// New connects to the session bus. Returns a usable (possibly degraded)
// Notifier even on error, so callers need not nil-check.
func New() *Notifier {
	n := &Notifier{client: &http.Client{Timeout: 10 * time.Second}}
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

// FetchCover downloads a cover image to a temp file and returns its path, or
// "" on any error. Caller passes the path to Notify as the icon.
func (n *Notifier) FetchCover(ctx context.Context, url string) string {
	if n == nil || url == "" {
		return ""
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	sum := sha1.Sum([]byte(url))
	dst := filepath.Join(dir, "fipindicateur-cover-"+hex.EncodeToString(sum[:8])+".jpg")

	if _, err := os.Stat(dst); err == nil {
		return dst // cached
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	f, err := os.Create(dst)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 8<<20)); err != nil {
		_ = os.Remove(dst)
		return ""
	}
	return dst
}

// Close releases the connection.
func (n *Notifier) Close() {
	if n != nil && n.conn != nil {
		_ = n.conn.Close()
	}
}
