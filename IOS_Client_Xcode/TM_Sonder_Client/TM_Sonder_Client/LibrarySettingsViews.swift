import SwiftUI
import SonderAPI

struct ServerSettingsTab: View {
    @ObservedObject var model: SonderClientModel
    @State private var publicText: String
    @State private var localText: String
    @State private var tokenText = ""
    @State private var addressError: String?
    @State private var showsSignIn = false

    init(model: SonderClientModel) {
        self.model = model
        _publicText = State(initialValue: model.addresses.publicURL?.absoluteString ?? "")
        _localText = State(initialValue: model.addresses.localURL?.absoluteString ?? "")
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                    SectionHeader(title: "Server", subtitle: "Connection, discovery, and sync", count: nil)
                    connectionCard
                    accountCard
                    manualConnectionCard
                    DisclosureGroup("Offline storage") { offlineDownloadsCard }
                        .sonderPanel()
                    DisclosureGroup("Server information") { discoveryCard }
                        .sonderPanel()
                    DisclosureGroup("Diagnostics") { eventLogCard }
                        .sonderPanel()
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .refreshable {
                await model.reload()
            }
            .navigationTitle("Settings")
            .sheet(isPresented: $showsSignIn) {
                ServerSignInView(model: model) { showsSignIn = false }
            }
            .onChange(of: model.needsSignIn) { _, needsSignIn in
                if needsSignIn == false { showsSignIn = false }
            }
    }

    private var connectionCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Image(systemName: connectionIconName)
                    .font(.title3.weight(.semibold))
                    .foregroundStyle(SonderPalette.ironGrey)
                    .frame(width: 34, height: 34)
                    .background(SonderPalette.accentMuted, in: RoundedRectangle(cornerRadius: 8))
                VStack(alignment: .leading, spacing: 2) {
                    Text(model.discovery?.name ?? model.health?.app ?? "Sonder Server")
                        .font(.headline)
                        .foregroundStyle(SonderPalette.text)
                    Text(model.connectionMode.label)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(SonderPalette.textLight)
                }
                Spacer()
            }

            HStack(spacing: 8) {
                Image(systemName: model.activeRoute == .local ? "house.fill" : "globe")
                    .foregroundStyle(SonderPalette.ironGrey)
                VStack(alignment: .leading, spacing: 1) {
                    Text(model.isSelectingRoute ? "Checking local address…" : "Active: \(model.activeRoute.label)")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(SonderPalette.text)
                        .accessibilityIdentifier("server.activeRoute")
                    Text(model.activeBaseURL.absoluteString)
                        .font(.caption)
                        .foregroundStyle(SonderPalette.textLight)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                Spacer()
                if model.addresses.localURL != nil {
                    Button {
                        Task {
                            await model.selectRoute()
                            await model.reload()
                        }
                    } label: {
                        Image(systemName: "arrow.triangle.2.circlepath")
                    }
                    .buttonStyle(.borderless)
                    .disabled(model.isSelectingRoute || model.isLoading)
                    .accessibilityLabel("Check the local address again")
                }
            }

            if model.pendingProgressCount > 0 {
                Label(
                    "\(model.pendingProgressCount) offline progress update\(model.pendingProgressCount == 1 ? "" : "s") will sync on the next successful refresh",
                    systemImage: "arrow.triangle.2.circlepath"
                )
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
                .fixedSize(horizontal: false, vertical: true)

                Button {
                    Task {
                        await model.flushPendingProgress()
                        await model.reload()
                    }
                } label: {
                    Label("Sync Offline Progress", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .tint(SonderPalette.ashGrey)
            }

            if let errorMessage = model.errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.ironGrey)
            }
        }
        .sonderPanel()
    }

    private var accountCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Account")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)
            if let username = model.signedInUsername, model.needsSignIn == false, model.accessToken.isEmpty == false {
                Label("Signed in as \(username)", systemImage: "person.crop.circle.badge.checkmark")
                    .font(.subheadline)
                    .foregroundStyle(SonderPalette.text)
                Button(role: .destructive) {
                    Task { await model.signOut() }
                } label: {
                    Label("Sign Out", systemImage: "rectangle.portrait.and.arrow.right")
                }
                .buttonStyle(.bordered)
            } else {
                Text(model.needsSignIn ? "This server needs you to sign in." : "Sign in with your Sonder account.")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
                Button {
                    showsSignIn = true
                } label: {
                    Label("Sign In", systemImage: "person.crop.circle")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(SonderPalette.ironGrey)
                .accessibilityIdentifier("settings.signIn")
            }
        }
        .sonderPanel()
    }

    private var manualConnectionCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Server addresses")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)

            ServerAddressFields(publicText: $publicText, localText: $localText, pairingToken: $tokenText)

            if let addressError {
                Label(addressError, systemImage: "exclamationmark.triangle")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.ironGrey)
            }

            Button {
                switch ServerAddressFields.addresses(publicText: publicText, localText: localText) {
                case let .failure(error):
                    addressError = error.message
                case let .success(addresses):
                    addressError = nil
                    let pairing = tokenText.trimmingCharacters(in: .whitespacesAndNewlines)
                    if model.saveServer(addresses: addresses, pairingToken: pairing.isEmpty ? nil : pairing) {
                        tokenText = ""
                        Task { await model.reload() }
                    }
                }
            } label: {
                Label("Save and Sync", systemImage: "arrow.triangle.2.circlepath")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(SonderPalette.ashGrey)
        }
        .sonderPanel()
    }

    private var discoveryCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Discovery")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)

            if let discovery = model.discovery {
                MetadataRow(label: "Mode", value: model.connectionMode.label)
                MetadataRow(
                    label: "Pairing",
                    value: (discovery.requiresPairing == true || model.serverSettings?.isPairingRequired == true) ? "Required" : "Open"
                )
                if let preferredURLString = discovery.preferredURLString {
                    MetadataRow(label: "Suggested URL", value: preferredURLString)
                }
                if discovery.tailscaleHint.isEmpty == false {
                    MetadataRow(label: "Tailscale", value: discovery.tailscaleHint)
                }
            } else {
                Text("No discovery document has been loaded yet. Bonjour can still find nearby servers; QR/link pairing can use this same connection shape later.")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .sonderPanel()
    }

    private var offlineDownloadsCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("On This iPhone")
                    .font(.headline)
                    .foregroundStyle(SonderPalette.text)
                Spacer()
                Text(ByteCountFormatter.string(fromByteCount: model.offlineDownloadBytes, countStyle: .file))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderPalette.textLight)
            }

            if model.offlineDownloadedItems.isEmpty {
                Text("Download a playable title or PDF from its detail page to keep a complete copy for offline use.")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
                    .fixedSize(horizontal: false, vertical: true)
            } else {
                ForEach(model.offlineDownloadedItems) { item in
                    HStack(spacing: 10) {
                        Image(systemName: item.kind == .ebook ? "doc" : "arrow.down.circle.fill")
                            .foregroundStyle(SonderPalette.ironGrey)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.title)
                                .font(.subheadline.weight(.semibold))
                                .foregroundStyle(SonderPalette.text)
                                .lineLimit(1)
                            if let download = model.offlineDownloads[item.id] {
                                Text(ByteCountFormatter.string(fromByteCount: download.byteCount, countStyle: .file))
                                    .font(.caption)
                                    .foregroundStyle(SonderPalette.textLight)
                            }
                        }
                        Spacer()
                        Button(role: .destructive) {
                            model.removeOfflineDownload(for: item)
                        } label: {
                            Image(systemName: "trash")
                        }
                        .buttonStyle(.borderless)
                        .accessibilityLabel("Remove offline copy of \(item.title)")
                    }
                    if item.id != model.offlineDownloadedItems.last?.id { Divider() }
                }
            }
        }
        .sonderPanel()
    }

    private var eventLogCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Recent Events")
                .font(.headline)
                .foregroundStyle(SonderPalette.text)

            if model.events.isEmpty {
                Text("Playback and sync errors will appear here.")
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
            } else {
                ForEach(model.events.prefix(12)) { event in
                    VStack(alignment: .leading, spacing: 3) {
                        HStack {
                            Text(event.title)
                                .font(.subheadline.weight(.semibold))
                                .foregroundStyle(SonderPalette.text)
                            Spacer()
                            Text(event.date, style: .time)
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(SonderPalette.textLight)
                        }
                        Text(event.detail)
                            .font(.caption)
                            .foregroundStyle(SonderPalette.textLight)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Divider()
                }
            }
        }
        .sonderPanel()
    }

    private var connectionIconName: String {
        switch model.connectionMode {
        case .local, .lan, .tailscale, .manual:
            return "checkmark.circle"
        case .stale:
            return "clock.badge.exclamationmark"
        case .offline:
            return "wifi.exclamationmark"
        case .unauthorized:
            return "lock.trianglebadge.exclamationmark"
        }
    }
}
