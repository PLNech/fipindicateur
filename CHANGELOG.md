# Changelog

## Non publié · Le panneau sans attente

### Added
- Simple clic gauche sur l'icône (GNOME) : l'item SNI exporte désormais un
  DBusMenu minimal (« Ouvrir le panneau ») ; l'extension AppIndicator n'envoie
  Activate qu'au double clic, mais l'ouverture de ce menu (ou le clic sur son
  entrée) devient le signal d'ouverture du panneau (#18). Le double clic et le
  clic simple KDE (Activate) continuent de basculer le panneau
- Le panneau s'ouvre sous le pointeur : à l'ouverture, la fenêtre se centre
  sur la position x du pointeur (donc sous l'icône qu'on vient de cliquer),
  bornée dans la zone de travail de l'écran du pointeur ; repli sur le coin
  haut-droit historique si aucun pointeur n'est interrogeable
- Volume en direct pendant le glissement : le curseur du panneau applique la
  valeur en continu (premier mouvement immédiat, puis au plus une commande
  toutes les 120 ms, valeur finale garantie au relâchement) au lieu d'attendre
  le relâchement ; côté cast, une valeur déjà affichée par l'appareil n'est
  pas renvoyée
### Changed
- `drawer --selftest` simule le geste complet sur les curseurs (pointerdown,
  valeur, input/change, pointerup) : sans pointerdown, le curseur de volume
  gardait en mémoire la valeur déjà envoyée d'un scénario à l'autre et cinq
  scénarios sur six lisaient un curseur muet. Le vrai glissement, lui, n'a
  jamais été cassé (350 interactions, 0 échec)
- Le filtre astats ne mesure plus que le niveau RMS global, le seul chiffre que
  lit le glyphe VU : la configuration par défaut calculait ~79 statistiques par
  fenêtre (analysées 6 fois par seconde) et les déversait toutes dans le
  journal à la destruction d'un handle, ce que le fondu fait maintenant à
  chaque zap (75 lignes de bruit pour trois zaps)
- Le popup GNOME du simple clic n'a plus rien à dire : l'entrée unique du
  DBusMenu devient un séparateur (l'extension AppIndicator exige un menu non
  vide pour réagir au simple clic, et rien dans le protocole ne permet de
  refermer un menu que le shell a ouvert : c'est lui qui le possède). Le clic
  reste détecté via l'événement « opened » de la racine, mais le popup affiche
  un simple filet au lieu d'un « Ouvrir le panneau » qui doublait le panneau en
  train de s'ouvrir
- Les couleurs suivent le fondu : la teinte du glyphe dans la barre et l'accent
  du panneau transitionnent désormais sur la durée du fondu audio configurée
  (au lieu de 10 s fixes pour l'icône et 0,8 s pour le panneau), plancher à
  0,3 s quand le fondu vaut 0. Un seul curseur règle ce qu'on entend et ce
  qu'on voit ; `drawer --selftest` vérifie que la page reçoit la durée
- Les événements `volume` du journal d'écoute sont regroupés par geste
  (premier niveau immédiat, niveau final en fin de fenêtre de 500 ms) : un
  glissement ne journalise plus des dizaines de lignes ; toujours enregistrés
  à la source, vidés avant `app_stop` à la fermeture
- Préchauffage du panneau : la fenêtre webkit se construit en arrière-plan
  quelques secondes après le démarrage (cachée, sans voler le focus), donc le
  premier clic ne paie plus tout le démarrage de WebKit (#14, partie
  ouverture)
- Instrumentation `FIP_TIMINGS=1` : logs `timing:` sur le chemin froid du
  panneau (gtk_init, fip_build, Show → present, clic → page interactive),
  l'ouverture chaude et la fin d'OnReady
### Fixed
- Le fondu enchaîné remarche : `gtk_init` (donc le préchauffage du panneau)
  appelle `setlocale(LC_ALL, "")` pour tout le processus, et libmpv REFUSE de
  créer un handle quand `LC_NUMERIC` n'est pas « C » (mpv écrit « Non-C locale
  detected »). Depuis que le panneau se préchauffe au démarrage, chaque flux
  entrant échouait donc à s'initialiser et le zap retombait en coupure sèche.
  La catégorie `LC_NUMERIC` est ramenée à « C » juste après `gtk_init` puis
  après la construction de la vue WebKit (le reste de la locale utilisateur est
  préservé), `drawer --selftest` le vérifie, et l'échec d'initialisation
  journalise désormais la valeur fautive
- Diagnostic de l'icône animée : « astats levels unavailable » nomme enfin
  l'étape qui a échoué (propriété absente, JSON illisible, clé manquante,
  valeur non numérique) au lieu de laisser chercher. Quand l'animation
  s'éteint, la teinte de station cesse de fondre : le glyphe reprenait la
  couleur d'un coup, ce qui se lisait comme un bug du fondu
- La fenêtre du panneau s'appelle « le FIPindicateur » (ce que montrent alt-tab
  et la liste des fenêtres) ; les maquettes gardent leurs titres marqués
- Le fondu enchaîné s'entend enfin : le `volume` interne de mpv applique un
  gain cubique (`(v/100)³`, vérifié dans mpv 0.37), donc la courbe équal-power
  envoyée telle quelle laissait la station entrante sous -30 dB pendant le
  premier cinquième du fondu (un trou de silence perçu comme une coupure
  sèche). Les rampes passent par la racine cubique inverse : le croisement se
  fait désormais à -3 dB de chaque côté, sans creux
- Zapping rapide pendant un fondu : le nouveau zap reprend la station en cours
  à son volume partiel et la fond vers le silence, au lieu de la claquer à
  plein volume (la coupure sèche entendue sur chaque enchaînement A→B→C)
- Fils d'Ariane `player: crossfade:` dans le journal : décision du zap (fondu
  ou chargement direct, avec la raison), délai avant l'audio entrant, cause de
  fin de rampe (complète, annulée, coupure après délai) : un fondu raté sur le
  terrain se diagnostique désormais à la lecture du log
- Le panneau fond ses couleurs au zap : l'accent de station (règle d'en-tête,
  bouton lecture, pastille sélectionnée) transitionne en 0.8 s via des
  propriétés CSS typées (`@property`), en écho au fondu audio ;
  `prefers-reduced-motion` coupe la transition
- Clic impatient pendant la première ouverture : le second Activate est
  désormais ignoré tant que le Show initial s'initialise, au lieu d'un Hide
  qui pouvait doubler le present sur le thread GTK et désynchroniser l'état

## Sprint 5 · 2026-07-26 · L'antenne dans les enceintes, le panneau dans la barre

Three releases in one day: v0.4.0 (Chromecast), v0.4.1 (hardware fixes), v0.5.0
(le panneau + SNI). Details live in the release sections below; this entry
records the sprint shape and what the releases cannot.

### Added
- Chromecast casting, stdlib-only mDNS + CASTV2 (#1, #3 · v0.4.0)
- « Le panneau » webkit2gtk drawer, cast-aware volume (#5 · v0.5.0)
- Hand-rolled SNI tray, panel as single Linux UI, test belt (#6 · v0.5.0)
### Fixed
- Discovery deaf to real devices (QU bit + multicast), slow-AVR launch
  timeout, `fip cast scan` debug CLI (#4 · v0.4.1)
### Infra
- CI: xvfb selftest + drawer screenshots as artifacts, Go caching (#7)
- `feat/avr-zones` branch: offline eISCP zone client groundwork (#10, pass 2
  pending a user-approved live moment)
### Doctrine
- Hardware is the only test bench for hardware: every "green" cast milestone
  needed one more fix after touching the real amp.
- Name your mocks: an unlabeled dev-harness window stacked pixel-identical on
  the real panel simulated a total UI outage for an hour.
- Ship the hotfix before building the belt; never batch P0 fixes behind P1
  infrastructure.

## v0.5.0 · 2026-07-26 · Le panneau

### Added
- **« Le panneau »**, a branded quick-control drawer (Linux/GNOME): a
  transient undecorated popup at the top-left of the screen, rendered by
  webkit2gtk from our own embedded HTML/CSS/JS (self-contained, system fonts,
  light/dark following the desktop). Now playing with the station's brand
  color and artwork, play/pause, a real 0-100 volume slider with mute, a
  « Sortie » picker (this computer + Chromecast devices, rescan), and the 13
  stations as colored chips. Opened from the tray icon; Escape or a click
  elsewhere hides it, and it stays resident for an instant reopen. The old
  volume preset submenu is gone on Linux (macOS/Windows keep it); every panel
  action flows through the same measurable chokepoints as the menu.
- **Cast-aware volume and transport.** While casting, the panel's slider and
  mute drive the DEVICE's own volume, quantized to its stepInterval and
  displayed from RECEIVER_STATUS, never invented: on an AV receiver
  (controlType "master") that is the amp's master level, so we read first and
  only ever send user-chosen values. Play/pause controls the active sink:
  PAUSE/PLAY on the device's media session while casting (new events
  cast_pause/cast_resume), the local player otherwise. The menu and media-key
  semantics are unchanged (play while casting still brings the music home).
- `fipindicateur drawer [--dark|--light]`: open the panel standalone with
  mock state, for design iteration; the flags force a theme regardless of the
  desktop preference (useful headless, e.g. CI captures).

### Changed
- **The panel IS the interface on Linux.** The tray icon is now a hand-rolled
  StatusNotifierItem over godbus, with no DBusMenu and no popup menu at all:
  left-click toggles the panel, middle-click toggles play/pause, scrolling
  adjusts the volume, right-click opens the réglages view. Under GNOME the
  appindicator extension delivers the left-click activation on DOUBLE-click
  only (its constraint, documented, not ours to fix). fyne/systray now builds
  for macOS/Windows only; those targets keep the classic menu UI.
- Full menu parity moved into the panel: audio outputs merged into a single
  « Sortie » view (« Sur cet appareil » / « Sur le réseau »), crossfade and
  the « À venir » programme joined the réglages and main views, and the main
  view compacted by 37%. The panel gains true compositor transparency, sticks
  across workspaces, fixes the historique CSS overflow, and hooks the WebKit
  console so page errors land in the app log.

### Infra
- The panel test belt: `TestDrawerActionsWired` keeps every drawer action
  wired through the telemetry chokepoint, and `fipindicateur drawer
  --selftest` (aka `make selftest`) drives 350 scripted interactions through
  the real page in the real webkit, headless via xvfb.
- CI: Go module caching on every job; a `drawer-visual` job runs the wiring
  self-test under xvfb, then captures the panel's three views in light and
  dark as a `drawer-screenshots` artifact (14-day retention).

### Packaging
- The .deb now depends on libwebkit2gtk-4.1-0; the AUR packages on
  webkit2gtk-4.1. macOS/Windows builds are untouched (a no-op stub keeps them
  webkit-free).

## Correctif · 2026-07-26 · L'oreille qui traîne

### Fixed
- Discovery now hears real devices. The v0.4.0 mDNS query was a legacy
  unicast question from an ephemeral port; real hardware (a Pioneer VSX-933
  on the bench) never answered it. The query now sets the QU bit (RFC 6762
  §5.4) so QU-honouring responders reply straight to our socket, and a second
  socket joins the 224.0.0.251:5353 multicast group to hear everyone else.
  Confirmed on the receiver itself.
- Casting waits out slow AV receivers: the LAUNCH answer is now bounded by a
  30s launchTimeout instead of the 10s handshake bound. Receivers cold-boot
  their cast module on LAUNCH (8.2s measured on a warm Pioneer; cold exceeds
  10s), so v0.4.0 gave up just before the music started.
- The « Diffusion impossible » notification now hints that the device may
  need a few seconds: réessayez.

### Added
- `fipindicateur cast scan` (or plain `cast`): a standalone mDNS scan that
  lists the Chromecast devices on the local network, one per line, no running
  instance needed. The debug companion to « Rechercher les appareils »;
  documented in the man page.

## Diffusion · 2026-07-26 · L'antenne dans les enceintes

### Added
- « Diffuser sur… » : cast the current station to a Chromecast speaker. A new
  stdlib-only `internal/cast` package hand-rolls the whole exchange (mDNS
  discovery of `_googlecast._tcp` devices, the CASTV2 length-prefixed protobuf
  channel over TLS, launch of the Default Media Receiver, LOAD of the icecast
  URL) so casting costs zero new dependencies. Devices are discovered fresh at
  startup and on demand (« Rechercher les appareils »), nothing is persisted.
  While casting, local playback pauses so the audio never doubles; zapping
  stations or toggling Haute qualité re-LOADs the stream on the device; « Cet
  ordinateur » (or the play button, or `fip play`) brings the music back. A
  lost device surfaces as a notification and the menu resets, never a crash.
- Two behaviour events, `cast_start` and `cast_stop`, recorded at source.
  Privacy unchanged: events record the behaviour only, never the device name
  or address.

## Distribution · 2026-07-12 · Les rayonnages

### Added
- Source-build packaging recipes, one per channel, all sidestepping the
  cgo/libmpv crux by compiling on the user's own machine: AUR `PKGBUILD`s
  (`fipindicateur` versioned plus a `fipindicateur-git` rolling variant, each
  with a generated `.SRCINFO`), a Homebrew tap formula, and a repo-root
  `flake.nix` (buildGoModule + an `apps` output + a `go`/libmpv/pkg-config
  devShell).
- `.deb` packaging via nfpm (`packaging/nfpm.yaml`), wired into the release
  workflow downstream of the existing Linux build: nfpm only assembles the
  already-built binary (`Depends: libmpv2`), never cross-compiles. The `.deb`
  is attached to the GitHub Release alongside the tarball.

### Docs
- `docs/INSTALL.md`: a `go install` note (needs a local C toolchain, not
  Windows) and a Package channels section with the exact command and honest
  status per channel (available now: `.deb`, Nix; prepared: AUR pending
  registration, Homebrew tap pending creation).

Groundwork from the distribution research in #6. The AUR recipes and the
Homebrew tap are authored and validated but deliberately unpublished (AUR
registration disabled; no tap repo created).

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
