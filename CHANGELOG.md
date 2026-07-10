# Changelog

## Sprint 1 · 2026-07-10 · La radio en couleurs

### Added
- Report bars wear each webradio's official brand color, sourced from
  radiofrance.fr's own CSS design tokens, with a computed per-theme
  legibility clamp (3:1 vs track) so gold and navy stay visible (#3).
- The animated tray VU bars crossfade to the active station's color over
  10 s when zapping, riding the existing 6 fps frames (at most 16 extra
  icon updates per change). Paused stays neutral: color only while music
  plays (#4).

### Infra
- Session watchdog (`fipindicateur-watchdog`, systemd user unit, installed
  by `make install`): probes gnome-shell liveness over D-Bus and CPU while
  the app runs, and kills the radio (never the shell) on sustained trouble.
  Belt and braces after the 2026-07-09 freeze (#2).

### Fixed
- Reinstalled post-freeze: autostart entry re-armed with the canonical
  installed binary path, quarantine lifted (#1).
