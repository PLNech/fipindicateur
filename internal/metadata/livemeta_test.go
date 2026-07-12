package metadata

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseLivemetaFixture(t *testing.T) {
	// Real station-7 payload: two levels (an expression/show grain over a song
	// grain), a track playing inside "Club Jazzafip", and future shows queued.
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
	if np.Start.IsZero() || np.End.IsZero() {
		t.Error("expected start/end times")
	}
	if !np.End.After(np.Start) {
		t.Error("expected end after start")
	}
	// The current track plays within a show: the expression is resolved via the
	// song's fatherStepId, carrying its stable concept identity and date-free
	// name.
	if np.Show == nil {
		t.Fatal("expected a current show (the track plays within Club Jazzafip)")
	}
	if np.Show.ConceptUUID == "" {
		t.Error("expected a stable conceptUuid on the current show")
	}
	if !strings.Contains(np.Show.Title, "Club Jazzafip") {
		t.Errorf("show title: got %q, want it to contain Club Jazzafip", np.Show.Title)
	}
	if np.Show.Start.IsZero() || !np.Show.End.After(np.Show.Start) {
		t.Errorf("show should carry a valid time window: %v .. %v", np.Show.Start, np.Show.End)
	}
	// Future programming is returned in the same poll, chronologically ordered.
	if len(np.UpcomingShows) == 0 {
		t.Fatal("expected upcoming shows in the fixture")
	}
	for i := 1; i < len(np.UpcomingShows); i++ {
		if np.UpcomingShows[i].Start.Before(np.UpcomingShows[i-1].Start) {
			t.Error("upcoming shows should be sorted by start time")
		}
	}
	for _, s := range np.UpcomingShows {
		if s.Title == "" {
			t.Error("an upcoming show has no display title (titleConcept/title both empty)")
		}
	}
	t.Logf("parsed: %q by %q; show %q (%s), %d upcoming", np.Title, np.Artist, np.Show.Title, np.Show.ConceptUUID, len(np.UpcomingShows))
}

func TestParseLivemetaFipTapeOneLevel(t *testing.T) {
	// Real station-7 payload captured during "Fip Tape": a single level whose
	// current item is the expression itself (a continuous mix with no song
	// grain). The show must surface, with no phantom track presented in its place.
	data, err := os.ReadFile("testdata/livemeta_fiptape_one_level.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	np, err := parseLivemeta(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// No phantom track: the expression is not dressed up as a song.
	if np.Title != "" || np.Artist != "" {
		t.Errorf("expected no track, got title %q artist %q", np.Title, np.Artist)
	}
	if !np.Empty() {
		t.Error("a show-only poll carries no track, so it must report Empty()")
	}
	// The current show is surfaced with its stable identity.
	if np.Show == nil {
		t.Fatal("expected the current show (Fip Tape) to be surfaced")
	}
	if np.Show.ConceptUUID == "" {
		t.Error("expected a stable conceptUuid on the current show")
	}
	if np.Show.Title != "Fip Tape" {
		t.Errorf("show title: got %q want %q (titleConcept absent, raw title used)", np.Show.Title, "Fip Tape")
	}
	if np.Show.Start.IsZero() || !np.Show.End.After(np.Show.Start) {
		t.Errorf("show should carry a valid time window: %v .. %v", np.Show.Start, np.Show.End)
	}
	// Upcoming programming is still extracted (the Club Jazzafip nights queued
	// after Fip Tape), chronologically ordered and each with a display title.
	if len(np.UpcomingShows) == 0 {
		t.Fatal("expected upcoming shows queued after Fip Tape")
	}
	sawJazz := false
	for i, s := range np.UpcomingShows {
		if s.Title == "" {
			t.Error("an upcoming show has no display title")
		}
		if strings.Contains(s.Title, "Club Jazzafip") {
			sawJazz = true
		}
		if i > 0 && s.Start.Before(np.UpcomingShows[i-1].Start) {
			t.Error("upcoming shows should be sorted by start time")
		}
	}
	if !sawJazz {
		t.Error("expected the queued Club Jazzafip nights among upcoming shows")
	}
	t.Logf("show %q (%s), %d upcoming", np.Show.Title, np.Show.ConceptUUID, len(np.UpcomingShows))
}

func TestParseLivemetaSongMetadata(t *testing.T) {
	// A fully-credited track (single level of songs, webradio-shaped) round-trips
	// album, year, label and cover.
	in := `{"steps":{"k":{"embedType":"song","title":"So What","authors":"Miles Davis",` +
		`"titreAlbum":"Kind of Blue","anneeEditionMusique":1959,"label":"COLUMBIA",` +
		`"visual":"cover.jpg","path":"/link","start":1000,"end":2000}},` +
		`"levels":[{"items":["k"],"position":0}]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Album != "Kind of Blue" || np.Year != 1959 || np.Label != "COLUMBIA" || np.CoverURL != "cover.jpg" {
		t.Errorf("song metadata mismatch: %+v", np)
	}
	if np.Show != nil || len(np.UpcomingShows) != 0 {
		t.Errorf("a lone song carries no show: show=%v upcoming=%d", np.Show, len(np.UpcomingShows))
	}
}

func TestParseLivemetaShowFromFather(t *testing.T) {
	// A song whose fatherStepId points at an expression step is inside that show.
	in := `{"steps":{
		"e1":{"embedType":"expression","stepId":"e1","conceptUuid":"c-jazz",` +
		`"titleConcept":"Club Jazzafip ","title":"Club Jazzafip du dimanche","description":"desc",` +
		`"path":"fip/podcasts/club-jazzafip","start":1000,"end":9000},
		"s1":{"embedType":"song","stepId":"s1","fatherStepId":"e1","title":"T","authors":"A","start":1200,"end":1400}
	},"levels":[
		{"items":["e1"],"position":0},
		{"items":["s1"],"position":0}
	]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Title != "T" || np.Artist != "A" {
		t.Errorf("track: got %q by %q", np.Title, np.Artist)
	}
	if np.Show == nil {
		t.Fatal("expected a show resolved via fatherStepId")
	}
	if np.Show.ConceptUUID != "c-jazz" {
		t.Errorf("conceptUuid: got %q want c-jazz", np.Show.ConceptUUID)
	}
	// titleConcept (date-free) is preferred and trimmed.
	if np.Show.Title != "Club Jazzafip" {
		t.Errorf("title: got %q want %q", np.Show.Title, "Club Jazzafip")
	}
}

func TestParseLivemetaNoShowWhenNoFather(t *testing.T) {
	// A song outside any show (null fatherStepId) yields no current show, even
	// when an expression grain exists in the payload.
	in := `{"steps":{
		"e1":{"embedType":"expression","stepId":"e1","conceptUuid":"c","titleConcept":"X","start":1,"end":2},
		"s1":{"embedType":"song","stepId":"s1","fatherStepId":null,"title":"T","authors":"A"}
	},"levels":[
		{"items":["e1"],"position":0},
		{"items":["s1"],"position":0}
	]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Show != nil {
		t.Errorf("a song with no father is outside any show, got %+v", np.Show)
	}
}

func TestParseLivemetaUpcomingShowsExtracted(t *testing.T) {
	// Expression grain with one current and two future shows: only the two after
	// position are upcoming, sorted by start, deduplicated by stepId.
	in := `{"steps":{
		"cur":{"embedType":"expression","stepId":"cur","conceptUuid":"c0","titleConcept":"Now","start":100,"end":200},
		"f2":{"embedType":"expression","stepId":"f2","conceptUuid":"c2","title":"Later","start":400,"end":500},
		"f1":{"embedType":"expression","stepId":"f1","conceptUuid":"c1","title":"Soon","start":200,"end":300},
		"s":{"embedType":"song","stepId":"s","title":"T","authors":"A"}
	},"levels":[
		{"items":["cur","f1","f2"],"position":0},
		{"items":["s"],"position":0}
	]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(np.UpcomingShows) != 2 {
		t.Fatalf("expected 2 upcoming shows, got %d", len(np.UpcomingShows))
	}
	if np.UpcomingShows[0].ConceptUUID != "c1" || np.UpcomingShows[1].ConceptUUID != "c2" {
		t.Errorf("upcoming order/identity: %+v", np.UpcomingShows)
	}
	// Future steps carry no titleConcept: the raw title is the display name.
	if np.UpcomingShows[0].Title != "Soon" {
		t.Errorf("future show title fallback: got %q want Soon", np.UpcomingShows[0].Title)
	}
}

func TestParseLivemetaWebradioNoShows(t *testing.T) {
	// A webradio payload is a single level of songs with no expression grain:
	// zero shows, current or upcoming, and no panic.
	in := `{"stationId":65,"steps":{
		"a":{"embedType":"song","stepId":"a","title":"T0","authors":"A0"},
		"b":{"embedType":"song","stepId":"b","title":"T1","authors":"A1"}
	},"levels":[{"items":["a","b"],"position":0}]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Show != nil {
		t.Errorf("webradio has no show, got %+v", np.Show)
	}
	if len(np.UpcomingShows) != 0 {
		t.Errorf("webradio has no upcoming shows, got %d", len(np.UpcomingShows))
	}
}

func TestParseLivemetaMalformedNoPanic(t *testing.T) {
	// Defensive: odd shapes must return an error or empty result, never panic.
	cases := []string{
		`{"steps":{"s":{"embedType":"song","fatherStepId":"missing","title":"T","authors":"A"}},"levels":[{"items":["s"],"position":0}]}`, // father points nowhere
		`{"steps":{"e":{"embedType":"expression","stepId":"e"}},"levels":[{"items":["e"],"position":-1},{"items":["e"],"position":0}]}`,   // negative position on a level
		`{"steps":{},"levels":[{"items":[],"position":0}]}`, // empty items
	}
	for i, in := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d panicked: %v", i, r)
				}
			}()
			_, _ = parseLivemeta([]byte(in))
		}()
	}
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
