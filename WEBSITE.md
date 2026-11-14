# The fipindicateur website · brief

Status: a brief to adjust with implementers, not a spec. Everything marked
**(decide)** is an open call for the owner; everything marked **(verify)** is
a fact about a third-party service that must be checked before building on it.

Home: `https://me.nech.pl/fipindicateur/`

## Why

Two things, one site.

1. **A landing page.** The project has a README and a release page, no face.
   It deserves one that is quiet and a little cool: it shows what the thing
   is in ten seconds and lets the curious play with it.
2. **A page per track.** Today a click on the playing track opens a Wikipedia
   search for the artist, and the "link" item opens whatever Radio France's
   metadata carries (often an Apple Music page). Both are frequently wrong,
   dead ends, or behind a login. We can do better most of the time: one page,
   ours, that loads by itself, finds the track everywhere it lives, and lets
   the listener confirm or skip each match.

## Part 1 · the landing page (`/fipindicateur/`)

Tone: understated. No hero slogan in capitals, no feature grid with icons.
French first (the app's UI language), English one click away.

What it should carry, top to bottom:

- **The object itself.** A faithful, live HTML rendition of the tray icon
  (four bars) and of the macOS card: cover, title, the two verdict buttons,
  the fader, the dial. Not a screenshot: the real layout, in the page, in the
  visitor's light or dark scheme.
- **The dial as a toy.** The thirteen stations on one scale, draggable and
  throwable exactly like in the app, the page accent following the station
  colour (`internal/stations/stations.go` is the source of the palette).
  Stopping on a station shows what is on air there right now.
- **"En ce moment sur FIP".** One discreet line fed by the same public
  metadata endpoint the app polls **(verify: CORS from a browser, or a tiny
  cached proxy)**. Each title links to its track page: the landing page
  demonstrates part 2 for free.
- **What it does not do.** The privacy stance is a feature: local logs only,
  no account, no telemetry, the listening report is an offline HTML file. Say
  it in three lines and link the report's privacy section.
- **Install.** Linux (GNOME, the supported target), Arch, macOS; the release
  download and the two-line build. Version read from the GitHub releases API
  at build time, not at view time.
- **Colophon.** GPL-3.0, source link, changelog link.

Ideas worth a try, all optional:

- A sample of the listening report (synthetic data, clearly labelled so).
- Play a station from the page itself (an `<audio>` on the public stream)
  with Media Session metadata **(decide: is the site a player, or only a
  shop window? A player is a different product; default: no)**.
- The four bars animated from a real analyser when the visitor plays
  something, static stubs otherwise **(verify: the stream's CORS headers
  allow a Web Audio analyser)**.
- View Transitions between landing and track page so the cover travels.

## Part 2 · the track page (`/fipindicateur/SLUG`)

### What the listener sees

1. The page opens already knowing the track (artist, title, and when the app
   has them: album, year, label, cover, station, time heard).
2. A header in the station's colour: cover, title, artist, "entendu sur FIP
   Jazz à 23 h 41".
3. A column of **source cards**, each resolving on its own as the page loads:
   a skeleton, then a match with a confidence mark, or "rien trouvé".
4. On every card two actions: **C'est ça** (confirm) and **Suivant** (show
   the next candidate from that source). A confirmed card pins to the top and
   becomes a plain, fast link next time.
5. One primary button, chosen by the listener once ("ouvrir dans…"): after
   that the page can skip itself and jump straight to the confirmed match in
   their preferred service.

### Sources, and what each is good for

| Source | Use | Access **(verify all)** |
|---|---|---|
| MusicBrainz + Cover Art Archive | the pivot: recording, release, ISRC, credits, cover | open, browser-callable, 1 request per second, needs a descriptive user agent |
| Wikidata / Wikipedia | artist and album articles in the visitor's language, not a search page | open, CORS with `origin=*` |
| Deezer | match by ISRC or search, 30 s preview | public search without key; JSONP rather than CORS |
| iTunes Search | Apple Music link, preview, artwork | public, JSONP |
| Odesli (song.link) | from one confirmed platform URL, every other platform | free tier rate-limited, key for more |
| Spotify | link, embed | needs an app token: server side, or reached through Odesli |
| YouTube | the video, embedded with the no-cookie player | Data API needs a key and has a daily quota; fallback is a search link |
| SoundCloud, Bandcamp | often the only home of the odd FIP find | no usable open search: a prepared search link, marked as such |
| Genius | link to the lyrics and annotations page only | API needs a token; never copy lyrics into our page |
| Discogs | pressings, label, personnel | token, rate limit |
| Radio France open API | programme context: which show, which host | key required |

The honest rule for the UI: a card says how it got its answer. "Même ISRC"
is not "le premier résultat d'une recherche", and the page must show the
difference (three tiers are enough: certain, probable, à vérifier).

### Matching

- Normalise both sides (case, accents, "feat.", "remastered 2011", live and
  edit suffixes) before comparing; keep the raw strings for display.
- Prefer identifiers over strings: once one source yields an ISRC or a
  MusicBrainz recording id, the others are looked up by it.
- Score candidates on title, artist, duration, year, album; expose the score
  as the tier above, never as a fake percentage.
- FIP plays plenty of tracks no catalogue has. "Rien trouvé" is a normal
  result and should look calm, with the prepared search links as a way out.

### The slug, and the privacy call **(decide, this is the big one)**

The app's promise is local-only data and no network telemetry. A click that
opens a page on our own server tells that server what the user was listening
to. Today the same click tells Wikipedia; the difference is that the server
would now be ours, so the choice must be deliberate and announced.

- **Option A · the fragment.** The app opens
  `/fipindicateur/t#artist=…&title=…&station=…`. A fragment is never sent to
  the server: the page is one static file and all resolution happens in the
  visitor's browser, straight to the sources. Nothing to log, nothing to
  host but files. Cost: no link previews, no pretty URL, sources that need a
  secret key are out or go through Odesli.
- **Option B · real slugs.** `/fipindicateur/dorothy-ashby-mystery-of-love`,
  rendered and cached at the edge. Shareable, previewable, indexable,
  keys stay server side, results are cached for everyone. Cost: the server
  sees every opened track; needs a no-log policy we can actually state, and
  a line in the app's onboarding banner.
- **Suggested: both.** The app opens A by default. The page offers "partager
  ce morceau", which is the moment a B slug is minted, by an explicit act.
  Slugs are `artist-title`, kebab case, with `-year` or a short id only when
  two tracks collide.

Whichever is picked: the click stays a measured action in the app
(behaviour only, as today), and track identity still never enters
`events.jsonl`.

### Remembering confirmations

- First: the visitor's own browser (`localStorage`), nothing leaves.
- Later **(decide)**: an anonymous shared table of confirmed matches, so the
  tenth listener of a track gets the first one's answer. Needs option B's
  server, abuse limits, and no identifiers at all.

## Technology

Fair game: anything that ships as plain web. Suggested defaults, in the
project's spirit (dependency-light, boring where it can be):

- Static first: a small generator (Astro, Eleventy or hand-rolled Vite) that
  outputs files; islands of plain TypeScript for the dial and the resolvers.
  No client framework unless a part proves it needs one.
- CSS does the show: OKLCH and `color-mix` for the station accent, scroll
  driven animations, View Transitions, container queries, system fonts plus
  at most one variable display face, self-hosted.
- Resolvers are small independent modules with one interface
  (`resolve(track) -> candidates[]`), each with its own timeout and cache,
  so a dead source never blocks the page.
- If option B: one edge function per concern (render, resolve, share) and a
  key-value cache. Where `me.nech.pl` is hosted decides the runtime
  **(decide)**.
- Budgets: landing page interactive under one second on a laptop, under
  100 kB of script; the track page shows its header at once and never waits
  on a source.
- Works without script for the essentials: the landing page reads, and a
  track page degrades to the prepared search links.
- Same house rules as the repo: French copy with accents, no em dash, no
  third-party trackers, fonts and scripts self-hosted.

## What changes in the app

- The track click (`openTrack` in `internal/ui/ui.go`, the card's title, the
  banner's action) opens the track page instead of the Wikipedia search; the
  Radio France link stays as one card among others on that page.
- A setting keeps the old direct behaviour for those who want no detour
  **(decide: default on which side)**.
- Offline or site unreachable: fall back to today's behaviour, silently.
- The onboarding banner and the report's privacy section gain one sentence
  describing what opening a track page does and does not send.

## Not in scope

- Accounts, playlists, scrobbling, a hosted listening history.
- Reproducing lyrics, or streaming anything but previews the sources allow.
- A mobile app. The page should simply be good on a phone.

## Open questions for the owner

1. Option A, B, or both for the slug?
2. Is the landing page allowed to play audio?
3. Hosting and runtime for `me.nech.pl`.
4. Which keys are we willing to hold (Spotify, YouTube, Genius, Discogs), and
   which sources do we reach only through Odesli or a search link?
5. One language per page with a switch, or French only at first?

## A possible order of work

1. The fragment track page (option A) with MusicBrainz, Wikipedia, Deezer,
   iTunes and prepared search links; confirm and next; local memory.
2. The app opens it; setting and fallback.
3. The landing page with the live dial and "en ce moment".
4. Odesli fan-out, YouTube and Spotify cards.
5. If chosen: share slugs, edge cache, shared confirmations.
