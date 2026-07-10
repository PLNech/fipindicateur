package icon

import (
	"image/color"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// The static tray glyph is the same 4-bar equalizer as the animation, frozen:
// one visual identity whether the music moves or not. The old radio-waves mark
// (icon.Active) remains the app/launcher icon but no longer appears in the
// tray, where swapping marks read as a flash.

// restPose is the frozen "equalizer at rest" skyline shown when nothing
// animates (paused, stopped, animation off or broken).
var restPose = vu.Heights{3, 7, 4, 9}

// restDimAlpha dims the ink for the paused glyph, carrying the pause
// affordance the dim static variants used to provide.
const restDimAlpha = 0x8C

// Rest returns the static bars glyph. A zero tint uses neutral theme ink,
// dimmed when paused; a non-zero tint paints the station's brand ink at full
// strength, so the FIP colors persist on the frozen glyph even with the
// animated icon turned off (color only while music plays: callers pass the
// zero tint when paused/stopped). It rides BarsIcon's cache: each variant
// renders once.
func Rest(paused bool, tint color.NRGBA) []byte {
	if (tint != color.NRGBA{}) {
		return BarsIcon(restPose, tint)
	}
	ink := ThemeInk(panelIsDark())
	if paused {
		ink = color.NRGBA{ink.R, ink.G, ink.B, restDimAlpha}
	}
	return BarsIcon(restPose, ink)
}
