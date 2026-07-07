# Contributing

Thanks for helping out. This is a small, deliberately boring codebase: keep it
that way.

## Setup

You need Go 1.26+ and libmpv development headers:

```sh
sudo apt install libmpv-dev   # Ubuntu/Debian
# brew install mpv            # macOS
# sudo pacman -S mpv          # Arch
```

## Before you push

CI checks formatting and will fail on unformatted code. Run:

```sh
make fix    # gofmt -w + go mod tidy
make lint   # exactly what CI runs: gofmt check, go vet, go test, go build, no em dashes
```

## Layout

- `cmd/fipindicateur`: entry point (systray run loop, signal handling).
- `internal/player`: libmpv cgo wrapper. Live-radio semantics: pause = stop,
  play = loadfile (rejoin live).
- `internal/metadata`: `Provider` interface with two impls, `livemeta`
  (primary, polls Radio France) and `icy` (fallback, parses mpv's media-title).
  `Manager` composes them.
- `internal/mpris`: MPRIS2 D-Bus, `//go:build linux` (no-op stub elsewhere).
- `internal/notify`: desktop notifications, `//go:build linux` (stub elsewhere).
- `internal/histlog`: opt-in JSONL track log (one JSON object per line).
- `internal/stations`: the 13 webradios and stream-URL builder.
- `internal/ui`: the tray menu and wiring.
- `internal/open`: open URLs, per-GOOS.
- `internal/icon`: embedded tray icons + `gen/` generator.

## Portability

Linux is the target today; macOS/Arch are kept buildable. Platform-specific
code lives behind `//go:build` tags with a no-op stub for other OSes. The
`build-macos` CI job is the canary: if it breaks, fix the gating, don't delete
the job.

## Style

- Small, conventional commits.
- Metadata or network failures must **never** affect playback: back off, log,
  carry on.
- No em dashes anywhere (code, docs, UI copy); `make lint` enforces it. Use a
  colon, comma, parentheses, or « guillemets » instead.
- Don't add dependencies or non-standard tooling without a clear reason.
