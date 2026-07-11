// Package prefs appends explicit taste verdicts (like / dislike) on the
// currently-playing track to a local JSONL file, one JSON object per line.
// Opt-in, local-only, no network ever, mirroring internal/histlog and
// internal/events: versioned JSONL, zero new dependencies, greppable, and
// best-effort writes that must never affect playback.
//
// prefs is a separate consent and a separate file from histlog (track changes)
// and events (behaviour): a like/dislike is an explicit, deliberate signal,
// distinct from the passive track log and from anonymous behaviour counters.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the current value of the "v" field.
const SchemaVersion = 1

// Verdict is the explicit taste signal on a track.
type Verdict string

const (
	VerdictLike    Verdict = "like"
	VerdictDislike Verdict = "dislike"
)

// Entry is one taste line. V versions the schema. Optional fields are omitted
// when empty so the log stays lean and greppable.
type Entry struct {
	V       int       `json:"v"`
	TS      time.Time `json:"ts"`
	Station string    `json:"station,omitempty"`
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	Verdict Verdict   `json:"verdict"` // "like" | "dislike"
}

// DefaultPath returns ~/.local/share/fipindicateur/prefs.jsonl (honoring
// XDG_DATA_HOME), creating the directory if needed. A separate file from
// history.jsonl and events.jsonl: separate consent, separate purpose.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	dir := filepath.Join(base, "fipindicateur")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "prefs.jsonl"), nil
}

// Append writes one entry as a JSON line to the given file, creating it if
// absent. The entry's V is forced to SchemaVersion and TS to now if unset.
func Append(path string, e Entry) error {
	e.V = SchemaVersion
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// Load reads all entries from a JSONL file. A missing file is not an error (it
// means "no verdicts yet"): it returns a nil slice. Malformed lines are skipped
// so a partially-written tail never fails the whole read.
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		line := data[start:i]
		start = i + 1
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil {
			continue // tolerate a torn final line
		}
		out = append(out, e)
	}
	if start < len(data) {
		var e Entry
		if json.Unmarshal(data[start:], &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
