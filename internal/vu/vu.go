// Package vu holds the pure logic of the animated VU tray icon: dB-to-height
// mapping, per-bar variation, and VU-meter envelope physics (fast attack,
// slow decay). No rendering, no I/O: everything here is deterministic and
// unit-tested.
package vu

import "math"

// Bars is the number of equalizer bars.
const Bars = 4

// MaxHeight is the top quantized bar height; heights range 0..MaxHeight.
// 12 discrete levels (0..11): finer than the original 8 for smoother motion
// at 6 fps, while still few enough that frames cache well and identical
// frames are skipped cheaply. The 44px canvas gives each level a uniform 3px
// step (see the icon package).
const MaxHeight = 11

// Heights is a quantized icon state: one height per bar. It is a comparable
// array on purpose: it serves directly as the frame-cache key and as the
// "same frame, skip SetIcon" test.
type Heights [Bars]uint8

// dB mapping bounds: typical music RMS sits well inside -60..0 dBFS.
const (
	minDB = -60.0
	maxDB = 0.0
)

// LevelFromDB maps an RMS level in dB to 0..1, clamped.
func LevelFromDB(db float64) float64 {
	if math.IsNaN(db) {
		return 0
	}
	l := (db - minDB) / (maxDB - minDB)
	if l < 0 {
		return 0
	}
	if l > 1 {
		return 1
	}
	return l
}

// perBarGain returns a deterministic multiplier for bar i at the given frame,
// so bars don't move in lockstep when no spectrum is available. Each bar
// oscillates around 1.0 with its own phase and speed; amplitude is small
// enough to preserve the overall level reading.
func perBarGain(i, frame int) float64 {
	phase := float64(i) * 1.7
	speed := 0.9 + 0.13*float64(i)
	return 1.0 + 0.22*math.Sin(phase+speed*float64(frame))
}

// quantize maps a 0..1 level to a bar height.
func quantize(level float64) uint8 {
	q := int(math.Round(level * MaxHeight))
	if q < 0 {
		q = 0
	}
	if q > MaxHeight {
		q = MaxHeight
	}
	return uint8(q)
}

// Targets derives per-bar target heights from one level in 0..1 at a frame
// counter, quantized to 0..MaxHeight. The per-bar sine is the fallback for
// when the spectrum reading is missing.
func Targets(level float64, frame int) Heights {
	var h Heights
	for i := 0; i < Bars; i++ {
		h[i] = quantize(level * perBarGain(i, frame))
	}
	return h
}

// Bands is one 0..1 level per bar, low to high. A zero value means silence.
type Bands [Bars]float64

// TargetsFromBands maps four band levels to bar heights. No sine: the
// differences between bars come from the sound.
func TargetsFromBands(b Bands) Heights {
	var h Heights
	for i := 0; i < Bars; i++ {
		h[i] = quantize(b[i])
	}
	return h
}

// decayPerFrame is how many height units a bar may fall per frame. Attack is
// instant (a rising target is taken immediately); decay is slower, which is
// what makes a VU meter feel alive instead of jittery. Tuned for 6 fps: the
// per-frame decay is half the old 3 fps value, so the wall-clock fall speed
// stays familiar while motion doubles in temporal resolution.
const decayPerFrame = 1

// Envelope applies VU physics: instant attack, bounded decay.
func Envelope(prev, target Heights) Heights {
	var h Heights
	for i := 0; i < Bars; i++ {
		switch {
		case target[i] >= prev[i]: // attack: jump straight to target
			h[i] = target[i]
		case prev[i] >= decayPerFrame && prev[i]-decayPerFrame > target[i]: // decay, floor at target
			h[i] = prev[i] - decayPerFrame
		default:
			h[i] = target[i]
		}
	}
	return h
}
