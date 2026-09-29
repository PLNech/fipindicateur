package ui

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/config"
	"github.com/PLNech/fipindicateur/internal/events"
)

// TestVolumeSettleGateDropsRestartPhantom pins the grace window: an ao-volume
// report arriving right after a playback restart is a phantom (the sink input
// exists before PulseAudio applied the restored level, ao-volume reads 0) and
// must not clobber the real level. The same report once the window has passed
// is trusted (a genuine 0 still lands).
func TestVolumeSettleGateDropsRestartPhantom(t *testing.T) {
	a := &App{cfg: config.Config{Volume: 40}, rec: events.NewRecorder(false)}

	a.markVolumeSettle()
	a.onExternalVolume(0)
	if a.cfg.Volume != 40 {
		t.Fatalf("phantom zero inside the settle window was adopted: volume=%d", a.cfg.Volume)
	}

	a.volSettleMu.Lock()
	a.volSettleUntil = time.Time{}
	a.volSettleMu.Unlock()
	a.onExternalVolume(0)
	if a.cfg.Volume != 0 {
		t.Fatalf("genuine report outside the window was dropped: volume=%d", a.cfg.Volume)
	}
}

// The settled re-read must not be swallowed by its own gate: it fires past the
// window it is meant to clear.
func TestVolumeSettleReadDelayExceedsGrace(t *testing.T) {
	if volumeSettleReadDelay <= volumeSettleGrace {
		t.Fatalf("re-read at %v would race the window closing at %v", volumeSettleReadDelay, volumeSettleGrace)
	}
}
