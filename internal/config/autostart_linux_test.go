package config

import "testing"

func TestIsEphemeralPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/tmp/fipindicateur", true},                               // the 2026-07-09 freeze case
		{"/var/tmp/fipindicateur", true},                           //
		{"/home/user/.cache/go-build/abc/exe/fipindicateur", true}, // `go run`
		{"/tmp/go-build123/b001/exe/fipindicateur", true},          // `go run` in /tmp
		{"/run/user/1000/fipindicateur", true},                     // runtime dir
		{"", true},                                                 // unknown -> treat as ephemeral
		{"/home/user/.local/bin/fipindicateur", false},             // the installed path
		{"/usr/local/bin/fipindicateur", false},                    // a system install
		{"/opt/fip/fipindicateur", false},                          //
	}
	for _, c := range cases {
		if got := isEphemeralPath(c.path); got != c.want {
			t.Errorf("isEphemeralPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestAutostartExecPrefersInstalledOverEphemeral checks that when there is no
// installed binary on disk, a running ephemeral path is never persisted: the
// canonical install location is baked instead.
func TestAutostartExecPrefersInstalledOverEphemeral(t *testing.T) {
	// Point HOME at an empty dir so installedBinary() finds nothing on disk.
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := autostartExec("/tmp/fipindicateur")
	want := canonicalInstallPath()
	if got != want {
		t.Fatalf("autostartExec(ephemeral) = %q, want canonical %q", got, want)
	}
	// A real (non-ephemeral) running path with no installed binary is honoured.
	real := "/usr/local/bin/fipindicateur"
	if got := autostartExec(real); got != real {
		t.Fatalf("autostartExec(%q) = %q, want the same real path", real, got)
	}
}
