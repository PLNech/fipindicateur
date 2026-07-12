// Package histlog appends track changes to a local JSONL file, one JSON
// object per line. Opt-in (default off). JSONL over SQLite: zero new
// dependencies, greppable, and the schema extends naturally when
// likes/dislikes arrive later. Writes are best-effort: an error is returned
// for logging but must never affect playback.
package histlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the current value of the "v" field.
const SchemaVersion = 1

// Entry is one history line. V versions the schema.
type Entry struct {
	V       int       `json:"v"`
	TS      time.Time `json:"ts"`
	Station string    `json:"station"`
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	Album   string    `json:"album,omitempty"`
	Year    int       `json:"year,omitempty"`
	Label   string    `json:"label,omitempty"`
	Link    string    `json:"link,omitempty"`  // "listen elsewhere" link (often Apple Music), when Radio France provides one
	Cover   string    `json:"cover,omitempty"` // cover-art URL, when present
	// Show and ShowConcept name the programme ("émission") this track played
	// within, on the main antenna. Show is the display name (date-free);
	// ShowConcept is the stable conceptUuid used to aggregate airings of the
	// same recurring show across nights. Both omitempty and absent from a track
	// outside any show or on a webradio: older logs without them read back with
	// empty strings, so the schema grows without breaking existing files.
	Show        string `json:"show,omitempty"`
	ShowConcept string `json:"show_concept,omitempty"`
}

// DefaultPath returns ~/.local/share/fipindicateur/history.jsonl (honoring
// XDG_DATA_HOME), creating the directory if needed.
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
	return filepath.Join(dir, "history.jsonl"), nil
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
// means "no history yet"): it returns a nil slice. Malformed lines are skipped
// so a partially-written tail never fails the whole read. Mirrors events.Load
// and prefs.Load: the stats derivation joins this log against the behaviour log.
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
