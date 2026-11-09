//go:build darwin

// Package tray is the hand-rolled macOS tray: an NSStatusItem and its NSMenu
// owned in-tree, the AppKit edition of the internal/sni precedent on Linux.
// It exists so the macOS build owns its status item instead of
// remote-controlling the fyne/systray darwin backend (see
// docs/macos-ui-rethink.md, option C, milestone M1). The surface is exactly
// what internal/ui consumes: the run/quit lifecycle, the status item's icon
// and tooltip, and a menu vocabulary of items, checkboxes, submenus,
// separators, tooltips and hide/disable, with clicks delivered on each item's
// ClickedCh. On top of the parity surface sit the two volume gestures: the
// status item's scroll wheel (SetScrollHook, deltas folded into unit steps)
// and the inline NSSlider hosted as a top-level menu entry (InsertSlider).
//
// Threading: AppKit lives on the process main thread. cmd/fipindicateur pins
// the main goroutine with runtime.LockOSThread before ui.Run, and Run blocks
// it inside [NSApp run]. Every other entry point marshals to the main thread
// and waits, the contract the library this replaced used, so callers keep
// working from any goroutine. Clicks come back on the main thread as a
// non-blocking send on ClickedCh, the exact contract App.on consumes.
package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// The native side lives in tray_darwin.m, not in this preamble: a Go file
// using //export has its preamble compiled twice, which would duplicate an
// @implementation at link time (static functions are immune, Objective-C
// classes are not; the established pattern is
// internal/notify/delegate_darwin.m). These are the .m entry points; the
// char* arguments are freed by the callee after conversion.
extern void trayRegister(void);
extern void trayRunLoop(void);
extern void trayQuit(void);
extern void traySetIconBytes(char *png, int len);
extern void traySetStatusTitle(char *title);
extern void traySetStatusTooltip(char *tip);
extern void trayAddItem(int itemID, int parentID, char *title, char *tooltip,
                        short disabled, short checked);
extern void trayAddSeparator(void);
extern void traySetItemTitle(int itemID, char *title);
extern void traySetItemTooltip(int itemID, char *tip);
extern void traySetItemEnabled(int itemID, short enabled);
extern void traySetItemChecked(int itemID, short checked);
extern void traySetItemHidden(int itemID, short hidden);
extern void trayInsertSlider(int beforeID, int sliderID, int value);
extern void traySetSliderState(int sliderID, int value, short enabled);
extern void trayAppearanceChanged(void);
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// AppKit must run on the process main thread. The main goroutine starts on
// it, but it can be handed to a different thread long before ui.Run: a
// blocking syscall (config load, instance lock) detaches its M and the
// scheduler may resume the goroutine elsewhere, and main's late
// LockOSThread would then pin the wrong thread (AppKit silently runs on a
// side thread and [NSApp run] traps on "NSUpdateCycle was already
// initialized"). Package init runs on the main goroutine before any of that,
// so pin here. fyne/systray carried exactly this init for the same reason;
// owning the tray means owning the pin too.
func init() {
	runtime.LockOSThread()
}

// Item is one menu entry, top level or nested in a submenu. ClickedCh gets
// one send per click; like the library this replaced the send is
// non-blocking, so a click with no reader is dropped rather than queued.
type Item struct {
	ClickedCh chan struct{}

	id uint32
}

var (
	mu       sync.RWMutex
	items    = map[uint32]*Item{}
	sliders  = map[uint32]*Slider{}
	nextID   atomic.Uint32
	readyFn  func()
	exitFn   func()
	exitOnce sync.Once
	quitOnce sync.Once
)

// newItem registers a fresh item and pushes its spec to the menu tree.
func newItem(title, tooltip string, parentID int, checked bool) *Item {
	it := &Item{ClickedCh: make(chan struct{}), id: nextID.Add(1)}
	mu.Lock()
	items[it.id] = it
	mu.Unlock()
	ct, ctip := C.CString(title), C.CString(tooltip)
	disabled, chk := C.short(0), C.short(0)
	if checked {
		chk = 1
	}
	C.trayAddItem(C.int(it.id), C.int(parentID), ct, ctip, disabled, chk)
	return it
}

// AddMenuItem adds a top-level menu item.
func AddMenuItem(title, tooltip string) *Item {
	return newItem(title, tooltip, 0, false)
}

// AddSeparator adds a separator bar to the top-level menu (the only place
// the UI uses one).
func AddSeparator() {
	C.trayAddSeparator()
}

// AddSubMenuItem nests an entry under it, creating the submenu on demand.
func (it *Item) AddSubMenuItem(title, tooltip string) *Item {
	return newItem(title, tooltip, int(it.id), false)
}

// AddSubMenuItemCheckbox is AddSubMenuItem with an initial checkmark state.
func (it *Item) AddSubMenuItemCheckbox(title, tooltip string, checked bool) *Item {
	return newItem(title, tooltip, int(it.id), checked)
}

// The mutators below are nil-safe: the shared chokepoints in internal/ui call
// them on slots that may not exist (tests, a menu not built yet), the same
// discipline the Linux menu-item wrapper lives by.

// SetTitle changes the item's label.
func (it *Item) SetTitle(s string) {
	if it == nil {
		return
	}
	C.traySetItemTitle(C.int(it.id), C.CString(s))
}

// SetTooltip changes the item's hover tooltip.
func (it *Item) SetTooltip(s string) {
	if it == nil {
		return
	}
	C.traySetItemTooltip(C.int(it.id), C.CString(s))
}

// Enable makes the item clickable again.
func (it *Item) Enable() {
	if it == nil {
		return
	}
	C.traySetItemEnabled(C.int(it.id), 1)
}

// Disable grays the item out; a disabled item never fires ClickedCh.
func (it *Item) Disable() {
	if it == nil {
		return
	}
	C.traySetItemEnabled(C.int(it.id), 0)
}

// Check shows the item's checkmark.
func (it *Item) Check() {
	if it == nil {
		return
	}
	C.traySetItemChecked(C.int(it.id), 1)
}

// Uncheck clears the item's checkmark.
func (it *Item) Uncheck() {
	if it == nil {
		return
	}
	C.traySetItemChecked(C.int(it.id), 0)
}

// Hide removes the item from the (sub)menu until Show.
func (it *Item) Hide() {
	if it == nil {
		return
	}
	C.traySetItemHidden(C.int(it.id), 1)
}

// Show puts a hidden item back.
func (it *Item) Show() {
	if it == nil {
		return
	}
	C.traySetItemHidden(C.int(it.id), 0)
}

// Run installs the tray, starts the AppKit main loop and blocks until Quit.
// onReady fires on its own goroutine once the application finished launching
// (the menu is then built through the marshaled setters above); onExit runs
// exactly once when the loop is over, on the goroutine returning from Run,
// so app_stop and the recorder flush happen on the quit path like on Linux.
func Run(onReady, onExit func()) {
	readyCh := make(chan struct{})
	mu.Lock()
	readyFn = func() { close(readyCh) }
	exitFn = onExit
	mu.Unlock()
	go func() {
		<-readyCh
		if onReady != nil {
			onReady()
		}
	}()
	C.trayRegister()
	C.trayRunLoop()
	exitOnce.Do(fireExit)
}

// fireExit runs the OnExit callback at most once, whichever path gets there
// first: the loop returning (Quit) or applicationWillTerminate (terminate:).
func fireExit() {
	mu.Lock()
	fn := exitFn
	exitFn = nil
	mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Quit ends the tray run loop. Safe to call from any goroutine and more than
// once; Run returns and onExit runs afterwards.
func Quit() {
	quitOnce.Do(func() { C.trayQuit() })
}

// SetIcon shows PNG bytes in the status item. Deliberate deviation from the
// replaced backend: the image is sized from the PNG itself (the tray hands
// 44 px @2x bytes, drawn at half the pixel size, 22 pt) instead of being
// flattened to a hard-coded 16x16.
func SetIcon(png []byte) {
	if len(png) == 0 {
		return
	}
	// The bytes are read synchronously on this thread (NSData copies them)
	// before the marshaled call, so the Go slice needs no copy.
	C.traySetIconBytes((*C.char)(unsafe.Pointer(&png[0])), C.int(len(png)))
}

// SetTitle sets the status item's text next to its icon (empty in this app).
func SetTitle(title string) {
	C.traySetStatusTitle(C.CString(title))
}

// SetTooltip sets the status item's hover tooltip.
func SetTooltip(tip string) {
	C.traySetStatusTooltip(C.CString(tip))
}

//export trayReady
func trayReady() {
	mu.Lock()
	fn := readyFn
	mu.Unlock()
	if fn != nil {
		fn()
	}
}

//export trayExit
func trayExit() {
	exitOnce.Do(fireExit)
}

//export trayItemSelected
func trayItemSelected(id C.int) {
	mu.RLock()
	it := items[uint32(id)]
	mu.RUnlock()
	if it == nil {
		return
	}
	select {
	case it.ClickedCh <- struct{}{}:
	default:
	}
}

// --- scroll-wheel volume (M2) ---

// The scroll gesture: the overlay view on the status item's button hands raw
// events over (trayScrollWheel, below), the accumulator folds them into unit
// steps, and the consumer goroutine delivers each step to the hook on its own
// goroutine, exactly like a click delivery: the AppKit loop is never blocked
// by a volume write.
const (
	// scrollPxPerStep is the precise (trackpad) travel, in points, that makes
	// one step: small touch samples accumulate instead of firing an event per
	// finger movement, so a two-finger glide reads as a few steps and not as
	// dozens. One point of reference: a discrete wheel notch is one step, and
	// AppKit counts roughly ten points per line, so 15 is a notch and a bit.
	scrollPxPerStep = 15.0
	// scrollQueue bounds the steps waiting for the consumer; when the sink is
	// backed up a dropped step beats a blocked UI thread (the next notch
	// corrects the level anyway).
	scrollQueue = 16
	// maxStepsPerEvent caps one event's yield so a burst cannot empty the
	// whole scale at once; the accumulator keeps the excess for the next
	// event instead of discarding it.
	maxStepsPerEvent = 8
	// stepDownDelta/stepUpDelta are the unit deltas handed to the hook, in
	// App.scrollVolume's own sign convention (positive = volume down; on the
	// Linux SNI a wheel-up arrives negative too, so both platforms share the
	// same call unchanged).
	stepDownDelta = 1
	stepUpDelta   = -1
)

var (
	scrollMu   sync.Mutex
	scrollFn   func(delta int)
	scrollAcc  float64
	scrollOnce sync.Once
)

// scrollSteps carries the folded steps to the delivery goroutine. Allocated
// at init: scroll events can arrive between the overlay's installation and
// the hook registration, so the export must never dereference a nil channel.
var scrollSteps = make(chan int, scrollQueue)

// SetScrollHook installs fn as the receiver of the status item's scroll
// gestures. Each step arrives as a unit delta in scrollVolume's sign
// convention (see stepDownDelta): the native deltas (a positive
// scrollingDeltaY means the content moves up, natural scrolling included) are
// folded and inverted here, so the call site is a pass-through. A nil fn
// disables the gestures.
func SetScrollHook(fn func(delta int)) {
	scrollOnce.Do(func() {
		go func() {
			for d := range scrollSteps {
				scrollMu.Lock()
				f := scrollFn
				scrollMu.Unlock()
				if f != nil {
					f(d)
				}
			}
		}()
	})
	scrollMu.Lock()
	scrollFn = fn
	scrollMu.Unlock()
}

//export trayScrollWheel
func trayScrollWheel(deltaY C.double, precise C.int) {
	var steps [maxStepsPerEvent]int
	n := 0
	scrollMu.Lock()
	if precise != 0 {
		acc := scrollAcc
		if (acc > 0 && deltaY < 0) || (acc < 0 && deltaY > 0) {
			acc = 0 // direction reversal: the pending travel is stale
		}
		acc += float64(deltaY)
		if acc > scrollPxPerStep {
			acc = scrollPxPerStep
		} else if acc < -scrollPxPerStep {
			acc = -scrollPxPerStep
		}
		for acc >= scrollPxPerStep && n < maxStepsPerEvent {
			steps[n] = stepUpDelta // the content moved up: volume up
			n++
			acc -= scrollPxPerStep
		}
		for acc <= -scrollPxPerStep && n < maxStepsPerEvent {
			steps[n] = stepDownDelta
			n++
			acc += scrollPxPerStep
		}
		scrollAcc = acc
	} else if deltaY != 0 {
		// Discrete wheel: one notch, one step (the sign only, hosts scale
		// ticks differently). A positive deltaY is a notch up.
		scrollAcc = 0 // wheel and trackpad never mix within one gesture
		if deltaY > 0 {
			steps[n] = stepUpDelta
		} else {
			steps[n] = stepDownDelta
		}
		n++
	}
	scrollMu.Unlock()
	for i := 0; i < n; i++ {
		select {
		case scrollSteps <- steps[i]:
		default:
		}
	}
}

// --- inline volume slider (M3) ---

// Slider is an inline NSSlider hosted as a menu item's custom view, the
// darwin edition of the volume control, at the top level of the tray menu.
// User drags arrive on the onChange callback (one dedicated goroutine, latest
// pending value wins: a fast drag applies the freshest level, never a stale
// one, and never blocks the menu's tracking loop); SetValue moves the handle
// programmatically without firing the callback.
type Slider struct {
	id uint32

	fn   func(int)
	wake chan struct{}

	mu      sync.Mutex
	pending int
	has     bool
}

// sliderNeutral is the handle position shown while the active sink has not
// reported a level (a cast device before its first RECEIVER_STATUS): the
// slider is grayed out there, so the position claims nothing, the same rule
// that keeps a wheel notch from inventing a master-volume level.
const sliderNeutral = 50

// InsertSlider hosts a 0..100 NSSlider as a top-level menu entry, inserted
// immediately before before (the Volume item: one click opens the menu, one
// drag sets the level, no submenu hop; the Volume submenu keeps the mute
// toggle and the presets). A nil before appends the slider at the end, the
// same nil-safe discipline as the item mutators. The slider is continuous:
// every drag tick fires onChange with the new level, which lands on the same
// write path as the menu presets; the events log is coalesced per gesture by
// App.recordVolume, so only the write is per tick.
func InsertSlider(before *Item, value int, onChange func(int)) *Slider {
	s := &Slider{fn: onChange, wake: make(chan struct{}, 1)}
	mu.Lock()
	s.id = nextID.Add(1)
	sliders[s.id] = s
	mu.Unlock()
	beforeID := 0
	if before != nil {
		beforeID = int(before.id)
	}
	C.trayInsertSlider(C.int(beforeID), C.int(s.id), C.int(value))
	go s.pump()
	return s
}

// pump delivers drag ticks to the consumer: one goroutine per slider, woken
// by the exported callback, always applying the latest pending value.
func (s *Slider) pump() {
	for range s.wake {
		s.mu.Lock()
		v, ok := s.pending, s.has
		s.has = false
		fn := s.fn
		s.mu.Unlock()
		if ok && fn != nil {
			fn(v)
		}
	}
}

// changed records a user drag tick and wakes the pump (called from the main
// thread by traySliderChanged).
func (s *Slider) changed(v int) {
	s.mu.Lock()
	s.pending = v
	s.has = true
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default: // already awake: the pump will read the latest pending value
	}
}

// SetState moves the handle to value and toggles dragging. known=false (a
// cast device that has not reported a level) grays the slider out at the
// neutral midpoint instead of displaying an invented level. The programmatic
// move never fires the change callback, so a refresh cannot echo back into
// Go as if the user had moved the handle.
func (s *Slider) SetState(value int, known bool) {
	if s == nil {
		return
	}
	if !known {
		value = sliderNeutral
	}
	en := C.short(0)
	if known {
		en = 1
	}
	C.traySetSliderState(C.int(s.id), C.int(value), en)
}

//export traySliderChanged
func traySliderChanged(id C.int, value C.int) {
	mu.RLock()
	s := sliders[uint32(id)]
	mu.RUnlock()
	if s != nil {
		s.changed(int(value))
	}
}

// OnMenuOpen registers fn to run just before the tray's menu opens (main
// thread): the hook re-syncs the state the menu displays but does not own,
// above all the slider's position, which must show the ACTIVE sink's level
// now and not the level of build time.
var (
	menuOpenMu sync.Mutex
	menuOpenFn func()
)

func OnMenuOpen(fn func()) {
	menuOpenMu.Lock()
	menuOpenFn = fn
	menuOpenMu.Unlock()
}

//export trayMenuOpen
func trayMenuOpen() {
	menuOpenMu.Lock()
	fn := menuOpenFn
	menuOpenMu.Unlock()
	if fn != nil {
		fn()
	}
}

// --- live appearance (the Auto flip) ---

var (
	appearanceMu sync.Mutex
	appearanceFn func()
)

// OnAppearanceChange registers fn to run when macOS posts the effective
// appearance change notification: the moment the Auto (wallpaper-driven)
// appearance flips the menu bar dark or light. The consumer re-probes the
// panel darkness and re-inks the glyph; a boot-time answer frozen forever is
// exactly the bug this exists to prevent. One hook (the app is the only
// consumer, same as OnMenuOpen); nil clears.
func OnAppearanceChange(fn func()) {
	appearanceMu.Lock()
	appearanceFn = fn
	appearanceMu.Unlock()
}

//export trayAppearanceChanged
func trayAppearanceChanged() {
	appearanceMu.Lock()
	fn := appearanceFn
	appearanceMu.Unlock()
	if fn != nil {
		fn()
	}
}
