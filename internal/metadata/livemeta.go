package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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

type livemetaStep struct {
	Title      string `json:"title"`
	Authors    string `json:"authors"`
	Performers string `json:"performers"`
	TitreAlbum string `json:"titreAlbum"`
	Year       int    `json:"anneeEditionMusique"`
	Label      string `json:"label"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Visual     string `json:"visual"`
	Path       string `json:"path"`
}

// parseLivemeta extracts the current NowPlaying from a raw livemeta payload.
func parseLivemeta(data []byte) (NowPlaying, error) {
	var resp livemetaResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NowPlaying{}, fmt.Errorf("livemeta: decode: %w", err)
	}
	if len(resp.Levels) == 0 {
		return NowPlaying{}, fmt.Errorf("livemeta: no levels")
	}
	lvl := resp.Levels[0]
	if lvl.Position < 0 || lvl.Position >= len(lvl.Items) {
		return NowPlaying{}, fmt.Errorf("livemeta: position %d out of range (%d items)", lvl.Position, len(lvl.Items))
	}
	key := lvl.Items[lvl.Position]
	step, ok := resp.Steps[key]
	if !ok {
		return NowPlaying{}, fmt.Errorf("livemeta: step %q not found", key)
	}

	artist := step.Authors
	if artist == "" {
		artist = step.Performers
	}

	np := NowPlaying{
		Artist:   artist,
		Title:    step.Title,
		Album:    step.TitreAlbum,
		Label:    step.Label,
		Year:     step.Year,
		CoverURL: step.Visual,
		Link:     step.Path,
	}
	if step.Start > 0 {
		np.Start = time.Unix(step.Start, 0)
	}
	if step.End > 0 {
		np.End = time.Unix(step.End, 0)
	}
	return np, nil
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
