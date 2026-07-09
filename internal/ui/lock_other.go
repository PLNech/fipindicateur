//go:build !linux

package ui

// AcquireInstanceLock is a no-op on non-Linux platforms: the flock-based
// single-instance guard is Linux-only (matching the MPRIS/SNI target). It
// exists so the cross-platform entrypoint compiles without OS build guards.
func AcquireInstanceLock() error { return nil }
