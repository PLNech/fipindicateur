package icon

import (
	"bytes"
	"image/color"
	"testing"
)

// A playing station tint must survive onto the frozen glyph, so the FIP colors
// persist on the tray even with the animated icon off. Paused/stopped (zero
// tint) stays neutral, matching the animator's fade to theme ink.
func TestRestTintPersistsWhilePlaying(t *testing.T) {
	fip := color.NRGBA{0xE2, 0x00, 0x7A, 0xFF}

	neutral := Rest(false, color.NRGBA{})
	tinted := Rest(false, fip)
	if bytes.Equal(neutral, tinted) {
		t.Error("a playing station tint must recolor the frozen glyph")
	}

	// The tinted rest glyph is exactly BarsIcon at the rest pose in that ink:
	// one visual identity whether the bars move or not.
	if !bytes.Equal(tinted, BarsIcon(restPose, fip)) {
		t.Error("tinted rest must render the rest pose in the station ink")
	}

	// Paused ignores the tint and falls back to neutral (color only while
	// music plays), so the paused glyph is not the playing-tinted one.
	if bytes.Equal(Rest(true, color.NRGBA{}), tinted) {
		t.Error("paused glyph must not carry the playing tint")
	}
}
