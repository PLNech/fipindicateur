package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveRefusesRealConfigDir pins the mechanical guard: a test binary that
// points the config dir outside os.TempDir() must panic before any write, so a
// forgotten t.Setenv can never clobber the developer's real config.json.
func TestSaveRefusesRealConfigDir(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("no HOME to stand in for a real config dir")
	}
	// Not under os.TempDir(): a path the guard must reject. Nothing is created
	// here, the guard fires before path() could write anything.
	fake := filepath.Join(home, ".config-guard-probe-does-not-exist")
	if insideTemp(fake) {
		t.Skipf("HOME %s is inside the temp dir, cannot probe the guard", home)
	}
	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.Contains(msg, "outside os.TempDir()") {
			t.Fatalf("guard did not refuse %s, recovered %v", fake, r)
		}
	}()
	guardTestWrite(fake)
}

// TestSaveIsolatedUnderTempDir checks the happy path: with XDG_CONFIG_HOME in a
// temp dir, Save writes there and Load reads it back.
func TestSaveIsolatedUnderTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	c := Default()
	c.Volume = 34
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fipindicateur", "config.json")); err != nil {
		t.Fatalf("config not written under the isolated dir: %v", err)
	}
	if got := Load(); got.Volume != 34 {
		t.Fatalf("Load().Volume = %d, want 34", got.Volume)
	}
}

// TestLoadPartialKeepsDefaults: keys absent from the file keep their defaults.
func TestLoadPartialKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeRaw(t, dir, `{"station":"jazz","volume":34}`)

	got := Load()
	want := Default()
	want.Station = "jazz"
	want.Volume = 34
	if got != want {
		t.Fatalf("partial config lost defaults:\n got  %+v\n want %+v", got, want)
	}
	if !got.AnimatedIcon || !got.ShowNotifications {
		t.Fatalf("absent keys did not keep defaults: %+v", got)
	}
}

// TestLoadCorruptYieldsDefaults: unreadable JSON, an empty file and a
// truncated write all fall back to the defaults, never to a zero struct.
func TestLoadCorruptYieldsDefaults(t *testing.T) {
	for name, raw := range map[string]string{
		"garbage":   "not json at all",
		"empty":     "",
		"truncated": `{"station":"jazz","animated_ic`,
		"null":      "null",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			writeRaw(t, dir, raw)
			if got := Load(); got != Default() {
				t.Fatalf("corrupt config (%s) did not yield defaults:\n got  %+v\n want %+v", name, got, Default())
			}
		})
	}
}

func writeRaw(t *testing.T, xdg, body string) {
	t.Helper()
	d := filepath.Join(xdg, "fipindicateur")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadIgnoresRetiredKeys: a config.json written by a build that still had
// the panel carries panel_scale_pct and theme. Those keys no longer exist in
// Config; Load must neither fail nor lose the settings it still knows, and the
// next Save drops the dead keys.
func TestLoadIgnoresRetiredKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeRaw(t, dir, `{"station":"jazz","volume":34,"crossfade_secs":7,"panel_scale_pct":200,"theme":"dark","made_up_future_key":{"a":1}}`)

	got := Load()
	want := Default()
	want.Station = "jazz"
	want.Volume = 34
	want.CrossfadeSecs = 7
	if got != want {
		t.Fatalf("retired keys disturbed Load:\n got  %+v\n want %+v", got, want)
	}

	if err := got.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "fipindicateur", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dead := range []string{"panel_scale_pct", "theme"} {
		if strings.Contains(string(saved), dead) {
			t.Errorf("Save re-wrote the retired key %q", dead)
		}
	}
}
