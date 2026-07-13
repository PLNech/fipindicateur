//go:build windows

package ui

// AcquireInstanceLock is a no-op on Windows: the flock-based single-instance
// guard covers the Unix targets (Linux, macOS, BSD), where syscall.Flock is
// available. It exists so the cross-platform entrypoint compiles without OS
// build guards.
func AcquireInstanceLock() error { return nil }
