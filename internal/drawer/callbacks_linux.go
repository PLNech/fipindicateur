//go:build linux && cgo

package drawer

// The //export callbacks live in their own file because cgo forbids C
// definitions in the preamble of a file that exports Go functions: this
// preamble carries only includes, the C helpers live in drawer_linux.go.

/*
#cgo pkg-config: webkit2gtk-4.1 gtk+-3.0

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
*/
import "C"

import (
	"encoding/json"
	"log"
	"unsafe"
)

// fipDrawerIdle drains one queued closure on the GTK thread. One g_idle_add
// per queued closure keeps the pairing exact; both the channel and idle
// sources are FIFO, so order is preserved.
//
//export fipDrawerIdle
func fipDrawerIdle(data C.gpointer) C.gboolean {
	select {
	case fn := <-gtkCalls:
		fn()
	default:
	}
	return C.gboolean(0) // G_SOURCE_REMOVE
}

// fipDrawerScriptMessage receives the page's postMessage payloads (the "fip"
// handler): a JSON Command per message.
//
//export fipDrawerScriptMessage
func fipDrawerScriptMessage(m *C.WebKitUserContentManager, r *C.WebKitJavascriptResult, data C.gpointer) {
	v := C.webkit_javascript_result_get_js_value(r)
	cs := C.jsc_value_to_string(v)
	if cs == nil {
		return
	}
	msg := C.GoString(cs)
	C.g_free(C.gpointer(unsafe.Pointer(cs)))
	d := currentDrawer()
	if d == nil {
		return
	}
	var cmd Command
	if err := json.Unmarshal([]byte(msg), &cmd); err != nil {
		log.Printf("drawer: unparsable page message %q: %v", msg, err)
		return
	}
	d.dispatch(cmd)
}

// fipDrawerKeyPress hides on Escape (GTK-level backstop; the page also sends
// a "hide" action, since the webview usually owns keyboard focus). Losing
// focus deliberately does NOT hide: the panel floats until dismissed.
//
//export fipDrawerKeyPress
func fipDrawerKeyPress(w *C.GtkWidget, e *C.GdkEventKey, data C.gpointer) C.gboolean {
	if e.keyval == C.GDK_KEY_Escape {
		if d := currentDrawer(); d != nil {
			d.hideOnGTK()
		}
		return C.gboolean(1)
	}
	return C.gboolean(0)
}

// fipDrawerDelete turns a close request into a hide: the window stays
// resident so reopening is instant.
//
//export fipDrawerDelete
func fipDrawerDelete(w *C.GtkWidget, e *C.GdkEvent, data C.gpointer) C.gboolean {
	if d := currentDrawer(); d != nil {
		d.hideOnGTK()
	}
	return C.gboolean(1) // handled: never destroy
}
