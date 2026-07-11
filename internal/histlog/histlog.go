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
