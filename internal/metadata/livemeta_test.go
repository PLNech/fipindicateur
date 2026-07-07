package metadata

import (
	"os"
	"testing"
	"time"
)

func TestParseLivemetaFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/livemeta_fip.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	np, err := parseLivemeta(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if np.Title == "" {
		t.Error("expected a title")
	}
	if np.Artist == "" {
		t.Error("expected an artist")
	}
	if np.Album == "" {
		t.Error("expected an album")
	}
	if np.Year == 0 {
		t.Error("expected a year")
	}
	if np.CoverURL == "" {
		t.Error("expected a cover URL")
	}
	if np.Start.IsZero() || np.End.IsZero() {
		t.Error("expected start/end times")
	}
	if !np.End.After(np.Start) {
		t.Error("expected end after start")
	}
	t.Logf("parsed: %q by %q, album %q (%d), label %q", np.Title, np.Artist, np.Album, np.Year, np.Label)
}

func TestParseLivemetaErrors(t *testing.T) {
	cases := map[string]string{
		"empty":        ``,
		"not json":     `nope`,
		"no levels":    `{"steps":{},"levels":[]}`,
		"bad position": `{"steps":{},"levels":[{"items":["a"],"position":5}]}`,
		"missing step": `{"steps":{},"levels":[{"items":["a"],"position":0}]}`,
		"neg position": `{"steps":{"a":{}},"levels":[{"items":["a"],"position":-1}]}`,
	}
	for name, in := range cases {
		if _, err := parseLivemeta([]byte(in)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestParseLivemetaAuthorsFallback(t *testing.T) {
	// No authors → performers used as artist.
	in := `{"steps":{"k":{"title":"T","performers":"P","authors":""}},"levels":[{"items":["k"],"position":0}]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Artist != "P" {
		t.Errorf("expected performers fallback, got %q", np.Artist)
	}
}

func TestNextDelayClamps(t *testing.T) {
	now := time.Unix(1000, 0)
	if d := nextDelay(time.Time{}, now); d != pollMin {
		t.Errorf("zero end: got %s want %s", d, pollMin)
	}
	if d := nextDelay(time.Unix(1005, 0), now); d != pollMin {
		t.Errorf("near end: got %s want clamp to %s", d, pollMin)
	}
	if d := nextDelay(time.Unix(100000, 0), now); d != pollMax {
		t.Errorf("far end: got %s want clamp to %s", d, pollMax)
	}
	if d := nextDelay(time.Unix(1100, 0), now); d < pollMin || d > pollMax {
		t.Errorf("mid: got %s out of range", d)
	}
}

func TestParseICY(t *testing.T) {
	np := parseICY("Miles Davis - So What")
	if np.Artist != "Miles Davis" || np.Title != "So What" {
		t.Errorf("got %q / %q", np.Artist, np.Title)
	}
	np = parseICY("  JustATitle  ")
	if np.Artist != "" || np.Title != "JustATitle" {
		t.Errorf("no-dash: got %q / %q", np.Artist, np.Title)
	}
	if !parseICY("").Empty() {
		t.Error("empty title should be Empty()")
	}
	// Stream-filename titles (mpv media-title before ICY tags) must be ignored.
	for _, junk := range []string{"fip-midfi.mp3?id=radiofrance", "fipcultes_hifi.m3u8", "stream.aac"} {
		if !parseICY(junk).Empty() {
			t.Errorf("stream name %q should be Empty()", junk)
		}
	}
}
