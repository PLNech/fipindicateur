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

// VolumeStatus is the device-side volume as reported by RECEIVER_STATUS.
// ControlType matters: "master" (e.g. an AV receiver) means the level IS the
// amplifier's master volume, so callers treat it with respect (display the
// device's own level, never push a default). StepInterval is the device's
// volume granularity; SetVolume quantizes to it.
type VolumeStatus struct {
	Level        float64
	Muted        bool
	StepInterval float64
	ControlType  string
}

// setVolumeRequest is {"type":"SET_VOLUME","volume":{...}} on the receiver
// namespace. Level and Muted are pointers so each request carries exactly one
// field: devices reject or misread a combined write less predictably.
type setVolumeRequest struct {
	Type      string        `json:"type"`
	RequestID int           `json:"requestId"`
	Volume    volumePayload `json:"volume"`
}

type volumePayload struct {
	Level *float64 `json:"level,omitempty"`
	Muted *bool    `json:"muted,omitempty"`
}

func setVolumeLevelPayload(reqID int, level float64) string {
	return mustJSON(setVolumeRequest{Type: "SET_VOLUME", RequestID: reqID, Volume: volumePayload{Level: &level}})
}

func setVolumeMutedPayload(reqID int, muted bool) string {
	return mustJSON(setVolumeRequest{Type: "SET_VOLUME", RequestID: reqID, Volume: volumePayload{Muted: &muted}})
}

// mediaCommandRequest is PLAY/PAUSE on the media namespace, addressed to the
// transport and scoped to the media session learned from MEDIA_STATUS.
type mediaCommandRequest struct {
	Type           string `json:"type"`
	RequestID      int    `json:"requestId"`
	MediaSessionID int    `json:"mediaSessionId"`
}

func mediaCommandPayload(reqID int, typ string, mediaSessionID int) string {
	return mustJSON(mediaCommandRequest{Type: typ, RequestID: reqID, MediaSessionID: mediaSessionID})
}

// quantizeLevel snaps a 0..1 level to the device's stepInterval grid (and
// clamps). A zero/absent step means the device declared no grid: pass through.
func quantizeLevel(level, step float64) float64 {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	if step <= 0 {
		return level
	}
	q := float64(int(level/step+0.5)) * step
	if q > 1 {
		q = 1
	}
	return q
}

// parseReceiverVolume extracts the volume block of a RECEIVER_STATUS payload.
// ok is false when the payload carries no volume (e.g. an app-list-only
// status), so a caller never overwrites known state with zeroes.
func parseReceiverVolume(payload string) (VolumeStatus, bool) {
	var st struct {
		Status struct {
			Volume *struct {
				Level        *float64 `json:"level"`
				Muted        bool     `json:"muted"`
				StepInterval float64  `json:"stepInterval"`
				ControlType  string   `json:"controlType"`
			} `json:"volume"`
		} `json:"status"`
	}
	if json.Unmarshal([]byte(payload), &st) != nil || st.Status.Volume == nil || st.Status.Volume.Level == nil {
		return VolumeStatus{}, false
	}
	v := st.Status.Volume
	return VolumeStatus{Level: *v.Level, Muted: v.Muted, StepInterval: v.StepInterval, ControlType: v.ControlType}, true
}

// parseMediaStatus extracts the media session id and player state from a
// MEDIA_STATUS payload. The status array can be empty (session ended): ok is
// false then, and the caller keeps its last-known id.
func parseMediaStatus(payload string) (mediaSessionID int, playerState string, ok bool) {
	var st struct {
		Status []struct {
			MediaSessionID int    `json:"mediaSessionId"`
			PlayerState    string `json:"playerState"`
		} `json:"status"`
	}
	if json.Unmarshal([]byte(payload), &st) != nil || len(st.Status) == 0 {
		return 0, "", false
	}
	return st.Status[0].MediaSessionID, st.Status[0].PlayerState, true
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
