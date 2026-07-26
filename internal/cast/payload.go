package cast

import "encoding/json"

// The CASTV2 payloads are JSON documents. Structs (rather than Sprintf) keep
// the escaping correct for stream URLs and station names.

type launchRequest struct {
	Type      string `json:"type"`
	AppID     string `json:"appId"`
	RequestID int    `json:"requestId"`
}

func launchPayload(reqID int) string {
	return mustJSON(launchRequest{Type: "LAUNCH", AppID: defaultReceiverApp, RequestID: reqID})
}

// mediaMetadata is MusicTrackMediaMetadata (metadataType 3): the device UI
// shows title/artist like a music player.
type mediaMetadata struct {
	MetadataType int    `json:"metadataType"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
}

type mediaInfo struct {
	ContentID   string        `json:"contentId"`
	ContentType string        `json:"contentType"`
	StreamType  string        `json:"streamType"`
	Metadata    mediaMetadata `json:"metadata"`
}

type loadRequest struct {
	Type      string    `json:"type"`
	RequestID int       `json:"requestId"`
	Media     mediaInfo `json:"media"`
	Autoplay  bool      `json:"autoplay"`
}

func loadPayload(reqID int, url, contentType, title string) string {
	return mustJSON(loadRequest{
		Type:      "LOAD",
		RequestID: reqID,
		Media: mediaInfo{
			ContentID:   url,
			ContentType: contentType,
			StreamType:  "LIVE", // an icecast radio stream: no seeking, no duration
			Metadata:    mediaMetadata{MetadataType: 3, Title: title, Artist: "Radio France"},
		},
		Autoplay: true,
	})
}

type stopRequest struct {
	Type      string `json:"type"`
	RequestID int    `json:"requestId"`
	SessionID string `json:"sessionId"`
}

func stopPayload(reqID int, sessionID string) string {
	return mustJSON(stopRequest{Type: "STOP", RequestID: reqID, SessionID: sessionID})
}

// payloadType extracts the "type" discriminator of an incoming JSON payload
// ("" when unparsable, which callers treat as "not for me").
func payloadType(payload string) string {
	var t struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(payload), &t)
	return t.Type
}

// findDefaultReceiver extracts the Default Media Receiver's session and
// transport ids from a RECEIVER_STATUS payload, if the app is running.
func findDefaultReceiver(payload string) (sessionID, transportID string, ok bool) {
	var st struct {
		Status struct {
			Applications []struct {
				AppID       string `json:"appId"`
				SessionID   string `json:"sessionId"`
				TransportID string `json:"transportId"`
			} `json:"applications"`
		} `json:"status"`
	}
	if json.Unmarshal([]byte(payload), &st) != nil {
		return "", "", false
	}
	for _, app := range st.Status.Applications {
		if app.AppID == defaultReceiverApp && app.TransportID != "" {
			return app.SessionID, app.TransportID, true
		}
	}
	return "", "", false
}

// mustJSON marshals a payload struct. The inputs are plain strings and ints,
// so a marshal error cannot happen in practice; "{}" is the safe fallback.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
