// The Réglages window: everything the tray menu used to nest three levels
// deep, as one native form. Like the card it holds no state of its own: a
// control shows the last Snapshot and reports an Action; Go flips the value
// and the next Snapshot brings it back.

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

struct SettingsView: View {
    let model: FipModel
    @State private var fade: Double = 0
    @State private var fading = false
    @State private var askHistory = false
    @State private var askPrefs = false

    var body: some View {
        let s = model.s.settings
        Form {
            Section("Son") {
                flip("Haute qualité", "AAC 192 kbit/s", s.hifi, .hifiToggle)
                Picker("Sortie audio", selection: pick(s.audioDevice, .audioDevice)) {
                    ForEach(s.audioDevices, id: \.name) { Text($0.label).tag($0.name) }
                }
                LabeledContent {
                    HStack(spacing: 10) {
                        Slider(value: $fade, in: 0...10, step: 1) { editing in
                            fading = editing
                            if !editing { model.send(.crossfade, String(Int(fade))) }
                        }
                        .frame(width: 150)
                        Text(fade == 0 ? "Coupé" : "\(Int(fade)) s").monospacedDigit()
                            .foregroundStyle(.secondary).frame(width: 44, alignment: .trailing)
                    }
                } label: {
                    Text("Fondu entre stations")
                }
                Picker(selection: pick(String(s.amp), .amp)) {
                    Text("Aucune").tag("0")
                    Text("Douce (+4 dB)").tag("1")
                    Text("Costaude (+8 dB)").tag("2")
                } label: {
                    Text("Amplification")
                    Text("FIP joue plus discret que les plateformes")
                }
            }
            Section("Notifications") {
                flip("Morceaux", "Une bannière à chaque nouveau titre", s.notifications, .notifToggle)
                flip("Émissions", "Prévenir quand une émission commence", s.showNotifications, .showNotifToggle)
            }
            Section("Affichage") {
                flip("Icône animée", "Des barres qui suivent le niveau audio", s.animatedIcon, .animToggle)
                flip("Calendrier dans le menu", "Les prochaines émissions de l'antenne", s.showCalendar, .showCalendarToggle)
            }
            Section("Au lancement") {
                flip("Lancer la lecture", "Sinon l'antenne attend en pause", s.playOnStart, .playOnStartToggle)
                flip("Chercher les mises à jour", "Un contrôle discret, sinon jamais", s.updateStartup, .updateStartupToggle)
                LabeledContent("le fipindicateur \(s.version)") {
                    HStack {
                        Button("Vérifier maintenant") { model.send(.updateCheck) }
                        Button("À propos") { model.send(.openAbout) }
                    }
                }
            }
            Section {
                flip("Historique d'écoute", "Titres joués et gestes, dans deux fichiers sur ce Mac", s.history, .historyToggle)
                LabeledContent("Vos données") {
                    HStack {
                        Button("Voir le rapport") { model.send(.openReport) }
                        Button("Ouvrir le dossier") { model.send(.openDataDir) }
                    }
                }
                LabeledContent("Effacer") {
                    HStack {
                        Button("L'historique…", role: .destructive) { askHistory = true }
                        Button("Mes goûts…", role: .destructive) { askPrefs = true }
                    }
                }
            } header: {
                Text("Données")
            } footer: {
                Text("Tout reste sur cette machine : rien n'est envoyé, jamais.")
            }
        }
        .formStyle(.grouped)
        // Taller than a laptop screen if left to its natural height: the
        // form scrolls instead, and the window can be pulled longer.
        .frame(minWidth: 480, maxWidth: 480, minHeight: 420, idealHeight: 660, maxHeight: .infinity)
        .onAppear { fade = Double(s.crossfade) }
        .onChange(of: s.crossfade) { _, now in if !fading { fade = Double(now) } }
        .confirmationDialog("Effacer l'historique d'écoute ?", isPresented: $askHistory) {
            Button("Effacer", role: .destructive) { model.send(.clearHistory) }
            Button("Annuler", role: .cancel) {}
        } message: {
            Text("history.jsonl et events.jsonl sont supprimés. Vos J'aime ne sont pas touchés.")
        }
        .confirmationDialog("Effacer vos goûts ?", isPresented: $askPrefs) {
            Button("Effacer", role: .destructive) { model.send(.clearPrefs) }
            Button("Annuler", role: .cancel) {}
        } message: {
            Text("prefs.jsonl (vos J'aime et Pas pour moi) est supprimé.")
        }
    }

    /// A switch that shows Go's value and asks Go to flip it.
    private func flip(_ title: String, _ note: String, _ on: Bool, _ action: Action) -> some View {
        Toggle(isOn: Binding(get: { on }, set: { _ in model.send(action) })) {
            Text(title)
            Text(note)
        }
    }

    /// A choice that shows Go's value and sends the picked one.
    private func pick(_ current: String, _ action: Action) -> Binding<String> {
        Binding(get: { current }, set: { if $0 != current { model.send(action, $0) } })
    }
}

/// One ordinary window, created on first use and kept. The app is a menu bar
/// accessory, so it has to be brought forward by hand.
final class SettingsController {
    static let shared = SettingsController()
    private var window: NSWindow?

    func show() {
        if window == nil {
            let host = NSHostingController(rootView: SettingsView(model: .shared))
            let w = NSWindow(contentViewController: host)
            w.title = "Réglages"
            w.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            w.isReleasedWhenClosed = false
            w.center()
            window = w
        }
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }
}
