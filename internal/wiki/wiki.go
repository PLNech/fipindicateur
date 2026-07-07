// Package wiki resolves an artist name to a Wikipedia article URL using the
// opensearch API, preferring French (FIP's audience) and falling back to
// English (niche artists often exist only on en.wp), then to the fr search
// page, which at worst shows suggestions: never a dead end. Best-effort by
// design: any network error skips straight down the chain.
package wiki

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

// requestTimeout bounds each opensearch call so a click never feels hung.
const requestTimeout = 1500 * time.Millisecond

// Resolver looks up artists on Wikipedia. The zero value is not usable; call
// NewResolver (endpoints are a field so tests can stub them).
type Resolver struct {
	Client    *http.Client
	Endpoints []string // opensearch api.php endpoints, tried in order
}

// NewResolver returns a resolver trying fr.wikipedia then en.wikipedia.
func NewResolver() *Resolver {
	return &Resolver{
		Client: &http.Client{Timeout: requestTimeout},
		Endpoints: []string{
			"https://fr.wikipedia.org/w/api.php",
			"https://en.wikipedia.org/w/api.php",
		},
	}
}

// SearchPageURL is the guaranteed fallback: the fr.wp search page for the
// given query (shows the article if the title matches, suggestions if not).
func SearchPageURL(query string) string {
	return "https://fr.wikipedia.org/w/index.php?search=" + url.QueryEscape(query)
}

// ArtistURL resolves an artist to the best Wikipedia URL. It always returns
// a usable URL (falls back to the fr search page) and never blocks past
// len(Endpoints) x requestTimeout.
func (r *Resolver) ArtistURL(ctx context.Context, artist string) string {
	for _, ep := range r.Endpoints {
		if u := r.opensearch(ctx, ep, artist); u != "" {
			return u
		}
	}
	return SearchPageURL(artist)
}

// opensearch queries one endpoint, returning the first hit's URL or "".
func (r *Resolver) opensearch(ctx context.Context, endpoint, query string) string {
	q := url.Values{
		"action": {"opensearch"},
		"limit":  {"1"},
		"format": {"json"},
		"search": {query},
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return ""
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<18))
	if err != nil {
		return ""
	}
	// opensearch response: [query, [titles], [descriptions], [urls]]
	var payload []json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || len(payload) < 4 {
		return ""
	}
	var urls []string
	if err := json.Unmarshal(payload[3], &urls); err != nil || len(urls) == 0 {
		return ""
	}
	return urls[0]
}
