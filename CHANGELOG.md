# Changelog

## Unreleased

### Added
- macOS, expérimental (`make swiftui`, macOS 14+) : un clic gauche sur
  l'icône ouvre une carte SwiftUI en haut à droite de l'écran, sous la barre
  de menus : pochette, lecture, J'aime / Pas pour moi qui restent allumés sur
  le morceau jugé, volume avec le vumètre à quatre barres de l'icône (les
  mêmes images, calculées une fois), et un cadran des stations qu'on fait
  glisser ou qu'on lance. Elle se referme au clic ailleurs ou avec Échap, et
  tant qu'elle est ouverte les bannières qui la répéteraient sont omises. Le
  menu passe au clic droit et se réduit à une liste courte : les réglages
  sont au dos de la carte, qui se retourne sur place (Réglages… ou la roue
  dentée) : deux pages, Écoute et Appli. La carte est liée dans le même
  binaire, sans second processus, et chacun de ses contrôles passe par le
  registre d'actions qui alimente déjà le menu : rien n'échappe à la mesure.
  La construction par défaut ne change pas et n'a pas besoin de Swift

### Fixed
- Un flux qui meurt (relais icecast, réseau) ne laisse plus l'app dans un état
  zombie : l'icône animée se figeait sur ses tigettes, le menu gardait
  « ⏸ Pause » et le premier clic sur Play « mettait en pause » un lecteur déjà
  mort. La fin de flux (eof ou erreur, pas nos propres arrêts) resynchronise
  maintenant l'état lecture ; pas de reconnexion automatique, le silence
  s'annonce et vous décidez
- Fondu enchaîné : si la station entrante ne démarre pas dans le délai
  d'attente, la lecture s'arrête proprement et l'état se resynchronise, au
  lieu de couper vers le silence en croyant jouer
- Reprendre la lecture (bouton Play, touche média, « Cet ordinateur ») habille
  à nouveau l'icône de la couleur de la station : la teinte ne suivait que les
  zaps, une reprise dansait en gris neutre jusqu'au zap suivant
- Une rafale de métadonnées de l'ancienne station ne traverse plus un zap :
  l'écart est identifié et jeté au lieu de revendiquer un instant le label,
  l'historique ou une notification
- Le sous-menu Historique ne se remplit plus pendant une pause : il suivait
  ce qui passait à l'antenne pendant que history.jsonl, lui, respectait
  l'écoute réelle. Même contrat des deux côtés
- Réactiver « Icône animée » quand elle s'est auto-désactivée cette session
  prévient au lieu de cocher un réglage qui ne peut pas fonctionner
- Sortie du fondu bornée : un mpv bloqué ne peut plus suspendre le clic du
  tray plus de deux secondes

### Changed
- L'historique d'écoute passe en activé par défaut, avec un seul réglage : la
  case « Historique d'écoute (local) » des Réglages pilote d'un coup les deux
  journaux locaux, `history.jsonl` (les titres) et `events.jsonl` (les
  actions). Les deux vivent sur la machine, rien ne quitte jamais le réseau ;
  le rapport est là quand on le cherche, au lieu d'attendre un opt-in que
  personne ne trouvait. L'ancienne paire (history_file, stats) part avec le
  prochain enregistrement de la configuration. Un message unique annonce le
  changement à la première relance : son clic ouvre le rapport directement sur
  la rubrique confidentialité. « Effacer l'historique… » couvre les deux
  fichiers d'une seule confirmation ; « Effacer mes goûts… » garde son entrée
  et son consentement propres. Qui préfère le silence décoche la case, et les
  journaux s'arrêtent net
- Les couleurs de station sur le tray ne se délavent plus. L'encre garde
  désormais la saturation maximale de sa teinte (le teal Jazz passe de
  S=0,76 terne à un teal franc, le hip-hop d'un gris-mauve S=0,15 à un
  violet assumé) et seule la luminosité bouge, juste assez pour tenir le
  contraste 3:1 sur le panel. L'ancien mélange vers le blanc atteignait le
  même ratio en transformant les marques en pastels
- Le liseré fantôme autour du glyphe devient noir sous une encre colorée (il
  creuse le fond au lieu de l'éclaircir, et la couleur paraît plus dense) ;
  le liseré blanc accordé aux encres neutres ne bouge pas
- La configuration et la station courante se lisent et s'écrivent sous
  verrou : l'écho de volume de mpv, les polls de métadonnées, l'animateur et
  les clics du tray se marchaient dessus sans le savoir (les tests passent
  sous `-race`)

## v0.8.0 · 2026-11-06 · La roue des genres et le tray natif macOS

### Removed
- Le panneau webkit2gtk disparaît pour de bon : le paquet `internal/drawer`
  (page, shim cgo, pilote de maquette), les sous-commandes `drawer` et
  `drawer --selftest`, `make selftest`, le job CI `drawer-visual`, le code de
  relance d'un panneau mort et les réglages `panel_scale_pct` et `theme`. Les
  anciens `config.json` qui les contiennent se chargent toujours sans erreur,
  et les anciens `events.jsonl` (genres `drawer_open`, `panel_recover`,
  `panel_scale`, `theme`) restent lisibles par le rapport de statistiques
- Le binaire ne dépend plus de webkit2gtk ni de GTK : le paquet `.deb`
  (`libwebkit2gtk-4.1-0`) et les paquets AUR (`webkit2gtk-4.1`) ne les
  demandent plus, seul libmpv reste

### Changed
- Les binaires livrés sont construits avec `-trimpath` et `-ldflags "-s -w"`
  (environ 30 % plus petits)

### Added
- macOS : les notifications de titre, d'émission, de diffusion et de goût
  passent par les bannières du système. La première affiche la demande
  d'autorisation ; un refus laisse la lecture intacte
- macOS : « Lancer au démarrage » inscrit `FipIndicateur.app` dans les
  éléments d'ouverture (le bundle, pas le binaire seul, pour que l'icône
  reste dans la barre)
- macOS : `make app` pose `FipIndicateur.app` dans `/Applications`. Spotlight,
  Launchpad et `open -a FipIndicateur` le trouvent. L'app est un agent (pas
  d'icône dans le Dock). L'exécutable du bundle est le binaire lui-même
  (la même construction que `make install`)
- macOS : les touches média, le Centre de contrôle, l'écran verrouillé et le
  clic du casque pilotent la lecture. L'app s'inscrit auprès de Now Playing
  (MPNowPlayingInfoCenter), le chemin que macOS utilise à la place de MPRIS.
  Play, pause et le basculement passent par les mêmes méthodes que le menu,
  donc les statistiques les voient pareil. Le titre en cours s'affiche dans
  le Centre de contrôle
- macOS : précédent et suivant (touches, casque, écran verrouillé) zappent la
  webradio suivante ou précédente de la liste : un flux live n'a pas de file
  de morceaux, la paire sert la grille des radios
- La roue des genres : le cycle précédent/suivant et le sous-menu Radios
  suivent un voisinage éditorial (FIP, Rock, Metal, Electro, Hip-Hop, Reggae,
  Monde, Jazz, Groove, Pop, Nouveautés, Sacré français !, Cultes, retour à
  FIP) : six duos affines, FIP à la couture, chaque accord se raconte. À
  terme, le voisinage pourra être recalé sur les transitions réellement
  observées dans les statistiques d'écoute
- macOS : la bannière du morceau devient interactive. Deux boutons, J'aime et
  Pas pour moi, notent le titre en cours par le même chemin que le menu (le
  clic est le consentement) ; cliquer la bannière ouvre le lien d'écoute du
  morceau. Les bannières système (diffusion, statistiques) restent sans
  boutons
- macOS : la bannière du morceau se lit en trois lignes (titre, artiste,
  album (année) · label · station) et porte la pochette ré-encodée ; la
  bannière de verdict (C'est noté) porte la pochette aussi
- Changer de radio (menu, touches précédent/suivant, `fip station`) annonce
  la nouvelle station par une bannière « En écoute » : depuis les touches,
  il n'y a pas de coche de menu à voir, et le premier titre de la station
  n'arrive qu'à la prochaine lecture de métadonnées. La bannière du titre la
  remplace dès qu'il est connu

### Changed
- L'icône animée suit le spectre : où l'énergie se place, sa largeur, et
  où elle retombe. Les quatre barres (grave, bas-médium, présence, aigu)
  en découlent, et le niveau global fixe leur hauteur. La mesure ne change
  pas ce qui sort des enceintes. Sans cette lecture, l'ancienne variation
  reste en repli
- macOS : la bannière du morceau porte la vraie pochette. Les images de la
  grille Radio France sont des JPEG progressifs, que la pièce jointe de
  notification n'affiche pas de façon fiable ; chaque pochette est
  ré-encodée en PNG de base (en cache, une fois par image) avant l'envoi,
  ce qui vaut aussi pour l'icône de notification Linux
- macOS : la teinte suit l'apparence en direct. Le mode Auto basculait
  l'interface vers le sombre sans prévenir l'app, qui dessinait alors de
  l'encre sombre sur la barre sombre (icône invisible) ; la détection lit
  maintenant l'apparence effective du système et suit chaque bascule. Le
  liseré blanc revient quand la barre est sombre, pour que l'icône reste
  trouvable au clic
- macOS : le plateau est désormais notre propre composant AppKit
  (internal/tray), sur le modèle du plateau Linux artisanal : taille
  d'icône fidèle au PNG, position persistée par AppKit, OnExit réellement
  exécuté à la fermeture. Curseur de volume natif dans le sous-menu Volume,
  et molette sur l'icône pour monter ou baisser le volume de la sortie
  active. Le sous-menu à paliers reste pour les clics rapides

### Fixed
- macOS : les touches média ne répondaient pas. Deux causes : le fil principal
  n'était pas verrouillé (Go déplaçait la goroutine principale, AppKit tournait
  ailleurs, et les blocs déposés sur la file principale n'étaient jamais
  servis), et l'inscription auprès de Now Playing se faisait hors du fil
  principal d'AppKit, ce que macOS ignore en silence. Le fil est verrouillé et
  l'inscription passe par la file principale ; le titre publié suit le même
  chemin
- Le réglage « Notifications » envoie une bannière de test à l'activation :
  le chemin complet (permission, affichage) s'exerce tout de suite, sans
  attendre le prochain morceau. Si le système refuse la permission, l'app
  ouvre une fois le panneau Réglages système · Notifications, le seul endroit
  où l'interrupteur existe
- macOS : le processus sans bundle (make run, le binaire fip seul) ne s'arrête
  plus à la première notification : le centre de notifications n'existe pas
  hors d'un bundle, on reste silencieux et la lecture continue
- macOS : les tests qui touchent le menu cast ne plantent plus quand le menu
  n'est pas construit (refreshCastMenu sort tôt au lieu d'appeler un item
  systray nul)
- macOS : sur un MacBook à encoche, l'icône se retrouvait dessous, invisible
  alors que le son jouait. Au lancement elle prend la place à droite des
  extras, hors de l'encoche. Un déplacement au clic-Commande est conservé
- macOS : l'icône de la barre des menus restait hors écran (origine négative)
  quand `FipIndicateur.app` lançait le binaire via un script shell. macOS
  classe alors le processus comme tâche de fond, et place l'item hors de la
  barre. L'exécutable du bundle est maintenant le binaire Mach-O
- macOS : « Sortie audio » ne liste plus chaque appareil deux fois. mpv
  annonce les mêmes sorties en coreaudio et en avfoundation ; seul
  coreaudio reste (l'écran HDMI, les haut-parleurs, Zoom)
- macOS : le volume ne revient plus à 100 % à chaque lecture. CoreAudio oublie
  le niveau quand la sortie se ferme (pause, relance), et la relecture du
  flux adoptait ce 100 par défaut, puis l'enregistrait. Au redémarrage de la
  lecture le niveau mémorisé est réappliqué. Linux inchangé : PulseAudio se
  souvient du niveau tout seul, et l'écrire aurait écrasé un réglage fait dans
  le mixeur pendant un appel

## v0.7.0 · 2026-10-11 · Le menu fait tout

### Changed
- Le menu de l'icône est désormais toute l'interface Linux : un dbusmenu
  dynamique maison (sections, coches, radios, entrées masquables) remplace le
  panneau webkit comme surface principale. Le clic du milieu (lecture/pause) et
  la molette (volume) sont conservés. Le titre en cours s'affiche avec sa
  pochette en tuile, et l'ensemble prend la teinte magenta FIP
- Le panneau webkit n'est plus branché : le code est toujours dans l'arbre et
  sa suppression viendra dans une version ultérieure
- Le sous-menu « Sortie audio » ne liste plus la cinquantaine d'entrées brutes
  de mpv, seulement les sorties utiles
- Un appel redondant au menu ne republie plus le menu entier sur D-Bus

### Added
- `FIP_MPRIS_NAME` choisit le nom D-Bus MPRIS de l'instance, pour lancer
  plusieurs instances en parallèle (démos, essais) sans collision

### Fixed
- Les tests n'écrasent plus la vraie configuration : `config.Save` refuse
  d'écrire hors du dossier temporaire sous `go test` (panique) et le test du
  volume passait par la vraie config
- `make lint` : la garde des tirets cadratins ne lit plus que les fichiers
  suivis, donc un fichier non suivi (docs/, .claude/) ne peut plus la faire
  échouer

## v0.6.0 · 2026-09-26 · Le panneau sans attente

### Added
- Amplification du flux : les radios FIP mesurent -18,5 LUFS, soit ~4,5 dB sous
  la loudness des plateformes (mesuré sur fip-hifi.aac avec ffmpeg ebur128).
  Trois niveaux au menu Réglages et dans le panneau : Désactivée (défaut),
  Doux (+4 dB, parité streaming) et Costaud (+8 dB sous limiteur alimiter,
  crêtes plafonnées à -0,5 dBFS vérifiées). Le gain précède le tap astats dans
  la chaîne af, donc l'icône VU danse sur ce qu'on entend ; appliqué à chaud,
  persisté dans la config, chaque changement enregistre un événement amp_toggle
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
- Le popup GNOME du simple clic porte le nom de l'app (« le FIPindicateur »)
  au lieu de « Ouvrir le panneau » : il se lit comme un cartouche de titre pour
  le panneau qui apparaît dessous, plutôt qu'un ordre qui répète ce qui est déjà
  en train de se produire. Le popup lui-même est irréductible : l'extension
  AppIndicator n'ouvre le menu au clic simple que s'il n'est pas vide, et c'est
  cette ouverture qui nous sert de signal ; rien dans le protocole ne referme un
  menu que le shell a ouvert. Le rendre muet (un séparateur) a été essayé et
  annulé : GNOME masque un séparateur en bord de menu et n'ouvre jamais un menu
  sans rien à montrer, donc le clic simple mourait
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
