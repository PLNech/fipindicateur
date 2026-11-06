package ui

import (
	"strings"

	"github.com/PLNech/fipindicateur/internal/player"
)

// audioChoice is one line of the "Sortie audio" submenu: the mpv device name
// to select and the label to show.
type audioChoice struct {
	Name  string
	Label string
}

// isMonitorSource reports a PulseAudio/PipeWire monitor source: a capture
// loopback of an output, listed by mpv beside the real sinks but never a
// place to send the radio.
func isMonitorSource(dev player.AudioDevice) bool {
	return strings.HasSuffix(dev.Name, ".monitor") ||
		strings.HasPrefix(strings.ToLower(dev.Description), "monitor of ")
}

// preferredAudioBackend is the one backend whose devices the submenu shows,
// when mpv lists several backends for the same physical outputs. PipeWire
// and Pulse each own the stream level; ALSA beside them is a duplicate.
// coreaudio is the macOS default and the one whose ao-volume is the app's
// own level; avfoundation lists the same devices again and is not the
// output the player opens.
func preferredAudioBackend(devs []player.AudioDevice) string {
	for _, want := range []string{"pipewire", "pulse", "coreaudio"} {
		for _, dev := range devs {
			if isBackendDevice(dev.Name, want) {
				return want
			}
		}
	}
	return ""
}

// menuAudioChoices turns mpv's raw audio-device-list (often ~50 entries of
// ALSA plugin variants, monitors and bridges, or the same CoreAudio device
// listed a second time under avfoundation) into a short submenu:
//   - "auto" is always first (synthesized if mpv did not list it);
//   - monitor sources are dropped;
//   - where a preferred backend exists (pipewire, else pulse, else
//     coreaudio), only that backend's devices stay;
//   - entries with an identical label collapse into the first one;
//   - the currently selected device is always visible, whatever the rules
//     above say (it takes the place of a same-label duplicate);
//   - mpv's order is kept, so the menu never reshuffles between runs.
func menuAudioChoices(devs []player.AudioDevice, selected string) []audioChoice {
	backend := preferredAudioBackend(devs)

	out := []audioChoice{{Name: "auto", Label: "Automatique"}}
	byLabel := map[string]int{"Automatique": 0}
	for _, dev := range devs {
		if dev.Name == "auto" {
			continue
		}
		isSel := dev.Name == selected
		if !isSel && (isMonitorSource(dev) || (backend != "" && !isBackendDevice(dev.Name, backend))) {
			continue
		}
		label := dev.Description
		if label == "" {
			label = dev.Name // raw name when mpv gave no description
		}
		if i, dup := byLabel[label]; dup {
			if isSel && i > 0 {
				out[i].Name = dev.Name // the active device wins its label
			}
			continue
		}
		byLabel[label] = len(out)
		out = append(out, audioChoice{Name: dev.Name, Label: label})
	}
	return out
}

// isBackendDevice reports whether an mpv device name belongs to the audio
// backend (names look like "pipewire" or "pipewire/alsa_output...").
func isBackendDevice(name, backend string) bool {
	return name == backend || strings.HasPrefix(name, backend+"/")
}
