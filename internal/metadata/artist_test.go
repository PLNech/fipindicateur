package metadata

import "testing"

func TestCleanArtist(t *testing.T) {
	cases := []struct{ in, want string }{
		// real-world nasty separators
		{"Nina Simone, Piano", "Nina Simone"},
		{"Herbie Hancock / Chick Corea", "Herbie Hancock"},
		{"Ali Farka Touré & Toumani Diabaté", "Ali Farka Touré"},
		{"Burna Boy feat. Damian Marley", "Burna Boy"},
		{"Angélique Kidjo Feat. Yemi Alade", "Angélique Kidjo"},
		{"Oumou Sangaré; Béla Fleck", "Oumou Sangaré"},
		{"Rosalía, Björk & Arca", "Rosalía"},
		// multiple separators: earliest wins
		{"A feat. B, C / D", "A"},
		// accents preserved
		{"Café Tacvba", "Café Tacvba"},
		{"Michèle Arnaud", "Michèle Arnaud"},
		// names containing separator chars WITHOUT surrounding spaces survive
		{"AC/DC", "AC/DC"},
		// whitespace hygiene
		{"  Fela Kuti  ", "Fela Kuti"},
		{"", ""},
		// a space-concatenated blob has no separators: cleaning is a no-op
		// (highlightedArtists handles these upstream)
		{"Fat Freddys Drop Dallas Tamaira Toby Laing Warryn Maxwell",
			"Fat Freddys Drop Dallas Tamaira Toby Laing Warryn Maxwell"},
	}
	for _, c := range cases {
		if got := CleanArtist(c.in); got != c.want {
			t.Errorf("CleanArtist(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPrimaryArtistPrefersHighlighted(t *testing.T) {
	// The curated field wins over the credit blob.
	got := primaryArtist([]string{"Fat Freddys Drop"},
		"Fat Freddys Drop Dallas Tamaira Toby Laing Warryn Maxwell")
	if got != "Fat Freddys Drop" {
		t.Errorf("got %q", got)
	}
	// Empty or missing highlighted falls back to cleaning.
	if got := primaryArtist(nil, "A, B"); got != "A" {
		t.Errorf("nil highlighted: got %q", got)
	}
	if got := primaryArtist([]string{"  "}, "A & B"); got != "A" {
		t.Errorf("blank highlighted: got %q", got)
	}
}

func TestParseLivemetaSetsPrimaryArtist(t *testing.T) {
	in := `{"steps":{"k":{"title":"T","authors":"X Y Z","highlightedArtists":["X"]}},"levels":[{"items":["k"],"position":0}]}`
	np, err := parseLivemeta([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if np.Artist != "X Y Z" {
		t.Errorf("display artist should keep the full credit: %q", np.Artist)
	}
	if np.PrimaryArtist != "X" {
		t.Errorf("primary artist should use highlightedArtists[0]: %q", np.PrimaryArtist)
	}
}

func TestParseICYSetsPrimaryArtist(t *testing.T) {
	np := parseICY("Ibrahim Maalouf & Haïdouti Orkestar - True Sorry")
	if np.Artist != "Ibrahim Maalouf & Haïdouti Orkestar" {
		t.Errorf("display: %q", np.Artist)
	}
	if np.PrimaryArtist != "Ibrahim Maalouf" {
		t.Errorf("primary: %q", np.PrimaryArtist)
	}
}
