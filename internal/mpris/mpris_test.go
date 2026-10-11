//go:build linux

package mpris

import (
	"testing"
)

// TestWellKnownName pins the FIP_MPRIS_NAME override: absent/empty keeps the
// production name (and its single-instance-lock semantics), a full
// org.mpris.MediaPlayer2. name is taken as-is, and a bare name is prefixed so
// media-key routing keeps seeing a proper MPRIS player.
func TestWellKnownName(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", defaultWellKnownName},
		{"   ", defaultWellKnownName},
		{"org.mpris.MediaPlayer2.fipindicateur.dev", "org.mpris.MediaPlayer2.fipindicateur.dev"},
		{"fipindicateur.dev", "org.mpris.MediaPlayer2.fipindicateur.dev"},
		{"  org.mpris.MediaPlayer2.protoB  ", "org.mpris.MediaPlayer2.protoB"},
		{"protoB", "org.mpris.MediaPlayer2.protoB"},
	}
	for _, c := range cases {
		t.Setenv("FIP_MPRIS_NAME", c.env)
		if got := wellKnownName(); got != c.want {
			t.Errorf("FIP_MPRIS_NAME=%q: name = %q, want %q", c.env, got, c.want)
		}
	}
}
