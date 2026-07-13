package ui

import (
	"encoding/json"
	"fmt"
	"testing"
)

// fakeControlApp is a controlApp double: it records the actions it received and
// returns canned data, so the line protocol is testable without a socket, tray
// or mpv.
type fakeControlApp struct {
	calls      []string
	status     Status
	stationErr error
}

func newFakeControlApp() *fakeControlApp {
	return &fakeControlApp{
		status: Status{Station: "jazz", Playing: true, Artist: "A", Title: "T", Show: "S", Volume: 42, Mute: false, Version: "vtest"},
	}
}

func (f *fakeControlApp) Play()                 { f.calls = append(f.calls, "play") }
func (f *fakeControlApp) Pause()                { f.calls = append(f.calls, "pause") }
func (f *fakeControlApp) Toggle()               { f.calls = append(f.calls, "toggle") }
func (f *fakeControlApp) ControlStatus() Status { return f.status }
func (f *fakeControlApp) ControlStation(id string) error {
	f.calls = append(f.calls, "station:"+id)
	return f.stationErr
}
func (f *fakeControlApp) StationIDs() []string { return []string{"fip", "jazz", "rock"} }

func TestHandleControl(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		want     string
		wantCall string // action expected on the app, "" for none
	}{
		{"play", "play", "ok\n", "play"},
		{"pause", "pause", "ok\n", "pause"},
		{"toggle", "toggle", "ok\n", "toggle"},
		{"station ok", "station jazz", "ok\n", "station:jazz"},
		{"station trims", "  station   rock  ", "ok\n", "station:rock"},
		{"stations", "stations", "fip\njazz\nrock\n", ""},
		{"station missing arg", "station", "err usage: station <id>\n", ""},
		{"empty", "", "err empty command\n", ""},
		{"unknown", "frobnicate", "err unknown command\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeControlApp()
			got := handleControl(f, tt.line)
			if got != tt.want {
				t.Fatalf("handleControl(%q) = %q, want %q", tt.line, got, tt.want)
			}
			if tt.wantCall != "" {
				if len(f.calls) != 1 || f.calls[0] != tt.wantCall {
					t.Fatalf("expected app call %q, got %v", tt.wantCall, f.calls)
				}
			} else if len(f.calls) != 0 {
				t.Fatalf("expected no app call, got %v", f.calls)
			}
		})
	}
}

func TestHandleControlStatusJSON(t *testing.T) {
	f := newFakeControlApp()
	got := handleControl(f, "status")
	if got == "" || got[len(got)-1] != '\n' {
		t.Fatalf("status response must be a single newline-terminated line, got %q", got)
	}
	var s Status
	if err := json.Unmarshal([]byte(got), &s); err != nil {
		t.Fatalf("status response is not valid JSON: %v (%q)", err, got)
	}
	if s != f.status {
		t.Fatalf("status roundtrip mismatch: got %+v, want %+v", s, f.status)
	}
}

func TestHandleControlUnknownStation(t *testing.T) {
	f := newFakeControlApp()
	f.stationErr = fmt.Errorf("unknown station: bogus")
	got := handleControl(f, "station bogus")
	want := "err unknown station: bogus\n"
	if got != want {
		t.Fatalf("handleControl(station bogus) = %q, want %q", got, want)
	}
}
