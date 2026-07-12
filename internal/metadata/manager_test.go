package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/stations"
)

// TestManagerShowOnlyEnrichesICY verifies the Fip Tape arbitration: when
// livemeta reports a show with no song grain, the ICY fallback supplies the
// track title and inherits livemeta's Show, so the programme stays on air while
// the real track is displayed (and its history entry carries the show tag).
func TestManagerShowOnlyEnrichesICY(t *testing.T) {
	data, err := os.ReadFile("testdata/livemeta_fiptape_one_level.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	m := &Manager{
		livemeta: &LivemetaProvider{Client: srv.Client(), BaseURL: srv.URL + "/"},
		icy:      NewICY(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := m.Watch(ctx, stations.Station{Key: "fip", MetaID: 7})

	// First: the show-only livemeta poll (empty track, show set).
	select {
	case np := <-out:
		if !np.Empty() || np.Show == nil {
			t.Fatalf("first update should be the show-only livemeta poll: %+v", np)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the livemeta show-only update")
	}

	// A track title arrives over ICY: it should be emitted and carry the show.
	m.PushTitle("Some Artist - Some Track")
	select {
	case np := <-out:
		if np.Title != "Some Track" || np.Artist != "Some Artist" {
			t.Fatalf("expected the ICY track, got title %q artist %q", np.Title, np.Artist)
		}
		if np.Show == nil {
			t.Fatal("the ICY track should inherit livemeta's Show during a show-only broadcast")
		}
		if np.Show.Title != "Fip Tape" {
			t.Errorf("grafted show: got %q want Fip Tape", np.Show.Title)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the enriched ICY update")
	}
}

// TestManagerICYNotUsedWhenLivemetaHasTrack confirms the normal case is intact:
// with fresh livemeta carrying a real track, ICY titles are suppressed (no
// stale takeover, no spurious show grafting).
func TestManagerICYNotUsedWhenLivemetaHasTrack(t *testing.T) {
	// Two-level payload with a real song inside a show.
	body := `{"steps":{
		"e1":{"embedType":"expression","stepId":"e1","conceptUuid":"c","titleConcept":"Show","start":1,"end":9999999999},
		"s1":{"embedType":"song","stepId":"s1","fatherStepId":"e1","title":"Real","authors":"Band","start":1,"end":9999999999}
	},"levels":[{"items":["e1"],"position":0},{"items":["s1"],"position":0}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m := &Manager{
		livemeta: &LivemetaProvider{Client: srv.Client(), BaseURL: srv.URL + "/"},
		icy:      NewICY(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := m.Watch(ctx, stations.Station{Key: "fip", MetaID: 7})

	select {
	case np := <-out:
		if np.Title != "Real" {
			t.Fatalf("expected the livemeta track, got %q", np.Title)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the livemeta track")
	}

	// ICY title must be ignored: livemeta is fresh and already has the track.
	m.PushTitle("Intruder - Nope")
	select {
	case np := <-out:
		t.Fatalf("ICY should be suppressed while livemeta has a fresh track, got %+v", np)
	case <-time.After(300 * time.Millisecond):
		// expected: no emission
	}
}
