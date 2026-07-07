# le fipindicateur

[![CI](https://github.com/PLNech/fipindicateur/actions/workflows/ci.yml/badge.svg)](https://github.com/PLNech/fipindicateur/actions/workflows/ci.yml)

A tiny system-tray app to listen to **FIP** (Radio France) webradios. Pick a
station, see who you're hearing, play/pause from the tray or your media keys.
Unofficial client.

Linux (Ubuntu 24.04, GNOME/X11) is the supported target today. macOS and Arch
are kept buildable and planned.

![tray menu mockup](docs/screenshot.png)

> The image above is a **mockup render** of the menu, not a live screenshot.
> Real tray screenshot pending (TODO) — GNOME's tray menu can't be captured
> headlessly.

## What it does

- **13 FIP webradios** — FIP, Rock, Jazz, Groove, Monde, Nouveautés, Reggae,
  Electro, Hip-Hop, Metal, Sacré français !, Pop, Cultes.
- **Now playing** — artist and title, live, from Radio France's metadata (with
  an ICY stream-title fallback). Click the track to open its music link (often
  Apple Music) or a web search.
- **Play / pause** — for live radio, pause is a full stop and play rejoins the
  live edge (no stale buffer).
- **Historique** — the last ten tracks you heard, click to reopen.
- **Réglages** — high quality (AAC 192k), notifications, launch at login.
- **MPRIS2** — controllable with `playerctl` and desktop media keys; shows
  track + cover in your desktop's media widget.
- **Desktop notifications** on track change, with cover art, crediting the
  album / year / label when known.

## Install

Runtime needs **libmpv** (the app links libmpv via cgo, and mpv plays the
streams).

| OS | Runtime | Build |
|----|---------|-------|
| Ubuntu / Debian | `sudo apt install libmpv2` | `sudo apt install libmpv-dev` |
| Arch | `sudo pacman -S mpv` | `sudo pacman -S mpv` |
| macOS | planned | `brew install mpv` |

Then:

```sh
go build -o fipindicateur ./cmd/fipindicateur
./fipindicateur
```

Go 1.26+ is required (see `go.mod`).

> On GNOME the system-tray (StatusNotifierItem) needs the
> **ubuntu-appindicators** extension enabled — it ships enabled on Ubuntu.

## Usage

Launch `./fipindicateur`. A broadcast-waves glyph appears in your top bar; click
it for the menu. Your last station and settings are remembered in
`~/.config/fipindicateur/config.json`.

**Launch at login:** toggle *Réglages → Lancer au démarrage* (writes
`~/.config/autostart/fipindicateur.desktop`), or add it manually:

```sh
mkdir -p ~/.config/autostart
cp packaging/fipindicateur.desktop ~/.config/autostart/   # if provided
# or just toggle it in-app
```

**Media keys / playerctl:**

```sh
playerctl -p fipindicateur play-pause
playerctl -p fipindicateur metadata
```

## Streams (verified)

All 13 icecast `midfi` (128k MP3) streams verified with `curl -sI` on
2026-07-07 — every one returned **HTTP 200, `content-type: audio/mpeg`**:

| Station | slug | midfi.mp3 | livemeta id |
|---------|------|-----------|-------------|
| FIP | fip | 200 audio/mpeg | 7 |
| Rock | fiprock | 200 audio/mpeg | 64 |
| Jazz | fipjazz | 200 audio/mpeg | 65 |
| Groove | fipgroove | 200 audio/mpeg | 66 |
| Monde | fipworld | 200 audio/mpeg | 69 |
| Nouveautés | fipnouveautes | 200 audio/mpeg | 70 |
| Reggae | fipreggae | 200 audio/mpeg | 71 |
| Electro | fipelectro | 200 audio/mpeg | 74 |
| Hip-Hop | fiphiphop | 200 audio/mpeg | — (ICY) |
| Metal | fipmetal | 200 audio/mpeg | 77 |
| Sacré français ! | fipsacrefrancais | 200 audio/mpeg | — (ICY) |
| Pop | fippop | 200 audio/mpeg | — (ICY) |
| Cultes | fipcultes | 200 audio/mpeg | — (ICY) |

URL template: `https://icecast.radiofrance.fr/{slug}-{quality}.{ext}?id=radiofrance`
(quality/ext = `midfi/mp3` default, `hifi/aac` opt-in). `fipcultes` icecast is
present, so no HLS fallback is needed. The four stations without a known
livemeta id use the ICY stream-title fallback for now-playing.

## Development

```sh
make run     # build & run
make test    # go test ./...
make lint    # the exact checks CI runs (gofmt, vet, test, build)
make fix     # gofmt -w + go mod tidy
make icons   # regenerate the tray icons
```

CI checks formatting — run `make fix` before pushing. See `CONTRIBUTING.md`.

## Credits & attribution

**FIP / Radio France.** This is an **unofficial** client. The streams and all
now-playing metadata are the property of **Radio France**. Please listen to and
support FIP through their official channels — the
[FIP website](https://www.radiofrance.fr/fip) and the official Radio France app.
This project only points a tray menu at their public streams; it adds no
content of its own.

**Artists.** A point of this app is to surface *who you're hearing*. Every track
links out — to its music page (Radio France's `path`, often Apple Music) or a
search — precisely so artists get discovered and credited, and notifications
name the album, year and label when known.

**Code lineage.**
- The **player** (libmpv cgo wrapper) and **MPRIS2** layers derive from
  [fip-player](https://github.com/DucNg/fip-player) by DucNg (WTFPL).
- The tray uses [fyne-io/systray](https://github.com/fyne-io/systray).
- D-Bus via [godbus/dbus](https://github.com/godbus/dbus).
- Thanks to the community that documented the Radio France `livemeta` endpoint,
  notably [Zopieux's gist](https://gist.github.com/Zopieux/38c9cf4b9df3af521d7be1e0b1e26bda).

## License

GPL-3.0-or-later. See [LICENSE](LICENSE). The player/MPRIS code derives from
WTFPL-licensed work, which is GPL-compatible.
