//go:build !windows

package player

// On Linux and macOS libmpv is a system package, so cgo discovers its compiler
// and linker flags through pkg-config. This directive lives in its own file so
// the Windows build (which has no pkg-config and links against a vendored
// dev drop instead, see cgo_windows.go) can supply explicit flags without a
// conflicting pkg-config line. The empty import "C" carries only the directive.

// #cgo pkg-config: mpv
import "C"
