//go:build darwin && swiftui

// Package fipui is the cgo seam to the SwiftUI popover: the Swift sources in
// swift/ are compiled by `make swiftui` into one object, dropped here as
// fipui_darwin_<arch>.syso (generated, not committed), and linked into the Go
// binary. One process, no IPC.
//
// Why a package of its own: a .syso is linked whenever its package is linked,
// whatever the build tags say, and the Swift object references goFipAction.
// So every Go file here sits behind the swiftui tag: the default build does
// not see this package at all, never links the object and never needs a
// Swift toolchain. Import it only from code behind the same tag
// (internal/popover does).
//
// Why a .syso and not a static library: the Go build cache hashes a .syso
// with the package, but not an archive named by -l (a stale UI would be
// linked without a word).
package fipui

/*
#cgo LDFLAGS: -framework SwiftUI -framework AppKit
#include <stdlib.h>

// Implemented in swift/Bridge.swift (@_cdecl), linked from the .syso.
extern int fipui_push_state(const char *json);
extern void fipui_toggle(void *anchor);
extern void fipui_close(void);
extern void fipui_show_settings(void);
extern void fipui_show_carton(const char *json);
extern int fipui_is_shown(void);
extern void fipui_set_bars(double a, double b, double c, double d);
extern int fipui_debug_fire(const char *id, const char *arg);
extern int fipui_debug_render(const char *view, const char *path, const char *json, int dark);
*/
import "C"

import (
	"sync"
	"unsafe"
)

var (
	handlerMu sync.Mutex
	handler   func(id, arg string)
)

// SetHandler installs the one receiver of every action the popover fires. It
// runs on the AppKit main thread, inside a control's callback: it must hand
// off and return (internal/ui queues the action for its own worker).
func SetHandler(fn func(id, arg string)) {
	handlerMu.Lock()
	handler = fn
	handlerMu.Unlock()
}

//export goFipAction
func goFipAction(cid, carg *C.char) {
	handlerMu.Lock()
	fn := handler
	handlerMu.Unlock()
	if fn != nil {
		fn(C.GoString(cid), C.GoString(carg))
	}
}

// PushState hands one JSON snapshot to the Swift model (any goroutine; Swift
// applies it on the main thread). False means Swift could not decode it.
func PushState(json []byte) bool {
	cs := C.CString(string(json))
	defer C.free(unsafe.Pointer(cs))
	return C.fipui_push_state(cs) == 1
}

// Toggle shows the card on the screen of anchor (an NSView*, the status
// item's button) or closes it when it is shown.
func Toggle(anchor unsafe.Pointer) {
	C.fipui_toggle(anchor)
}

// DebugFire makes Swift fire an action as a control would: the round trip
// the link test proves. False means id is not in the Swift Action enum.
func DebugFire(id, arg string) bool {
	cid, carg := C.CString(id), C.CString(arg)
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(carg))
	return C.fipui_debug_fire(cid, carg) == 1
}

// Close puts the card away if it is up.
func Close() { C.fipui_close() }

// Shown reports whether the card is up. Safe from any goroutine.
func Shown() bool { return C.fipui_is_shown() == 1 }

// SetBars feeds the meter one frame of four bar heights (0..1). Cheap, and
// ignored while the card is hidden.
func SetBars(b [4]float64) {
	C.fipui_set_bars(C.double(b[0]), C.double(b[1]), C.double(b[2]), C.double(b[3]))
}

// ShowCarton shows one carton (JSON of popover.Carton) for a few seconds.
func ShowCarton(json []byte) {
	cs := C.CString(string(json))
	defer C.free(unsafe.Pointer(cs))
	C.fipui_show_carton(cs)
}

// ShowSettings turns the card to its Réglages face, showing it if needed.
func ShowSettings() { C.fipui_show_settings() }

// DebugRender draws a view ("card" or "settings") for a JSON snapshot into a
// PNG at path, offscreen.
// It needs the process main thread (AppKit), so it is for a test binary
// that pins it, not for the running app.
func DebugRender(view, path string, json []byte, dark bool) bool {
	cv, cp, cj := C.CString(view), C.CString(path), C.CString(string(json))
	defer C.free(unsafe.Pointer(cv))
	defer C.free(unsafe.Pointer(cp))
	defer C.free(unsafe.Pointer(cj))
	d := C.int(0)
	if dark {
		d = 1
	}
	return C.fipui_debug_render(cv, cp, cj, d) == 1
}
