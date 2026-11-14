//go:build !linux && !darwin

// Package notify is a no-op where no desktop notification service is wired
// (freedesktop on Linux, UserNotifications on macOS). The API matches those.
package notify

import "context"

// Actions carries the banner-interaction callbacks (macOS: the like/dislike
// buttons and click-to-open). Unused where no notification service exists.
type Actions struct {
	Like    func()
	Dislike func()
	Open    func(link string)
	Denied  func()
	Default func()
}

// Notifier is a no-op notifier.
type Notifier struct{}

// New returns a no-op notifier.
func New(Actions) *Notifier { return &Notifier{} }

// Notify is a no-op.
func (*Notifier) Notify(string, string, string, int) {}

// NotifyAction is a no-op.
func (*Notifier) NotifyAction(string, string, string, int, string, func()) {}

// NotifyTrack is a no-op.
func (*Notifier) NotifyTrack(string, string, string, string, string, int) {}

// FetchCover is a no-op returning no path.
func (*Notifier) FetchCover(context.Context, string) string { return "" }

// Close is a no-op.
func (*Notifier) Close() {}
