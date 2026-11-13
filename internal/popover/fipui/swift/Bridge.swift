// The SwiftUI surface of fipindicateur, compiled to one object and linked INTO
// the Go binary through cgo (make swiftui). One process, no IPC.
//
// The whole contract between the two languages is four C symbols:
//
//   Go -> Swift   fipui_push_state(json)   one Snapshot, any thread
//   Go -> Swift   fipui_toggle(anchor)     show or close the popover
//   Swift -> Go   goFipAction(id, arg)     every user action, one entry point
//   Go -> Swift   fipui_debug_fire(id,arg) the link test's round trip
//
// Swift never holds product state and never decides anything: it renders the
// last Snapshot and reports intents. The telemetry chokepoint stays in Go
// (internal/ui/actions.go).

import AppKit
import Observation
import SwiftUI

// MARK: - State (Go is the source of truth)

struct Station: Codable, Hashable {
    var key: String
    var name: String
    var color: String
}

/// Mirror of popover.Snapshot (internal/popover/snapshot.go), field for field.
struct Snapshot: Codable {
    var playing = false
    var station = "fip"
    var stations: [Station] = []
    var title = ""
    var artist = ""
    var album = ""
    var year = 0
    var label = ""
    var show = ""
    var start: Double = 0
    var end: Double = 0
    var volume = 60
    var muted = false
    var output = ""
    var verdict = 0 // -1 pas pour moi, 0 rien, 1 j'aime
    var level: Double = 0

    var current: Station {
        stations.first { $0.key == station } ?? Station(key: "fip", name: "FIP", color: "#e2007a")
    }
}

// MARK: - Actions (the only way out of Swift)

/// Every control in the UI fires exactly one of these. The raw values are the
/// ids Go registers in internal/ui/actions.go; TestPopoverActionsRegistered
/// parses this enum and fails the build when an id has no registered handler
/// (the App.on invariant, across the language boundary). Keep it a plain
/// enum of string cases: the test reads the source, not the binary.
enum Action: String, CaseIterable {
    case playToggle = "play_toggle"
    case like, dislike
    case volume, muteToggle = "mute_toggle"
    case station, stationNext = "station_next", stationPrev = "station_prev"
    case openWiki = "open_wiki"
    case openReport = "stats_view"
}

@_silgen_name("goFipAction")
private func goFipAction(_ id: UnsafePointer<CChar>, _ arg: UnsafePointer<CChar>)

@Observable
final class FipModel {
    static let shared = FipModel()
    var s = Snapshot()

    func send(_ action: Action, _ arg: String = "") {
        action.rawValue.withCString { id in arg.withCString { a in goFipAction(id, a) } }
    }
}

// MARK: - Popover

private final class PopoverController {
    static let shared = PopoverController()
    private lazy var popover: NSPopover = {
        let p = NSPopover()
        p.behavior = .transient
        p.contentViewController = NSHostingController(rootView: PopoverView(model: .shared))
        return p
    }()

    func toggle(anchor: NSView) {
        if popover.isShown {
            popover.performClose(nil)
            return
        }
        // A menu bar agent is never the active app: without this the popover
        // opens behind the key window's focus and a click elsewhere does not
        // dismiss it.
        NSApp.activate(ignoringOtherApps: true)
        popover.show(relativeTo: anchor.bounds, of: anchor, preferredEdge: .minY)
        popover.contentViewController?.view.window?.makeKey()
    }
}

// MARK: - C surface

/// Decodes one Snapshot and hands it to the main thread. Returns 1 when the
/// JSON was a Snapshot, 0 otherwise (the previous state stays on screen).
@_cdecl("fipui_push_state")
public func fipuiPushState(_ json: UnsafePointer<CChar>) -> Int32 {
    let data = Data(bytes: json, count: strlen(json))
    guard let snap = try? JSONDecoder().decode(Snapshot.self, from: data) else {
        return 0
    }
    DispatchQueue.main.async { FipModel.shared.s = snap }
    return 1
}

/// Shows the popover under anchor (the status item's button, an NSView the
/// tray keeps alive for the life of the process), or closes it when shown.
@_cdecl("fipui_toggle")
public func fipuiToggle(_ anchor: UnsafeMutableRawPointer?) {
    guard let anchor else { return }
    let view = Unmanaged<NSView>.fromOpaque(anchor).takeUnretainedValue()
    if Thread.isMainThread {
        PopoverController.shared.toggle(anchor: view)
    } else {
        DispatchQueue.main.async { PopoverController.shared.toggle(anchor: view) }
    }
}

/// Fires an action exactly as a control would, for the Go link test. Returns
/// 0 when id is not an Action: the enum stays the only vocabulary.
@_cdecl("fipui_debug_fire")
public func fipuiDebugFire(_ id: UnsafePointer<CChar>, _ arg: UnsafePointer<CChar>) -> Int32 {
    guard let action = Action(rawValue: String(cString: id)) else { return 0 }
    FipModel.shared.send(action, String(cString: arg))
    return 1
}
