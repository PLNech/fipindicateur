// The carton d'antenne: the card's small sibling. It stands in for the system
// banner (track, show, zap, verdict) in the card build: a little card under
// the menu bar, same corner and same look as the card, gone by itself after a
// few seconds. It never takes the keyboard (a space typed in your editor must
// not pause the music), ignores the pointer, and gives way to the card.
//
// It is fed whole by Go, one Carton per banner: no snapshot, no network, so it
// cannot show the previous track.

import AppKit
import Observation
import SwiftUI

struct Carton: Codable {
    var kind = "track" // track, show, zap, verdict-up, verdict-down
    var title = ""
    var subtitle = ""
    var detail = ""
    var cover = ""
    var station = ""
    var color = "#e2007a"
}

@Observable
final class CartonModel {
    var c = Carton()
    var image: NSImage?
    /// Bumped on every show: restarts the countdown line and the entrance.
    var shows = 0

    func apply(_ next: Carton) {
        c = next
        image = next.cover.isEmpty ? nil : NSImage(contentsOfFile: next.cover)
        shows += 1
    }
}

struct CartonView: View {
    let model: CartonModel
    let seconds: Double
    @Environment(\.colorScheme) private var scheme
    @State private var run = false

    var body: some View {
        let t = Theme.of(scheme), c = model.c
        let accent = t.station(c.color)
        VStack(spacing: 0) {
            HStack(spacing: 12) {
                tile(t, accent)
                    .frame(width: 52, height: 52)
                    .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                VStack(alignment: .leading, spacing: 2) {
                    Text(c.title).font(.system(size: 14, weight: .heavy)).lineLimit(1)
                    if !c.subtitle.isEmpty {
                        Text(c.subtitle).font(.system(size: 12.5, weight: .semibold)).lineLimit(1)
                    }
                    if !c.detail.isEmpty {
                        Text(c.detail).font(.system(size: 11)).foregroundStyle(t.muted).lineLimit(1)
                    }
                    if c.station != c.title {
                        HStack(spacing: 5) {
                            Circle().fill(accent).frame(width: 6, height: 6)
                            Text(c.station).font(.system(size: 10.5, weight: .medium)).foregroundStyle(t.muted)
                        }
                        .padding(.top, 1)
                    }
                }
                .frame(maxHeight: .infinity, alignment: .center)
                Spacer(minLength: 0)
            }
            // One height for every carton (room for four lines): the panel is
            // sized once from this, so a later, taller carton is never clipped.
            .frame(height: 66)
            .padding(.init(top: 10, leading: 10, bottom: 10, trailing: 14))
            // The countdown: the station's colour drains out over the time on show.
            GeometryReader { g in
                Rectangle().fill(accent)
                    .frame(width: run ? 0 : g.size.width, height: 2)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(height: 2)
        }
        .frame(width: 340)
        .background(t.bg)
        .foregroundStyle(t.ink)
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(t.hairline.opacity(0.7), lineWidth: 1))
        .id(model.shows)
        .onAppear { start() }
        .onChange(of: model.shows) { _, _ in start() }
    }

    private func start() {
        run = false
        DispatchQueue.main.async { withAnimation(.linear(duration: seconds)) { run = true } }
    }

    /// The cover when there is one, else a tile in the station's colour with
    /// the banner's own glyph.
    @ViewBuilder
    private func tile(_ t: Theme, _ accent: Color) -> some View {
        if let img = model.image {
            Image(nsImage: img).resizable().aspectRatio(contentMode: .fill)
        } else {
            ZStack {
                accent.opacity(0.22)
                switch model.c.kind {
                case "verdict-up":
                    Wave(rising: true).stroke(accent, style: .init(lineWidth: 2.2, lineCap: .round, lineJoin: .round))
                        .frame(width: 30, height: 20)
                case "verdict-down":
                    Wave(rising: false).stroke(t.amber, style: .init(lineWidth: 2.2, lineCap: .round, lineJoin: .round))
                        .frame(width: 30, height: 20)
                case "show":
                    Image(systemName: "mic.fill").font(.system(size: 20, weight: .semibold)).foregroundStyle(accent)
                default:
                    Image(systemName: "dot.radiowaves.left.and.right").font(.system(size: 20, weight: .semibold))
                        .foregroundStyle(accent)
                }
            }
        }
    }
}

/// A borderless panel that can never become key or main: it is a toast, not
/// a window.
private final class CartonPanel: NSPanel {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class CartonController {
    static let shared = CartonController()
    static let seconds: Double = 5.5
    let model = CartonModel()
    private var panel: CartonPanel?
    private var timer: Timer?

    func show(_ c: Carton) {
        // The card says it all, and sits in the same corner.
        guard !CardController.shared.isShown, let screen = NSScreen.main else { return }
        model.apply(c)
        let p = panel ?? makePanel()
        panel = p
        guard let host = p.contentView else { return }
        let size = host.fittingSize
        let area = screen.visibleFrame
        let frame = NSRect(x: area.maxX - size.width - 12, y: area.maxY - size.height - 8,
                           width: size.width, height: size.height)
        let first = !p.isVisible
        p.setFrame(frame, display: true)
        if first {
            // Drops in from just under the menu bar.
            p.setFrame(frame.offsetBy(dx: 0, dy: 10), display: false)
            p.alphaValue = 0
            p.orderFrontRegardless()
            NSAnimationContext.runAnimationGroup { ctx in
                ctx.duration = 0.28
                ctx.timingFunction = CAMediaTimingFunction(name: .easeOut)
                p.animator().setFrame(frame, display: true)
                p.animator().alphaValue = 1
            }
        } else {
            // A carton arriving mid-fade must come back to full strength.
            NSAnimationContext.runAnimationGroup { ctx in
                ctx.duration = 0.12
                p.animator().alphaValue = 1
            }
        }
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: Self.seconds, repeats: false) { [weak self] _ in self?.hide() }
    }

    func hide() {
        timer?.invalidate()
        timer = nil
        guard let p = panel, p.isVisible else { return }
        NSAnimationContext.runAnimationGroup({ ctx in
            ctx.duration = 0.3
            p.animator().alphaValue = 0
        }, completionHandler: { [weak self] in
            // A new carton may have arrived during the fade.
            if self?.timer == nil { p.orderOut(nil) }
        })
    }

    private func makePanel() -> CartonPanel {
        let host = NSHostingView(rootView: CartonView(model: model, seconds: Self.seconds))
        let p = CartonPanel(contentRect: NSRect(origin: .zero, size: host.fittingSize),
                            styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        p.isOpaque = false
        p.backgroundColor = .clear
        p.hasShadow = true
        p.level = .statusBar
        p.ignoresMouseEvents = true
        p.collectionBehavior = [.transient, .moveToActiveSpace, .fullScreenAuxiliary]
        p.isReleasedWhenClosed = false
        p.contentView = host
        return p
    }
}
