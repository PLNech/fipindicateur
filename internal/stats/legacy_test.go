package stats

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PLNech/fipindicateur/internal/events"
)

// TestLegacyEventLogStillReads pins the compatibility contract of the retired
// panel: an events.jsonl written by an older build holds kinds that no code
// emits any more (drawer_open, panel_recover, panel_scale, theme) and the
// cast_pause/cast_resume transport pair. The raw lines must still load, the
// dead kinds must be ignored by the derivation, and the cast pair must keep
// shaping listening time exactly as before.
func TestLegacyEventLogStillReads(t *testing.T) {
	raw := `{"v":1,"ts":"2026-07-08T14:00:00+02:00","kind":"drawer_open","station":"fip"}
{"v":1,"ts":"2026-07-08T14:00:00+02:00","kind":"cast_start","station":"fip"}
{"v":1,"ts":"2026-07-08T14:00:10+02:00","kind":"panel_scale","value":150}
{"v":1,"ts":"2026-07-08T14:00:20+02:00","kind":"theme","value":2}
{"v":1,"ts":"2026-07-08T14:30:00+02:00","kind":"cast_pause"}
{"v":1,"ts":"2026-07-08T14:35:00+02:00","kind":"cast_resume"}
{"v":1,"ts":"2026-07-08T14:36:00+02:00","kind":"panel_recover","station":"fip"}
{"v":1,"ts":"2026-07-08T14:45:00+02:00","kind":"cast_stop","station":"fip"}
`
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	evs, err := events.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 8 {
		t.Fatalf("loaded %d events, want all 8 (dead kinds must not be dropped at read)", len(evs))
	}
	r := Build(evs, nil, nil, nil, base)
	// 30min before the pause + 10min after the resume; the 5min pause and the
	// dead kinds add nothing.
	if want := int64(40 * 60); r.Totals.ListeningSec != want {
		t.Errorf("listening: got %d want %d", r.Totals.ListeningSec, want)
	}
}
