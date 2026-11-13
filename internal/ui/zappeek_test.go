package ui

import (
	"testing"
	"time"

	"github.com/PLNech/fipindicateur/internal/metadata"
	"github.com/PLNech/fipindicateur/internal/stations"
)

func TestZapPeeked(t *testing.T) {
	a := &App{current: stations.ByKey("jazz")}
	np := metadata.NowPlaying{Artist: "A", Title: "T"}
	if a.zapPeeked(np) {
		t.Fatal("nothing announced yet")
	}
	a.zapPeek = zapPeek{key: "jazz", artist: "A", title: "T", at: time.Now()}
	if !a.zapPeeked(np) {
		t.Fatal("same track on the same station must be muted")
	}
	if a.zapPeeked(metadata.NowPlaying{Artist: "A", Title: "Next"}) {
		t.Fatal("another track must still be announced")
	}
	a.current = stations.ByKey("rock")
	if a.zapPeeked(np) {
		t.Fatal("another station must still be announced")
	}
	a.current = stations.ByKey("jazz")
	a.zapPeek.at = time.Now().Add(-2 * time.Minute)
	if a.zapPeeked(np) {
		t.Fatal("a stale zap must not mute")
	}
}
