package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// guardTestWrite refuses to let a test binary write a config under a
// directory that is not inside os.TempDir(). A test that builds an App and
// triggers a save without isolating XDG_CONFIG_HOME would otherwise overwrite
// the developer's real config.json with whatever fixture struct it holds. The
// panic is deliberate: a returned error would only be logged by the caller and
// the offending test would stay green.
func guardTestWrite(dir string) {
	if !testing.Testing() {
		return
	}
	if insideTemp(dir) {
		return
	}
	panic(fmt.Sprintf("config: test tried to write %s outside os.TempDir(); isolate with t.Setenv(\"XDG_CONFIG_HOME\", t.TempDir())", dir))
}

func insideTemp(dir string) bool {
	tmp := resolve(os.TempDir())
	d := resolve(dir)
	return d == tmp || strings.HasPrefix(d, tmp+string(filepath.Separator))
}

// resolve follows symlinks when it can, so /tmp -> /private/tmp style aliases
// compare equal; an unresolvable path is compared as given.
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
