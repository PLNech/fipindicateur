package player

import (
	"math"
	"testing"
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
