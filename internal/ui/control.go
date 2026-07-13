package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PLNech/fipindicateur/internal/stations"
	"github.com/PLNech/fipindicateur/internal/version"
)

// Status is the single-line JSON payload returned by the control socket's
// `status` command: enough to render an external now-playing line without
// touching the tray. It is behaviour + display state only, no track history.
type Status struct {
	Station string `json:"station"`
	Playing bool   `json:"playing"`
	Artist  string `json:"artist"`
	Title   string `json:"title"`
	Show    string `json:"show"`
	Volume  int    `json:"volume"`
	Mute    bool   `json:"mute"`
	Version string `json:"version"`
}

// controlApp is the surface the line protocol drives. *App satisfies it; a fake
// implements it in the tests so the protocol is verifiable without a socket, a
// tray, or a live mpv. Play/Pause/Toggle are the same methods MPRIS calls.
type controlApp interface {
	Play()
	Pause()
	Toggle()
	ControlStatus() Status
	ControlStation(id string) error
	StationIDs() []string
}

// handleControl runs one command line against the app and returns the response
// text (already newline-terminated, possibly multi-line). Pure and
// side-effect-free except through app, so it is table-testable. The protocol is
// deliberately tiny and plain-text: one verb per line, an "ok"/"err ..." reply
// for actions, JSON for status, one id per line for stations.
func handleControl(app controlApp, line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "err empty command\n"
	}
	switch fields[0] {
	case "status":
		b, err := json.Marshal(app.ControlStatus())
		if err != nil {
			return "err status\n"
		}
		return string(b) + "\n"
	case "play":
		app.Play()
		return "ok\n"
	case "pause":
		app.Pause()
		return "ok\n"
	case "toggle":
		app.Toggle()
		return "ok\n"
	case "stations":
		var sb strings.Builder
		for _, id := range app.StationIDs() {
			sb.WriteString(id)
			sb.WriteByte('\n')
		}
		return sb.String()
	case "station":
		if len(fields) < 2 {
			return "err usage: station <id>\n"
		}
		if err := app.ControlStation(fields[1]); err != nil {
			return "err " + err.Error() + "\n"
		}
		return "ok\n"
	default:
		return "err unknown command\n"
	}
}

// ControlStatus snapshots the current playback state for the `status` command.
// a.now is guarded by a.mu; a.current/a.cfg are read without a lock, matching
// the existing MPRIS/menu access pattern (single writer per field in practice).
func (a *App) ControlStatus() Status {
	a.mu.Lock()
	np := a.now
	a.mu.Unlock()
	var show string
	if np.Show != nil {
		show = np.Show.Title
	}
	playing := false
	if a.player != nil {
		playing = a.player.IsPlaying()
	}
	return Status{
		Station: a.current.Key,
		Playing: playing,
		Artist:  np.Artist,
		Title:   np.Title,
		Show:    show,
		Volume:  a.cfg.Volume,
		Mute:    a.cfg.Mute,
		Version: version.String(),
	}
}

// ControlStation validates the id and switches to it through setStation, the
// same path the tray radio menu uses (records the Markov transition, updates
// checkmarks, persists config). An unknown id is rejected rather than silently
// tuning fip (ByKey's fallback).
func (a *App) ControlStation(id string) error {
	if !stations.Exists(id) {
		return fmt.Errorf("unknown station: %s", id)
	}
	a.setStation(id)
	return nil
}

// StationIDs lists the known station keys for the `stations` command.
func (a *App) StationIDs() []string { return stations.Keys() }
