# fipindicateur - project conventions

A tiny Go system-tray client for FIP (Radio France) webradios. Linux/GNOME/X11
is the supported target; macOS and Arch are kept buildable.

## Build / run / test

```
make run      # build + run with version stamped from git describe
make build    # build ./fipindicateur (ldflags stamp internal/version.Version)
make test     # go test ./...
make lint     # gofmt check, em-dash ban, go vet, go test, go build (the CI gate)
make install  # user-level install (binary + launcher + icons), no sudo
```

`fipindicateur stats [--out file.html] [--no-open]` builds the local listening
report. `fipindicateur version` prints the stamped version.

## House style (enforced by `make lint`, do not break)

- **No em dashes (U+2014) anywhere**, including HTML/JS/CSS. Use middot (`·`),
  colon, or parentheses. CI greps the whole tree for it.
- `gofmt` clean (the `internal/icon/gen` generator is exempt).
- Stdlib-only for new code. This app is deliberately dependency-light (see
  `go.mod`): adding a module is spending an innovation token, justify it.
- French UI copy, accents included. Comments and identifiers in English.
- Never version-suffix artifacts (`_v2`). Improve in place; git is the history.

## Telemetry is measurable by design (the core invariant)

Listening analytics live in `internal/events` (the log) and `internal/stats`
(the derivation + report). The point of the design is that **you cannot add a
user action without it being measurable**:

- Every clickable tray item is wired through **`App.on(item, kind, fn)`** in
  `internal/ui/ui.go`. `on` records a typed `events.Event` before running the
  handler. The raw click loop `onClick` is called in exactly one place (inside
  `on`); **`TestSingleOnClickCallSite` fails the build if a second raw
  `go a.onClick(` appears.** Add menu items via `a.on`, never `onClick`.
- Fixed-kind actions (open link, quit, view report) pass their `events.Kind`
  to `on`. State-dependent actions pass `""` and record at their source with
  the accurate value: play/pause in `setPlayingUI` (the single chokepoint for
  menu + MPRIS + media keys), volume/mute in their setters (including external
  pavucontrol changes), station changes in `startStation` (the Markov edge).
- App lifecycle (`app_start`/`app_stop`) records in `OnReady`/`OnExit`.
- New action kinds go in `internal/events` (the `Kind` constants) and must be
  wired in `ui.go`; `TestActionKindsWired` checks none is left dangling.

When you add a feature with a new user action: define the `Kind`, wire it via
`on` (or record at source if state-dependent), and the report picks it up.

## Privacy by design (non-negotiable)

- **Opt-in, default off.** `config.Stats` gates the recorder; nothing is written
  until the user enables "Statistiques d'ecoute (local)".
- **Local only, no network telemetry, ever.** Events are appended to
  `~/.local/share/fipindicateur/events.jsonl`. The report is a self-contained
  offline HTML page (system fonts, inline everything, no CDN); it is served over
  an ephemeral `127.0.0.1` port only because Snap Firefox cannot open `file://`
  under hidden dirs.
- **See / edit / delete.** The Statistiques submenu opens the report, opens the
  data folder, and deletes the log (two-click confirm). `Recorder.Clear` removes
  only `events.jsonl`, never `history.jsonl` (a separate consent, a separate
  file). Users can also just delete or grep the JSONL by hand.
- Events record **behaviour** (play, pause, zap, volume), never track identity
  (artist/title stay in `internal/histlog`), to avoid duplicating listening PII.

## Storage: append-only versioned JSONL

Both logs (`histlog`, `events`) are one JSON object per line, with a `v` schema
version, best-effort writes that never block or crash playback, and no rotation.
JSONL over SQLite: zero dependencies, greppable, trivially productisable, and
the schema extends by adding omitempty fields. The recorder writes async on a
buffered channel and drops rather than blocks if disk stalls.

## Stats derivation is pure and calibrated

`stats.Build([]events.Event, now)` is a pure function, unit-tested over
synthetic event slices (`stats_test.go`). It reconstructs sessions from
play/pause boundaries, splits listening time across stations and hour/weekday
buckets, builds the row-normalised station transition matrix (the Markov graph),
and evaluates achievements. Redundant same-state events are deduplicated in the
walk, so the UI may emit play/pause liberally without corrupting counts.

The report **states its own sample size** (`Calibration`) and warns when the
session count is small: indicative, not significant. Keep that honesty when
adding metrics, and when adding charts prefer direct labels over color-only
identity (13 stations exceed any categorical palette).

## Layout

```
cmd/fipindicateur   entrypoint + subcommand dispatch (stats, version)
internal/ui         tray menu, wiring, the App.on telemetry chokepoint
internal/events     opt-in event log + async Recorder
internal/stats      pure derivation + embedded self-contained HTML report
internal/config     persisted settings (~/.config/fipindicateur/config.json)
internal/histlog    opt-in track-change log (separate from events)
internal/prefs      explicit taste signals (like/dislike); click IS the consent
internal/update     GitHub-releases update check (read-only, no self-replace)
internal/version    build version (ldflags-stamped)
internal/{player,metadata,mpris,notify,stations,icon,wiki,vu,open}  the rest
```
