package metadata

import "strings"

// artistSeparators mark the boundary after which a credit string stops being
// the primary artist ("A, B", "A / B", "A feat. B", "A & C", "A; B"). The
// leading-space forms avoid cutting names that legitimately contain the
// characters (e.g. "AC/DC" has no spaces around the slash).
var artistSeparators = []string{",", " / ", " feat", " Feat", " FEAT", " & ", ";"}

// CleanArtist reduces a multi-artist credit string to its first artist by
// cutting at the earliest separator, then trimming. It cannot split blobs
// with no separators at all (livemeta sometimes concatenates names with bare
// spaces): those are handled upstream by preferring highlightedArtists.
func CleanArtist(s string) string {
	s = strings.TrimSpace(s)
	cut := len(s)
	for _, sep := range artistSeparators {
		if i := strings.Index(s, sep); i >= 0 && i < cut {
			cut = i
		}
	}
	return strings.TrimSpace(s[:cut])
}

// primaryArtist picks the best single-artist name for link resolution:
// the curated highlightedArtists[0] when livemeta provides it, else the
// cleaned credit string.
func primaryArtist(highlighted []string, credit string) string {
	if len(highlighted) > 0 {
		if h := strings.TrimSpace(highlighted[0]); h != "" {
			return h
		}
	}
	return CleanArtist(credit)
}
