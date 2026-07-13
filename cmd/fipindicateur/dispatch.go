package main

import "path/filepath"

// actionKind is what a given invocation resolves to. The mapping from
// (program name, args) to an actionKind is the whole dispatch decision, kept
// pure in decide so it is table-testable without a tray, a socket or exit().
type actionKind int

const (
	actLaunchTray actionKind = iota // start the system-tray app
	actStats                        // build the local listening report
	actVersion                      // print the stamped version
	actControl                      // talk to the running instance over the socket
	actHelp                         // print usage to stdout, exit 0
	actUsageErr                     // print usage to stderr, exit 2 (unknown command)
)

// decision carries the resolved action and the argument slice to forward to it
// (stats flags, or the control command line). args is empty for the actions
// that take none.
type decision struct {
	kind actionKind
	args []string
}

// controlVerbs are the subcommands served by the running instance over the
// control socket. Shared by decide and the usage screen.
var controlVerbs = map[string]bool{
	"status": true, "play": true, "pause": true,
	"toggle": true, "stations": true, "station": true,
}

// decide resolves an invocation to an action. base is filepath.Base(argv[0])
// (so a `fip` symlink and the `fipindicateur` binary differ), args is
// os.Args[1:]. It performs no I/O.
//
// Differences by name:
//   - `fip` never launches the tray: bare `fip` runs `status`, and an unknown
//     subcommand is a usage error (exit 2).
//   - `fipindicateur` bare launches the tray; an unknown first arg is also a
//     usage error (it no longer silently launches the tray).
func decide(base string, args []string) decision {
	asFip := filepath.Base(base) == "fip"

	// Help is universal and wins over everything.
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "help":
			return decision{kind: actHelp}
		}
	}

	// Bare invocation: fip queries the running instance, fipindicateur opens
	// the tray.
	if len(args) == 0 {
		if asFip {
			return decision{kind: actControl, args: []string{"status"}}
		}
		return decision{kind: actLaunchTray}
	}

	switch {
	case args[0] == "stats":
		return decision{kind: actStats, args: args[1:]}
	case args[0] == "version" || args[0] == "--version" || args[0] == "-v":
		return decision{kind: actVersion}
	case controlVerbs[args[0]]:
		return decision{kind: actControl, args: args}
	default:
		// Unknown subcommand: never fall through to launching the tray, whether
		// invoked as fip or fipindicateur.
		return decision{kind: actUsageErr}
	}
}

// usage is the French help screen, matching the CLI voice used elsewhere
// ("fipindicateur n'est pas lancé"). Returned as a string (newline-terminated)
// so main can send it to stdout (help) or stderr (usage error).
func usage() string {
	return `le fipindicateur, un client FIP (Radio France) non officiel.

Usage:
  fipindicateur [commande]
  fip [commande]

Sans argument, fipindicateur lance l'icône de la barre système (le tray).
Sans argument, fip interroge l'instance en cours (équivaut à "fip status").

Commandes:
  status              État de lecture (JSON: station, lecture, titre, volume)
  play                Lance la lecture
  pause               Met la lecture en pause
  toggle              Bascule lecture / pause
  station <id>        Change de webradio (voir "stations" pour les identifiants)
  stations            Liste les identifiants de webradios
  stats [--out <fichier.html>] [--no-open]
                      Construit le rapport d'écoute local (page HTML hors ligne).
                      --out écrit le rapport dans un fichier; --no-open
                      n'ouvre pas le navigateur.
  version             Affiche la version
  help, -h, --help    Affiche cette aide

status, play, pause, toggle, station et stations dialoguent avec l'instance en
cours via une socket locale. Sans instance lancée, elles affichent
"fipindicateur n'est pas lancé" et sortent avec le code 1.
`
}
