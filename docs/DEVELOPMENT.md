# Development

## Make targets

```sh
make run      # build & run (version stamped from git describe)
make build    # build ./fipindicateur
make test     # go test ./...
make lint     # the exact checks CI runs: gofmt, vet, test, build, no em dashes
make fix      # gofmt -w + go mod tidy
make icons    # regenerate the tray icons
make web      # rebuild internal/stats/report.html.tmpl from web/ (needs Node)
make windows  # cross-build the Windows tray binary
```

CI checks formatting: run `make fix` before pushing. See
[CONTRIBUTING.md](../CONTRIBUTING.md).

## House style (enforced by `make lint`)

- **No em dashes (U+2014) anywhere**, including HTML/JS/CSS. Use middot (`·`),
  colon, or parentheses. CI greps the whole tree.
- `gofmt` clean (the `internal/icon/gen` generator is exempt).
- Stdlib-only for new Go code; the app is deliberately dependency-light.
- French UI copy, accents included. Comments and identifiers in English.
- Never version-suffix artifacts (`_v2`). Improve in place; git is the history.

## Layout

```
cmd/fipindicateur   entrypoint + subcommand dispatch (stats, version)
internal/ui         tray menu, wiring, the App.on telemetry chokepoint
internal/events     opt-in event log + async Recorder
internal/stats      pure derivation + embedded self-contained HTML report
internal/config     persisted settings (~/.config/fipindicateur/config.json)
internal/histlog    opt-in track-change log (separate from events)
internal/prefs      opt-in like/dislike verdict log
internal/update     GitHub-releases update check (read-only, no self-replace)
internal/version    build version (ldflags-stamped)
internal/{player,metadata,mpris,notify,stations,icon,wiki,vu,open}  the rest
```

## Telemetry is measurable by design (the core invariant)

Listening analytics live in `internal/events` (the log) and `internal/stats`
(the derivation + report). The design guarantees that **you cannot add a user
action without it being measurable**:

- Every clickable tray item is wired through **`App.on(item, kind, fn)`** in
  `internal/ui/ui.go`. `on` records a typed `events.Event` before running the
  handler. The raw click loop `onClick` is called in exactly one place (inside
  `on`); `TestSingleOnClickCallSite` fails the build if a second raw
  `go a.onClick(` appears. Add menu items via `a.on`, never `onClick`.
- Fixed-kind actions (open link, quit, view report) pass their `events.Kind` to
  `on`. State-dependent actions pass `""` and record at their source with the
  accurate value: play/pause in `setPlayingUI`, volume/mute in their setters,
  station changes in `startStation` (the Markov edge).
- App lifecycle (`app_start`/`app_stop`) records in `OnReady`/`OnExit`.
- New action kinds go in `internal/events` (the `Kind` constants) and must be
  wired in `ui.go`; `TestActionKindsWired` checks none is left dangling.

When you add a feature with a new user action: define the `Kind`, wire it via
`on` (or record at source if state-dependent), and the report picks it up.

## Storage: append-only versioned JSONL

Both logs (`histlog`, `events`, and `prefs`) are one JSON object per line, with
a `v` schema version, best-effort writes that never block or crash playback, and
no rotation. JSONL over SQLite: zero dependencies, greppable, trivially
productisable, and the schema extends by adding omitempty fields. The recorder
writes async on a buffered channel and drops rather than blocks if disk stalls.

## Stats derivation is pure and calibrated

`stats.Build(...)` is a pure function, unit-tested over synthetic event slices
(`stats_test.go`). It reconstructs sessions from play/pause boundaries, splits
listening time across stations and hour/weekday buckets, builds the
row-normalised station transition matrix (the Markov graph), derives
release-year epochs and taste signals, folds in the optional enrichment, and
evaluates achievements. Redundant same-state events are deduplicated in the
walk, so the UI may emit play/pause liberally without corrupting counts.

The report **states its own sample size** (`Calibration`) and warns when the
session count is small (indicative, not significant). Keep that honesty when
adding metrics; prefer direct labels over color-only identity (13 stations
exceed any categorical palette). Design tokens and voice: see
[DESIGN.md](../DESIGN.md) and [PRODUCT.md](../PRODUCT.md).

## The report toolchain (`web/`)

The report is one self-contained HTML file (inlined CSS, JS, D3, subsetted WOFF2
fonts) compiled at dev time into `internal/stats/report.html.tmpl`. The Go
binary replaces a single `__FIP_DATA__` placeholder with the marshaled report
JSON at render time; no Node at build time.

```sh
cd web
npm install
npm run build     # bundle web/src -> internal/stats/report.html.tmpl  (make web)
npm run fonts     # re-subset the WOFF2 faces (only when bumping font versions)
```

Dev-only helper scripts (not part of the build):

- `web/shoot.py`: screenshot the report in both themes at desktop + mobile.
- `web/shoot_report.py`: render the report from the fictional fixture
  (`web/fixtures/report-enriched.json`) and regenerate `docs/stats-report.png`.
- `web/hero/build.py`: build the social-preview hero (`docs/social-preview.png`)
  from `web/hero/hero.template.html`, inlining the committed font subsets.

## `tools/enrich` (optional companion)

A separate opt-in Python tool that resolves the artists you heard against public
Wikidata/Wikipedia APIs and writes `enriched.json` (genres, countries, labels,
descriptions, and a 2D affinity projection) for the report's carte du ciel. The
Go binary stays stdlib-only; enrichment lives here on purpose. Full docs:
[tools/enrich/README.md](../tools/enrich/README.md).
