//go:build windows

package player

// Windows has no pkg-config, so we link against a vendored libmpv dev drop
// (headers + import library) unpacked into build/windows/mpv-dev at the repo
// root. ${SRCDIR} expands to this file's directory (internal/player) at build
// time, so the relative path resolves from any checkout location. `-lmpv`
// resolves to libmpv.dll.a (the import library shipped in the dev drop); the
// matching libmpv-2.dll must sit next to the produced .exe at runtime.
//
// See the Makefile `windows` target and its prerequisite comment for how to
// obtain the dev drop. The empty import "C" carries only the directives.

// #cgo CFLAGS: -I${SRCDIR}/../../build/windows/mpv-dev/include
// #cgo LDFLAGS: -L${SRCDIR}/../../build/windows/mpv-dev -lmpv
import "C"
