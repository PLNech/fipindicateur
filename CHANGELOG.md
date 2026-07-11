# Changelog

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
