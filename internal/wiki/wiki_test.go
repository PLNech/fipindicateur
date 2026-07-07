package wiki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stub returns an httptest server answering opensearch with the given URL
// list (empty = no hit).
func stub(t *testing.T, urls string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "opensearch" {
			http.Error(w, "bad action", 400)
			return
		}
		q := r.URL.Query().Get("search")
		_, _ = w.Write([]byte(`["` + q + `",["T"],["D"],[` + urls + `]]`))
	}))
}

func resolver(endpoints ...string) *Resolver {
	return &Resolver{Client: http.DefaultClient, Endpoints: endpoints}
}

func TestArtistURLFirstEndpointHit(t *testing.T) {
	fr := stub(t, `"https://fr.wikipedia.org/wiki/Nina_Simone"`)
	defer fr.Close()
	en := stub(t, `"https://en.wikipedia.org/wiki/NEVER"`)
	defer en.Close()

	got := resolver(fr.URL, en.URL).ArtistURL(context.Background(), "Nina Simone")
	if got != "https://fr.wikipedia.org/wiki/Nina_Simone" {
		t.Errorf("got %q", got)
	}
}

func TestArtistURLFallsBackToSecondEndpoint(t *testing.T) {
	fr := stub(t, ``) // no fr hit
	defer fr.Close()
	en := stub(t, `"https://en.wikipedia.org/wiki/Niche_Artist"`)
	defer en.Close()

	got := resolver(fr.URL, en.URL).ArtistURL(context.Background(), "Niche Artist")
	if got != "https://en.wikipedia.org/wiki/Niche_Artist" {
		t.Errorf("got %q", got)
	}
}

func TestArtistURLFallsBackToSearchPage(t *testing.T) {
	fr := stub(t, ``)
	defer fr.Close()
	en := stub(t, ``)
	defer en.Close()

	got := resolver(fr.URL, en.URL).ArtistURL(context.Background(), "Café Tacvba")
	want := SearchPageURL("Café Tacvba")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if !strings.Contains(got, "fr.wikipedia.org/w/index.php?search=") {
		t.Errorf("fallback must be the fr search page: %q", got)
	}
}

func TestArtistURLNetworkErrorFallsThrough(t *testing.T) {
	// A dead endpoint (closed server) must not error out: straight to the
	// next endpoint, then the search page.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close() // now refuses connections
	en := stub(t, `"https://en.wikipedia.org/wiki/Resilient"`)
	defer en.Close()

	got := resolver(dead.URL, en.URL).ArtistURL(context.Background(), "Resilient")
	if got != "https://en.wikipedia.org/wiki/Resilient" {
		t.Errorf("got %q", got)
	}

	// All endpoints dead: the search page, never an empty string.
	got = resolver(dead.URL).ArtistURL(context.Background(), "Anyone")
	if got != SearchPageURL("Anyone") {
		t.Errorf("got %q", got)
	}
}

func TestArtistURLGarbageResponse(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"not":"opensearch"}`))
	}))
	defer bad.Close()

	got := resolver(bad.URL).ArtistURL(context.Background(), "X")
	if got != SearchPageURL("X") {
		t.Errorf("got %q", got)
	}
}
