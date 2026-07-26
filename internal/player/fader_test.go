package player

import (
	"math"
	"testing"
	"time"
)

// TestEqualPowerGainsEndpoints checks the fade boundaries: silent-in/full-out
// at the start, the reverse at the end.
func TestEqualPowerGainsEndpoints(t *testing.T) {
	in, out := equalPowerGains(0)
	if math.Abs(in-0) > 1e-9 || math.Abs(out-100) > 1e-9 {
		t.Fatalf("t=0: got in=%v out=%v, want in=0 out=100", in, out)
	}
	in, out = equalPowerGains(1)
	if math.Abs(in-100) > 1e-9 || math.Abs(out-0) > 1e-9 {
		t.Fatalf("t=1: got in=%v out=%v, want in=100 out=0", in, out)
	}
}

// TestEqualPowerGainsMidpoint checks the equal-power crossover: both channels
// at 100/sqrt(2) halfway, i.e. equal and each ~70.71.
func TestEqualPowerGainsMidpoint(t *testing.T) {
	in, out := equalPowerGains(0.5)
	want := 100 / math.Sqrt2
	if math.Abs(in-want) > 1e-9 || math.Abs(out-want) > 1e-9 {
		t.Fatalf("t=0.5: got in=%v out=%v, want both %v", in, out, want)
	}
}

// TestEqualPowerConstantEnergy verifies the defining property: in^2 + out^2 is
// constant (== 100^2) across the whole fade, so loudness does not dip midway.
func TestEqualPowerConstantEnergy(t *testing.T) {
	for i := 0; i <= 100; i++ {
		tt := float64(i) / 100
		in, out := equalPowerGains(tt)
		energy := in*in + out*out
		if math.Abs(energy-10000) > 1e-6 {
			t.Fatalf("t=%v: in^2+out^2=%v, want 10000", tt, energy)
		}
	}
}

// TestEqualPowerMonotonic checks the incoming gain rises and the outgoing gain
// falls monotonically, so neither channel wobbles during the fade.
func TestEqualPowerMonotonic(t *testing.T) {
	prevIn, prevOut := equalPowerGains(0)
	for i := 1; i <= 100; i++ {
		in, out := equalPowerGains(float64(i) / 100)
		if in < prevIn-1e-9 {
			t.Fatalf("incoming not monotonic at t=%v: %v < %v", float64(i)/100, in, prevIn)
		}
		if out > prevOut+1e-9 {
			t.Fatalf("outgoing not monotonic at t=%v: %v > %v", float64(i)/100, out, prevOut)
		}
		prevIn, prevOut = in, out
	}
}

// TestEqualPowerClamps checks out-of-range progress is clamped to the endpoints
// rather than overshooting (sin/cos would otherwise dip back).
func TestEqualPowerClamps(t *testing.T) {
	in, out := equalPowerGains(-0.5)
	if math.Abs(in-0) > 1e-9 || math.Abs(out-100) > 1e-9 {
		t.Fatalf("t<0: got in=%v out=%v, want in=0 out=100", in, out)
	}
	in, out = equalPowerGains(1.5)
	if math.Abs(in-100) > 1e-9 || math.Abs(out-0) > 1e-9 {
		t.Fatalf("t>1: got in=%v out=%v, want in=100 out=0", in, out)
	}
}

// mpvAmp is mpv's softvol law: the linear amplitude produced by a `volume`
// property value (player/audio.c: gain = pow(volume/100, 3)).
func mpvAmp(vol float64) float64 { return math.Pow(vol/100, 3) }

// TestMpvVolForAmpInvertsCubicKnob checks the mapping is the exact inverse of
// mpv's cubic volume law, endpoints included.
func TestMpvVolForAmpInvertsCubicKnob(t *testing.T) {
	for i := 0; i <= 100; i++ {
		a := float64(i) / 100
		if got := mpvAmp(mpvVolForAmp(a)); math.Abs(got-a) > 1e-9 {
			t.Fatalf("amp %v: knob produces %v", a, got)
		}
	}
	if mpvVolForAmp(-0.1) != 0 || mpvVolForAmp(1.1) != 100 {
		t.Fatalf("out-of-range amplitudes must clamp to 0/100")
	}
}

// TestFadeVolumesEqualPowerThroughKnob verifies the defining acoustic property
// AFTER the cubic knob: the summed power of the two streams' actual amplitudes
// stays constant across the fade (no mid-fade silence hole).
func TestFadeVolumesEqualPowerThroughKnob(t *testing.T) {
	for i := 0; i <= 100; i++ {
		tt := float64(i) / 100
		in, out := fadeVolumes(tt, 100)
		ai, ao := mpvAmp(in), mpvAmp(out)
		if energy := ai*ai + ao*ao; math.Abs(energy-1) > 1e-6 {
			t.Fatalf("t=%v: acoustic power %v, want 1", tt, energy)
		}
	}
}

// TestFadeVolumesAudibleEarly guards the field regression: two seconds into a
// ten-second fade (t=0.2) the incoming station must already be at a clearly
// audible level (better than -20 dB). The uncompensated sin^3 curve sat at
// -30.6 dB there, which the ear read as a hard cut with a silence hole.
func TestFadeVolumesAudibleEarly(t *testing.T) {
	in, _ := fadeVolumes(0.2, 100)
	db := 20 * math.Log10(mpvAmp(in))
	if db < -20 {
		t.Fatalf("incoming at t=0.2 is %.1f dB, want >= -20 dB", db)
	}
}

// TestFadeVolumesOutStart checks a superseded fade's down-ramp: the outgoing
// starts exactly at its inherited partial volume (no snap up to 100) and still
// reaches silence.
func TestFadeVolumesOutStart(t *testing.T) {
	_, out := fadeVolumes(0, 37)
	if math.Abs(out-37) > 1e-9 {
		t.Fatalf("t=0 outStart=37: out=%v, want 37", out)
	}
	// The cube root magnifies cos(pi/2)'s ~1e-16 float residue into ~1e-4
	// volume units; anything under a millivolume is silence.
	_, out = fadeVolumes(1, 37)
	if math.Abs(out) > 1e-3 {
		t.Fatalf("t=1 outStart=37: out=%v, want 0", out)
	}
}

// TestZapDecision covers the Play gate: only a genuine live zap (fade
// configured, currently playing, different URL) crossfades; initial start,
// resume, the hi-fi reload and a disabled fade all plain-load.
func TestZapDecision(t *testing.T) {
	cases := []struct {
		name    string
		cross   time.Duration
		playing bool
		curURL  string
		url     string
		want    bool
	}{
		{"live zap", 10 * time.Second, true, "a", "b", true},
		{"disabled", 0, true, "a", "b", false},
		{"initial start", 10 * time.Second, false, "", "b", false},
		{"resume same station", 10 * time.Second, false, "b", "b", false},
		{"reload same URL", 10 * time.Second, true, "b", "b", false},
	}
	for _, c := range cases {
		got, reason := zapDecision(c.cross, c.playing, c.curURL, c.url)
		if got != c.want {
			t.Errorf("%s: crossfade=%v (%s), want %v", c.name, got, reason, c.want)
		}
		if reason == "" {
			t.Errorf("%s: empty reason", c.name)
		}
	}
}
