//go:build darwin && swiftui

package popover

import (
	"testing"

	"github.com/PLNech/fipindicateur/internal/popover/fipui"
)

// The link test: with the Swift object linked in, an action fired on the
// Swift side lands in the Go handler, and a snapshot pushed from Go decodes
// on the Swift side. No window, no run loop: it proves the binary is whole.
func TestBridgeRoundTrip(t *testing.T) {
	var gotID, gotArg string
	SetHandler(func(id, arg string) { gotID, gotArg = id, arg })
	defer SetHandler(nil)

	if !fipui.DebugFire("volume", "42") {
		t.Fatal("Swift does not know the action id volume")
	}
	if gotID != "volume" || gotArg != "42" {
		t.Fatalf("handler got (%q, %q), want (volume, 42)", gotID, gotArg)
	}
	if fipui.DebugFire("no_such_action", "") {
		t.Fatal("Swift fired an id outside its Action enum")
	}
	if !Push(Snapshot{Playing: true, Station: "jazz"}) {
		t.Fatal("Swift could not decode a snapshot")
	}
	if fipui.PushState([]byte("{")) {
		t.Fatal("Swift accepted malformed JSON")
	}
}
