package main

import (
	"fmt"
	"time"

	"github.com/PLNech/fipindicateur/internal/cast"
)

// castScanWindow mirrors the tray's discovery window (internal/ui): one mDNS
// scan listens this long for answers.
const castScanWindow = 3 * time.Second

// runCastScan is the `cast scan` debug subcommand: a standalone Chromecast
// discovery (no running instance needed), one line per device found. Always
// exits 0: an empty network is an answer, not an error.
func runCastScan() int {
	fmt.Printf("Recherche d'appareils Chromecast (%.0f s)...\n", castScanWindow.Seconds())
	devs := cast.Discover(castScanWindow)
	if len(devs) == 0 {
		fmt.Println("Aucun appareil trouvé.")
		return 0
	}
	for _, d := range devs {
		fmt.Printf("%s · %s:%d\n", d.Name, d.Addr, d.Port)
	}
	return 0
}
