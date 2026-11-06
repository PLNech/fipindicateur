package player

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestSpectrumBandsFromSilentTone checks that a tone in one band lifts that
// bar and leaves the others down. The file is generated in the test temp dir
// and played with the output device disconnected, so nothing reaches a speaker.
func TestSpectrumBandsFromSilentTone(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	tones := []struct {
		name string
		hz   int
		bar  int
	}{
		{"bass", 60, 0},
		{"lowmid", 220, 1},
		{"presence", 900, 2},
		{"air", 4000, 3},
	}
	for _, tone := range tones {
		t.Run(tone.name, func(t *testing.T) {
			wav := filepath.Join(dir, tone.name+".wav")
			cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi",
				"-i", fmt.Sprintf("sine=frequency=%d:sample_rate=44100:duration=2", tone.hz),
				"-c:a", "pcm_s16le", wav)
			cmd.Stdout, cmd.Stderr = nil, nil
			if err := cmd.Run(); err != nil {
				t.Fatalf("synth: %v", err)
			}
			if _, err := os.Stat(wav); err != nil {
				t.Fatal(err)
			}
			m := &MPV{}
			m.Silence()
			if err := m.Initialize(); err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			m.Play(wav)
			var got [4]float64
			seen := false
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(150 * time.Millisecond)
				b, ok := m.SpectrumBands()
				if !ok {
					continue
				}
				got = b
				seen = true
				if loudest(b) == tone.bar && b[tone.bar] > 0.3 {
					return
				}
			}
			if !seen {
				t.Fatalf("%d Hz: no band reading (output stayed disconnected)", tone.hz)
			}
			t.Fatalf("%d Hz: want bar %d loudest and above 0.3, got %v", tone.hz, tone.bar, got)
		})
	}
}

func loudest(b [4]float64) int {
	i := 0
	for k := 1; k < len(b); k++ {
		if b[k] > b[i] {
			i = k
		}
	}
	return i
}
