package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
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

	// panelMu guards the cached probe below. The probe itself is cheap after
	// the first call (an in-process NSAppearance read on darwin), and the UI
	// re-probes on the appearance's own schedule (RefreshPanelDark from the
	// tray's flip notification), so no timer or polling is involved.
	panelMu    sync.RWMutex
	panelDark  bool
	panelKnown bool
)

type barsKey struct {
	h    vu.Heights
	dark bool
	// tint is the station brand ink; the zero value means "use theme ink".
	// color.NRGBA is comparable, so it keys the cache directly and the tint
	// step joins (heights, dark) in the frame identity.
	tint color.NRGBA
}

// panelIsDark returns the panel-darkness probe, cached between re-probes.
// The cache is not a sync.Once: under the Auto appearance the panel flips
// dark and light on the system's schedule, so a boot-time answer frozen
// forever drew dark ink on an evening bar. The UI invalidates through
// RefreshPanelDark when the effective appearance changes; every cache key
// below carries dark, so a flip re-renders instead of serving stale pixels.
func panelIsDark() bool {
	panelMu.RLock()
	if panelKnown {
		d := panelDark
		panelMu.RUnlock()
		return d
	}
	panelMu.RUnlock()
	panelMu.Lock()
	defer panelMu.Unlock()
	if panelKnown {
		return panelDark
	}
	panelDark = darkPanel()
	panelKnown = true
	return panelDark
}

// PanelIsDark exposes the cached panel-darkness probe so callers (the UI)
// resolve station colors against the same surface without a per-frame probe.
func PanelIsDark() bool { return panelIsDark() }

// RefreshPanelDark re-probes the panel darkness (the appearance changed) and
// reports whether the state actually flipped, so the caller repaints only on
// a real change. The next BarsIcon/TrayFrame call keys on the new state; the
// caches need no clearing.
func RefreshPanelDark() bool {
	d := darkPanel()
	panelMu.Lock()
	defer panelMu.Unlock()
	changed := panelKnown && d != panelDark
	panelDark = d
	panelKnown = true
	return changed
}

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
	mask := barMask(h)

	// Phantom outline first (under the ink), then the bars on top so the glyph
	// body stays crisp and only the surrounding ring carries the halo. The rim
	// exists where the panel downscales the 44px PNG (haloOnPanel); a colored
	// brand ink carries it on every panel (the black keyline is panel-proof),
	// a platform that draws the PNG at its native point size skips it otherwise.
	if inkIsColored(ink) || haloOnPanel(dark) {
		drawHalo(img, mask[:], barsSize, haloInk(ink))
	}
	for y := 0; y < barsSize; y++ {
		for x := 0; x < barsSize; x++ {
			if mask[y*barsSize+x] {
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

// barMask marks every glyph (bar) pixel for the given heights, bottom-aligned
// in the barsSize canvas. The phantom halo is derived from it.
func barMask(h vu.Heights) (mask [barsSize * barsSize]bool) {
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
				mask[y*barsSize+x] = true
			}
		}
	}
	return mask
}

// Phantom-outline tuning, shared with the gen generator (internal/icon/gen).
// A semi-transparent white ring is baked around every glyph edge so the glyph
// clears a near-black panel (GNOME's default top bar) with a visible rim, while
// the white ring stays near-invisible on a light panel (white on near-white).
// It rides under the ink at full strength (never dimmed with the paused glyph),
// so the outline survives every state.
//
// Tuned for the real display scale. The tray hands 44px bytes to systray and
// GNOME downscales to ~22px, which halves both the ring width and its coverage:
// a nominal 0.55 alpha lands near 0.42 effective on #1a1a1a at 22px (a clear
// rim), while on #f5f5f5 the same ring stays under ~2% channel lift (invisible).
// The pre-fix 0.30 alpha washed out to ~0.18 effective: the rim was there in the
// 44px render but sub-perceptual once the panel shrank it.
const (
	haloAlpha = 0.55 // ring opacity: clear rim on black, invisible on white
	haloInner = 1.0  // px at full strength before the feather begins
)

// haloWhite and haloBlack are the two phantom-outline inks. White is the
// tuned default for the neutral theme glyphs (it lifts them off a near-black
// panel and vanishes on a light one). A colored brand ink gets a black rim
// instead: a dark surround reads as a keyline on any panel, and unlike white
// it raises the perceived saturation of the color it frames instead of
// washing it.
var (
	haloWhite = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	haloBlack = color.NRGBA{0x00, 0x00, 0x00, 0xFF}
)

// haloInk picks the outline color for a glyph ink: black under a saturated
// brand color, the tuned white under the neutral theme inks.
func haloInk(ink color.NRGBA) color.NRGBA {
	if inkIsColored(ink) {
		return haloBlack
	}
	return haloWhite
}

// haloWidth is the outer ring radius in pixels. It grows slower than the canvas
// (a fixed offset plus a small fraction), so the ring is relatively thinner at
// larger sizes: ~1.6px look at 22px display, ~3.2px on the 44px bars canvas.
func haloWidth(size int) float64 { return 1.0 + float64(size)*0.05 }

// drawHalo paints the phantom outline into img for the glyph described by mask
// (true == glyph pixel), in the given outline color. Each transparent pixel
// within haloWidth of a glyph pixel gets that color at haloAlpha, feathered to
// zero at the ring's outer edge. Mask pixels are left untouched: the caller
// paints the ink over them.
func drawHalo(img *image.NRGBA, mask []bool, size int, halo color.NRGBA) {
	width := haloWidth(size)
	ring := haloRing(size)
	at := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < size && y < size && mask[y*size+x]
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if at(x, y) {
				continue // glyph body; ink is painted here later
			}
			// The ring is sorted by distance, so the first glyph pixel found
			// is the nearest one.
			best := math.MaxFloat64
			for _, o := range ring {
				if at(x+o.dx, y+o.dy) {
					best = o.d
					break
				}
			}
			if best > width {
				continue
			}
			f := 1.0
			if best > haloInner {
				f = 1.0 - (best-haloInner)/(width-haloInner)
			}
			a := haloAlpha * clamp01Halo(f)
			if a <= 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{halo.R, halo.G, halo.B, uint8(a * 255)})
		}
	}
}

// haloOffset is one neighbour of the search square and its distance.
type haloOffset struct {
	dx, dy int
	d      float64
}

var haloRings sync.Map // size -> []haloOffset

// haloRing lists the offsets of the (2r+1)^2 search square, nearest first,
// computed once per canvas size. Distances come from the same Hypot call the
// brute-force scan used, so the result is bit-identical.
func haloRing(size int) []haloOffset {
	if v, ok := haloRings.Load(size); ok {
		return v.([]haloOffset)
	}
	r := int(math.Ceil(haloWidth(size)))
	var ring []haloOffset
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			ring = append(ring, haloOffset{dx, dy, math.Hypot(float64(dx), float64(dy))})
		}
	}
	sort.SliceStable(ring, func(i, j int) bool { return ring[i].d < ring[j].d })
	haloRings.Store(size, ring)
	return ring
}

// clamp01Halo clamps to [0,1] (gen has its own clamp01; the icon package keeps
// this local so the two halo implementations stay independent).
func clamp01Halo(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
