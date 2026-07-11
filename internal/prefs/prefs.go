// Package prefs appends explicit taste signals (like / dislike on the current
// track) to a local JSONL file, one JSON object per line. It mirrors histlog:
// zero new dependencies, greppable, versioned, best-effort writes that must
// never affect playback.
//
// Consent model (see the "Privacy by design" section of CLAUDE.md): there is
// NO separate opt-in gate here, unlike the events log. The reason is that the
// only way an entry is ever written is a deliberate "J'aime" / "J'aime pas"
// click on the track menu. The explicit click IS the consent: the user asked
// to remember this verdict about this track. Everything else the doctrine
// requires still holds. Local only, no network ever; the file lives under the
// XDG data dir next to history.jsonl and events.jsonl; and the user can see,
// grep, edit or delete it by hand. Unlike the behaviour log (events), a taste
// signal is meaningless without the track it is about, so an entry carries the
// track identity (artist/title), the same category of data histlog already
// stores under its own consent.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the current value of the "v" field.
const SchemaVersion = 1

// Verdict values.
const (
	Like    = "like"
	Dislike = "dislike"
)

// Entry is one taste line. V versions the schema; optional fields are omitted
// when empty so the log stays lean and greppable.
type Entry struct {
	V       int       `json:"v"`
	TS      time.Time `json:"ts"`
	Verdict string    `json:"verdict"` // "like" | "dislike"
	Station string    `json:"station"`
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	Album   string    `json:"album,omitempty"`
	Year    int       `json:"year,omitempty"`
	Label   string    `json:"label,omitempty"`
	Link    string    `json:"link,omitempty"` // "listen elsewhere" link (often Apple Music), when present
}

// DefaultPath returns ~/.local/share/fipindicateur/prefs.jsonl (honoring
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
