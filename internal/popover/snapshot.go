// Package popover is the Go half of the macOS SwiftUI popover: the state it
// renders (Snapshot, pure and built on every platform) and, behind the
// swiftui build tag on darwin, the calls into the linked Swift object
// (internal/popover/fipui).
//
// The direction of authority is one way: Go owns the state and pushes whole
// snapshots; Swift renders the last one and reports intents by action id.
package popover

import "encoding/json"

// Station is one stop on the popover's dial.
type Station struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color"` // #rrggbb, the station's identity colour
}

// Snapshot is everything the popover shows, pushed whole on every change.
// The Swift struct of the same name (fipui/swift/Bridge.swift) mirrors it
// field for field; a field Swift does not know is ignored, a field Go omits
// keeps its Swift default.
type Snapshot struct {
	Playing  bool      `json:"playing"`
	Station  string    `json:"station"`
	Stations []Station `json:"stations"`
	Title    string    `json:"title"`
	Artist   string    `json:"artist"`
	Album    string    `json:"album"`
	Year     int       `json:"year"`
	Label    string    `json:"label"`
	Show     string    `json:"show"`
	Cover    string    `json:"cover"` // path of the cached cover PNG, "" when none yet
	Start    float64   `json:"start"` // unix seconds, 0 when unknown
	End      float64   `json:"end"`
	Volume   int       `json:"volume"` // 0..100, the active sink's
	Muted    bool      `json:"muted"`
	Verdict  int       `json:"verdict"` // -1 pas pour moi, 0 none, 1 j'aime
}

// JSON encodes the snapshot for the bridge. A nil Stations slice is sent as
// an empty list: Swift decodes a non-optional array and rejects null.
func (s Snapshot) JSON() []byte {
	if s.Stations == nil {
		s.Stations = []Station{}
	}
	b, _ := json.Marshal(s) // plain fields only: cannot fail
	return b
}
