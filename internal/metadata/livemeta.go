package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

const defaultLivemetaBase = "https://api.radiofrance.fr/livemeta/pull/"

// Poll scheduling bounds.
const (
	pollMin      = 10 * time.Second
	pollMax      = 5 * time.Minute
	pollPad      = 2 * time.Second // poll shortly after a track's declared end
	backoffStart = 30 * time.Second
	backoffMax   = 5 * time.Minute
)

// LivemetaProvider polls the Radio France livemeta API.
type LivemetaProvider struct {
	Client  *http.Client
	BaseURL string // defaults to the public endpoint
}

// NewLivemeta returns a provider with sane defaults.
func NewLivemeta() *LivemetaProvider {
	return &LivemetaProvider{
		Client:  &http.Client{Timeout: 15 * time.Second},
		BaseURL: defaultLivemetaBase,
	}
}

// livemetaResponse mirrors the parts of the API we consume.
type livemetaResponse struct {
	Steps  map[string]livemetaStep `json:"steps"`
	Levels []livemetaLevel         `json:"levels"`
}

type livemetaLevel struct {
	Items    []string `json:"items"`
	Position int      `json:"position"`
}

// livemetaStep is one entry in the "steps" map. It carries both the song grain
// (a track: title, authors, album, ...) and the shallower "expression" grain (a
// programme: conceptUuid, titleConcept, description). embedType tells the two
// apart. Radio France has reshaped this payload twice in ten years, so every
// field the app consumes is decoded here, behind this single parse boundary,
// and nothing structural is dropped: adding a use later needs no re-capture.
type livemetaStep struct {
	// Song (track) fields.
	Title        string   `json:"title"`
	Authors      string   `json:"authors"`
	Performers   string   `json:"performers"`
	Highlighted  []string `json:"highlightedArtists"`
	TitreAlbum   string   `json:"titreAlbum"`
	Year         int      `json:"anneeEditionMusique"`
	Label        string   `json:"label"`
	Composers    string   `json:"composers"`
	SongID       string   `json:"songId"`
	ReleaseID    string   `json:"releaseId"`
	CoverUUID    string   `json:"coverUuid"`
	Visual       string   `json:"visual"`
	YouTube      string   `json:"lienYoutube"`
	YouTubeThumb string   `json:"visuelYoutube"`

	// Structural fields (both grains).
	UUID      string `json:"uuid"`
	StepID    string `json:"stepId"`
	EmbedType string `json:"embedType"` // "expression" (a show) or "song"
	Depth     int    `json:"depth"`     // 1 = expression, 3 = song
	// FatherStepID points a song at the expression step of the show it plays
	// within; it is null (empty) for a song outside any show.
	FatherStepID string `json:"fatherStepId"`
	StationID    int    `json:"stationId"`
	DiscJockey   string `json:"discJockey"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	TitleSlug    string `json:"titleSlug"`
	Path         string `json:"path"`

	// Expression (show) fields.
	ExpressionUUID    string `json:"expressionUuid"` // per-episode identity
	ConceptUUID       string `json:"conceptUuid"`    // stable recurring-show identity
	TitleConcept      string `json:"titleConcept"`   // show name without the date
	Description       string `json:"description"`    // recurring-show description
	ExpressionDesc    string `json:"expressionDescription"`
	BusinessReference string `json:"businessReference"`
	MagnetothequeID   string `json:"magnetothequeId"`
}

// isExpression reports whether the step is a programme rather than a song.
func (s livemetaStep) isExpression() bool { return s.EmbedType == "expression" }

// parseLivemeta extracts the current NowPlaying from a raw livemeta payload.
//
// The track lives in the deepest grain. Historically every station returned a
// single level of songs; the main antenna (station 7) now nests a shallower
// "expression" (show) grain above the songs, so it returns two levels. Reading
// the last level yields the song in both shapes, and the earlier level, when
// present, carries the programme schedule.
func parseLivemeta(data []byte) (NowPlaying, error) {
	var resp livemetaResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NowPlaying{}, fmt.Errorf("livemeta: decode: %w", err)
	}
	if len(resp.Levels) == 0 {
		return NowPlaying{}, fmt.Errorf("livemeta: no levels")
	}
	lvl := resp.Levels[len(resp.Levels)-1] // deepest grain: the song
	if lvl.Position < 0 || lvl.Position >= len(lvl.Items) {
		return NowPlaying{}, fmt.Errorf("livemeta: position %d out of range (%d items)", lvl.Position, len(lvl.Items))
	}
	key := lvl.Items[lvl.Position]
	step, ok := resp.Steps[key]
	if !ok {
		return NowPlaying{}, fmt.Errorf("livemeta: step %q not found", key)
	}

	// Some programmes (a continuous DJ mixtape like "Fip Tape") expose no song
	// grain at all: even the deepest level's current item is the expression
	// itself. Presenting it as a track would show a phantom song (the show name,
	// with an empty artist), so instead we surface it as the current Show and
	// leave the track empty. The real track title then comes from the ICY
	// fallback, which the manager keeps arbitrated against this show-only
	// livemeta so the programme stays on air in the menu (see manager.go).
	if step.isExpression() {
		sh := stepToShow(step)
		np := NowPlaying{
			Show:          &sh,
			UpcomingShows: upcomingShows(resp),
		}
		if step.Start > 0 {
			np.Start = time.Unix(step.Start, 0)
		}
		if step.End > 0 {
			np.End = time.Unix(step.End, 0)
		}
		return np, nil
	}

	artist := step.Authors
	if artist == "" {
		artist = step.Performers
	}

	np := NowPlaying{
		Artist:        artist,
		PrimaryArtist: primaryArtist(step.Highlighted, artist),
		Title:         step.Title,
		Album:         step.TitreAlbum,
		Label:         step.Label,
		Year:          step.Year,
		CoverURL:      step.Visual,
		Link:          step.Path,
		Show:          currentShow(resp, step),
		UpcomingShows: upcomingShows(resp),
	}
	if step.Start > 0 {
		np.Start = time.Unix(step.Start, 0)
	}
	if step.End > 0 {
		np.End = time.Unix(step.End, 0)
	}
	return np, nil
}

// stepToShow projects an expression step into a Show. The display Title prefers
// the date-free titleConcept and falls back to the raw title (future steps
// carry no titleConcept but a date-free title).
func stepToShow(s livemetaStep) Show {
	title := strings.TrimSpace(s.TitleConcept)
	if title == "" {
		title = strings.TrimSpace(s.Title)
	}
	sh := Show{
		ConceptUUID: s.ConceptUUID,
		Title:       title,
		Description: s.Description,
		Path:        s.Path,
	}
	if s.Start > 0 {
		sh.Start = time.Unix(s.Start, 0)
	}
	if s.End > 0 {
		sh.End = time.Unix(s.End, 0)
	}
	return sh
}

// currentShow resolves the programme the current track plays within, or nil.
// A track inside a show points at its expression step through fatherStepId;
// a track outside any show (the normal rotation, or any webradio) has none, so
// this returns nil. fatherStepId is the authoritative signal: the current
// expression grain and the father always agree on the main antenna, and keying
// off the father is what keeps "between shows" honestly show-less.
func currentShow(resp livemetaResponse, song livemetaStep) *Show {
	if song.FatherStepID == "" {
		return nil
	}
	parent, ok := resp.Steps[song.FatherStepID]
	if !ok || !parent.isExpression() {
		return nil
	}
	sh := stepToShow(parent)
	return &sh
}

// upcomingShows collects the expression steps scheduled after the current
// position across every level (only the expression grain holds expressions, so
// song levels contribute nothing), de-duplicated by stepId and ordered by start
// time. Empty on the webradios, which carry no expression grain.
func upcomingShows(resp livemetaResponse) []Show {
	var out []Show
	seen := map[string]bool{}
	for _, lvl := range resp.Levels {
		if lvl.Position < 0 {
			continue
		}
		for i := lvl.Position + 1; i < len(lvl.Items); i++ {
			s, ok := resp.Steps[lvl.Items[i]]
			if !ok || !s.isExpression() {
				continue
			}
			if s.StepID != "" {
				if seen[s.StepID] {
					continue
				}
				seen[s.StepID] = true
			}
			out = append(out, stepToShow(s))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// fetch retrieves and parses the current track for a station id.
func (p *LivemetaProvider) fetch(ctx context.Context, id int) (NowPlaying, error) {
	url := fmt.Sprintf("%s%d", p.BaseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return NowPlaying{}, err
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return NowPlaying{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NowPlaying{}, fmt.Errorf("livemeta: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return NowPlaying{}, err
	}
	return parseLivemeta(body)
}

// nextDelay computes the delay until the next poll from a track's end time,
// clamped to [pollMin, pollMax].
func nextDelay(end time.Time, now time.Time) time.Duration {
	if end.IsZero() {
		return pollMin
	}
	d := end.Sub(now) + pollPad
	if d < pollMin {
		return pollMin
	}
	if d > pollMax {
		return pollMax
	}
	return d
}

// Watch implements Provider. It emits an update on each successful poll and
// schedules the next poll near the current track's end; on error it backs off
// exponentially and keeps trying, never crashing.
func (p *LivemetaProvider) Watch(ctx context.Context, station stations.Station) <-chan NowPlaying {
	out := make(chan NowPlaying, 1)
	go func() {
		defer close(out)
		if station.MetaID == 0 {
			<-ctx.Done() // nothing to poll; ICY handles these
			return
		}
		backoff := backoffStart
		delay := time.Duration(0) // fetch immediately on first pass
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			np, err := p.fetch(ctx, station.MetaID)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("metadata: livemeta %s error: %v (backoff %s)", station.Key, err, backoff)
				delay = backoff
				backoff *= 2
				if backoff > backoffMax {
					backoff = backoffMax
				}
				continue
			}
			backoff = backoffStart
			select {
			case out <- np:
			case <-ctx.Done():
				return
			}
			delay = nextDelay(np.End, time.Now())
		}
	}()
	return out
}
