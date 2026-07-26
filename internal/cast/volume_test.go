package cast

import (
	"math"
	"testing"
)

func TestSetVolumeLevelPayload(t *testing.T) {
	m := decode(t, setVolumeLevelPayload(9, 0.35))
	if m["type"] != "SET_VOLUME" || m["requestId"] != float64(9) {
		t.Fatalf("set_volume envelope wrong: %v", m)
	}
	vol, _ := m["volume"].(map[string]any)
	if vol == nil {
		t.Fatal("set_volume payload has no volume object")
	}
	if vol["level"] != 0.35 {
		t.Errorf("level = %v, want 0.35", vol["level"])
	}
	if _, has := vol["muted"]; has {
		t.Errorf("a level write must not carry muted: %v", vol)
	}
}

func TestSetVolumeMutedPayload(t *testing.T) {
	m := decode(t, setVolumeMutedPayload(4, true))
	vol, _ := m["volume"].(map[string]any)
	if vol == nil || vol["muted"] != true {
		t.Fatalf("muted payload wrong: %v", m)
	}
	if _, has := vol["level"]; has {
		t.Errorf("a mute write must not carry a level: %v", vol)
	}
}

func TestMediaCommandPayloads(t *testing.T) {
	for _, typ := range []string{"PAUSE", "PLAY"} {
		m := decode(t, mediaCommandPayload(11, typ, 42))
		if m["type"] != typ || m["requestId"] != float64(11) || m["mediaSessionId"] != float64(42) {
			t.Errorf("%s payload wrong: %v", typ, m)
		}
	}
}

func TestQuantizeLevel(t *testing.T) {
	cases := []struct {
		level, step, want float64
	}{
		{0.33, 0.02, 0.34},     // snaps to the nearest step (Pioneer reports 0.02)
		{0.5, 0.05, 0.5},       // already on the grid
		{0.987, 0.02, 0.98},    // rounds down when closer
		{1.2, 0.02, 1},         // clamped high
		{-0.3, 0.02, 0},        // clamped low
		{0.377, 0, 0.377},      // no declared step: pass through
		{0.999999, 0.02, 1.00}, // never exceeds 1 after snapping
	}
	for _, c := range cases {
		got := quantizeLevel(c.level, c.step)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("quantizeLevel(%v, %v) = %v, want %v", c.level, c.step, got, c.want)
		}
	}
}

// receiverStatusWithVolume mirrors what a Pioneer VSX-933 answers: controlType
// "master" (the level IS the amp's master volume) and a 0.02 stepInterval.
const receiverStatusWithVolume = `{"type":"RECEIVER_STATUS","requestId":2,"status":{"applications":[{"appId":"CC1AD845","sessionId":"s1","transportId":"t1"}],"volume":{"controlType":"master","level":0.34,"muted":false,"stepInterval":0.02}}}`

func TestParseReceiverVolume(t *testing.T) {
	v, ok := parseReceiverVolume(receiverStatusWithVolume)
	if !ok {
		t.Fatal("volume block not parsed")
	}
	if v.Level != 0.34 || v.Muted || v.StepInterval != 0.02 || v.ControlType != "master" {
		t.Errorf("parsed volume wrong: %+v", v)
	}
}

func TestParseReceiverVolumeAbsent(t *testing.T) {
	if _, ok := parseReceiverVolume(`{"type":"RECEIVER_STATUS","status":{"applications":[]}}`); ok {
		t.Error("a status without a volume block must not report ok")
	}
	if _, ok := parseReceiverVolume(`not json`); ok {
		t.Error("garbage must not report ok")
	}
}

func TestParseMediaStatus(t *testing.T) {
	payload := `{"type":"MEDIA_STATUS","status":[{"mediaSessionId":7,"playerState":"PLAYING","currentTime":12.3}],"requestId":0}`
	id, state, ok := parseMediaStatus(payload)
	if !ok || id != 7 || state != "PLAYING" {
		t.Errorf("parseMediaStatus = (%d, %q, %v), want (7, PLAYING, true)", id, state, ok)
	}
	if _, _, ok := parseMediaStatus(`{"type":"MEDIA_STATUS","status":[]}`); ok {
		t.Error("an empty status array must not report ok")
	}
}
