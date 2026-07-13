package stats

import (
	"sort"

	"github.com/PLNech/fipindicateur/internal/histlog"
	"github.com/PLNech/fipindicateur/internal/prefs"
	"github.com/PLNech/fipindicateur/internal/stations"
)

// Discotheque is the searchable listening archive: one row per histlog track
// line, reverse-chronological, joined with the optional enrichment (genres,
// country per raw artist name) and the explicit taste verdicts (prefs). Unlike
// the aggregate blocks it exposes the raw plays, so the report can offer a
// fulltext search over everything that was ever heard. It is nil (omitted) when
// the history is empty, so an events-only report collapses the section.
type Discotheque struct {
	Rows     []DiscoRow `json:"rows"`     // reverse-chronological (newest first)
	Plays    int        `json:"plays"`    // total play rows (len(Rows))
	Distinct int        `json:"distinct"` // distinct tracks (artist + title)
	// GeneratedAt is the enrichment's own generated_at passthrough (empty when
	// enriched.json is absent), and Unenriched counts the rows that ended up
	// with no genre. Together they drive the staleness caption ("enrichi il y a
	// N jours, X ecoutes sans genre").
	GeneratedAt string `json:"generatedAt,omitempty"`
	Unenriched  int    `json:"unenriched"`
}

// DiscoRow is one heard track: the histlog line plus the joined-in genres and
// country (from enrichment, artist-level) and the liked/disliked verdict (from
// prefs). Optional fields are omitempty so the JSON stays lean over a long
// history.
type DiscoRow struct {
	TS       string   `json:"ts"`
	Station  string   `json:"station"` // display name (e.g. "FIP Jazz")
	Artist   string   `json:"artist"`
	Title    string   `json:"title"`
	Album    string   `json:"album,omitempty"`
	Year     int      `json:"year,omitempty"`
	Label    string   `json:"label,omitempty"`
	Link     string   `json:"link,omitempty"`  // "listen elsewhere" URL, when present
	Cover    string   `json:"cover,omitempty"` // cover-art URL, when present
	Show     string   `json:"show,omitempty"`  // programme display name, when inside a show
	Genres   []string `json:"genres,omitempty"`
	Country  string   `json:"country,omitempty"`
	Liked    bool     `json:"liked,omitempty"`
	Disliked bool     `json:"disliked,omitempty"`
}

// buildDiscotheque turns the histlog into the searchable archive. Each row is a
// play; genres/country come from enr (keyed by the RAW artist string, exactly
// as histlog stores it), and liked/disliked from the most recent prefs verdict
// for the (station, artist, title) triple. Returns nil when there is no history
// at all, so the section collapses like the other companion blocks.
func buildDiscotheque(hist []histlog.Entry, prf []prefs.Entry, enr *Enriched) *Discotheque {
	if len(hist) == 0 {
		return nil
	}

	// Latest verdict per (station, artist, title): a later click supersedes an
	// earlier one (a user can change their mind about a track).
	type vkey struct{ station, artist, title string }
	verdicts := map[vkey]string{}
	verdictTS := map[vkey]int64{}
	for _, p := range prf {
		k := vkey{p.Station, p.Artist, p.Title}
		if ts := p.TS.UnixNano(); ts >= verdictTS[k] {
			verdictTS[k] = ts
			verdicts[k] = p.Verdict
		}
	}

	sorted := make([]histlog.Entry, len(hist))
	copy(sorted, hist)
	// Reverse-chronological: newest first. Stable so equal timestamps keep their
	// original (append) order deterministically.
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TS.After(sorted[j].TS) })

	rows := make([]DiscoRow, 0, len(sorted))
	distinct := map[string]bool{}
	unenriched := 0
	for _, e := range sorted {
		distinct[e.Artist+"\x00"+e.Title] = true

		row := DiscoRow{
			TS:      e.TS.Format("2006-01-02T15:04:05Z07:00"),
			Station: stationDisplay(e.Station),
			Artist:  e.Artist,
			Title:   e.Title,
			Album:   e.Album,
			Year:    e.Year,
			Label:   e.Label,
			Link:    e.Link,
			Cover:   e.Cover,
			Show:    e.Show,
		}

		if enr != nil {
			if a, ok := enr.Artists[e.Artist]; ok {
				if len(a.Genres) > 0 {
					row.Genres = a.Genres
				}
				row.Country = a.Country
			}
		}
		if len(row.Genres) == 0 {
			unenriched++
		}

		switch verdicts[vkey{e.Station, e.Artist, e.Title}] {
		case prefs.Like:
			row.Liked = true
		case prefs.Dislike:
			row.Disliked = true
		}

		rows = append(rows, row)
	}

	d := &Discotheque{
		Rows:       rows,
		Plays:      len(rows),
		Distinct:   len(distinct),
		Unenriched: unenriched,
	}
	if enr != nil {
		d.GeneratedAt = enr.GeneratedAt
	}
	return d
}

// stationDisplay resolves a station key to its display name, falling back to
// the raw key so a row is never blank for an unknown station.
func stationDisplay(key string) string {
	if key == "" {
		return ""
	}
	if d := stations.ByKey(key).Display; d != "" {
		return d
	}
	return key
}
