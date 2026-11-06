// Command specprobe plays a webradio stream for a few seconds and prints the
// audio-level analysis the tray animator consumes. A dev probe, not shipped:
// it needs a working speaker or muted output to exercise the same filter chain
// as the app.
package main

import (
	"fmt"
	"time"

	"github.com/PLNech/fipindicateur/internal/player"
	"github.com/PLNech/fipindicateur/internal/vu"
)

func main() {
	m := &player.MPV{}
	if err := m.Initialize(); err != nil {
		fmt.Println("init:", err)
		return
	}
	defer m.Close()
	m.Play("https://icecast.radiofrance.fr/fipjazz-midfi.mp3?id=radiofrance")
	time.Sleep(3 * time.Second)
	if bands, ok := m.SpectrumBands(); ok {
		fmt.Printf("spectrum %v\n", vu.Bands(bands))
	} else {
		fmt.Println("spectrum indisponible:", m.AudioLevelDiag())
	}
	if db, ok := m.RMSLevelDB(); ok {
		fmt.Printf("rms %.1f dB\n", db)
	}
}
