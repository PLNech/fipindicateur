package cast

import (
	"encoding/json"
	"testing"
)

// decode is a test helper: every payload must be a valid JSON object.
func decode(t *testing.T, payload string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("payload is not valid JSON: %v\n%s", err, payload)
	}
	return m
}

func TestLaunchPayload(t *testing.T) {
	m := decode(t, launchPayload(7))
	if m["type"] != "LAUNCH" || m["appId"] != "CC1AD845" || m["requestId"] != float64(7) {
		t.Fatalf("launch payload wrong: %v", m)
	}
}

func TestLoadPayload(t *testing.T) {
	url := `https://icecast.radiofrance.fr/fiprock-midfi.mp3?id=radiofrance`
	m := decode(t, loadPayload(3, url, "audio/mpeg", `FIP · Sacré français !`))
	if m["type"] != "LOAD" || m["requestId"] != float64(3) || m["autoplay"] != true {
		t.Fatalf("load envelope wrong: %v", m)
	}
	media, _ := m["media"].(map[string]any)
	if media == nil {
		t.Fatal("load payload has no media object")
	}
	if media["contentId"] != url {
		t.Errorf("contentId = %v, want the stream URL untouched", media["contentId"])
	}
	if media["contentType"] != "audio/mpeg" || media["streamType"] != "LIVE" {
		t.Errorf("contentType/streamType wrong: %v", media)
	}
	meta, _ := media["metadata"].(map[string]any)
	if meta == nil || meta["metadataType"] != float64(3) || meta["title"] != "FIP · Sacré français !" || meta["artist"] != "Radio France" {
		t.Errorf("metadata wrong (accents and middot must survive JSON escaping): %v", meta)
	}
}

func TestStopPayload(t *testing.T) {
	m := decode(t, stopPayload(9, "sess-42"))
	if m["type"] != "STOP" || m["sessionId"] != "sess-42" || m["requestId"] != float64(9) {
		t.Fatalf("stop payload wrong: %v", m)
	}
}

func TestPayloadType(t *testing.T) {
	if got := payloadType(`{"type":"PING"}`); got != "PING" {
		t.Errorf("payloadType = %q, want PING", got)
	}
	if got := payloadType(`not json at all`); got != "" {
		t.Errorf("payloadType on garbage = %q, want empty", got)
	}
}

func TestFindDefaultReceiver(t *testing.T) {
	status := `{"type":"RECEIVER_STATUS","requestId":1,"status":{"applications":[` +
		`{"appId":"E8C28D3C","sessionId":"idle","transportId":"idle-t"},` +
		`{"appId":"CC1AD845","sessionId":"s-1","transportId":"t-1","displayName":"Default Media Receiver"}` +
		`],"volume":{"level":0.5}}}`
	sess, transport, ok := findDefaultReceiver(status)
	if !ok || sess != "s-1" || transport != "t-1" {
		t.Fatalf("findDefaultReceiver = %q %q %v, want s-1 t-1 true", sess, transport, ok)
	}

	// A status without the app (e.g. the backdrop only) is a keep-waiting.
	if _, _, ok := findDefaultReceiver(`{"type":"RECEIVER_STATUS","status":{"applications":[{"appId":"E8C28D3C","transportId":"x"}]}}`); ok {
		t.Fatal("found the Default Media Receiver where none runs")
	}
	if _, _, ok := findDefaultReceiver(`garbage`); ok {
		t.Fatal("found a receiver in garbage")
	}
}
