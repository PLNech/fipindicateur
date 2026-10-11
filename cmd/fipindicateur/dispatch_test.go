package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		base string
		args []string
		want decision
	}{
		// fipindicateur: bare launches the tray.
		{"fipindicateur bare launches tray", "fipindicateur", nil, decision{kind: actLaunchTray}},
		{"absolute path bare launches tray", "/usr/local/bin/fipindicateur", nil, decision{kind: actLaunchTray}},
		// fip: bare queries the running instance (status), never the tray.
		{"fip bare runs status", "fip", nil, decision{kind: actControl, args: []string{"status"}}},
		{"fip path bare runs status", "/home/u/.local/bin/fip", nil, decision{kind: actControl, args: []string{"status"}}},

		// Help is universal, exit-0 action, for both names.
		{"help word", "fipindicateur", []string{"help"}, decision{kind: actHelp}},
		{"help -h", "fipindicateur", []string{"-h"}, decision{kind: actHelp}},
		{"help --help", "fip", []string{"--help"}, decision{kind: actHelp}},
		{"help wins over trailing args", "fip", []string{"--help", "station", "jazz"}, decision{kind: actHelp}},

		// Version.
		{"version word", "fipindicateur", []string{"version"}, decision{kind: actVersion}},
		{"version --version", "fipindicateur", []string{"--version"}, decision{kind: actVersion}},
		{"version -v", "fip", []string{"-v"}, decision{kind: actVersion}},

		// Stats forwards its flags untouched.
		{"stats no flags", "fipindicateur", []string{"stats"}, decision{kind: actStats, args: []string{}}},
		{"stats with flags", "fipindicateur", []string{"stats", "--out", "r.html", "--no-open"}, decision{kind: actStats, args: []string{"--out", "r.html", "--no-open"}}},

		// Control verbs forward the whole command line (verb included).
		{"status via fipindicateur", "fipindicateur", []string{"status"}, decision{kind: actControl, args: []string{"status"}}},
		{"play", "fip", []string{"play"}, decision{kind: actControl, args: []string{"play"}}},
		{"pause", "fip", []string{"pause"}, decision{kind: actControl, args: []string{"pause"}}},
		{"toggle", "fipindicateur", []string{"toggle"}, decision{kind: actControl, args: []string{"toggle"}}},
		{"stations", "fip", []string{"stations"}, decision{kind: actControl, args: []string{"stations"}}},
		{"station with id", "fip", []string{"station", "jazz"}, decision{kind: actControl, args: []string{"station", "jazz"}}},

		// cast: bare `cast` behaves as `cast scan`; unknown subactions error.
		{"cast bare scans", "fipindicateur", []string{"cast"}, decision{kind: actCastScan}},
		{"cast scan", "fipindicateur", []string{"cast", "scan"}, decision{kind: actCastScan}},
		{"cast scan via fip", "fip", []string{"cast", "scan"}, decision{kind: actCastScan}},
		{"cast unknown subaction", "fip", []string{"cast", "frobnicate"}, decision{kind: actUsageErr}},
		{"cast scan extra args", "fip", []string{"cast", "scan", "x"}, decision{kind: actUsageErr}},

		// Unknown subcommand: usage error (exit 2), never the tray, for both names.
		{"unknown via fipindicateur", "fipindicateur", []string{"frobnicate"}, decision{kind: actUsageErr}},
		{"unknown via fip", "fip", []string{"frobnicate"}, decision{kind: actUsageErr}},
		{"unknown flag", "fipindicateur", []string{"--nope"}, decision{kind: actUsageErr}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decide(tt.base, tt.args)
			if got.kind != tt.want.kind {
				t.Fatalf("decide(%q, %v).kind = %d, want %d", tt.base, tt.args, got.kind, tt.want.kind)
			}
			if !reflect.DeepEqual(got.args, tt.want.args) {
				t.Fatalf("decide(%q, %v).args = %#v, want %#v", tt.base, tt.args, got.args, tt.want.args)
			}
		})
	}
}

// TestUsageMentionsEveryCommand guards the help screen against drifting out of
// sync with the dispatch: each subcommand a user can type must be documented.
func TestUsageMentionsEveryCommand(t *testing.T) {
	u := usage()
	for _, cmd := range []string{"status", "play", "pause", "toggle", "station", "stations", "stats", "cast", "version", "help", "--out", "--no-open"} {
		if !strings.Contains(u, cmd) {
			t.Errorf("usage() does not mention %q", cmd)
		}
	}
	// It must explain the no-argument behaviour of both invocations.
	for _, want := range []string{"fipindicateur", "fip", "tray"} {
		if !strings.Contains(u, want) {
			t.Errorf("usage() does not mention %q", want)
		}
	}
}
