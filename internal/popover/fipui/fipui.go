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
extern int fipui_debug_fire(const char *id, const char *arg);
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

// Toggle shows the popover under anchor (an NSView*, the status item's
// button) or closes it when it is shown.
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
