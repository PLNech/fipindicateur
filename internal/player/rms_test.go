package player

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// The af chain measures only measure_overall=RMS_level, so libmpv returns a
// one-key map. fullAstats fakes the old default (~79 stats per window) to show
// what the parse would cost had the chain not been trimmed.
const oneKeyAstats = `{"lavfi.astats.Overall.RMS_level":"-18.402531"}`

func fullAstats() string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 79; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"lavfi.astats.Overall.Stat_%02d":"-%d.123456"`, i, i)
	}
	b.WriteString(`,"lavfi.astats.Overall.RMS_level":"-18.402531"}`)
	return b.String()
}

func TestParseFilterMean(t *testing.T) {
	mag, ok := parseFilterMean(`{"lavfi.aspectralstats.1.mean":"0.05","lavfi.aspectralstats.2.mean":"0.01"}`)
	if !ok || mag != 0.05 {
		t.Fatalf("mean = %v ok=%v, want 0.05", mag, ok)
	}
	if _, ok := parseFilterMean(`{"lavfi.astats.Overall.RMS_level":"-18"}`); ok {
		t.Error("a loudness map is not a band mean")
	}
	if _, ok := parseFilterMean("nope"); ok {
		t.Error("bad json is a miss")
	}
}

func TestShapeToBandsPicksTheTone(t *testing.T) {
	// A 60 Hz tone measured live: centroid near 80, narrow spread, rolloff
	// just above the tone. The bass bar should win.
	b := shapeToBands(80, 530, 86)
	if loudestBand(b) != 0 {
		t.Errorf("low centroid should light the bass bar, got %v", b)
	}
	hi := shapeToBands(4800, 400, 6000)
	if loudestBand(hi) != 3 {
		t.Errorf("high centroid should light the air bar, got %v", hi)
	}
}

func loudestBand(b vu.Bands) int {
	i := 0
	for k := 1; k < len(b); k++ {
		if b[k] > b[i] {
			i = k
		}
	}
	return i
}

func TestBandsFromMeansRanksLoudest(t *testing.T) {
	b := bandsFromMeans([vu.Bars]float64{0.2, 0.001, 0.001, 0.001}, true)
	if b[0] <= b[3] {
		t.Errorf("a loud bass mean should outrank a quiet air mean, got %v", b)
	}
}

func TestParseRMSLevel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		db   float64
		bad  bool
	}{
		{"one key", oneKeyAstats, -18.402531, false},
		{"full map", fullAstats(), -18.402531, false},
		{"silence", `{"lavfi.astats.Overall.RMS_level":"-inf"}`, -60, false},
		{"missing key", `{"other":"1"}`, 0, true},
		{"not json", `nope`, 0, true},
		{"not numeric", `{"lavfi.astats.Overall.RMS_level":"abc"}`, 0, true},
		{"empty map", `{}`, 0, true},
	}
	for _, c := range cases {
		db, reason := parseRMSLevel(c.in)
		if (reason != "") != c.bad {
			t.Errorf("%s: reason = %q, want failure=%v", c.name, reason, c.bad)
			continue
		}
		if !c.bad && db != c.db {
			t.Errorf("%s: db = %v, want %v", c.name, db, c.db)
		}
	}
}

func BenchmarkParseRMSLevelOneKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseRMSLevel(oneKeyAstats)
	}
}

func BenchmarkParseRMSLevelFullMap(b *testing.B) {
	full := fullAstats()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseRMSLevel(full)
	}
}
