package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/PLNech/fipindicateur/internal/vu"
)

// Animated-icon rendering with a quantized frame cache. The icon state is the
// tuple of 4 quantized bar heights; each unique state is rasterized to PNG
// exactly once and cached, so steady playback costs map lookups, not pixels.
// The real per-frame cost is SetIcon (dbus + shell redraw), which the caller
// skips when the state tuple is unchanged.

// barsSize matches the 44px embedded static icons: at 44px each of the 12
// quantized heights gets a uniform 3px step (at 22px the choice was 8 chunky
// levels of 2px or 12 squat levels of 1px, so the canvas grew instead).
const barsSize = 44

var (
	barsCacheMu sync.Mutex
	barsCache   = map[barsKey][]byte{}

	panelOnce sync.Once
	panelDark bool
)

type barsKey struct {
	h    vu.Heights
	dark bool
	// tint is the station brand ink; the zero value means "use theme ink".
	// color.NRGBA is comparable, so it keys the cache directly and the tint
	// step joins (heights, dark) in the frame identity.
	tint color.NRGBA
}

// panelIsDark caches the gsettings probe: it spawns a process, which must not
// happen per frame.
func panelIsDark() bool {
	panelOnce.Do(func() { panelDark = darkPanel() })
	return panelDark
}

// PanelIsDark exposes the cached panel-darkness probe so callers (the UI)
// resolve station colors against the same surface without a per-frame spawn.
func PanelIsDark() bool { return panelIsDark() }

// ThemeInk is the neutral bar ink for the current panel: the color used when
// no tint is applied. Callers ease a first tint in from this so the intro is a
// fade rather than a jump.
func ThemeInk(dark bool) color.NRGBA {
	if dark {
		return color.NRGBA{0xF5, 0xF5, 0xF5, 0xFF} // light ink for dark panels
	}
	return color.NRGBA{0x2B, 0x2B, 0x2B, 0xFF} // dark ink for light panels
}

// BarsIcon returns the PNG for the given quantized bar heights, theme-aware.
// A non-zero tint recolors the bars (the active station's legible brand ink);
// the zero value falls back to the theme ink. Cached: rendering happens once
// per unique (heights, panel, tint) state.
func BarsIcon(h vu.Heights, tint color.NRGBA) []byte {
	key := barsKey{h: h, dark: panelIsDark(), tint: tint}

	barsCacheMu.Lock()
	if b, ok := barsCache[key]; ok {
		barsCacheMu.Unlock()
		return b
	}
	barsCacheMu.Unlock()

	b := renderBars(h, key.dark, tint)

	barsCacheMu.Lock()
	barsCache[key] = b
	barsCacheMu.Unlock()
	return b
}

// renderBars rasterizes 4 vertical bars, bottom-aligned, in the glyph palette.
func renderBars(h vu.Heights, dark bool, tint color.NRGBA) []byte {
	ink := ThemeInk(dark)
	if (tint != color.NRGBA{}) {
		ink = tint // station brand ink while music plays
	}

	img := image.NewNRGBA(image.Rect(0, 0, barsSize, barsSize))

	const (
		barW   = 6
		gap    = 4
		bottom = barsSize - 3
		unit   = 3 // pixels per height step: 11 levels above stub = 33px travel
		stub   = 3 // minimum visible bar so silence still shows life
	)
	// 4*6 + 3*4 = 36 px of bars, centered in 44.
	x0 := (barsSize - (vu.Bars*barW + (vu.Bars-1)*gap)) / 2

	for i := 0; i < vu.Bars; i++ {
		barH := stub + int(h[i])*unit // 3..36 px
		xs := x0 + i*(barW+gap)
		for y := bottom - barH; y < bottom; y++ {
			for x := xs; x < xs+barW; x++ {
				img.SetNRGBA(x, y, ink)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// Cannot fail on an in-memory NRGBA in practice; fall back to the
		// static glyph so the tray never goes blank.
		return Active(false)
	}
	return buf.Bytes()
}
