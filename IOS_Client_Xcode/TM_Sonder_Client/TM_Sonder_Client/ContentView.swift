import SwiftUI
import UIKit
import SonderAPI

struct ContentView: View {
    @Environment(\.scenePhase) private var scenePhase
    @StateObject private var model = SonderClientModel()
    @StateObject private var browser = SonderServerBrowser()

    var body: some View {
        NavigationStack {
            Group {
                if model.hasConfiguredServer || model.items.isEmpty == false {
                    LibraryView(model: model)
                } else {
                    ServerOnboardingView(model: model, browser: browser)
                }
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .navigationTitle("Sonder")
            .task {
                browser.start()
                await model.loadIfNeeded()
            }
            .refreshable {
                browser.start()
                await model.reload()
            }
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active else { return }
            model.refreshOfflineDownloads()
            browser.start()
            Task { await model.reload() }
        }
        .onChange(of: browser.endpoints) { _, endpoints in
            guard let endpoint = endpoints.first(where: model.shouldAutomaticallyUseDiscoveredEndpoint) else { return }
            Task { await model.connect(to: endpoint) }
        }
        .tint(SonderPalette.accent)
    }
}

struct ServerOnboardingView: View {
    @ObservedObject var model: SonderClientModel
    @ObservedObject var browser: SonderServerBrowser
    @State private var manualURL = ""
    @State private var token = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                hero
                discoveryPanel
                manualPanel
            }
            .padding(20)
        }
        .background(SonderPalette.background)
    }

    private var hero: some View {
        VStack(alignment: .leading, spacing: 14) {
            ZStack {
                RoundedRectangle(cornerRadius: 8)
                    .fill(SonderPalette.accent)
                Image(systemName: "play.tv.fill")
                    .font(.system(size: 34, weight: .semibold))
                    .foregroundStyle(SonderPalette.darkText)
            }
            .frame(width: 66, height: 66)

            VStack(alignment: .leading, spacing: 6) {
                Text("TM Sonder")
                    .font(.system(.largeTitle, design: .rounded).weight(.bold))
                    .foregroundStyle(SonderPalette.text)
                Text("Find your media server and open your private library.")
                    .font(.title3)
                    .foregroundStyle(SonderPalette.textLight)
                    .fixedSize(horizontal: false, vertical: true)
            }

            HStack(spacing: 8) {
                CapabilityPill(title: "Movies", icon: "film")
                CapabilityPill(title: "Books", icon: "book")
                CapabilityPill(title: "Audio", icon: "headphones")
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(SonderPalette.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border))
    }

    private var discoveryPanel: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Nearby Servers", systemImage: "dot.radiowaves.left.and.right")
                    .font(.headline)
                    .foregroundStyle(SonderPalette.text)
                Spacer()
                if browser.isSearching {
                    ProgressView()
                        .controlSize(.small)
                }
                Button {
                    browser.start()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Refresh discovery")
            }

            if browser.endpoints.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Searching for Sonder on your local network")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(SonderPalette.text)
                    Text("Make sure LAN sharing is enabled on the Mac server. Tailscale and private URLs can be entered below.")
                        .font(.caption)
                        .foregroundStyle(SonderPalette.textLight)
                }
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 8))
            } else {
                ForEach(browser.endpoints) { endpoint in
                    Button {
                        Task { await model.connect(to: endpoint) }
                    } label: {
                        HStack(spacing: 12) {
                            Image(systemName: "server.rack")
                                .foregroundStyle(SonderPalette.accentStrong)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(endpoint.name)
                                    .font(.headline)
                                    .foregroundStyle(SonderPalette.text)
                                Text(endpoint.url.absoluteString)
                                    .font(.caption)
                                    .foregroundStyle(SonderPalette.textLight)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                            Spacer()
                            Image(systemName: "chevron.right")
                                .foregroundStyle(SonderPalette.textLight)
                        }
                        .padding(12)
                        .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 8))
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(16)
        .background(SonderPalette.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border))
    }

    private var manualPanel: some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("Manual or Tailscale", systemImage: "link")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)

            TextField("http://sonder.local:8797", text: $manualURL)
                .textFieldStyle(.plain)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .padding(12)
                .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border))

            SecureField("Optional token for restricted servers", text: $token)
                .textFieldStyle(.plain)
                .autocorrectionDisabled()
                .padding(12)
                .background(SonderPalette.surfaceDeep, in: RoundedRectangle(cornerRadius: 8))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border))

            Button {
                connectManualURL()
            } label: {
                Label("Connect", systemImage: "arrow.right.circle.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(URL(string: manualURL.trimmingCharacters(in: .whitespacesAndNewlines)) == nil)
        }
        .padding(16)
        .background(SonderPalette.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderPalette.border))
    }

    private func connectManualURL() {
        guard let url = URL(string: manualURL.trimmingCharacters(in: .whitespacesAndNewlines)) else { return }
        model.saveServer(baseURL: url, accessToken: token)
        Task { await model.reload() }
    }
}

struct CapabilityPill: View {
    let title: String
    let icon: String

    var body: some View {
        Label(title, systemImage: icon)
            .font(.caption.weight(.semibold))
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .foregroundStyle(SonderPalette.darkText)
            .background(SonderPalette.accentMuted, in: Capsule())
    }
}

enum SonderPalette {
    static let dustGrey = Color(red: 222 / 255, green: 219 / 255, blue: 210 / 255)
    static let ashGrey = Color(red: 176 / 255, green: 196 / 255, blue: 177 / 255)
    static let ironGrey = Color(red: 74 / 255, green: 87 / 255, blue: 89 / 255)

    static let background = Color(uiColor: .systemGroupedBackground)
    static let surface = Color(uiColor: .secondarySystemGroupedBackground)
    static let surfaceDeep = Color(uiColor: .tertiarySystemFill)
    static let border = Color(uiColor: .separator).opacity(0.3)
    static let accent = ironGrey
    static let accentStrong = ironGrey
    static let accentMuted = ashGrey.opacity(0.2)
    static let text = Color.primary
    static let textLight = Color.secondary
    static let darkText = ironGrey
}

#Preview {
    ContentView()
}
