//go:build linux

package ui

import (
	"path/filepath"
	"testing"
)

// TestSingleInstanceFlock verifies the core single-instance mechanism: the
// first flock succeeds and a second flock on the same path (a stand-in for a
// second process launch) is refused. flock is per open-file-description, so two
// independent opens contend even inside one test process.
func TestSingleInstanceFlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fipindicateur.lock")

	first, err := tryFlock(path)
	if err != nil {
		t.Fatalf("first flock should succeed: %v", err)
	}
	defer first.Close()

	second, err := tryFlock(path)
	if err == nil {
		second.Close()
		t.Fatal("second flock on a held lock must fail (single-instance guard)")
	}

	// Releasing the first lock must let a later launch acquire it again (no
	// stale lock after the holder exits).
	if err := first.Close(); err != nil {
		t.Fatalf("closing first lock: %v", err)
	}
	third, err := tryFlock(path)
	if err != nil {
		t.Fatalf("flock should be reacquirable once released: %v", err)
	}
	third.Close()
}

// TestLockPathUsesRuntimeDir checks the lock lives under XDG_RUNTIME_DIR when
// that is set, so it is per-user and on tmpfs.
func TestLockPathUsesRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	if got, want := lockPath(), "/run/user/4242/fipindicateur.lock"; got != want {
		t.Fatalf("lockPath() = %q, want %q", got, want)
	}
}
