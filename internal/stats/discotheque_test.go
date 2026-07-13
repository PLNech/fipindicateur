package stats

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/prefs"
)

// histAt builds a histlog entry at a base-relative minute offset.
func histAt(min int, station, artist, title string) histlog.Entry {
	return histlog.Entry{TS: at(min), Station: station, Artist: artist, Title: title}
}

func TestDiscothequeEmptyHistory(t *testing.T) {
	if d := buildDiscotheque(nil, nil, nil); d != nil {
		t.Errorf("empty history must collapse the block, got %+v", d)
	}
	if d := buildDiscotheque([]histlog.Entry{}, nil, nil); d != nil {
		t.Errorf("empty (non-nil) history must collapse the block, got %+v", d)
	}
}

func TestDiscothequeReverseChronologicalAndCounts(t *testing.T) {
	hist := []histlog.Entry{
		histAt(0, "fip", "Nina Simone", "Feeling Good"),
		histAt(10, "jazz", "Bill Evans", "Peace Piece"),
		histAt(20, "fip", "Nina Simone", "Feeling Good"), // repeat: same distinct track
	}
	d := buildDiscotheque(hist, nil, nil)
	if d == nil {
		t.Fatal("expected a block for a non-empty history")
	}
	if d.Plays != 3 {
		t.Errorf("plays: got %d want 3", d.Plays)
	}
	if d.Distinct != 2 {
		t.Errorf("distinct: got %d want 2", d.Distinct)
	}
	// Newest first: the at(20) repeat leads.
	if d.Rows[0].TS != at(20).Format(time.RFC3339) {
		t.Errorf("rows must be reverse-chronological: first ts %q", d.Rows[0].TS)
	}
	if d.Rows[len(d.Rows)-1].TS != at(0).Format(time.RFC3339) {
		t.Errorf("oldest row must be last: %q", d.Rows[len(d.Rows)-1].TS)
	}
	// Station display names resolved from the key.
	if d.Rows[0].Station != "FIP" {
		t.Errorf("station display: got %q want FIP", d.Rows[0].Station)
	}
}

func TestDiscothequeEnrichmentJoin(t *testing.T) {
	hist := []histlog.Entry{
		histAt(0, "fip", "Nina Simone", "Feeling Good"),
		histAt(10, "fip", "Unknown Act", "Untitled"),
	}
	enr := &Enriched{
		GeneratedAt: "2026-07-01T00:00:00Z",
		Artists: map[string]EnrichedArtist{
			"Nina Simone": {Genres: []string{"jazz", "soul"}, Country: "United States"},
		},
	}
	d := buildDiscotheque(hist, nil, enr)
	if d.GeneratedAt != "2026-07-01T00:00:00Z" {
		t.Errorf("generatedAt passthrough: got %q", d.GeneratedAt)
	}
	// Row order is newest first: index 0 is the Unknown Act (at 10), index 1 Nina.
	nina := d.Rows[1]
	if len(nina.Genres) != 2 || nina.Genres[0] != "jazz" || nina.Country != "United States" {
		t.Errorf("enriched row mismatch: %+v", nina)
	}
	unknown := d.Rows[0]
	if len(unknown.Genres) != 0 || unknown.Country != "" {
		t.Errorf("unmatched artist must carry no genre/country: %+v", unknown)
	}
	if d.Unenriched != 1 {
		t.Errorf("unenriched count: got %d want 1", d.Unenriched)
	}
}

func TestDiscothequeMissingEnrichment(t *testing.T) {
	hist := []histlog.Entry{histAt(0, "fip", "Nina Simone", "Feeling Good")}
	d := buildDiscotheque(hist, nil, nil) // nil enrichment, must degrade gracefully
	if d == nil {
		t.Fatal("history alone must still produce a block")
	}
	if d.GeneratedAt != "" {
		t.Errorf("no enrichment: generatedAt must be empty, got %q", d.GeneratedAt)
	}
	if d.Unenriched != 1 {
		t.Errorf("no enrichment: every row is unenriched, got %d want 1", d.Unenriched)
	}
	if len(d.Rows[0].Genres) != 0 {
		t.Errorf("no enrichment: rows must carry no genres, got %+v", d.Rows[0].Genres)
	}
}

func TestDiscothequePrefsJoin(t *testing.T) {
	hist := []histlog.Entry{
		histAt(0, "fip", "Nina Simone", "Feeling Good"),
		histAt(10, "jazz", "Bill Evans", "Peace Piece"),
		histAt(20, "fip", "Chilly Gonzales", "Gogol"),
	}
	prf := []prefs.Entry{
		{TS: at(1), Verdict: prefs.Like, Station: "fip", Artist: "Nina Simone", Title: "Feeling Good"},
		{TS: at(2), Verdict: prefs.Dislike, Station: "jazz", Artist: "Bill Evans", Title: "Peace Piece"},
		// A later verdict on Nina supersedes the earlier like: she ends disliked.
		{TS: at(30), Verdict: prefs.Dislike, Station: "fip", Artist: "Nina Simone", Title: "Feeling Good"},
	}
	d := buildDiscotheque(hist, prf, nil)
	byArtist := map[string]DiscoRow{}
	for _, r := range d.Rows {
		byArtist[r.Artist] = r
	}
	if !byArtist["Nina Simone"].Disliked || byArtist["Nina Simone"].Liked {
		t.Errorf("latest verdict (dislike) must win for Nina: %+v", byArtist["Nina Simone"])
	}
	if !byArtist["Bill Evans"].Disliked {
		t.Errorf("Bill Evans should be disliked: %+v", byArtist["Bill Evans"])
	}
	if byArtist["Chilly Gonzales"].Liked || byArtist["Chilly Gonzales"].Disliked {
		t.Errorf("Chilly Gonzales has no verdict: %+v", byArtist["Chilly Gonzales"])
	}
}
