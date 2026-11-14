package popover

import (
	"encoding/json"
	"strings"
	"testing"
)

// Swift decodes Snapshot.stations as a non-optional array: a JSON null there
// makes the whole snapshot unreadable and the popover keeps a stale state.
func TestJSONNeverSendsNullStations(t *testing.T) {
	b := Snapshot{}.JSON()
	if strings.Contains(string(b), "null") {
		t.Fatalf("empty snapshot encodes a null: %s", b)
	}
	var back Snapshot
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	in := Snapshot{
		Playing: true, Station: "jazz", Title: "Sacré français !", Volume: 65,
		Stations: []Station{{Key: "jazz", Name: "Jazz", Color: "#13898d"}},
		Start:    1700000000, End: 1700000236,
	}
	var out Snapshot
	if err := json.Unmarshal(in.JSON(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Title != in.Title || out.Stations[0] != in.Stations[0] || out.End != in.End {
		t.Fatalf("round trip lost data: %+v", out)
	}
}
