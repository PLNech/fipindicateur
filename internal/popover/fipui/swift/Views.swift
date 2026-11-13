// The popover: cover, transport, fader, station dial. Tokens are DESIGN.md's,
// declared in OKLCH like the report. Every control calls model.send and
// nothing else: the views hold no product state (see Bridge.swift).

import SwiftUI

// MARK: - Tokens

func oklch(_ l: Double, _ c: Double, _ h: Double, _ alpha: Double = 1) -> Color {
    let a = c * cos(h * .pi / 180), b = c * sin(h * .pi / 180)
    let l1 = pow(l + 0.3963377774 * a + 0.2158037573 * b, 3)
    let m1 = pow(l - 0.1055613458 * a - 0.0638541728 * b, 3)
    let s1 = pow(l - 0.0894841775 * a - 1.2914855480 * b, 3)
    func gamma(_ x: Double) -> Double {
        let v = min(max(x, 0), 1)
        return v <= 0.0031308 ? 12.92 * v : 1.055 * pow(v, 1 / 2.4) - 0.055
    }
    return Color(
        .sRGB,
        red: gamma(4.0767416621 * l1 - 3.3077115913 * m1 + 0.2309699292 * s1),
        green: gamma(-1.2684380046 * l1 + 2.6097574011 * m1 - 0.3413193965 * s1),
        blue: gamma(-0.0041960863 * l1 - 0.7034186147 * m1 + 1.7076147010 * s1),
        opacity: alpha)
}

struct Theme {
    var bg, surface, raised, ink, muted, hairline, fip, amber: Color
    var dark: Bool

    /// « Antenne de nuit »
    static let nuit = Theme(
        bg: oklch(0.16, 0.012, 50), surface: oklch(0.20, 0.015, 55), raised: oklch(0.24, 0.016, 55),
        ink: oklch(0.94, 0.025, 85), muted: oklch(0.74, 0.02, 80), hairline: oklch(0.36, 0.02, 60),
        fip: oklch(0.63, 0.23, 356), amber: oklch(0.80, 0.12, 75), dark: true)
    /// « Édition papier »
    static let papier = Theme(
        bg: oklch(0.965, 0.004, 356), surface: oklch(0.93, 0.005, 356), raised: oklch(0.99, 0.002, 356),
        ink: oklch(0.24, 0.02, 40), muted: oklch(0.45, 0.02, 40), hairline: oklch(0.80, 0.01, 40),
        fip: oklch(0.55, 0.24, 356), amber: oklch(0.62, 0.12, 70), dark: false)

    static func of(_ scheme: ColorScheme) -> Theme { scheme == .dark ? .nuit : .papier }

    /// Station brand colour, nudged toward the ink until it reads on the
    /// current background (the Swift twin of icon.Legible; the real build
    /// would receive the already-legible colour from Go).
    func station(_ hex: String) -> Color {
        var v: UInt64 = 0
        Scanner(string: hex.replacingOccurrences(of: "#", with: "")).scanHexInt64(&v)
        var r = Double((v >> 16) & 0xff) / 255, g = Double((v >> 8) & 0xff) / 255, b = Double(v & 0xff) / 255
        let target: Double = dark ? 1 : 0
        for _ in 0..<12 {
            let lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
            if dark ? lum > 0.30 : lum < 0.42 { break }
            r += (target - r) * 0.12; g += (target - g) * 0.12; b += (target - b) * 0.12
        }
        return Color(.sRGB, red: r, green: g, blue: b)
    }
}

// MARK: - Drawn glyphs (no emoji, no stock icons for the signature controls)

/// A wave whose amplitude rises (j'aime) or falls (pas pour moi).
struct Wave: Shape {
    var rising: Bool
    func path(in r: CGRect) -> Path {
        var p = Path()
        let steps = 48
        for i in 0...steps {
            let t = Double(i) / Double(steps)
            let amp = (rising ? t : 1 - t) * 0.42 + 0.06
            let y = r.midY - sin(t * .pi * 5) * amp * r.height
            let pt = CGPoint(x: r.minX + t * r.width, y: y)
            if i == 0 { p.move(to: pt) } else { p.addLine(to: pt) }
        }
        return p
    }
}

struct PlayGlyph: Shape {
    var playing: Bool
    func path(in r: CGRect) -> Path {
        var p = Path()
        if playing {
            let w = r.width * 0.28
            p.addRoundedRect(in: CGRect(x: r.minX + r.width * 0.12, y: r.minY, width: w, height: r.height), cornerSize: .init(width: 1.5, height: 1.5))
            p.addRoundedRect(in: CGRect(x: r.maxX - r.width * 0.12 - w, y: r.minY, width: w, height: r.height), cornerSize: .init(width: 1.5, height: 1.5))
        } else {
            p.move(to: CGPoint(x: r.minX + r.width * 0.18, y: r.minY))
            p.addLine(to: CGPoint(x: r.maxX, y: r.midY))
            p.addLine(to: CGPoint(x: r.minX + r.width * 0.18, y: r.maxY))
            p.closeSubpath()
        }
        return p
    }
}

/// Stand-in sleeve until the snapshot carries the fetched cover
/// (internal/notify/cover.go already caches it as a PNG).
struct MockCover: View {
    var body: some View {
        GeometryReader { g in
            let s = g.size.width
            ZStack {
                oklch(0.30, 0.07, 215)
                ForEach(0..<7) { i in
                    Circle()
                        .stroke(oklch(0.80, 0.12, 75, 0.20 + Double(i) * 0.09), lineWidth: s * 0.012)
                        .frame(width: s * (0.98 - Double(i) * 0.12))
                        .offset(x: s * 0.16, y: -s * 0.10)
                }
                Circle().fill(oklch(0.74, 0.15, 55)).frame(width: s * 0.26).offset(x: s * 0.16, y: -s * 0.10)
            }
        }
        .aspectRatio(1, contentMode: .fit)
        .clipped()
    }
}

// MARK: - Popover

struct PopoverView: View {
    let model: FipModel
    @Environment(\.colorScheme) private var scheme
    static let width: CGFloat = 340

    var body: some View {
        let t = Theme.of(scheme)
        VStack(spacing: 0) {
            Hero(model: model, t: t)
            VStack(spacing: 14) {
                Transport(model: model, t: t)
                Fader(model: model, t: t)
                Dial(model: model, t: t)
            }
            .padding(.horizontal, 16)
            .padding(.top, 14)
            Footer(model: model, t: t)
        }
        .frame(width: Self.width)
        .background(t.bg)
        .foregroundStyle(t.ink)
    }
}

/// The cover, edge to edge. The rundown line sits on top of it, the title
/// block on a scrim at its foot, and the hairline at the very bottom is the
/// track's progress (livemeta gives start and end).
private struct Hero: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        let night = Theme.nuit
        ZStack(alignment: .bottomLeading) {
            MockCover()
            LinearGradient(
                stops: [.init(color: night.bg.opacity(0.55), location: 0), .init(color: .clear, location: 0.22),
                        .init(color: .clear, location: 0.48), .init(color: night.bg.opacity(0.94), location: 1)],
                startPoint: .top, endPoint: .bottom)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 8) {
                    Text(clock(s.start)).monospacedDigit()
                    Rectangle().frame(width: 18, height: 1).opacity(0.6)
                    Text(s.show.isEmpty ? s.current.name : s.show)
                    Spacer()
                }
                .font(.system(size: 11, weight: .medium))
                .foregroundStyle(night.ink.opacity(0.86))
                Spacer()
                Text(s.title)
                    .font(.system(size: 22, weight: .heavy))
                    .tracking(-0.3)
                    .lineLimit(2)
                Text(s.artist).font(.system(size: 14, weight: .semibold))
                HStack {
                    Text([s.album, s.year > 0 ? String(s.year) : "", s.label].filter { !$0.isEmpty }.joined(separator: " · "))
                        .lineLimit(1)
                    Spacer()
                    Text(remaining(s)).monospacedDigit()
                }
                .font(.system(size: 11))
                .foregroundStyle(night.muted)
            }
            .foregroundStyle(night.ink)
            .padding(.init(top: 12, leading: 16, bottom: 14, trailing: 16))
            GeometryReader { g in
                ZStack(alignment: .leading) {
                    Rectangle().fill(night.hairline)
                    Rectangle().fill(night.ink).frame(width: g.size.width * progress(s))
                }
            }
            .frame(height: 2)
        }
        .frame(width: PopoverView.width, height: PopoverView.width)
        .contentShape(Rectangle())
        .onTapGesture { model.send(.openWiki) }
        .help("Ouvrir l'artiste sur Wikipédia")
    }

    private func clock(_ unix: Double) -> String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: Date(timeIntervalSince1970: unix))
    }

    private func progress(_ s: Snapshot) -> Double {
        guard s.end > s.start else { return 0 }
        return min(max((Date().timeIntervalSince1970 - s.start) / (s.end - s.start), 0), 1)
    }

    private func remaining(_ s: Snapshot) -> String {
        let left = Int(max(s.end - Date().timeIntervalSince1970, 0))
        return s.end > s.start ? String(format: "-%d:%02d", left / 60, left % 60) : ""
    }
}

private struct Transport: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        HStack(spacing: 0) {
            verdict(rising: false, on: s.verdict < 0, tint: t.amber.opacity(0.8), help: "Pas pour moi", .dislike)
            Spacer()
            step("chevron.left", "Station précédente", .stationPrev)
            Spacer()
            Button { model.send(.playToggle) } label: {
                ZStack {
                    Circle().fill(s.playing ? t.fip : .clear)
                    Circle().stroke(s.playing ? .clear : t.hairline, lineWidth: 1)
                    PlayGlyph(playing: s.playing)
                        .fill(s.playing ? Theme.nuit.bg : t.ink)
                        .frame(width: 15, height: 16)
                        .offset(x: s.playing ? 0 : 1.5)
                }
                .frame(width: 48, height: 48)
            }
            .buttonStyle(.plain)
            .keyboardShortcut(.space, modifiers: [])
            .help(s.playing ? "Pause" : "Lecture")
            Spacer()
            step("chevron.right", "Station suivante", .stationNext)
            Spacer()
            verdict(rising: true, on: s.verdict > 0, tint: t.fip, help: "J'aime ce morceau", .like)
        }
    }

    private func verdict(rising: Bool, on: Bool, tint: Color, help: String, _ action: Action) -> some View {
        Button { model.send(action) } label: {
            Wave(rising: rising)
                .stroke(on ? tint : t.muted, style: .init(lineWidth: on ? 2.2 : 1.5, lineCap: .round, lineJoin: .round))
                .frame(width: 30, height: 22)
                .padding(7)
                .background(on ? tint.opacity(0.14) : .clear, in: RoundedRectangle(cornerRadius: 8))
        }
        .buttonStyle(.plain)
        .help(help)
    }

    private func step(_ symbol: String, _ help: String, _ action: Action) -> some View {
        Button { model.send(action) } label: {
            Image(systemName: symbol).font(.system(size: 15, weight: .semibold)).frame(width: 32, height: 32)
        }
        .buttonStyle(.plain)
        .foregroundStyle(t.muted)
        .help(help)
    }
}

/// The volume fader. The mute button is the VU meter: two amber bars that
/// follow the level, struck out when muted.
private struct Fader: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        HStack(spacing: 12) {
            Button { model.send(.muteToggle) } label: {
                HStack(alignment: .bottom, spacing: 3) {
                    bar(s.muted ? 0.12 : 0.25 + s.level * 0.75)
                    bar(s.muted ? 0.12 : 0.18 + s.level * 0.55)
                }
                .frame(width: 14, height: 18, alignment: .bottom)
            }
            .buttonStyle(.plain)
            .help(s.muted ? "Rétablir le son" : "Couper le son")

            GeometryReader { g in
                let w = g.size.width, x = w * Double(s.volume) / 100
                ZStack(alignment: .leading) {
                    ForEach(0...10, id: \.self) { i in
                        Rectangle().fill(t.hairline)
                            .frame(width: 1, height: i % 5 == 0 ? 9 : 5)
                            .position(x: w * Double(i) / 10, y: g.size.height / 2 + 9)
                    }
                    Rectangle().fill(t.hairline).frame(height: 1)
                    Rectangle().fill(t.ink).frame(width: x, height: 1)
                    RoundedRectangle(cornerRadius: 3)
                        .fill(t.raised)
                        .overlay(RoundedRectangle(cornerRadius: 3).stroke(t.hairline, lineWidth: 1))
                        .overlay(Rectangle().fill(t.fip).frame(width: 2, height: 12))
                        .frame(width: 14, height: 22)
                        .position(x: x, y: g.size.height / 2)
                }
                .contentShape(Rectangle())
                .gesture(DragGesture(minimumDistance: 0).onChanged { v in
                    let pct = Int((min(max(v.location.x / w, 0), 1) * 100).rounded())
                    if pct != s.volume { model.send(.volume, String(pct)) }
                })
            }
            .frame(height: 30)

            Text("\(s.volume)").font(.system(size: 12)).monospacedDigit()
                .foregroundStyle(t.muted).frame(width: 24, alignment: .trailing)
        }
    }

    private func bar(_ h: Double) -> some View {
        RoundedRectangle(cornerRadius: 1).fill(t.amber).frame(width: 4, height: 18 * min(max(h, 0.1), 1))
    }
}

/// The tuner dial: the thirteen antennas on one scale, in wheel order. The
/// needle stays put, the scale slides. Identity is the label, the colour only
/// seconds it (13 stations exceed any palette).
private struct Dial: View {
    let model: FipModel
    let t: Theme
    private let pitch: CGFloat = 92

    var body: some View {
        let s = model.s
        let n = max(s.stations.count, 1)
        let cur = s.stations.firstIndex { $0.key == s.station } ?? 0
        GeometryReader { g in
            let mid = g.size.width / 2
            ZStack {
                ForEach(Array(s.stations.enumerated()), id: \.element.key) { i, st in
                    let d = ((i - cur + n + n / 2) % n) - n / 2
                    Button { model.send(.station, st.key) } label: {
                        Text(st.name)
                            .font(.system(size: d == 0 ? 17 : 12, weight: d == 0 ? .heavy : .medium))
                            .foregroundStyle(d == 0 ? t.ink : t.muted)
                            .fixedSize()
                    }
                    .buttonStyle(.plain)
                    .opacity(max(0, 1 - abs(Double(d)) * 0.42))
                    .position(x: mid + CGFloat(d) * pitch, y: 12)
                }
                ForEach(-22...22, id: \.self) { i in
                    Rectangle().fill(t.hairline)
                        .frame(width: 1, height: i % 5 == 0 ? 10 : 5)
                        .position(x: mid + CGFloat(i) * pitch / 5, y: i % 5 == 0 ? 39 : 41.5)
                }
                Rectangle().fill(t.station(s.current.color)).frame(width: 2, height: 16).position(x: mid, y: 36)
            }
            .animation(.snappy(duration: 0.28), value: cur)
        }
        .frame(height: 46)
        .mask(LinearGradient(
            stops: [.init(color: .clear, location: 0), .init(color: .black, location: 0.16),
                    .init(color: .black, location: 0.84), .init(color: .clear, location: 1)],
            startPoint: .leading, endPoint: .trailing))
    }
}

private struct Footer: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        HStack(spacing: 4) {
            if !model.s.output.isEmpty {
                Label(model.s.output, systemImage: "hifispeaker")
                    .font(.system(size: 11)).lineLimit(1)
                    .help("Sortie audio")
            }
            Spacer()
            icon("chart.bar.xaxis", "Rapport d'écoute", .openReport)
        }
        .foregroundStyle(t.muted)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .overlay(alignment: .top) { Rectangle().fill(t.hairline.opacity(0.6)).frame(height: 1) }
        .padding(.top, 10)
    }

    private func icon(_ symbol: String, _ help: String, _ action: Action) -> some View {
        Button { model.send(action) } label: {
            Image(systemName: symbol).font(.system(size: 13)).frame(width: 28, height: 24)
        }
        .buttonStyle(.plain)
        .help(help)
    }
}
