// The card: sleeve and billing, transport, fader, station dial, in a wide
// low format (a large notification, not a window). Tokens are DESIGN.md's,
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

/// The sleeve shown while a track has no cover (a programme without tracks,
/// a cover still downloading, a fetch that failed): never an empty square.
struct BlankSleeve: View {
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

// MARK: - Card

struct CardView: View {
    let model: FipModel
    let meter: Meter
    @Environment(\.colorScheme) private var scheme
    static let width: CGFloat = 440
    static let sleeve: CGFloat = 124

    var body: some View {
        let t = Theme.of(scheme)
        VStack(spacing: 8) {
            HStack(alignment: .top, spacing: 14) {
                Sleeve(model: model, t: t)
                VStack(alignment: .leading, spacing: 0) {
                    Billing(model: model, t: t)
                    Spacer(minLength: 4)
                    Transport(model: model, t: t)
                }
                .frame(height: Self.sleeve)
            }
            Fader(model: model, meter: meter, t: t)
            Dial(model: model, t: t)
        }
        .padding(.init(top: 12, leading: 12, bottom: 10, trailing: 14))
        .frame(width: Self.width)
        .background(t.bg)
        .foregroundStyle(t.ink)
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(t.hairline.opacity(0.7), lineWidth: 1))
    }
}

/// The cover, square, with the track's progress as a hairline at its foot
/// (livemeta gives start and end). A click opens the artist's page.
private struct Sleeve: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        ZStack(alignment: .bottomLeading) {
            if let cover = model.cover {
                Image(nsImage: cover).resizable().aspectRatio(contentMode: .fill)
            } else {
                BlankSleeve()
            }
            // The clock ticks the hairline; the snapshot only brings the ends.
            TimelineView(.periodic(from: .now, by: 1)) { ctx in
                GeometryReader { g in
                    ZStack(alignment: .leading) {
                        Rectangle().fill(.black.opacity(0.35))
                        Rectangle().fill(.white).frame(width: g.size.width * progress(s, ctx.date))
                    }
                }
                .frame(height: 2)
            }
        }
        .frame(width: CardView.sleeve, height: CardView.sleeve)
        .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
        .contentShape(Rectangle())
        .onTapGesture { model.send(.openWiki) }
        .help("Ouvrir l'artiste sur Wikipédia")
    }

    private func progress(_ s: Snapshot, _ now: Date) -> Double {
        guard s.end > s.start else { return 0 }
        return min(max((now.timeIntervalSince1970 - s.start) / (s.end - s.start), 0), 1)
    }
}

/// The rundown line (start time, programme or antenna), then the track.
private struct Billing: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 7) {
                if s.start > 0 {
                    Text(clock(s.start)).monospacedDigit()
                    Rectangle().frame(width: 14, height: 1).opacity(0.5)
                }
                Text(s.show.isEmpty ? s.current.name : s.show).lineLimit(1)
                Spacer(minLength: 6)
                Button { model.send(.openReport) } label: {
                    Image(systemName: "chart.bar.xaxis").font(.system(size: 12)).frame(width: 22, height: 16)
                }
                .buttonStyle(.plain)
                .help("Rapport d'écoute")
            }
            .font(.system(size: 11, weight: .medium))
            .foregroundStyle(t.muted)
            .padding(.bottom, 3)
            Text(s.title.isEmpty ? s.current.name : s.title)
                .font(.system(size: 17, weight: .heavy))
                .tracking(-0.2)
                .lineLimit(2)
                .fixedSize(horizontal: false, vertical: true)
            if !s.artist.isEmpty {
                Text(s.artist).font(.system(size: 13, weight: .semibold)).lineLimit(1)
            }
            let credits = [s.album, s.year > 0 ? String(s.year) : "", s.label].filter { !$0.isEmpty }
            if !credits.isEmpty {
                Text(credits.joined(separator: " · "))
                    .font(.system(size: 11)).foregroundStyle(t.muted).lineLimit(1)
            }
        }
    }

    private func clock(_ unix: Double) -> String {
        let f = DateFormatter()
        f.dateFormat = "HH:mm"
        return f.string(from: Date(timeIntervalSince1970: unix))
    }
}

/// Pas pour moi, play, j'aime. The two waves latch: the verdict stays lit on
/// the track that received it (Go holds that state) and they are dimmed off
/// when no track is on air, since there is nothing to judge.
private struct Transport: View {
    let model: FipModel
    let t: Theme

    var body: some View {
        let s = model.s
        HStack(spacing: 10) {
            Button { model.send(.playToggle) } label: {
                ZStack {
                    Circle().fill(s.playing ? t.fip : .clear)
                    Circle().stroke(s.playing ? .clear : t.hairline, lineWidth: 1)
                    PlayGlyph(playing: s.playing)
                        .fill(s.playing ? Theme.nuit.bg : t.ink)
                        .frame(width: 12, height: 13)
                        .offset(x: s.playing ? 0 : 1.5)
                }
                .frame(width: 36, height: 36)
            }
            .buttonStyle(PressStyle())
            .keyboardShortcut(.space, modifiers: [])
            .help(s.playing ? "Pause" : "Lecture")
            Spacer()
            verdict(rising: false, on: s.verdict < 0, tint: t.amber, help: "Pas pour moi", .dislike)
            verdict(rising: true, on: s.verdict > 0, tint: t.fip, help: "J'aime ce morceau", .like)
        }
    }

    private func verdict(rising: Bool, on: Bool, tint: Color, help: String, _ action: Action) -> some View {
        let judgeable = !model.s.title.isEmpty || !model.s.artist.isEmpty
        return Button { model.send(action) } label: {
            Wave(rising: rising)
                .stroke(on ? tint : t.muted, style: .init(lineWidth: on ? 2.4 : 1.5, lineCap: .round, lineJoin: .round))
                .frame(width: 26, height: 18)
                .padding(.horizontal, 8).padding(.vertical, 7)
                .background(on ? tint.opacity(0.18) : t.surface, in: RoundedRectangle(cornerRadius: 8))
                .animation(.snappy(duration: 0.2), value: on)
        }
        .buttonStyle(PressStyle())
        .disabled(!judgeable)
        .opacity(judgeable ? 1 : 0.35)
        .help(help)
    }
}

/// The volume fader. The mute button is the VU meter: two amber bars that
/// follow the level, struck out when muted.
private struct Fader: View {
    let model: FipModel
    let meter: Meter
    let t: Theme

    var body: some View {
        let s = model.s
        HStack(spacing: 12) {
            Button { model.send(.muteToggle) } label: {
                HStack(alignment: .bottom, spacing: 3) {
                    bar(s.muted || !s.playing ? 0.12 : 0.25 + meter.level * 0.75)
                    bar(s.muted || !s.playing ? 0.12 : 0.18 + meter.level * 0.55)
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
            .frame(height: 26)

            Text("\(s.volume)").font(.system(size: 12)).monospacedDigit()
                .foregroundStyle(t.muted).frame(width: 24, alignment: .trailing)
        }
    }

    private func bar(_ h: Double) -> some View {
        RoundedRectangle(cornerRadius: 1).fill(t.amber).frame(width: 4, height: 18 * min(max(h, 0.1), 1))
    }
}

/// The tuner dial: the thirteen antennas on one scale, in wheel order, with
/// the previous / next steps at its ends. The needle stays put, the scale
/// slides. It can be dragged and thrown: the throw is deterministic (release
/// speed decides how many stations go by, a hard one makes a full turn), so
/// the hand learns it. Identity is the label, the colour only seconds it
/// (13 stations exceed any palette).
private struct Dial: View {
    let model: FipModel
    let t: Theme
    /// How far a throw carries, per point of predicted travel.
    private static let carry: CGFloat = 1.7

    /// The scale's position in stations, unbounded (it keeps turning the same
    /// way round after round); the tuned station is pos modulo the count.
    @State private var pos: Double = 0
    @State private var grabbed: Double?

    var body: some View {
        let s = model.s
        let n = max(s.stations.count, 1)
        let cur = s.stations.firstIndex { $0.key == s.station } ?? 0
        let scale = DialScale(s.stations)
        HStack(spacing: 2) {
            step("chevron.left", "Station précédente", .stationPrev)
            DialFace(pos: pos, stations: s.stations, scale: scale, needle: t.station(s.current.color), t: t) { i in
                tune(to: nearest(i, n), stations: s.stations)
            }
            .contentShape(Rectangle())
            .simultaneousGesture(DragGesture(minimumDistance: 4)
                .onChanged { v in
                    if grabbed == nil { grabbed = pos }
                    pos = scale.pos(scale.x(grabbed ?? pos) - v.translation.width)
                }
                .onEnded { v in
                    let from = grabbed ?? pos
                    grabbed = nil
                    let landing = scale.pos(scale.x(from) - v.predictedEndTranslation.width * Self.carry)
                    tune(to: landing.rounded(), stations: s.stations)
                })
            step("chevron.right", "Station suivante", .stationNext)
        }
        .onAppear { pos = Double(cur) }
        // Go moved the station (arrows, media keys, the menu, our own throw
        // once it lands): follow by the shortest way round.
        .onChange(of: cur) { _, now in
            guard grabbed == nil else { return }
            let target = nearest(now, n)
            if target != pos { withAnimation(.snappy(duration: 0.3)) { pos = target } }
        }
    }

    /// The position equivalent to station i that is closest to where the
    /// scale is now.
    private func nearest(_ i: Int, _ n: Int) -> Double {
        let turns = ((pos - Double(i)) / Double(n)).rounded()
        return Double(i) + turns * Double(n)
    }

    /// Lets the scale coast to target and tunes the station it lands on. The
    /// longer the way, the longer the coast: a full turn is seen going by.
    private func tune(to target: Double, stations: [Station]) {
        guard !stations.isEmpty else { return }
        let n = stations.count
        let way = abs(target - pos)
        withAnimation(.timingCurve(0.16, 0.84, 0.3, 1, duration: min(0.3 + way * 0.11, 1.6))) { pos = target }
        let i = ((Int(target) % n) + n) % n
        if stations[i].key != model.s.station { model.send(.station, stations[i].key) }
    }

    private func step(_ symbol: String, _ help: String, _ action: Action) -> some View {
        Button { model.send(action) } label: {
            Image(systemName: symbol).font(.system(size: 13, weight: .semibold))
                .foregroundStyle(t.muted).frame(width: 24, height: 40)
        }
        .buttonStyle(PressStyle())
        .help(help)
    }
}

/// Where each station sits on the scale, in points. Names differ a lot in
/// length ("FIP", "Sacré français"), so evenly spaced stations collide: each
/// one takes the room its name needs at its largest (under the needle) plus
/// a gap.
struct DialScale {
    /// Centre of station i, plus one closing entry: the length of a turn.
    let centers: [Double]
    private static let gap = 26.0

    init(_ stations: [Station]) {
        let font = NSFont.systemFont(ofSize: 16, weight: .heavy)
        let w = stations.map { Double(($0.name as NSString).size(withAttributes: [.font: font]).width) }
        var c = [0.0]
        for i in w.indices { c.append(c[i] + (w[i] + w[(i + 1) % w.count]) / 2 + Self.gap) }
        centers = c
    }

    var n: Int { centers.count - 1 }
    var length: Double { centers[n] }

    /// Points along the scale for a position in stations (unbounded both ways).
    func x(_ pos: Double) -> Double {
        guard n > 0 else { return 0 }
        let turns = (pos / Double(n)).rounded(.down)
        let p = pos - turns * Double(n)
        let i = min(Int(p), n - 1)
        return turns * length + centers[i] + (p - Double(i)) * (centers[i + 1] - centers[i])
    }

    /// The inverse: the position in stations under a point of the scale.
    func pos(_ x: Double) -> Double {
        guard n > 0 else { return 0 }
        let turns = (x / length).rounded(.down)
        let r = x - turns * length
        var i = 0
        while i < n - 1, centers[i + 1] <= r { i += 1 }
        return turns * Double(n) + Double(i) + (r - centers[i]) / (centers[i + 1] - centers[i])
    }

    /// Signed distance in points from the needle (at here) to a point of the
    /// scale, by the shortest way round.
    func offset(_ at: Double, from here: Double) -> Double {
        guard length > 0 else { return 0 }
        let d = (at - here + length / 2).truncatingRemainder(dividingBy: length)
        return (d < 0 ? d + length : d) - length / 2
    }
}

/// The sliding scale itself. Animatable on pos, so a coast is drawn frame by
/// frame as a real scroll instead of each label jumping to its new place.
private struct DialFace: View, Animatable {
    var pos: Double
    let stations: [Station]
    let scale: DialScale
    let needle: Color
    let t: Theme
    let pick: (Int) -> Void

    var animatableData: Double {
        get { pos }
        set { pos = newValue }
    }

    var body: some View {
        let here = scale.x(pos)
        GeometryReader { g in
            let mid = g.size.width / 2
            ZStack {
                ForEach(Array(stations.enumerated()), id: \.element.key) { i, st in
                    let off = scale.offset(scale.centers[i], from: here)
                    let near = max(0, 1 - abs(off) / 48) // 1 under the needle
                    Text(st.name)
                        .font(.system(size: 12 + 4 * near, weight: near > 0.5 ? .heavy : .medium))
                        .foregroundStyle(near > 0.5 ? t.ink : t.muted)
                        .fixedSize()
                        .opacity(max(0, 1 - abs(off) / 190))
                        .position(x: mid + off, y: 12)
                        .onTapGesture { pick(i) }
                }
                // A long graduation under each station, four short ones on
                // the way to the next.
                ForEach(0..<(scale.n * 5), id: \.self) { k in
                    let i = k / 5
                    let at = scale.centers[i] + Double(k % 5) / 5 * (scale.centers[i + 1] - scale.centers[i])
                    Rectangle().fill(t.hairline)
                        .frame(width: 1, height: k % 5 == 0 ? 9 : 5)
                        .position(x: mid + scale.offset(at, from: here), y: k % 5 == 0 ? 33 : 35)
                }
                Rectangle().fill(needle).frame(width: 2, height: 14).position(x: mid, y: 31)
            }
        }
        .frame(height: 40)
        .mask(LinearGradient(
            stops: [.init(color: .clear, location: 0), .init(color: .black, location: 0.16),
                    .init(color: .black, location: 0.84), .init(color: .clear, location: 1)],
            startPoint: .leading, endPoint: .trailing))
    }
}

/// Pressed feedback for the drawn controls: a plain button style gives none,
/// and a control that does not answer the finger reads as broken.
struct PressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.88 : 1)
            .opacity(configuration.isPressed ? 0.6 : 1)
            .animation(.snappy(duration: 0.12), value: configuration.isPressed)
    }
}
