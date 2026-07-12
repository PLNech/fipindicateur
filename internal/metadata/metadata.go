// Package metadata resolves the currently playing track for a FIP station.
//
// Two providers implement the Provider interface: Livemeta (primary, polls
// Radio France's public livemeta API) and ICY (fallback, parses the icecast
// stream title observed by the player). Metadata failure must never affect
// playback: providers back off on error and never panic.
package metadata

import (
	"context"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

// NowPlaying describes the current track.
type NowPlaying struct {
	Artist string // full credit string, for display (may list many names)
	// PrimaryArtist is the cleaned single-artist name used for link
	// resolution (Wikipedia lookup): livemeta's curated highlightedArtists[0]
	// when present, else the credit string cut at the first separator.
	PrimaryArtist string
	Title         string
	Album         string
	Label         string
	Year          int
	CoverURL      string
	Link          string
	Start         time.Time
	End           time.Time

	// Show is the programme ("émission") currently on air, or nil. It is set
	// only when the current track plays within a show (its livemeta step
	// declares a parent expression step). Shows exist only on the main antenna
	// (station 7); the webradios never carry one, so this stays nil there.
	Show *Show
	// UpcomingShows are the programmes scheduled after the current position, in
	// chronological order. A single livemeta poll returns two to three days of
	// programming ahead, so this is usually populated on the main antenna and
	// always empty on the webradios.
	UpcomingShows []Show
}

// Show is a FIP programme ("émission"): a named, recurring editorial broadcast
// (for example "Club Jazzafip") that brackets a span of the schedule and, over
// its run, plays songs credited to it. Unlike a track it recurs across dates
// under a stable ConceptUUID, which is the key to aggregate airings of the same
// show heard on different nights.
type Show struct {
	// ConceptUUID is the stable identity of the recurring show across dates
	// (the aggregation key). Title and the dated title change nightly; this
	// does not.
	ConceptUUID string
	// Title is the display name. It is the show's date-free titleConcept when
	// the API provides it (current and past steps), falling back to the raw
	// title (future steps carry no titleConcept but a date-free title).
	Title       string
	Description string
	Path        string
	Start       time.Time
	End         time.Time
}

// Empty reports whether there is no useful track information.
func (n NowPlaying) Empty() bool {
	return n.Artist == "" && n.Title == ""
}

// Provider streams NowPlaying updates for a station until ctx is cancelled.
type Provider interface {
	Watch(ctx context.Context, station stations.Station) <-chan NowPlaying
}
