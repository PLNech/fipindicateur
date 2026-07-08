// Package version exposes the build version shown in the tray menu and the
// stats report. It is overridable at link time so each build reflects its
// exact commit, which makes relaunching an updated build visible at a glance.
package version

// Version is stamped at build time via
//
//	-ldflags "-X github.com/PLNech/fipindicateur/internal/version.Version=$(git describe --tags --dirty)"
//
// (see the Makefile). Un-stamped builds (plain `go build`) report a dev
// placeholder rather than lying about a release number.
var Version = "v0.2.0-dev"

// String returns the version for display, always non-empty.
func String() string {
	if Version == "" {
		return "vdev"
	}
	return Version
}
