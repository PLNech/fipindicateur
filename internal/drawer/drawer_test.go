//go:build linux && cgo

package drawer

import "testing"

// TestAliveTracksDeadFlag pins the contract the app's recovery leans on: a
// drawer starts alive, and once markDead fires it stays dead (idempotent,
// no resurrection). See drawer_linux.go: the C heartbeat and the GTK
// goroutine tail are the only writers.
func TestAliveTracksDeadFlag(t *testing.T) {
	d := New(nil, nil)
	defer func() {
		currentMu.Lock()
		current = nil
		currentMu.Unlock()
	}()
	if !d.Alive() {
		t.Fatal("a fresh drawer must report alive before its GTK stack exists")
	}
	d.markDead("test")
	if d.Alive() {
		t.Fatal("a drawer whose GTK stack died must report dead")
	}
	d.markDead("test again")
	if d.Alive() {
		t.Fatal("markDead must be idempotent, not resurrect the panel")
	}
}
