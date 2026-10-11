package ui

import (
	"reflect"
	"testing"

	"github.com/PLNech/fipindicateur/internal/player"
)

func TestMenuAudioChoices(t *testing.T) {
	dev := func(n, d string) player.AudioDevice { return player.AudioDevice{Name: n, Description: d} }
	pw := []player.AudioDevice{
		dev("auto", "Autodetect device"),
		dev("alsa/default", "Default (alsa)"),
		dev("alsa/hdmi:CARD=PCH,DEV=0", "HDA Intel PCH, HDMI 0"),
		dev("pipewire", "PipeWire"),
		dev("pipewire/alsa_output.pci.analog-stereo", "Haut-parleurs"),
		dev("pipewire/alsa_output.pci.analog-stereo.monitor", "Monitor of Haut-parleurs"),
		dev("pipewire/bluez_output.AA", "Casque"),
		dev("pipewire/bluez_output.BB", "Casque"),
		dev("jack", "JACK audio server"),
	}
	tests := []struct {
		name     string
		devs     []player.AudioDevice
		selected string
		want     []audioChoice
	}{
		{"empty list still offers auto", nil, "auto", []audioChoice{{"auto", "Automatique"}}},
		{"pipewire: monitors, alsa, jack and duplicates hidden", pw, "auto", []audioChoice{
			{"auto", "Automatique"},
			{"pipewire", "PipeWire"},
			{"pipewire/alsa_output.pci.analog-stereo", "Haut-parleurs"},
			{"pipewire/bluez_output.AA", "Casque"},
		}},
		{"selected monitor stays visible", pw, "pipewire/alsa_output.pci.analog-stereo.monitor", []audioChoice{
			{"auto", "Automatique"},
			{"pipewire", "PipeWire"},
			{"pipewire/alsa_output.pci.analog-stereo", "Haut-parleurs"},
			{"pipewire/alsa_output.pci.analog-stereo.monitor", "Monitor of Haut-parleurs"},
			{"pipewire/bluez_output.AA", "Casque"},
		}},
		{"selected device from another backend stays", pw, "alsa/default", []audioChoice{
			{"auto", "Automatique"},
			{"alsa/default", "Default (alsa)"},
			{"pipewire", "PipeWire"},
			{"pipewire/alsa_output.pci.analog-stereo", "Haut-parleurs"},
			{"pipewire/bluez_output.AA", "Casque"},
		}},
		{"selected duplicate takes over its label's slot", pw, "pipewire/bluez_output.BB", []audioChoice{
			{"auto", "Automatique"},
			{"pipewire", "PipeWire"},
			{"pipewire/alsa_output.pci.analog-stereo", "Haut-parleurs"},
			{"pipewire/bluez_output.BB", "Casque"},
		}},
		{"auto not first in mpv's list is moved first", []player.AudioDevice{
			dev("alsa/default", "Default (alsa)"), dev("auto", "Autodetect device"),
		}, "auto", []audioChoice{{"auto", "Automatique"}, {"alsa/default", "Default (alsa)"}}},
		{"no pipewire/pulse: alsa stays, monitors by description go", []player.AudioDevice{
			dev("auto", "Autodetect device"), dev("alsa/front", "Front"), dev("alsa/mon", "Monitor of Front"), dev("alsa/raw", ""),
		}, "auto", []audioChoice{{"auto", "Automatique"}, {"alsa/front", "Front"}, {"alsa/raw", "alsa/raw"}}},
		{"pulse used when pipewire is absent", []player.AudioDevice{
			dev("auto", "x"), dev("alsa/default", "Default"), dev("pulse/sink1", "Sink 1"),
		}, "auto", []audioChoice{{"auto", "Automatique"}, {"pulse/sink1", "Sink 1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := menuAudioChoices(tt.devs, tt.selected)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %v\nwant %v", got, tt.want)
			}
			if again := menuAudioChoices(tt.devs, tt.selected); !reflect.DeepEqual(got, again) {
				t.Errorf("order not stable: %v vs %v", got, again)
			}
		})
	}
}
