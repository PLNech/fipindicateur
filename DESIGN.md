# Design system · « Fin d'émission »

The visual system for fipindicateur's designed surfaces: the listening report
(`fipindicateur stats`, brand register: the page IS the product) and, next, the
tray control popup (product register: same tokens, denser ergonomics). One
world, two rooms.

## Concept

One listener, 2am, dark room, the screen is the only light. The page glows like
a tuner dial; it never floodlights like a dashboard. Narratively, the report is
the *conducteur* (the minute-by-minute rundown sheet) of a night broadcast whose
host and sole audience are the same person. Statistics are read as programme
segments, in order, with the seriousness of a good producer and the warmth of a
night voice.

Anti-references (hard): Spotify Wrapped neon clones; SaaS cream dashboards and
hero-metric tile grids; emoji iconography; the saturated editorial-magazine lane
(italic display serif + tiny mono labels + rules); default chart-library looks.

## Color (OKLCH, committed strategy)

One committed color: FIP rose. It marks everything that is *the listener*:
their series, their star, their tastes, their unlocked insignia. Dial amber is
the supporting cast for context data. Everything else is night and ink.

### Dark · « Antenne de nuit » (default)

| Token | Value | Role |
|---|---|---|
| `--night-0` | `oklch(0.16 0.012 50)` | page background (warm lamp-lit black, never pure) |
| `--night-1` | `oklch(0.20 0.015 55)` | figure surfaces |
| `--night-2` | `oklch(0.24 0.016 55)` | raised elements, thumbs |
| `--ink` | `oklch(0.94 0.025 85)` | primary text, chart marks (backlit needle white) |
| `--ink-muted` | `oklch(0.74 0.02 80)` | captions, secondary (AA on night-0) |
| `--hairline` | `oklch(0.36 0.02 60)` | rules, graduations, locked outlines |
| `--fip` | `oklch(0.63 0.23 356)` | FIP rose: marks, fills, latched states |
| `--fip-ink` | `oklch(0.78 0.15 356)` | rose that passes AA as text on night-0 |
| `--amber` | `oklch(0.80 0.12 75)` | context data marks |
| `--amber-dim` | `oklch(0.55 0.08 70)` | de-emphasized context |

### Light · « Édition papier »

Cold newsprint, warm ink: the morning-after print edition of the night show.

| Token | Value | Role |
|---|---|---|
| `--paper-0` | `oklch(0.965 0.004 356)` | page background (true off-white, faint rose cast) |
| `--paper-1` | `oklch(0.93 0.005 356)` | figure surfaces |
| `--ink` | `oklch(0.24 0.02 40)` | text and marks (warm ink) |
| `--ink-muted` | `oklch(0.45 0.02 40)` | captions |
| `--hairline` | `oklch(0.80 0.01 40)` | rules |
| `--fip` | `oklch(0.55 0.24 356)` | rose, same role |
| `--amber` | `oklch(0.62 0.12 70)` | context data (darkened for paper) |

### Rules

- Rose is reserved for listener salience. Never decorative, never a gradient.
- Station brand colors appear only in station-identity charts, passed through
  the existing `legible()` contrast adjustment (3:1 minimum on current bg).
- Body text 4.5:1 minimum, large text 3:1, both themes, verified not assumed.
- Series identity never by color alone: direct labels always (13 stations
  exceed any categorical palette).

## Typography

Physical references: a 1970s French receiver faceplate; the typewritten ORTF
programme sheet. Two families, embedded as subsetted WOFF2 data URIs (the page
is self-contained; no font CDN):

- **Bricolage Grotesque** (OFL): display, headings, all numerals. Warm, slightly
  eccentric, French-designed. Weights 400 / 700 / 800.
- **Literata** (OFL): interpretive prose and captions. Weights 400 / 400italic / 700.

| Token | Spec |
|---|---|
| `--t-display` | Bricolage 800, `clamp(2.6rem, 7vw, 5rem)`, letter-spacing -0.02em |
| `--t-h2` | Bricolage 700, `clamp(1.5rem, 3vw, 2.25rem)` |
| `--t-h3` | Bricolage 700, 1.25rem |
| `--t-body` | Literata 400, 1.0625rem, line-height 1.65 (+0.05 on dark) |
| `--t-caption` | Literata 400 italic, 0.9375rem, `--ink-muted` |
| `--t-data` | Bricolage 400, 0.8125rem, `font-feature-settings: "tnum"` |

All data numerals are tabular (`tnum`). `text-wrap: balance` on h1-h3,
`text-wrap: pretty` on prose. No all-caps body; caps only for short dial labels.

## The conducteur system (the one kicker, named)

Each section opens with a rundown line: a tabular timecode, a thin rule, the
segment title. Example: `00:00 · Ouverture d'antenne`. This numbering is
earned: the page IS an ordered rundown, and the sequence carries meaning.
It is the only kicker grammar on the page; no other section gets an eyebrow,
a number, or a tracked uppercase label.

Rundown (segment order):

1. `Ouverture d'antenne` : totals, range, calibration statement
2. `La grille de tes nuits` : hours × weekdays
3. `Le zapping` : the Markov transition graph
4. `Les époques` : release-year timeline (enriched)
5. `La carte du ciel` : the 2D artist constellation (enriched, signature visual)
6. `L'économie du disque` : labels, majors vs independents (enriched)
7. `À ton goût` : explicit verdicts + implicit hints, hints stated as hints
8. `Palmarès` : insignia (achievements), no emoji
9. `Fin d'émission` : privacy statement, data paths, how to erase

Sections 4-7 render only when their data exists; absence collapses cleanly.

## Layout

- Prose column 68ch max; figures may widen to 1100px and bleed asymmetrically.
  The grid breaks deliberately for the constellation (full-bleed night sky).
- Spacing scale: 4 / 8 / 12 / 20 / 32 / 52 / 84 px; section separation
  `clamp(4rem, 10vh, 7rem)`. Tight inside figures, generous between segments.
- No card grids. Figures are figures in an article: title as a finding
  ("Tu écoutes surtout après 22h"), the viz, a caption stating n.
- z-index scale: `base(0) < graduation(1) < sticky(10) < overlay(20)`.

## Dataviz language

- Ink marks on night (or ink on paper): hairline graduations like dial
  markings, direct labels on marks, no legends, no gridlines heavier than
  `--hairline`.
- Percentage axes run 0-100. No truncated baselines. Every figure caption
  states its sample size; below the significance threshold it says
  "indicatif, pas significatif" (the existing Calibration contract).
- Custom D3 (build-time bundled), house-styled: no default library aesthetics.
- Hover/tooltip may enrich but never carries sole access to a value; keyboard
  focusable points, `focus-visible` rose outline 2px.
- The constellation: each artist a star; magnitude (radius + glow) = listening
  count; position = 2D projection of Wikidata affinity; rose star = most
  played; labels for the brightest N, others on hover/focus.

## Components

- **Figure**: finding-as-title, viz, calibrated caption. The unit of the page.
- **Inline stat**: Bricolage numeral inside prose, rose when it is "you".
  Never a tile wall.
- **Insigne** (achievement): a drawn SVG medallion (dial, needle, wave motifs
  per achievement ID). Locked: hairline outline, muted. Unlocked: rose accents.
  Zero emoji anywhere.
- **Player controls** (spec for the upcoming tray popup; the report does not
  ship them yet):
  - *Play/pause*: 40px circular, hairline ring, ink glyph; playing state fills
    rose with night glyph. Glyph morphs triangle to bars (150ms, reduced-motion
    swaps instantly).
  - *Like / dislike*: two drawn glyphs (rising wave / falling wave). States:
    idle (hairline), hover (ink), `liked` (rose fill, persists), `disliked`
    (amber-dim, persists). Latched states survive the popup closing.
  - *Volume*: horizontal fader, hairline track with tick graduations, `--night-2`
    rectangular fader cap with a rose position line. Keyboard: arrows step 5.
  - *Mini VU*: ties into the existing tray VU aesthetic, amber pair of bars.
- Every popup control routes through the Go telemetry chokepoint (`App.on`
  equivalent): controls arrive measurable or not at all.

## Motion

One opening choreography, then calm:

- Header: the dial arc draws itself and the needle sweeps once (600ms,
  ease-out-quint); headline numerals count up (800ms, once).
- Scroll entries: 200-300ms fade/rise per figure, content fully visible by
  default (enhancement only, never gating; hidden-tab safe).
- Charts: marks settle with at most one stagger per figure.
- `prefers-reduced-motion: reduce`: everything instant, needle pre-settled.

## Self-containment (build contract)

One HTML file: CSS, JS, fonts (subsetted WOFF2 data URIs), and drawn SVG all
inline. Zero CDN, zero runtime network. Data arrives as the `__FIP_DATA__`
JSON placeholder replaced by the Go binary. Build-time tooling (bundler, D3,
font subsetting) is welcome; shipped bytes are static. No em dash (U+2014)
anywhere, including generated output: use `·`, colon, or parentheses.
