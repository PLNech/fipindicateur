// Réglages: the back of the card. Everything the tray menu used to nest three
// levels deep, in the same place and the same hand as the front; the gear
// turns the card over. Like the front it holds no product state: a control
// shows the last Snapshot and reports an Action; Go flips the value and the
// next Snapshot brings it back.

import AppKit
import SwiftUI

struct AudioDevice: Codable, Hashable {
    var name: String
    var label: String
}

/// Mirror of popover.Settings (internal/popover/snapshot.go).
struct Settings: Codable {
    var hifi = false
    var crossfade = 0
    var amp = 0
    var audioDevice = "auto"
    var audioDevices: [AudioDevice] = []
    var notifications = true
    var showNotifications = false
    var showCalendar = false
    var animatedIcon = true
    var playOnStart = false
    var history = true
    var updateStartup = false
    var version = ""

    enum CodingKeys: String, CodingKey {
        case hifi, crossfade, amp, notifications, history, version
        case audioDevice = "audio_device", audioDevices = "audio_devices"
        case showNotifications = "show_notifications", showCalendar = "show_calendar"
        case animatedIcon = "animated_icon", playOnStart = "play_on_start"
        case updateStartup = "update_startup"
    }
}

struct SettingsFace: View {
    let model: FipModel
    let t: Theme
    @State private var fade: Double = 0
    @State private var fading = false
    /// Which erase is armed, waiting for its second click ("" for none).
    @State private var armed = ""

    /// 0 Écoute, 1 Appli. The pages slide; nothing scrolls.
    private var page: Int { model.settingsPage }

    private let two = [GridItem(.flexible(), spacing: 18), GridItem(.flexible())]

    var body: some View {
        let s = model.s.settings
        let accent = t.station(model.s.current.color)
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 6) {
                Button { model.settingsUp = false } label: {
                    Image(systemName: "chevron.left").font(.system(size: 12, weight: .bold))
                        .frame(width: 22, height: 22)
                }
                .buttonStyle(PressStyle())
                .keyboardShortcut(.cancelAction)
                .help("Retour à l'antenne")
                Text("Réglages").font(.system(size: 14, weight: .heavy))
                Spacer()
                tab("Écoute", 0, accent)
                tab("Appli", 1, accent)
            }
            .padding(.bottom, 10)
            Group {
                if page == 0 { listening(s, accent) } else { app(s, accent) }
            }
            .font(.system(size: 12))
            .transition(.push(from: page == 0 ? .leading : .trailing))
            .id(page)
            Spacer(minLength: 0)
        }
        .padding(.init(top: 10, leading: 14, bottom: 10, trailing: 14))
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(t.bg)
        .foregroundStyle(t.ink)
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(t.hairline.opacity(0.7), lineWidth: 1))
        .animation(.snappy(duration: 0.28), value: page)
        .onAppear { fade = Double(s.crossfade) }
        .onChange(of: s.crossfade) { _, now in if !fading { fade = Double(now) } }
        .onChange(of: model.settingsUp) { _, _ in armed = ""; model.settingsPage = 0 }
    }

    /// What is heard.
    private func listening(_ s: Settings, _ accent: Color) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            LazyVGrid(columns: two, alignment: .leading, spacing: 9) {
                flip("Haute qualité (192k)", s.hifi, accent, .hifiToggle)
                flip("Lecture au lancement", s.playOnStart, accent, .playOnStartToggle)
            }
            HStack(spacing: 8) {
                Text("Fondu entre stations")
                Slider(value: $fade, in: 0...10, step: 1) { editing in
                    fading = editing
                    if !editing { model.send(.crossfade, String(Int(fade))) }
                }
                .controlSize(.mini).tint(accent)
                Text(fade == 0 ? "non" : "\(Int(fade)) s").monospacedDigit()
                    .foregroundStyle(t.muted).frame(width: 28, alignment: .trailing)
            }
            HStack(spacing: 6) {
                Text("Amplification")
                Spacer()
                chips([("0", "Aucune"), ("1", "Douce +4 dB"), ("2", "Costaude +8 dB")],
                      String(s.amp), accent, .amp)
            }
            if s.audioDevices.count > 1 {
                HStack(spacing: 6) {
                    Text("Sortie")
                    Spacer(minLength: 12)
                    ScrollView(.horizontal, showsIndicators: false) {
                        chips(s.audioDevices.map { ($0.name, $0.label) }, s.audioDevice, accent, .audioDevice)
                            .padding(1)
                    }
                    .defaultScrollAnchor(.trailing)
                }
            }
        }
    }

    /// What the app does around the music, and what it keeps.
    private func app(_ s: Settings, _ accent: Color) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            LazyVGrid(columns: two, alignment: .leading, spacing: 9) {
                flip("Bannière des morceaux", s.notifications, accent, .notifToggle)
                flip("Bannière des émissions", s.showNotifications, accent, .showNotifToggle)
                flip("Icône animée", s.animatedIcon, accent, .animToggle)
                flip("Mises à jour auto", s.updateStartup, accent, .updateStartupToggle)
                flip("Historique d'écoute", s.history, accent, .historyToggle)
                HStack(spacing: 6) {
                    Spacer()
                    link("Rapport") { model.send(.openReport) }
                    link("Vérifier la version") { model.send(.updateCheck) }
                }
            }
            HStack(spacing: 6) {
                Text(armed.isEmpty ? "Effacer" : "Vraiment effacer ?")
                    .foregroundStyle(armed.isEmpty ? t.ink : t.amber)
                Spacer()
                erase("L'historique", "history", .clearHistory)
                erase("Mes goûts", "prefs", .clearPrefs)
            }
            HStack(spacing: 6) {
                Text("Tout reste sur ce Mac · le fipindicateur \(s.version)")
                    .font(.system(size: 10.5)).foregroundStyle(t.muted).lineLimit(1)
                Spacer()
                link("À propos") { model.send(.openAbout) }
            }
        }
    }

    private func tab(_ title: String, _ index: Int, _ accent: Color) -> some View {
        let on = page == index
        return Button { model.settingsPage = index } label: {
            Text(title).font(.system(size: 11, weight: on ? .bold : .regular))
                .padding(.horizontal, 9).padding(.vertical, 3)
                .background(on ? accent.opacity(0.22) : t.surface, in: RoundedRectangle(cornerRadius: 9))
                .overlay(RoundedRectangle(cornerRadius: 9).strokeBorder(on ? accent : .clear, lineWidth: 1))
        }
        .buttonStyle(PressStyle())
    }

    /// A switch that shows Go's value and asks Go to flip it.
    private func flip(_ title: String, _ on: Bool, _ accent: Color, _ action: Action) -> some View {
        Button { model.send(action) } label: {
            HStack(spacing: 6) {
                Text(title).lineLimit(1)
                Spacer(minLength: 4)
                Capsule().fill(on ? accent : t.hairline)
                    .frame(width: 26, height: 15)
                    .overlay(alignment: on ? .trailing : .leading) {
                        Circle().fill(t.dark || !on ? t.ink : t.bg).frame(width: 11, height: 11).padding(2)
                    }
                    .animation(.snappy(duration: 0.18), value: on)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(PressStyle())
    }

    /// One-of-several: shows Go's value, sends the picked one.
    private func chips(_ options: [(String, String)], _ current: String, _ accent: Color,
                       _ action: Action) -> some View {
        HStack(spacing: 4) {
            ForEach(options, id: \.0) { value, label in
                let on = value == current
                Button { if !on { model.send(action, value) } } label: {
                    Text(label).lineLimit(1)
                        .font(.system(size: 10.5, weight: on ? .bold : .regular))
                        .padding(.horizontal, 7).padding(.vertical, 3)
                        .background(on ? accent.opacity(0.22) : t.surface, in: RoundedRectangle(cornerRadius: 9))
                        .overlay(RoundedRectangle(cornerRadius: 9).strokeBorder(on ? accent : .clear, lineWidth: 1))
                }
                .buttonStyle(PressStyle())
            }
        }
    }

    private func link(_ title: String, _ run: @escaping () -> Void) -> some View {
        Button(action: run) {
            Text(title).font(.system(size: 10.5, weight: .medium))
                .padding(.horizontal, 8).padding(.vertical, 3)
                .background(t.surface, in: RoundedRectangle(cornerRadius: 9))
        }
        .buttonStyle(PressStyle())
    }

    /// Erasing takes two clicks on the same button: the first arms it (and
    /// says so), the second deletes. Flipping back disarms.
    private func erase(_ title: String, _ id: String, _ action: Action) -> some View {
        let hot = armed == id
        return Button {
            if hot { model.send(action); armed = "" } else { armed = id }
        } label: {
            Text(hot ? "Oui, \(title.lowercased())" : "\(title)…")
                .font(.system(size: 10.5, weight: hot ? .bold : .medium))
                .padding(.horizontal, 8).padding(.vertical, 3)
                .background(hot ? t.amber.opacity(0.25) : t.surface, in: RoundedRectangle(cornerRadius: 9))
                .overlay(RoundedRectangle(cornerRadius: 9).strokeBorder(hot ? t.amber : .clear, lineWidth: 1))
        }
        .buttonStyle(PressStyle())
    }
}

/// Turns a face of the card about its vertical axis. Animatable on the
/// angle so each face swaps in exactly when the card is edge-on.
struct Flip: ViewModifier, Animatable {
    var angle: Double
    let back: Bool

    var animatableData: Double {
        get { angle }
        set { angle = newValue }
    }

    func body(content: Content) -> some View {
        let facing = (angle < 90) != back
        content
            .opacity(facing ? 1 : 0)
            .allowsHitTesting(facing)
            .rotation3DEffect(.degrees(back ? angle - 180 : angle), axis: (x: 0, y: 1, z: 0), perspective: 0.35)
    }
}
