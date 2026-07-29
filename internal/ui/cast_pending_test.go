package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/PLNech/fipindicateur/internal/cast"
)

// TestDrawerStatePendingDial pins what the panel is told while a dial is in
// flight: the clicked device is named as pending, casting is NOT yet active
// (the sound is still local until the dial lands) and the device is still
// listed, so the page can mark that very chip « Connexion… ».
func TestDrawerStatePendingDial(t *testing.T) {
	a := &App{}
	a.castDevices = []cast.Device{{Name: "Salon"}, {Name: "Cuisine"}}
	a.castDialing = "Salon"

	st := a.drawerState()
	if st.Cast.Dialing != "Salon" {
		t.Errorf("Cast.Dialing = %q, want Salon (the panel cannot show pending without it)", st.Cast.Dialing)
	}
	if st.Cast.Active {
		t.Error("Cast.Active during a dial: the audio is still local until the dial lands")
	}
	if len(st.Devices) != 2 {
		t.Errorf("Devices = %v, want both devices still listed", st.Devices)
	}
}

// TestDrawerStateNoDial is the negative control: no dial, nothing pending.
func TestDrawerStateNoDial(t *testing.T) {
	a := &App{}
	a.castDevices = []cast.Device{{Name: "Salon"}}
	if st := a.drawerState(); st.Cast.Dialing != "" {
		t.Errorf("Cast.Dialing = %q with no dial in flight, want empty", st.Cast.Dialing)
	}
}

// TestStopCastingCancelsDial covers the third pending transition: the user
// picks local playback while a receiver is still taking its time. The pending
// state must clear AND the dial must be marked superseded, so the session it
// eventually opens is dropped instead of grabbing the audio ten seconds after
// the user asked for this machine.
func TestStopCastingCancelsDial(t *testing.T) {
	a := &App{}
	a.castDevices = []cast.Device{{Name: "Salon"}}
	a.castDialing = "Salon"
	gen := a.castDialGen

	a.stopCasting(true) // no live session: cancels the dial, resumes nothing

	if a.castDialing != "" {
		t.Errorf("castDialing = %q after a cancel, want cleared", a.castDialing)
	}
	if !a.castDialSuperseded(gen) {
		t.Error("the dial in flight was not marked superseded: its session would take over late")
	}
	if st := a.drawerState(); st.Cast.Dialing != "" {
		t.Errorf("the panel still shows %q as pending after the cancel", st.Cast.Dialing)
	}
}

// TestCastDialDoneKeepsNewerPending guards the ownership rule: a cancelled dial
// finishing late must not wipe the pending state of the dial the user started
// since. Only the generation that still owns the guard may release it.
func TestCastDialDoneKeepsNewerPending(t *testing.T) {
	a := &App{}
	old := a.castDialGen
	a.castDialing = "Salon"

	a.stopCasting(true) // cancels: bumps the generation
	a.castDialing = "Cuisine"

	a.castDialDone(old) // the cancelled goroutine returning at last
	if a.castDialing != "Cuisine" {
		t.Errorf("castDialing = %q; a stale dial released a newer dial's pending state", a.castDialing)
	}

	a.castDialDone(a.castDialGen) // the current dial finishing
	if a.castDialing != "" {
		t.Errorf("castDialing = %q; the owning dial failed to release the guard", a.castDialing)
	}
}

// TestActiveVolumeLocal pins the local branch of the active-sink resolver: with
// no cast session the level is the config one, and it is always known (mpv
// needs no round trip to tell us where it is).
func TestActiveVolumeLocal(t *testing.T) {
	a := &App{}
	a.cfg.Volume = 42
	pct, known := a.activeVolume()
	if !known || pct != 42 {
		t.Errorf("activeVolume() = (%d, %v), want (42, true)", pct, known)
	}
}

// TestLocalSinkWriteChokepoints is the volume-routing invariant, guarded the
// way the package guards its other seams (see TestSingleOnClickCallSite): the
// LOCAL player's volume is written in exactly two places, setLocalVolume (the
// recorded setter) and applyVolumeLive (the event-less zenity drag ticks). Any
// other path that changes the volume must go through setActiveVolume, which
// sends it to the cast device while casting; writing a.player.SetVolume
// directly would move a sink nobody is listening to.
func TestLocalSinkWriteChokepoints(t *testing.T) {
	allowed := map[string]bool{"setLocalVolume": true, "applyVolumeLive": true}
	fnHead := regexp.MustCompile(`^func \(a \*App\) ([A-Za-z]+)\(`)

	for _, f := range []string{"ui.go", "ui_menu.go", "ui_sni_linux.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fn := ""
		for _, line := range strings.Split(string(src), "\n") {
			if m := fnHead.FindStringSubmatch(line); m != nil {
				fn = m[1]
			}
			if !strings.Contains(line, "a.player.SetVolume(") {
				continue
			}
			if !allowed[fn] {
				t.Errorf("%s: %s writes the local sink directly (a.player.SetVolume); route it through setActiveVolume", f, fn)
			}
		}
	}
}
