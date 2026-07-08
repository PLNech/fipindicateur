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

	"fyne.io/systray"
	"github.com/PLNech/fipindicateur/internal/stats"
	"github.com/PLNech/fipindicateur/internal/ui"
)

func main() {
	// Subcommands run and exit without touching the tray. `stats` builds the
	// local listening report; no args launches the tray as usual.
	if len(os.Args) > 1 && os.Args[1] == "stats" {
		os.Exit(stats.RunCLI(os.Args[2:]))
	}

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("fipindicateur: ")

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
