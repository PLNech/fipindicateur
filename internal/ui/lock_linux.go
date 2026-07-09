package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// instanceLock holds the process-lifetime lock file. It is kept in a package
// variable so the descriptor is never garbage-collected or closed: the flock is
// released only when the process exits (or the fd is closed), which is exactly
// the single-instance semantics we want. The kernel drops the lock on process
// death, so it cannot go stale after a crash.
var instanceLock *os.File

// lockDir returns the directory for the lock file: XDG_RUNTIME_DIR when set
// (tmpfs, per-user, cleared on logout), else the OS temp dir as a fallback.
func lockDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

// lockPath is the full path of the single-instance lock file.
func lockPath() string {
	return filepath.Join(lockDir(), "fipindicateur.lock")
}

// tryFlock opens path and takes a non-blocking exclusive advisory lock. On
// success it returns the open file (whose fd holds the lock) which the caller
// must keep alive. A second call while the lock is held (this process or
// another) returns an error. flock is associated with the open file
// description, so two independent opens contend even within one process, which
// is what the test relies on.
func tryFlock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("locked by another instance: %w", err)
	}
	return f, nil
}

// AcquireInstanceLock takes the single-instance lock, holding it for the
// lifetime of the process. It returns an error if another instance already
// holds it, in which case the caller should exit without registering a tray
// icon. This runs before any tray/D-Bus/mpv setup so a second launch never
// double-registers a StatusNotifierItem (the MPRIS name check is a secondary
// guard that only fires later, once the session bus is reachable).
func AcquireInstanceLock() error {
	f, err := tryFlock(lockPath())
	if err != nil {
		return err
	}
	instanceLock = f
	return nil
}
