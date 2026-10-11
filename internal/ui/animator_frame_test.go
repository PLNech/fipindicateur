//go:build linux

package ui

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/PLNech/fipindicateur/internal/icon"
	"github.com/PLNech/fipindicateur/internal/sni"
)

// useSink points the process's SNI item at a bus-less item for one test and
// returns the pixmaps it receives, in order.
func useSink(t *testing.T) *[][]byte {
	t.Helper()
	var got [][]byte
	it := sni.NewDetached(func(w, h int, pix []byte) {
		got = append(got, append([]byte(nil), pix...))
	})
	sniMu.Lock()
	old := sniItem
	sniItem = it
	sniMu.Unlock()
	t.Cleanup(func() {
		sniMu.Lock()
		sniItem = old
		sniMu.Unlock()
	})
	return &got
}

// TestDrawFrameMatchesPNGRoute is the end-to-end golden test: for every frame
// of a realistic animation walk, the direct pixmap path hands the tray exactly
// the pixels the original PNG route (BarsIcon, setTrayIcon's Retint, SNI PNG
// decode) did.
func TestDrawFrameMatchesPNGRoute(t *testing.T) {
	frames := benchFrames()
	if len(frames) < 20 {
		t.Fatalf("frame walk too short: %d", len(frames))
	}

	var want [][]byte
	{
		got := useSink(t)
		for _, k := range frames {
			setTrayIcon(icon.BarsIcon(k.h, k.tint))
		}
		want = *got
	}
	got := useSink(t)
	a := &App{}
	for _, k := range frames {
		a.drawFrame(k)
	}
	if len(*got) != len(want) {
		t.Fatalf("direct route sent %d frames, PNG route %d", len(*got), len(want))
	}
	for i := range want {
		if !bytes.Equal((*got)[i], want[i]) {
			t.Fatalf("frame %d (%v) differs from the PNG route", i, frames[i])
		}
	}
}

// TestDrawFrameInvalidatesStaticBaseline: after an animated frame, re-setting
// the same static icon that was shown before the animation must still reach
// the tray (setIcon's dedupe baseline cannot claim the tray still shows it).
func TestDrawFrameInvalidatesStaticBaseline(t *testing.T) {
	got := useSink(t)
	a := &App{}
	rest := icon.Rest(false, color.NRGBA{})

	a.setIcon(rest)
	staticPix := (*got)[len(*got)-1]
	a.drawFrame(benchFrames()[5])
	a.setIcon(rest)

	if n := len(*got); n != 3 {
		t.Fatalf("static, frame, static sent %d pixmaps, want 3", n)
	}
	if !bytes.Equal((*got)[2], staticPix) {
		t.Error("the static icon came back different")
	}
}

// TestDrawFrameWithoutItemFallsBack: before the SNI item exists, a frame goes
// through the pending-icon route instead of being lost.
func TestDrawFrameWithoutItemFallsBack(t *testing.T) {
	sniMu.Lock()
	oldItem, oldPending := sniItem, sniPending
	sniItem, sniPending = nil, nil
	sniMu.Unlock()
	t.Cleanup(func() {
		sniMu.Lock()
		sniItem, sniPending = oldItem, oldPending
		sniMu.Unlock()
	})

	a := &App{}
	a.drawFrame(benchFrames()[3])
	sniMu.Lock()
	n := len(sniPending)
	sniMu.Unlock()
	if n == 0 {
		t.Error("frame drawn before the item existed was dropped, not stashed as pending")
	}
}
