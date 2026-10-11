package player

import (
	"fmt"
	"strings"
	"testing"
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
