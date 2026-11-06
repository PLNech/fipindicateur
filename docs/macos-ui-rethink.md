# macOS UI rethink: a hostile audit

Written 2026-11-06 as an adversarial design review. Every claim about
fyne/systray v1.12.2 is grounded in the sources at
`~/go/pkg/mod/fyne.io/systray@v1.12.2/` (file and line cited); Apple API
behavior is cited from memory of the documented contracts and marked
[inferred] where the confidence is less than full.

---

## 1) The harshest truths about the current macOS UI

These are structural, not nits. The theme of all of them: **macOS is a
second-class guest in a library whose darwin backend was written to be
merely adequate, and the codebase has already outgrown it.**

### 1.1 The menu is built to the Windows floor

`internal/ui/ui_menu.go` is `//go:build !linux`, so macOS shares its entire
menu vocabulary with Windows: title strings, checkboxes, nested submenus,
and nothing else. Consequences already visible in the file:

- Cover art per menu item is explicitly a Linux-only feature: `setNowArt`
  is a no-op here (`ui_menu.go:64-67`, "systray menu items carry no inline
  icons on these platforms"). Except that is a library limitation, not a
  platform one: systray's own `MenuItem.SetIcon` exists (systray_darwin.go:33)
  and lands in `setMenuItemIcon:` (systray_darwin.m:278-288). The comment is
  half wrong and the feature was amputated to the shared denominator.
- The volume "slider" on macOS is the zenity preset-checkbox submenu
  (`ui_menu.go:115-124`); `zenityAvailable()` returns false on macOS by
  design (`ui_dialog.go:26-29`). So macOS users get a discrete checkbox
  ladder as their primary volume control, in 2026, on the platform that
  invented the menu-bar audio widget.

### 1.2 Scroll volume exists and is dead code on darwin

`App.scrollVolume` is fully implemented (`ui.go:1878-1904`, 5 % steps,
active-sink resolution, cast-aware) and its only wiring is the Linux SNI
Scroll callback (`ui_sni_linux.go:315`). On macOS the single most-used
gesture of every menu-bar audio app (scroll on the icon to change volume)
does nothing. This is not a missing feature; it is a completed feature with
no socket to plug into.

### 1.3 Every tray repaint is a cross-thread rendezvous

systray's `runInMainThread` uses `performSelectorOnMainThread:waitUntilDone:YES`
(systray_darwin.m:391-396). The animated spectrum bars (`internal/ui/animator.go`)
push one PNG per frame through `setIcon` → `C.setIcon` → data copy → main
thread wait. While the AppKit loop is busy (menu tracking, modal loops), Go
goroutines pile up behind it. The Linux SNI path debounces this
(`ui.go:209-212`, `throttle.go`); on darwin the same bursts hit a
wait-until-done semaphore per call. It works, but the architecture is
"remote control a UI thread you do not own."

### 1.4 onReady runs on a side goroutine, racing the menu

`Register` runs `onReady` on a separate goroutine specifically "to avoid
blocking event loop" (systray.go:118-126). The entire menu tree of
`buildMenu()` is therefore constructed concurrently with the AppKit loop
already serving the status item. It is papered over by `initialMenuBuilt`
(systray.go:120-125), which nobody in this codebase waits on. Every
`add_or_update_menu_item` linear-scans the menu tree by tag
(`find_menu_item`, systray_darwin.m:238-256) while the user may be
interacting with that menu. Fine at startup; a latent race forever.

### 1.5 The left-click design is a monitor + a synthesized click

Left mouse down on the status button is swallowed by a process-global
`NSEvent` local monitor that calls `[statusItem.button performClick:]`
(systray_darwin.m:106-119, 290-297). The menu is attached, clicked
synthetically, then detached in `menuDidClose:` via a
`dispatch_async` (systray_darwin.m:299-305). This is a workaround for not
owning the button, and it means: the right-click detector NSView is a
sibling overlay (`RightClickDetector`, systray_darwin.m:55-71), the monitor
is never removed, and the whole click path is three hops away from
`systray_left_click` before Go even sees it. Any added behavior (scroll,
pressure, long-press) must bolt onto this Rube Goldberg machine.

### 1.6 The icon is hard-coded to 16 points with no 2x handling

`setIcon:` does `[image setSize:NSMakeSize(16, 16)]` (systray_darwin.m:402).
No `NSBitmapImageRep` density handling, no point-size control, no template
logic beyond a bool. A cover-as-tray-icon design needs 18-22 pt with a @2x
bitmap to look native; this backend cannot express that without editing the
vendored .m, i.e. without forking.

### 1.7 The notch workaround is coupled to the library's internals

`menuplace_darwin.go` writes `NSStatusItem Preferred Position Item-0`
because the library's first item is named "Item-0". The comment in the file
admits the coupling. Replace or reorder the tray backend and this key goes
quietly stale: the icon drifts under the notch again, silently.

### 1.8 What a tray menu cannot express (and what that costs)

An `NSMenu` can express: items, checkboxes, submenus, disabled items,
per-item icons, and (via custom views) arbitrary controls. It cannot
express: hover-affecting-anything, popover panels, a slider that lives
outside tracking, or an icon whose hover state changes. The state-of-the-art
menu-bar audio apps (SoundSource, Background Music's UI, OverSight's
pattern) all use exactly the four affordances the owner asked for: cover +
brand in the bar, scroll-to-volume on the icon, an inline slider in the
menu, and a click action with modifiers. All four are achievable in a
plain NSStatusItem + NSMenu; none are expressible through systray's public
API, and only one (title) is expressible without touching its .m.

## 2) Verdict on the orchestrator's patch-pile

Item by item. The pattern across all four: each is individually plausible,
and each is a pin pushed into a library that will pop it back out.

### 2.1 Scroll-wheel volume via a local NSEvent monitor: KEEP the goal, KILL the bolt-on

Feasibility: yes, in principle. A local monitor
(`addLocalMonitorForEventsMatchingMask:ScrollWheel`) sees events routed to
this process's windows before dispatch [grounded: documented AppKit
behavior], and the status item's button lives in a window owned by our
process (the existing left-click monitor matches on exactly that window,
systray_darwin.m:106-119). Since systray runs `[NSApp run]` inside our
binary (systray_darwin.m:376-381), scroll events over our button arrive.

The problems:

- Reaching the window. `statusItem` is a static inside
  `SystrayAppDelegate` with no accessor (systray_darwin.m:83-86). A Go-side
  monitor must identify the window by class-name sniffing
  [inferred: not a stable API surface]. The library's own monitor compares
  `event.window != self->statusItem.button.window` because it lives in the
  .m. The patch does not have that handle.
- Delta semantics. Trackpads deliver continuous `scrollingDeltaY` with
  `hasPreciseScrollingDeltas`; mice deliver discrete deltas; natural
  scrolling inverts the sign. A one-block monitor that ignores this feels
  instantly amateur (wrong direction, 40 steps per flick).
- While the menu is open, tracking runs a modal event loop; scroll events
  go to the menu, not the monitor [inferred]. Acceptable, but must be
  tested, not assumed.
- Lifetime. The monitor block strongly captures its context and must be
  removed on exit. systray's own monitor is never removed
  (systray_darwin.m:106): precedent for a leak, not for correctness. Two
  process-global hooks with no ownership story is how a `Relancer`
  restart leaves a zombie.

Verdict: scroll volume must exist, but where the button is owned: an
NSView overlay overriding `scrollWheel:` (exactly the `RightClickDetector`
precedent, systray_darwin.m:55-71, 123-131) or the native layer's own
monitor. Not a Go-side sniff of windows the library pretends not to have.

### 2.2 NSSlider in the Volume submenu via NSMenuDidBeginTrackingNotification: right idea, wrong host

Custom views in menu items do receive mouse events during menu tracking;
that is the documented purpose of `NSMenuItem.view` and the mechanism
SoundSource-class apps use for inline sliders [grounded on the API
contract, inferred on the tracking-mode details]. The patch's delivery
mechanism is the problem:

- The menu object it wants already exists: `self->menu`, owned by the
  delegate, created in `applicationDidFinishLaunching:`
  (systray_darwin.m:84, 94-95). Capturing it via a notification observer
  is breaking into a house whose key is under the mat. If the menu must be
  reached from outside, the .m must be edited - at which point you are
  forking anyway, so fork deliberately (options B/C).
- Injection by index/position is fragile. systray addresses items by tag
  with a recursive scan (`find_menu_item`, systray_darwin.m:238-256) and
  Go rebuilds/hides items concurrently (`ResetMenu`, `Hide`,
  ui_menu.go:156-163). Any "insert the slider at index N" assumption dies
  the first time the cast list refreshes mid-session.
- The slider bypasses `a.on` by construction: a view item does not fire
  `menuHandler:` (systray_darwin.m:191-195), its own target/action calls
  Go directly. The telemetry invariant survives only because
  `setActiveVolume` records at source (ui.go:1063-1077) - that path must
  be honored, and the two-click confirm items (`mStatsClear`,
  ui.go:2004-2063) must never be converted to views without a recorded
  kind.
- Live drag updates mean a main-thread hop per tick into
  `setActiveVolume`, which writes mpv/cast properties. The coalescer
  (`volcoalesce.go`) exists and must be in the path, or a fast drag
  floods the player with property writes.
- Interaction with the left-click monitor: none directly, but note that
  opening the menu relies on the attach-`performClick`-detach dance
  (systray_darwin.m:290-305); a custom view inside a menu opened by a
  synthetic click is double untested territory.
- ARC across cgo: the view's target/action block must not capture Go
  pointers (cgo pointer passing rules); the callback must be a registered
  exported Go function reached through a plain C function pointer
  [inferred: the rule bites blocks capturing Go allocations].

Verdict: MUTATE. A slider in the menu is the single biggest UX unlock and
should be built - in a tray layer that owns the menu, items, and delegate,
not notified into someone else's.

### 2.3 Cover-art tray icon: KEEP, and most of it survives even on systray

The renderer side is pure Go and house-approved: bake the rounded-rect
mask, the station brand ring and the paused dimming into the PNG
(`internal/icon/cover.go` `CoverIcon`, `internal/icon/tint.go` `Legible`;
the golden tests in `internal/icon/` pin rendering). That part needs no
native code and survives on any backend.

What the patch underestimates:

- `setIcon:` hard-codes `[image setSize:NSMakeSize(16, 16)]`
  (systray_darwin.m:402). A 44x44 cover renders at 16 pt: legal but
  anemic; the native menu-bar look wants 18-22 pt with a @2x bitmap
  [inferred: Apple's own status items are 18pt line-height era]. On
  systray, "stellar cover icon" tops out at "acceptable 16 pt cover icon"
  unless the .m is edited. Another fork pressure point.
- The spectrum animator must actually be suppressed, not paused-by-race:
  `applyIconState` / `anim.willRun()` gating exists (ui.go:883-889) but a
  cover override needs its own state bit so `drawFrame`
  (ui_menu.go:56-58) cannot stomp the cover on the next frame tick.
- Cover fetch is already best-effort with a cache (`internal/notify/cover.go`);
  reuse it, do not add a second fetch path.
- Concurrency: `App.setIcon` dedupes (trayicon.go) and `setIcon:`
  marshals to the main thread (systray_darwin.m:398-406), so a cover
  update racing an animation frame serializes correctly. The failure mode
  is not a crash, it is flicker between cover and bars if the suppression
  bit is set from the wrong goroutine.

Verdict: KEEP as designed, with the caveat that the 16 pt ceiling makes it
a partial win on un-forked systray.

### 2.4 Track title next to the cover via SetTitle: KILL (or demote to a setting)

`SetTitle` works (systray_darwin.m:169-184, `NSImageLeft` positioning) and
needs zero native code. That is the only thing it has going for it:

- Every track change relayouts the whole menu bar. On the owner's notched
  MacBook with a crowded bar, macOS hides overflowing items silently; a
  variable-width title is the single most effective way to make our own
  icon disappear under the notch. The `menuplace_darwin.go` hack mitigates
  position, not overflow.
- The status button title has no automatic truncation to a sane width;
  the Go side must measure and truncate (an em-dash-banned, accent-heavy
  French label at that), and a mismeasured label gets ellipsized by
  AppKit at an arbitrary point.
- Information density: the title in the bar duplicates `mNow`
  (ui_menu.go:77-78), which is one click away and already throttled
  (ui.go:209-212, 495-502). Stellar menu-bar apps show the station name
  or nothing; the track title belongs in the menu header and the
  notification, both of which already exist.

Verdict: KILL for the track title. Optionally KEEP as a default-off
"Afficher le nom de la station" setting (station names are short and
stable) - that is the only variant that does not fight the notch.

### 2.5 The pattern

Two of the four items are blocked by things the library hard-codes (16 pt
icon, no menu handle), one is implementable only by impersonating the
library's internals, and one works but hurts. This is not a patch-pile;
it is a fork wearing a trench coat. The correct reaction to needing four
cgo additions to a vendored third-party .m, in a codebase that already
hand-rolled its entire Linux tray (`internal/sni`), is not more pins. It
is option C.

## 3) Options A-D compared

| | A: systray + cgo pins | B: fork darwin backend into internal/tray | C: hand-roll darwin tray (sni precedent) | D: SwiftUI menu-bar extra + IPC |
|---|---|---|---|---|
| New native lines | ~150-250, scattered, unowned | ~300-500 delta on a vendored ~480-line .m | ~500-700 cgo/ObjC + ~300 Go adapter | ~1500+ (Swift app + IPC protocol + packaging) |
| New Go modules | 0 | 0 (vendored code, not a module) | 0 | 0 but a second toolchain |
| Unlocks scroll | via window-sniffing monitor [fragile] | yes, properly | yes, properly | yes (SwiftUI onScroll) |
| Unlocks slider item | via notification + index hacks [fragile] | yes (own the menu) | yes (own the menu) | yes, trivially |
| Cover at native size | no (16 pt hard cap, systray_darwin.m:402) | yes | yes | yes |
| Shared ui_menu.go | untouched | split: darwin takes its own builder | split: darwin takes its own builder | bypassed entirely |
| Guard tests | survive | survive (a.on unchanged) | survive (a.on unchanged) | broken: no Go click surface, telemetry needs re-proof |
| Windows story | shared file, unchanged | keeps upstream systray | keeps upstream systray | needs its own path anyway |
| Maintenance | highest per feature (every feature = a new pin) | low-ish, but merges against upstream forever | low: we own ~800 lines we understand | highest: Xcode, signing, two release artifacts |
| Single binary | yes | yes | yes | no (two processes, or embed + spawn) |
| Fits the house | no | partially | yes (internal/sni is the precedent) | no |

Prose on the ones the table flattens:

**A** is whack-a-mole. Each pin is individually testable and each will
work until the next macOS release renames a window class or changes menu
tracking; then all four pins fail at once and nobody owns the .m they
live in. It also permanently caps the design at 16 pt icons and
title-in-the-bar compromises.

**B** is the pragmatic half-step: copy `systray_darwin.m/.go` into
`internal/tray`, strip the cross-platform ID plumbing we do not need, add
the four features. It reuses battle-tested AppKit lifecycle code
(`[NSApp run]`, the pre-10.14 workaround) and keeps Windows on upstream
untouched. Its cost is quiet: a vendored fork is a promise to read
upstream diffs forever, and the fork's structure (delegate owning
everything, `runInMainThread` rendezvous) is the structure we would rip
out if we wrote it fresh.

**C** is the honest version of B. The darwin tray we actually want is
~800 lines total: one delegate class owning the status item and menu, one
scroll-catching overlay view, one slider view, and a thin Go adapter
giving `ui_menu.go`'s vocabulary (add item / checkbox / submenu / hide /
title / click channel). The repo has already done exactly this on Linux -
`internal/sni` is 590 lines of Go replacing a tray library, with the same
`menuItem` abstraction so the shared chokepoints (`a.on`, `setPlayingUI`,
`applyVolumeUI`) work unmodified (ui_sni_linux.go:62, 315). C is that
move again, on a platform where the code lives in-tree as .m files next
to `internal/nowplay` and `internal/notify` cgo that already work. Risk:
AppKit lifecycle details (retain cycles on the delegate, menu tracking
modes, LSUIElement activation policy) that systray's .m has already
solved - mitigated by reading it as the reference implementation while
writing a cleaner one.

**D** is insane for this project, and it is worth saying why precisely:
the entire value of fipindicateur is that it is one Go binary with the
telemetry chokepoint in Go and `make lint` as the only gate. D moves the
UI into a Swift process, which means IPC for every menu mutation (the
menu mutates on every metadata burst, ui.go:495-525), a second packaging
and signing story, a re-proof of every guard test, and a second language
in a stdlib-purist repo. It buys SwiftUI's polish (hover states, popovers)
that a tray menu cannot express - real, but not worth a second codebase
for a radio tray app. Revisit only if the product ever becomes a
full window UI, at which point it is a different app.

## 4) Recommendation: C - hand-roll the darwin tray, in five milestones

`internal/tray` (cgo, ARC, Cocoa framework only), with `internal/ui`
consuming it through the same tiny item abstraction Linux already uses.
Windows stays on upstream fyne/systray; `ui_menu.go` splits into a
platform-neutral builder fed by an interface, darwin and systray
implementations. Every milestone ships green `make lint` and is usable
on its own; nothing lands behind a half-finished flag.

### M1 - internal/tray core, feature parity (the big one, ~600-900 lines)

Own `NSStatusItem` + `NSMenu` + delegate: items, checkboxes, separators,
submenus, hide/disable, tooltips, click channels into Go, icon set
(template + colored, 18 pt with @2x PNG). Delegate is created on the main
thread we own from the start (`[NSApp run]` stays ours; `internal/nowplay`
already assumes an NSApplication). `placeMenuBarIcon` moves here and
names its own item. Gate: `make lint` green, the full menu tree of
ui_menu.go renders and every item fires `a.on` exactly as today. This
milestone ships zero new features on purpose - parity is the test.

### M2: scroll-wheel volume (~80 lines)

Scroll-catching NSView overlay on the status button (the
`RightClickDetector` shape), `scrollWheel:` maps deltas (precise vs
discrete, natural-scroll sign) to `App.scrollVolume(delta, orientation)`
unchanged (ui.go:1887-1904). Telemetry already records at source in
`setActiveVolume`; no new kind needed. Gate: wheel step = 5 %, trackpad
flick = smooth, direction correct under natural scrolling.

### M3: inline volume slider (~120 lines)

`NSMenuItem.view` hosting an NSSlider in the Volume submenu, target/action
through an exported Go callback, updates coalesced through `volcoalesce.go`
into `setActiveVolume` (which records at source, ui.go:1063-1077). The
slider mirrors the active sink, so it must read `activeVolume()` on open
(menu-will-open hook) not at build time. Keep the preset checkboxes under
it as the keyboard-accessible fallback. Gate: drag does not flood mpv
writes; slider position is correct while casting.

### M4: cover as the tray icon (~100 lines Go, ~20 native)

`internal/icon` gains the baked cover: rounded-rect mask, brand-color ring
(`tint.Legible`), paused dimming, @2x bitmap. `applyIconState` gains a
cover state bit that suppresses `drawFrame`; fetch reuses
`internal/notify/cover.go`. Bars stay the fallback (no cover yet, fetch
failure, "Icône animée" setting kept meaningful). Gate: no flicker on
track change, animation resumes when the cover clears.

### M5: status item title, demoted and safe (~60 lines)

Default-off "Afficher le nom de la station" (not the track title, per
2.4): fixed short labels, no per-track relayout. Only after M4, so the
width budget is measured with the cover present. Gate: with a crowded bar
on the notched MacBook, our item stays visible.

### M6: eviction and cleanup (~-200 lines)

Delete the zenity volume-slider item on darwin (`zenityAvailable()` gate),
delete `menuplace_darwin.go` in favor of the tray's own placement, delete
the systray darwin path from the darwin build (Windows keeps upstream
systray via the now-split menu builder). `CHANGELOG.md`, docs, done.

Sequencing rule: M1 must land alone and be used daily for a few days
before M2+; a tray rewrite that also changes behavior in the same commit
is unauditable. B is the sanctioned fallback if M1 exceeds ~1200 lines or
parity slips: fork the systray .m and continue with the same M2-M6.

## 5) What must NOT change

- **`App.on` and the click chokepoint.** Every clickable item still goes
  through `a.on` (ui.go:811-818); `TestSingleOnClickCallSite`,
  `TestActionKindsWired` and `TestSingleSetIconCallSite`
  (guard_test.go:20, 66, 115) keep failing the build. A custom-view slider
  or scroll surface records at source (`setActiveVolume`, `setPlayingUI`)
  exactly as volume/play do today - never a raw callback that skips the
  decision.
- **The volume/mute/play chokepoints.** `setActiveVolume`, `activeVolume`,
  `toggleActiveMute`, `setPlayingUI` remain the single writers; the new
  slider and scroll paths call them, they do not replace them. Cast-aware
  semantics (active sink resolution) carry over untouched.
- **Privacy posture.** Opt-in stats, local-only events, cover fetch
  best-effort from the existing cache, no new network path beyond what
  `internal/notify/cover.go` already does.
- **Stdlib-only Go.** C adds zero Go modules. The cgo is in-tree
  (`-framework Cocoa`, ARC on), beside `nowplay`/`notify`. GPL-3, single
  binary, `make lint` as the only CI gate, no em dashes anywhere (the .m
  comments included - they are grepped).
- **French UI copy with accents**, and the existing menu structure: the
  rethink upgrades surfaces (icon, slider, scroll), not the information
  architecture. The two-click confirms (`mStatsClear`, `mPrefsClear`)
  stay plain items, never custom views.
- **Linux parity.** `internal/sni` keeps its Scroll wiring
  (ui_sni_linux.go:315) and its menu; the shared builder refactor (if
  any) must keep the Linux build's behavior byte-identical. macOS going
  first-class must not silently fork the product's feature set.
- **The honesty invariant of the stats report** (sample size, warnings)
  applies here too: any new interaction surface lands with its telemetry
  decision written down in the same commit.

---

*Sources read for this audit: systray v1.12.2 `systray.go`,
`systray_darwin.go`, `systray_darwin.m` (module cache);
`internal/ui/ui_menu.go`, `ui.go` (chokepoints, scrollVolume),
`ui_sni_linux.go`, `menuplace_darwin.go`, `ui_dialog.go`,
`guard_test.go`, `control_test.go`; `internal/icon/{cover,tint,bars}.go`;
`internal/notify/cover.go`.*
