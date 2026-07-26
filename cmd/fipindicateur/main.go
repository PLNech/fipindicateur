// Command fipindicateur is a tiny system-tray app to listen to FIP webradios.
//
// le fipindicateur, an unofficial FIP (Radio France) client.
// Copyright (C) 2026  fipindicateur contributors
// Licensed under the GNU General Public License v3.0 (see LICENSE).
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"fmt"

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/stats"
	"github.com/PLNech/fipindicateur/internal/ui"
	"github.com/PLNech/fipindicateur/internal/version"
)

func main() {
	// Resolve the invocation (program name + args) to an action. The decision
	// is pure and lives in decide(); only the side effects (exit, I/O, tray)
	// happen here. Reached both as `fipindicateur` and, via the installed
	// symlink, as `fip` (which never launches the tray).
	switch d := decide(os.Args[0], os.Args[1:]); d.kind {
	case actHelp:
		fmt.Print(usage())
		os.Exit(0)
	case actUsageErr:
		fmt.Fprint(os.Stderr, usage())
		os.Exit(2)
	case actStats:
		os.Exit(stats.RunCLI(d.args))
	case actVersion:
		fmt.Println("fipindicateur " + version.String())
		os.Exit(0)
	case actControl:
		// Control-socket client: talk to the running instance and exit.
		os.Exit(ui.RunControlClient(d.args))
	case actCastScan:
		os.Exit(runCastScan())
	case actDrawer:
		os.Exit(runDrawer())
	case actLaunchTray:
		// Fall through to the tray setup below.
	}

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("fipindicateur: ")

	// Single-instance guard, before any tray/D-Bus/mpv setup: a second launch
	// (e.g. a double click in the GNOME app grid) must exit cleanly instead of
	// registering a second StatusNotifierItem and fighting the first over the
	// tray. Exit 0: this is expected, not an error.
	if err := ui.AcquireInstanceLock(); err != nil {
		log.Printf("le fipindicateur tourne déjà, sortie (%v)", err)
		os.Exit(0)
	}

	app := ui.New()

	// Translate termination signals into a clean systray shutdown, which in
	// turn runs onExit (mpv teardown, D-Bus close).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		log.Printf("received %s, shutting down", s)
		systray.Quit()
	}()

	systray.Run(app.OnReady, app.OnExit)
}
