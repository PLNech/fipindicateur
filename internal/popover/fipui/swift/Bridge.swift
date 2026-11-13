// The SwiftUI surface of fipindicateur, compiled to one object and linked INTO
// the Go binary through cgo (make swiftui). One process, no IPC.
//
// The whole contract between the two languages is a handful of C symbols:
//
//   Go -> Swift   fipui_push_state(json)   one Snapshot, any thread
//   Go -> Swift   fipui_set_bars(a,b,c,d)  the tray glyph's four bars, same frames
//   Go -> Swift   fipui_toggle(anchor)     show or close the card
//   Go -> Swift   fipui_close()            close it
//   Go -> Swift   fipui_show_carton(json) a banner: the carton d'antenne
//   Go -> Swift   fipui_show_settings()    turn the card to its Réglages face
//   Go -> Swift   fipui_is_shown()         is it up (Go then skips its banners)
//   Swift -> Go   goFipAction(id, arg)     every user action, one entry point
//   Go -> Swift   fipui_debug_*            the link test's round trip and renders
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
    var cover = "" // path of the cached cover PNG
    var start: Double = 0
    var end: Double = 0
    var volume = 60
    var muted = false
    var verdict = 0 // -1 pas pour moi, 0 rien, 1 j'aime
    var settings = Settings()

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
    // Réglages, the back of the card.
    case openSettings = "open_settings"
    case hifiToggle = "hifi_toggle"
    case crossfade, amp
    case audioDevice = "audio_device"
    case notifToggle = "notif_toggle"
    case showNotifToggle = "show_notif_toggle"
    case animToggle = "anim_toggle"
    case playOnStartToggle = "play_on_start_toggle"
    case historyToggle = "history_toggle"
    case updateStartupToggle = "update_startup_toggle"
    case updateCheck = "update_check"
    case clearHistory = "clear_history"
    case clearPrefs = "clear_prefs"
    case openAbout = "open_about"
}

@_silgen_name("goFipAction")
private func goFipAction(_ id: UnsafePointer<CChar>, _ arg: UnsafePointer<CChar>)

@Observable
final class FipModel {
    static let shared = FipModel()
    var s = Snapshot()
    /// The decoded cover, kept apart from the snapshot so a volume tick does
    /// not read the file again: it only changes when the path does.
    /// Which face of the card is up. View state, not product state: Go only
    /// asks for the back to be shown (the measured open_settings action).
    var settingsUp = false
    /// The page of Réglages on show (0 Écoute, 1 Appli).
    var settingsPage = 0
    private(set) var cover: NSImage?
    private var coverPath = ""

    func apply(_ snap: Snapshot) {
        if snap.cover != coverPath {
            coverPath = snap.cover
            cover = snap.cover.isEmpty ? nil : NSImage(contentsOfFile: snap.cover)
        }
        s = snap
    }

    func send(_ action: Action, _ arg: String = "") {
        action.rawValue.withCString { id in arg.withCString { a in goFipAction(id, a) } }
    }
}

/// The meter lives apart from the snapshot: it moves several times a second
/// and must only redraw the bars that read it. The four heights (0..1) are
/// the very frame Go's animator draws in the tray glyph.
@Observable
final class Meter {
    static let shared = Meter()
    var bars: [Double] = [0, 0, 0, 0]
}

// MARK: - The card (a temporary panel where notifications appear)

/// A borderless panel still has to take clicks on the first try and the
/// keyboard (space, escape) while it is up.
private final class CardPanel: NSPanel {
    override var canBecomeKey: Bool { true }
    /// Escape peels one layer: Réglages back to the antenna, then the card away.
    override func cancelOperation(_: Any?) { CardController.shared.escape() }
    override func keyDown(with e: NSEvent) {
        if !CardController.shared.handle(e) { super.keyDown(with: e) }
    }
}

/// The card is a menu in spirit: it appears top-right under the menu bar
/// (never over the status items), and any click elsewhere, escape, or a
/// second click on the icon puts it away. It is not anchored to the icon:
/// the icon only tells which screen to use.
final class CardController {
    static let shared = CardController()
    private var panel: CardPanel?
    private var monitors: [Any] = []
    var isShown: Bool { panel?.isVisible ?? false }

    /// The same fact for Go, which asks from its own goroutines: a flag
    /// behind a lock, written on the main thread by show and close.
    private static let lock = NSLock()
    private static var up = false
    static var shownAnyThread: Bool {
        get { lock.lock(); defer { lock.unlock() }; return up }
        set { lock.lock(); up = newValue; lock.unlock() }
    }

    // MARK: keys and auto-hide

    /// A card left up silently swallows every banner (Go skips them while it
    /// is shown), so it puts itself away after this long with no hover, key
    /// or drag. Réglages on show keeps it open: someone is reading.
    private static let idleLimit: TimeInterval = 8
    private var idleTimer: Timer?
    private var lastActivity = Date()

    private func bump() { lastActivity = Date() }

    private func startIdleTimer() {
        idleTimer?.invalidate()
        bump()
        idleTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            guard let self, let p = self.panel else { return }
            // Polling the pointer covers hover, drag and scroll in one test.
            if p.frame.contains(NSEvent.mouseLocation) || FipModel.shared.settingsUp { self.bump() }
            if Date().timeIntervalSince(self.lastActivity) > Self.idleLimit { self.close() }
        }
    }

    func escape() {
        if FipModel.shared.settingsUp { FipModel.shared.settingsUp = false } else { close() }
    }

    /// The card's keys, all through the same actions the controls fire:
    /// space play/pause, left and right the neighbouring stations, up and
    /// down the volume, M mute, Cmd-comma Réglages. On the Réglages face
    /// only Cmd-comma applies: the arrows belong to its slider.
    func handle(_ e: NSEvent) -> Bool {
        bump()
        let model = FipModel.shared
        let mods = e.modifierFlags.intersection([.command, .control, .option])
        let key = e.charactersIgnoringModifiers?.lowercased() ?? ""
        if mods == .command, key == "," {
            if !model.settingsUp { model.send(.openSettings) }
            return true
        }
        guard mods.isEmpty, !model.settingsUp else { return false }
        switch e.keyCode {
        case 49: model.send(.playToggle)
        case 123: model.send(.stationPrev)
        case 124: model.send(.stationNext)
        case 126: model.send(.volume, String(min(model.s.volume + 5, 100)))
        case 125: model.send(.volume, String(max(model.s.volume - 5, 0)))
        default:
            guard key == "m" else { return false }
            model.send(.muteToggle)
        }
        return true
    }

    func toggle(anchor: NSView) {
        isShown ? close() : show(on: anchor.window?.screen ?? NSScreen.main)
    }

    /// Turns the card to its Réglages face, bringing it up first if needed
    /// (the menu's Réglages… item, with the card closed).
    func showSettings() {
        if !isShown { show(on: NSScreen.main) }
        FipModel.shared.settingsUp = true
    }

    private func show(on screen: NSScreen?) {
        guard let screen else { return }
        FipModel.shared.settingsUp = false
        CartonController.shared.hide()
        Meter.shared.bars = [0, 0, 0, 0]
        let host = NSHostingView(rootView: CardView(model: .shared, meter: .shared))
        let size = host.fittingSize
        // visibleFrame excludes the menu bar: the card hangs just under it,
        // against the right edge, where the system's own banners arrive.
        let area = screen.visibleFrame
        let frame = NSRect(x: area.maxX - size.width - 12, y: area.maxY - size.height - 8,
                           width: size.width, height: size.height)
        let p = CardPanel(contentRect: frame, styleMask: [.borderless, .nonactivatingPanel],
                          backing: .buffered, defer: false)
        p.isOpaque = false
        p.backgroundColor = .clear
        p.hasShadow = true
        p.level = .statusBar
        p.collectionBehavior = [.transient, .moveToActiveSpace, .fullScreenAuxiliary]
        p.isReleasedWhenClosed = false
        p.contentView = host
        p.alphaValue = 0
        p.makeKeyAndOrderFront(nil)
        p.animator().alphaValue = 1
        panel = p
        Self.shownAnyThread = true
        startIdleTimer()

        // Clicks in other apps (global) and in our own windows outside the
        // card (local) dismiss it. A click on our status item is neither: it
        // reaches toggle, which closes.
        let downs: NSEvent.EventTypeMask = [.leftMouseDown, .rightMouseDown, .otherMouseDown]
        if let m = NSEvent.addGlobalMonitorForEvents(matching: downs, handler: { [weak self] _ in self?.close() }) {
            monitors.append(m)
        }
        if let m = NSEvent.addLocalMonitorForEvents(matching: downs, handler: { [weak self] e in
            if e.window !== self?.panel, !(e.window?.className.contains("StatusBar") ?? false) { self?.close() }
            return e
        }) {
            monitors.append(m)
        }
    }

    func close() {
        idleTimer?.invalidate()
        idleTimer = nil
        monitors.forEach(NSEvent.removeMonitor)
        monitors.removeAll()
        panel?.orderOut(nil)
        panel = nil
        Self.shownAnyThread = false
    }
}

private func onMain(_ work: @escaping () -> Void) {
    if Thread.isMainThread { work() } else { DispatchQueue.main.async(execute: work) }
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
    DispatchQueue.main.async { FipModel.shared.apply(snap) }
    return 1
}

/// Shows the card on the screen of anchor (the status item's button, an
/// NSView the tray keeps alive for the life of the process), or closes it.
@_cdecl("fipui_toggle")
public func fipuiToggle(_ anchor: UnsafeMutableRawPointer?) {
    guard let anchor else { return }
    let view = Unmanaged<NSView>.fromOpaque(anchor).takeUnretainedValue()
    onMain { CardController.shared.toggle(anchor: view) }
}

/// Puts the card away if it is up.
@_cdecl("fipui_close")
public func fipuiClose() {
    onMain { CardController.shared.close() }
}

/// Drops a carton d'antenne (one banner, JSON of popover.Carton). Ignored
/// while the card is up. A JSON that is not a Carton is dropped.
@_cdecl("fipui_show_carton")
public func fipuiShowCarton(_ json: UnsafePointer<CChar>) {
    let data = Data(bytes: json, count: strlen(json))
    guard let c = try? JSONDecoder().decode(Carton.self, from: data) else { return }
    onMain { CartonController.shared.show(c) }
}

/// Turns the card to its Réglages face (and shows it if it was closed).
@_cdecl("fipui_show_settings")
public func fipuiShowSettings() {
    onMain { CardController.shared.showSettings() }
}

/// 1 while the card is up, from any thread. Go skips the banners that would
/// land on the same corner to say what the card already shows.
@_cdecl("fipui_is_shown")
public func fipuiIsShown() -> Int32 {
    CardController.shownAnyThread ? 1 : 0
}

/// One frame of the meter: four bar heights (0..1). Dropped, without a hop
/// to the main thread, while the card is hidden: nothing reads it.
@_cdecl("fipui_set_bars")
public func fipuiSetBars(_ a: Double, _ b: Double, _ c: Double, _ d: Double) {
    guard CardController.shownAnyThread else { return }
    DispatchQueue.main.async { Meter.shared.bars = [a, b, c, d] }
}

/// Fires an action exactly as a control would, for the Go link test. Returns
/// 0 when id is not an Action: the enum stays the only vocabulary.
@_cdecl("fipui_debug_fire")
public func fipuiDebugFire(_ id: UnsafePointer<CChar>, _ arg: UnsafePointer<CChar>) -> Int32 {
    guard let action = Action(rawValue: String(cString: id)) else { return 0 }
    FipModel.shared.send(action, String(cString: arg))
    return 1
}

/// Renders a view ("card", "settings" or "settings-appli") offscreen to a PNG at 2x, in one
/// appearance, for a snapshot: the same view the app hosts, so a layout can
/// be looked at without a click. Returns 1 when the file was written.
@_cdecl("fipui_debug_render")
public func fipuiDebugRender(_ view: UnsafePointer<CChar>, _ path: UnsafePointer<CChar>, _ json: UnsafePointer<CChar>, _ dark: Int32) -> Int32 {
    _ = NSApplication.shared
    let model = FipModel()
    guard let snap = try? JSONDecoder().decode(Snapshot.self, from: Data(bytes: json, count: strlen(json)))
    else { return 0 }
    model.apply(snap)
    let meter = Meter()
    meter.bars = [0.55, 0.9, 0.7, 0.35]
    let scheme: ColorScheme = dark != 0 ? .dark : .light
    let which = String(cString: view)
    model.settingsUp = which.hasPrefix("settings")
    model.settingsPage = which == "settings-appli" ? 1 : 0
    let cartonModel = CartonModel()
    cartonModel.apply(Carton(
        kind: which == "carton-zap" ? "zap" : which == "carton-up" ? "verdict-up" : "track",
        title: which == "carton" ? snap.title : snap.current.name,
        subtitle: which == "carton" ? snap.artist : "En écoute",
        detail: which == "carton" ? "\(snap.album) (\(snap.year)) · \(snap.label)" : "",
        cover: which == "carton" ? snap.cover : "", station: snap.current.name, color: snap.current.color))
    let root: AnyView = which.hasPrefix("carton")
        ? AnyView(CartonView(model: cartonModel, seconds: 5.5))
        : AnyView(CardView(model: model, meter: meter))
    let host = NSHostingView(rootView: root.environment(\.colorScheme, scheme))
    let size = host.fittingSize
    let rect = NSRect(origin: .zero, size: size)
    host.frame = rect
    let window = NSWindow(contentRect: rect, styleMask: [.borderless], backing: .buffered, defer: false)
    window.appearance = NSAppearance(named: dark != 0 ? .darkAqua : .aqua)
    window.backgroundColor = .clear
    window.contentView = host
    host.layoutSubtreeIfNeeded()
    RunLoop.current.run(until: Date().addingTimeInterval(0.15))
    guard let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: Int(size.width * 2), pixelsHigh: Int(size.height * 2),
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)
    else { return 0 }
    rep.size = size
    host.cacheDisplay(in: host.bounds, to: rep)
    guard let png = rep.representation(using: .png, properties: [:]) else { return 0 }
    return (try? png.write(to: URL(fileURLWithPath: String(cString: path)))) != nil ? 1 : 0
}
