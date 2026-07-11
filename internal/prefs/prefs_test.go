package prefs

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.jsonl")

	e1 := Entry{
		Verdict: Like,
		Station: "fip",
		Artist:  "Quincy Jones",
		Title:   "Candy man",
		Album:   "You've got it bad girl",
		Year:    1973,
		Label:   "A&M",
		Link:    "https://music.apple.com/candy-man",
	}
	e2 := Entry{
		TS:      time.Date(2026, 7, 7, 13, 0, 0, 0, time.UTC),
		Verdict: Dislike,
		Station: "jazz",
		Artist:  "Earl Hines",
		Title:   "You can depend on me",
	}

	if err := Append(path, e1); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := Append(path, e2); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Each line is a standalone JSON object (JSONL contract), versioned.
	var got1, got2 Entry
	if err := json.Unmarshal([]byte(lines[0]), &got1); err != nil {
		t.Fatalf("line 1 not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &got2); err != nil {
		t.Fatalf("line 2 not valid JSON: %v", err)
	}

	if got1.V != SchemaVersion || got2.V != SchemaVersion {
		t.Errorf("schema version: got %d/%d want %d", got1.V, got2.V, SchemaVersion)
	}
	if got1.TS.IsZero() {
		t.Error("TS should be auto-filled when unset")
	}
	if !got2.TS.Equal(e2.TS) {
		t.Errorf("TS should be preserved: got %s", got2.TS)
	}
	if got1.Verdict != Like || got1.Artist != "Quincy Jones" || got1.Title != "Candy man" || got1.Year != 1973 {
		t.Errorf("entry 1 mismatch: %+v", got1)
	}
	if got1.Label != "A&M" || got1.Link == "" || got1.Album == "" {
		t.Errorf("entry 1 optional fields mismatch: %+v", got1)
	}
	if got2.Verdict != Dislike {
		t.Errorf("entry 2 verdict mismatch: %+v", got2)
	}

	// Optional fields absent when empty (schema stays lean).
	if strings.Contains(lines[1], "album") || strings.Contains(lines[1], "label") ||
		strings.Contains(lines[1], "year") || strings.Contains(lines[1], "link") {
		t.Errorf("empty optional fields should be omitted: %s", lines[1])
	}

	// No rotation, no truncation: a third append only grows the file.
	before, _ := os.Stat(path)
	if err := Append(path, e1); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if after.Size() <= before.Size() {
		t.Error("append should grow the file")
	}
}

func TestAppendBadPath(t *testing.T) {
	err := Append(filepath.Join(t.TempDir(), "no", "such", "dir", "p.jsonl"), Entry{Title: "x"})
	if err == nil {
		t.Error("expected error on unwritable path")
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "fipindicateur", "prefs.jsonl")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}
