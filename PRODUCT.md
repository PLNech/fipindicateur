# Product

## Register

brand

(The tray app itself is a utility with a micro-minimal tray presence; this file
governs its designed surfaces: the local listening report (`fipindicateur
stats`), where design IS the product (a personal editorial "Wrapped"), and the
upcoming on-click control popup (play/pause, like/dislike, volume), which
shares the same design system in product register. See DESIGN.md.)

## Users

One person: the listener themself. A FIP devotee on Linux/GNOME who opens the
report from the tray a few times a month, usually at night, out of curiosity
and self-portraiture. French UI, accents included. Technically literate, allergic
to marketing gloss, fond of honest numbers.

## Product Purpose

Turn the opt-in local logs (`events.jsonl` behaviour + `history.jsonl` tracks,
enriched offline via Wikidata/Wikipedia metadata) into a self-contained,
offline, data-rich editorial page: sessions, stations, hours, epochs, genres,
countries, labels, and a 2D constellation of listened artists. Success: the
listener scrolls the whole page, learns something true about themself, and
trusts every number because each states its own sample size.

## Brand Personality

Radio nocturne, chaleureux, grave. The warmth of FIP at 2am crossed with the
statistical seriousness of The Economist doing music sociology. Editorial
confidence: big committed typography, print-quality dataviz, ink discipline.
Singular, not templated: this should look like nothing else on anyone's screen.

## Anti-references

- Spotify Wrapped clones: neon gradients, duotone blobs, shouty share-cards.
- Generic SaaS/cream dashboards: tile grids of hero metrics, card monoculture.
- Emojis as visual language (current report's achievement badges are the
  offender to retire). Iconography is typographic or drawn SVG, or absent.
- Default-library chart aesthetics (Chart.js/ECharts out-of-the-box look).

## Design Principles

1. **Editorial, not dashboard.** The page narrates a listening year like a
   long-form feature: sequenced sections, captions that state a finding, prose
   that interprets. Charts are figures in an article, not widgets in a grid.
2. **Calibrated honesty.** Every metric carries its n; small samples say so
   ("indicatif, pas significatif"). No chart exaggerates: percentage axes run
   to 100, no truncated baselines.
3. **Direct labels over legends.** 13 stations exceed any categorical palette;
   identity comes from labels on the marks, never from color alone.
4. **Self-contained and local, always.** One HTML file, zero CDN, zero runtime
   network. Build-time tooling (bundler, viz libs, embeddings) is welcome;
   shipped bytes are static and offline.
5. **Measurable by design.** Any new interactive affordance on the page or in
   the tray stays wired through the telemetry chokepoints (`App.on`, recorded
   kinds); features arrive measurable or not at all.

## Accessibility & Inclusion

- WCAG AA contrast (4.5:1 body, 3:1 large text) in both light and dark themes.
- `prefers-reduced-motion` respected on all motion; reveals never gate content.
- Series identity never by color alone (direct labels, patterns, position).
- System font stack acceptable; if fonts ship, they are embedded, not fetched.
