//go:build !linux

// Package notify is a no-op on non-Linux platforms (desktop notifications are
// wired for freedesktop/D-Bus). The API mirrors the Linux build.
package notify

import "context"

// Notifier is a no-op notifier.
type Notifier struct{}

// New returns a no-op notifier.
func New() *Notifier { return &Notifier{} }

// Notify is a no-op.
func (*Notifier) Notify(string, string, string, int) {}

// FetchCover is a no-op returning no path.
func (*Notifier) FetchCover(context.Context, string) string { return "" }

// Close is a no-op.
func (*Notifier) Close() {}
