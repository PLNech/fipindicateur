// Command probe is a development harness for internal/avr: it exercises
// the eISCP protocol against a real amp on the LAN. It is a separate
// main package that nothing in the app imports, so it never ships in
// the fipindicateur binary; run it by hand with
//
//	go run ./internal/avr/probe -addr <amp-ip>
//
// Safety rules (non-negotiable, from the operator's research notes):
// nothing is sent without -addr being given explicitly; the MAIN zone
// power and volume are never written, only read; every zone 2 change is
// deliberate, logged, and restored to the initial state before exit.
// Run it ONLY at a moment the operator has explicitly approved: the
// amp is a shared household device (TV audio rides through it).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PLNech/fipindicateur/internal/avr"
)

func main() {
	addr := flag.String("addr", "", "amp IP or IP:port (required; eISCP port defaults to 60128)")
	hold := flag.Duration("hold", 20*time.Second, "how long to leave zone 2 playing for the audible check")
	flag.Parse()
	if *addr == "" {
		fmt.Fprintln(os.Stderr, "usage: probe -addr <amp-ip> [-hold 20s]")
		os.Exit(2)
	}

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	c, err := avr.Dial(*addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	c.Trace = func(dir string, raw []byte) {
		log.Printf("  %s % X", dir, raw)
	}

	// Step 1: read-only snapshot of both zones.
	log.Println("== step 1: Status() (read-only)")
	st, err := c.Status()
	if err != nil {
		log.Fatalf("status: %v", err)
	}
	log.Printf("main : power=%v volume=%d (0x%02X) source=%s", st.Main.Power, st.Main.Volume, st.Main.Volume, st.Main.Source)
	log.Printf("zone2: power=%v volume=%d source=%q", st.Zone2.Power, st.Zone2.Volume, st.Zone2.Source)

	// Step 2: the live SLI value while casting is the authoritative
	// cast-source selector, worth more than any catalog value.
	log.Println("== step 2: live cast-source values")
	sli, err := c.Query(avr.CmdMainSource)
	if err != nil {
		log.Fatalf("SLIQSTN: %v", err)
	}
	slz, _ := c.Query(avr.CmdZone2Source)
	log.Printf("LIVE SLI (main input while casting) = %q  <- authoritative cast source", sli)
	log.Printf("LIVE SLZ (zone2 input)              = %q", slz)
	log.Printf("catalog says NET = %q; live match: %v", avr.SourceNet, sli == avr.SourceNet)

	if !st.Main.Power {
		log.Println("main zone is not powered on; skipping the audible zone 2 test")
		return
	}

	// Remember the exact zone 2 state to restore, whatever it is.
	initPower := st.Zone2.Power
	initSource := st.Zone2.Source
	initVolume := st.Zone2.Volume
	restore := func() {
		log.Println("== restore: putting zone 2 back as found")
		if initPower {
			if initSource != "" {
				logErr("restore SLZ", c.SetZone2Source(initSource))
			}
			if initVolume >= 0 {
				logErr("restore ZVL", c.SetZone2Volume(initVolume))
			}
		} else {
			logErr("restore ZPW off", c.SetZone2Power(false))
		}
		time.Sleep(1500 * time.Millisecond)
		pw, err := c.Query(avr.CmdZone2Power)
		log.Printf("restored zone2 power = %q (err=%v); initial was on=%v", pw, err, initPower)
	}
	defer restore()

	// Step 3: deliberate zone 2 test. Power on, confirm, mirror the
	// live main source, confirm, keep the volume gentle.
	log.Println("== step 3: zone 2 on + mirror the cast source (DELIBERATE)")
	must("ZPW on", c.SetZone2Power(true))
	time.Sleep(1500 * time.Millisecond)
	pw, err := c.Query(avr.CmdZone2Power)
	must("ZPWQSTN", err)
	log.Printf("zone2 power now = %q", pw)
	if pw != avr.PowerOn {
		log.Printf("amp refused zone 2 power on (reply %q); stopping and restoring", pw)
		return
	}

	must("SLZ set to live SLI value", c.SetZone2Source(sli))
	time.Sleep(1500 * time.Millisecond)
	src, err := c.Query(avr.CmdZone2Source)
	must("SLZQSTN", err)
	log.Printf("zone2 source now = %q (wanted %q)", src, sli)

	vol, err := c.Query(avr.CmdZone2Volume)
	must("ZVLQSTN", err)
	log.Printf("zone2 volume now = %q", vol)

	log.Println("*********************************************************")
	log.Println("* ZONE 2 IS NOW ON, MIRRORING THE MAIN (CAST) SOURCE.   *")
	log.Println("* You should hear FIP on the zone 2 speakers right now. *")
	log.Printf("* Holding for %s, then restoring zone 2 to as-found.  *", *hold)
	log.Println("*********************************************************")
	time.Sleep(*hold)
	// The deferred restore puts zone 2 back exactly as found.
}

func must(what string, err error) {
	if err != nil {
		// The deferred restore still runs: log.Fatalf would skip it.
		log.Printf("FAILED %s: %v; stopping and restoring", what, err)
		panic(err)
	}
}

func logErr(what string, err error) {
	if err != nil {
		log.Printf("restore step %s failed: %v", what, err)
	}
}
