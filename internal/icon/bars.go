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
}

// panelIsDark caches the gsettings probe: it spawns a process, which must not
// happen per frame.
func panelIsDark() bool {
	panelOnce.Do(func() { panelDark = darkPanel() })
	return panelDark
}

// BarsIcon returns the PNG for the given quantized bar heights, theme-aware.
// Cached: rendering happens once per unique state.
func BarsIcon(h vu.Heights) []byte {
	key := barsKey{h: h, dark: panelIsDark()}

	barsCacheMu.Lock()
	if b, ok := barsCache[key]; ok {
		barsCacheMu.Unlock()
		return b
	}
	barsCacheMu.Unlock()

	b := renderBars(h, key.dark)

	barsCacheMu.Lock()
	barsCache[key] = b
	barsCacheMu.Unlock()
	return b
}

// renderBars rasterizes 4 vertical bars, bottom-aligned, in the glyph palette.
func renderBars(h vu.Heights, dark bool) []byte {
	ink := color.NRGBA{0x2B, 0x2B, 0x2B, 0xFF} // dark ink for light panels
	if dark {
		ink = color.NRGBA{0xF5, 0xF5, 0xF5, 0xFF} // light ink for dark panels
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
