# Features

Everything `fipindicateur` does, in detail. For a 30-second overview see the
[README](../README.md).

![tray menu mockup](screenshot.png)

> The image above is a **mockup render** of the menu, not a live screenshot.
> A real tray screenshot is pending: GNOME's tray menu cannot be captured
> headlessly.

## Listening

- **13 FIP webradios**: FIP, Rock, Jazz, Groove, Monde, Nouveautés, Reggae,
  Electro, Hip-Hop, Metal, Sacré français !, Pop, Cultes.
- **Now playing**: artist and title, live, from Radio France's metadata (with
  an ICY stream-title fallback). Click the track to open the artist's
  **Wikipédia article**: the app cleans the credit string down to the primary
  artist (Radio France's curated `highlightedArtists` when present, else the
  credit cut at the first separator), resolves it via the opensearch API on
  fr.wikipedia then en.wikipedia (niche artists often live on en only), and
  falls back to the fr search page, so a click never dead-ends. The « Voir… »
  submenu also offers the Radio France music link (often Apple Music) as a
  secondary option. When no artist is known, a DuckDuckGo search steps in.
- **Play / pause**: for live radio, pause is a full stop and play rejoins the
  live edge (no stale buffer). Zapping between stations crossfades (~4 s,
  equal-power) instead of hard-cutting; `crossfade_secs` in the config, 0 for
  the old cut.
- **Historique**: the last ten tracks you heard, click to reopen.
- **Réglages**: high quality (AAC 192k), notifications, launch at login, local
  history log, and the two opt-in analytics toggles below.

## Desktop integration

- **MPRIS2**: controllable with `playerctl` and desktop media keys; shows track
  and cover in your desktop's media widget. The Volume property is read/write,
  so `playerctl volume 0.5` works and the menu follows.
- **Volume, one knob**: the tray submenu (Muet + a zenity slider, or 10/25/50/
  75/100 % presets as fallback) drives the app's **PulseAudio stream volume**
  (mpv `ao-volume`), the same per-app slider pavucontrol and GNOME show. Adjust
  it there and the menu + MPRIS follow. PulseAudio remembers the level across
  restarts (module-stream-restore); the app never overwrites it at startup, and
  only an explicit preset click, Muet, or MPRIS write touches the stream.
- **Desktop notifications** on track change, with cover art, crediting the
  album, year and label when known.
- **Icône animée**: while playing, the tray glyph becomes a 4-bar VU meter
  driven by real audio levels (mpv astats filter), with VU physics (instant
  attack, slow decay). Engineered cheap: 6 fps cap, 12 quantized levels, frame
  cache, identical frames never redrawn. Measured (pidstat, 60 s, hifi stream):
  ~6.4% of one core playing with animation vs ~4.8% without, so the animation
  itself costs ~1.6%; the bulk is mpv's audio decoding. Toggle in Réglages, on
  by default.

## Local analytics (opt-in, off by default)

Two separate toggles, two separate files, two separate consents. Nothing is
written until you enable them, nothing leaves your machine, ever. See the
[README's privacy note](../README.md#privacy) for the full contract.

- **Historique local**: append every track change to
  `~/.local/share/fipindicateur/history.jsonl`, one JSON object per line
  (`{v, ts, station, artist, title, album, year, label}`). Greppable, easy to
  post-process, and the schema can grow.
- **Goûts (likes / dislikes)**: an explicit verdict on the current track,
  straight from the tray, logged to its own `prefs.jsonl`. The verdict persists
  on the menu item.
- **Statistiques d'écoute**: logs your *actions* (play, pause, station changes,
  volume) to `events.jsonl`, turned by `fipindicateur stats` into the
  self-contained offline **« Fin d'émission »** report: a late-night radio
  rundown with listening hours, a night clock, session lengths, the Markov
  graph of your zapping, release-year epochs, a 2D artist constellation, label
  economics, taste verdicts, and an Achievements wall. It states its own sample
  size and flags a small session count as indicative. See it, open the data
  folder, or erase it (two-click confirm) from the same submenu; deleting only
  touches `events.jsonl`, never your track history.

## Les couleurs de FIP

Every webradio wears its own official color, lifted straight from
radiofrance.fr. The tray VU glyph is inked in whatever station you're on, and
when you zap it crossfades to the next color over 10 seconds (6 fps, smoothstep
easing, quantized to 16 steps so the icon cache stays tiny). Each hue is nudged
just enough to stay legible on your panel (3:1 contrast, hue kept), and the same
palette is reused verbatim by the local stats report's per-station bars, so one
set of colors runs from tray to report.

![Le glyphe passe de Rock à Jazz en fondu](tint-transition.gif)

The 13 stations, each in its panel-legible tint:

![Les 13 webradios FIP, chacune dans sa couleur officielle](station-colors.png)

## Streams (verified)

All 13 icecast `midfi` (128k MP3) streams verified with `curl -sI` on
2026-07-07; every one returned **HTTP 200, `content-type: audio/mpeg`**:

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
| Hip-Hop | fiphiphop | 200 audio/mpeg | (ICY) |
| Metal | fipmetal | 200 audio/mpeg | 77 |
| Sacré français ! | fipsacrefrancais | 200 audio/mpeg | (ICY) |
| Pop | fippop | 200 audio/mpeg | (ICY) |
| Cultes | fipcultes | 200 audio/mpeg | (ICY) |

URL template: `https://icecast.radiofrance.fr/{slug}-{quality}.{ext}?id=radiofrance`
(quality/ext = `midfi/mp3` default, `hifi/aac` opt-in). `fipcultes` icecast is
present, so no HLS fallback is needed. The four stations without a known
livemeta id use the ICY stream-title fallback for now-playing.

## Credits & attribution

**FIP / Radio France.** This is an **unofficial** client. The streams and all
now-playing metadata are the property of **Radio France**. Please listen to and
support FIP through their official channels, the
[FIP website](https://www.radiofrance.fr/fip) and the official Radio France app.
This project only points a tray menu at their public streams; it adds no content
of its own.

**Artists.** A point of this app is to surface *who you're hearing*. Every track
links out (Wikipédia first, then the Radio France music link or a search)
precisely so artists get discovered and credited, and notifications name the
album, year and label when known.

**Code lineage.**

- The **player** (libmpv cgo wrapper) and **MPRIS2** layers derive from
  [fip-player](https://github.com/DucNg/fip-player) by DucNg (WTFPL).
- The tray uses [fyne-io/systray](https://github.com/fyne-io/systray).
- D-Bus via [godbus/dbus](https://github.com/godbus/dbus).
- Thanks to the community that documented the Radio France `livemeta` endpoint,
  notably [Zopieux's gist](https://gist.github.com/Zopieux/38c9cf4b9df3af521d7be1e0b1e26bda).
