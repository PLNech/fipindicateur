package events

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withDataDir points XDG_DATA_HOME at a temp dir so the Recorder writes into
// an isolated location, and returns the resolved events.jsonl path.
func withDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	return filepath.Join(dir, "fipindicateur", "events.jsonl")
}

func TestAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")

	e1 := Event{Kind: KindPlay, Station: "fip"}
	e2 := Event{TS: time.Date(2026, 7, 8, 22, 0, 0, 0, time.UTC), Kind: KindStationChange, From: "jazz", To: "groove"}

	if err := Append(path, e1); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := Append(path, e2); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].V != SchemaVersion || got[1].V != SchemaVersion {
		t.Errorf("schema version: got %d/%d want %d", got[0].V, got[1].V, SchemaVersion)
	}
	if got[0].TS.IsZero() {
		t.Error("TS should be auto-filled when unset")
	}
	if got[0].Kind != KindPlay || got[0].Station != "fip" {
		t.Errorf("event 1 mismatch: %+v", got[0])
	}
	if got[1].From != "jazz" || got[1].To != "groove" || !got[1].TS.Equal(e2.TS) {
		t.Errorf("event 2 mismatch: %+v", got[1])
	}
}

func TestLoadMissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestLoadToleratesTornLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	_ = Append(path, Event{Kind: KindPlay})
	// Simulate a torn tail: a half-written final line, no newline.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(`{"v":1,"kind":"pa`)
	_ = f.Close()

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("torn line should be skipped, got %d events", len(got))
	}
}

func TestRecorderDisabledWritesNothing(t *testing.T) {
	path := withDataDir(t)

	r := NewRecorder(false)
	r.Record(Event{Kind: KindPlay})
	r.Record(Event{Kind: KindPause})
	r.Close()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled recorder must not create the log file (stat err: %v)", err)
	}
}

func TestRecorderEnabledWrites(t *testing.T) {
	path := withDataDir(t)

	r := NewRecorder(true)
	r.Record(Event{Kind: KindPlay, Station: "fip"})
	r.Record(Event{Kind: KindStationChange, From: "fip", To: "jazz"})
	r.Close() // flushes

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[1].To != "jazz" {
		t.Errorf("second event mismatch: %+v", got[1])
	}
}

func TestRecorderToggleAtRuntime(t *testing.T) {
	path := withDataDir(t)

	r := NewRecorder(false)
	r.Record(Event{Kind: KindPlay}) // dropped: disabled
	r.SetEnabled(true)
	r.Record(Event{Kind: KindPlay}) // kept
	r.SetEnabled(false)
	r.Record(Event{Kind: KindPause}) // dropped: disabled again
	r.Close()

	got, _ := Load(path)
	if len(got) != 1 {
		t.Fatalf("only the enabled-window event should persist, got %d", len(got))
	}
}

func TestRecorderClearRemovesOnlyEvents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	eventsPath := filepath.Join(dir, "fipindicateur", "events.jsonl")
	historyPath := filepath.Join(dir, "fipindicateur", "history.jsonl")

	r := NewRecorder(true)
	r.Record(Event{Kind: KindPlay})
	r.Close()

	// A sibling history.jsonl (different consent) must survive Clear.
	if err := os.WriteFile(historyPath, []byte("{\"title\":\"x\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r = NewRecorder(true)
	if err := r.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	r.Close()

	if _, err := os.Stat(eventsPath); !os.IsNotExist(err) {
		t.Errorf("events.jsonl should be gone after Clear (stat err: %v)", err)
	}
	if _, err := os.Stat(historyPath); err != nil {
		t.Errorf("history.jsonl must be untouched by Clear: %v", err)
	}
}

func TestClearMissingFileIsNoError(t *testing.T) {
	withDataDir(t)
	r := NewRecorder(false)
	if err := r.Clear(); err != nil {
		t.Fatalf("clearing a nonexistent log should not error: %v", err)
	}
}
