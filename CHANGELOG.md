# Changelog

## Sprint 3 · 2026-07-11 · La fin d'émission

### Added
- Taste signals: explicit like/dislike on the current track, straight from the
  tray. Opt-in and logged to its own `prefs.jsonl` (a separate consent, a
  separate file from history), with the verdict persisted on the menu item.
- « Fin d'émission » report: the listening page rebuilt as a late-night radio
  rundown (a conducteur with timecodes), dark and light. Four new dataviz on
  top of the existing grille/zapping/palmarès: Les époques (release-year bars),
  La carte du ciel (a 2D artist constellation), L'économie du disque (indie vs
  major labels), and À ton goût (explicit verdicts plus implicit hints).
- Extended stats derivation: release-year epochs, artist-metadata enrichment
  (genres, countries, labels, constellation coords), and taste stats that pair
  explicit verdicts with implicit zap-out and early-pause signals. The report
  states its own sample size and flags a small session count as indicative.

### Infra
- `tools/enrich` companion: resolves the artists you heard against Wikidata
  (genre, country, label, description) and projects them to a 2D affinity map
  via embeddings, cached locally, feeding the report's carte du ciel.
- `web/` SPA toolchain: an esbuild bundle plus D3 and subsetted WOFF2 fonts,
  compiled at dev time into a single self-contained `report.html.tmpl` (no Node
  at build time). `make web` regenerates it; CI and the em-dash lint skip
  `node_modules`.

### Docs
- README reworked as a 30-second human read (1574 to 410 words): hero image,
  pitch, quick start, links. The detail moved intact to `docs/FEATURES.md`,
  `docs/INSTALL.md` and `docs/DEVELOPMENT.md`.
- `docs/social-preview.png`: a 1280x640 tuner-dial hero in the « Fin
  d'émission » tokens (source under `web/hero/`), ready for GitHub's social
  preview. Report screenshot regenerated from fictional fixture data; the
  stale Markov capture removed.

## Sprint 2 · 2026-07-11 · Le fondu et la fenêtre

### Added
- Zapping between stations while playing now crossfades (~4 s, equal-power
  sin/cos on mpv's internal volume) instead of hard-cutting: the incoming
  stream buffers on a second libmpv handle while the outgoing keeps playing,
  then the two rivers meet. `crossfade_secs` in the config, 0 = old cut (#1).
- Real sliders via zenity: volume with live apply while dragging (Esc reverts,
  OK records exactly one event), and the crossfade duration (0-10 s) under
  Réglages. Preset checkboxes remain as fallback when zenity is absent (#3).

### Changed
- The HiFi quality toggle stops before reloading: fading a station against
  itself at another bitrate would phase/echo, so it stays a clean cut (#1).

### Infra
- Windows 80/20: every non-cgo package compiles for GOOS=windows;
  `make windows` cross-builds fipindicateur.exe (mingw-w64 + pinned shinchiro
  libmpv dev drop) with the tray icon wrapped PNG-in-ICO through the single
  setTrayIcon chokepoint. Ship the exe next to libmpv-2.dll. Untested on real
  Windows hardware yet (#2).
- CI: `build-windows` job cross-builds and uploads exe+dll; releases attach
  `fipindicateur-windows-x86_64.zip` (#2).

## Sprint 1 · 2026-07-10 · La radio en couleurs

### Added
- Report bars wear each webradio's official brand color, sourced from
  radiofrance.fr's own CSS design tokens, with a computed per-theme
  legibility clamp (3:1 vs track) so gold and navy stay visible (#3).
- The animated tray VU bars crossfade to the active station's color over
  10 s when zapping, riding the existing 6 fps frames (at most 16 extra
  icon updates per change). Paused stays neutral: color only while music
  plays (#4).
- With the animated icon off, the frozen bars glyph now still wears the
  active station's brand tint while playing, so the FIP colors persist
  without the VU motion. Paused/stopped stays neutral, same as the
  animator's fade-out (#5).

### Infra
- Session watchdog (`fipindicateur-watchdog`, systemd user unit, installed
  by `make install`): probes gnome-shell liveness over D-Bus and CPU while
  the app runs, and kills the radio (never the shell) on sustained trouble.
  Belt and braces after the 2026-07-09 freeze (#2).

### Fixed
- Reinstalled post-freeze: autostart entry re-armed with the canonical
  installed binary path, quarantine lifted (#1).
