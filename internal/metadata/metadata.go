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
	Artist   string
	Title    string
	Album    string
	Label    string
	Year     int
	CoverURL string
	Link     string
	Start    time.Time
	End      time.Time
}

// Empty reports whether there is no useful track information.
func (n NowPlaying) Empty() bool {
	return n.Artist == "" && n.Title == ""
}

// Provider streams NowPlaying updates for a station until ctx is cancelled.
type Provider interface {
	Watch(ctx context.Context, station stations.Station) <-chan NowPlaying
}
