package stations

import (
	"strings"
	"testing"
)

func TestAllThirteen(t *testing.T) {
	if len(All) != 13 {
		t.Fatalf("expected 13 stations, got %d", len(All))
	}
	seen := map[string]bool{}
	for _, s := range All {
		if seen[s.Key] {
			t.Errorf("duplicate key %q", s.Key)
		}
		seen[s.Key] = true
		if s.Display == "" || s.Slug == "" {
			t.Errorf("station %q missing display/slug", s.Key)
		}
	}
}

func TestStreamURL(t *testing.T) {
	fip := ByKey("fip")
	if got := fip.StreamURL(Midfi); got != "https://icecast.radiofrance.fr/fip-midfi.mp3?id=radiofrance" {
		t.Errorf("midfi: %s", got)
	}
	if got := fip.StreamURL(Hifi); got != "https://icecast.radiofrance.fr/fip-hifi.aac?id=radiofrance" {
		t.Errorf("hifi: %s", got)
	}
	// Every station builds a plausible icecast URL.
	for _, s := range All {
		u := s.StreamURL(Midfi)
		if !strings.HasPrefix(u, "https://icecast.radiofrance.fr/"+s.Slug) {
			t.Errorf("%s: unexpected URL %s", s.Key, u)
		}
	}
}

func TestByKeyFallback(t *testing.T) {
	if ByKey("does-not-exist").Key != "fip" {
		t.Error("unknown key should fall back to fip")
	}
	if ByKey("rock").Slug != "fiprock" {
		t.Error("rock should map to fiprock")
	}
}
