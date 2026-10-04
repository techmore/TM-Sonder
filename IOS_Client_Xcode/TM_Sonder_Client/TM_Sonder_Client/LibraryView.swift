import SwiftUI
import SonderAPI

/// A six-destination, app-owned bottom bar. UIKit's native iPhone tab bar moves a
/// sixth destination into "More", while Sonder needs every core offering available
/// in one tap. The switch renders only the selected destination, which also avoids
/// constructing all catalog grids during startup.
struct LibraryView: View {
    @ObservedObject var model: SonderClientModel
    @State private var destination: SonderLibraryDestination = .home
    @State private var readingKind: SonderMediaKind = .audiobook

    var body: some View {
        Group {
            switch destination {
            case .home:
                LibraryHomeTab(model: model)
            case .movies:
                MediaKindTab(title: "Movies", subtitle: "Films from your Sonder server", kind: .movie, model: model)
            case .tv:
                TVShowsTab(model: model)
            case .library:
                LibraryHubTab(model: model, kind: $readingKind)
            case .downloads:
                OfflineDownloadsTab(model: model)
            case .settings:
                ServerSettingsTab(model: model)
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            VStack(spacing: 0) {
                SonderAudiobookMiniPlayer(audio: model.audiobookPlayer, model: model)
                SonderBottomBar(selection: $destination)
            }
        }
        .tint(SonderPalette.ironGrey)
        .background(SonderPalette.background.ignoresSafeArea())
    }
}

private enum SonderLibraryDestination: String, CaseIterable, Identifiable {
    case home
    case movies
    case tv
    case library
    case downloads
    case settings

    var id: String { rawValue }

    var title: String {
        switch self {
        case .home: "Home"
        case .movies: "Movies"
        case .tv: "TV"
        case .library: "Library"
        case .downloads: "Downloads"
        case .settings: "Settings"
        }
    }

    var icon: String {
        switch self {
        case .home: "house"
        case .movies: "film"
        case .tv: "tv"
        case .library: "books.vertical"
        case .downloads: "arrow.down.circle"
        case .settings: "gearshape"
        }
    }
}

private struct SonderBottomBar: View {
    @Binding var selection: SonderLibraryDestination
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                ScrollView(.horizontal, showsIndicators: false) { buttons }
            } else {
                buttons
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 6)
        .padding(.bottom, 4)
        .background(.regularMaterial)
        .overlay(alignment: .top) {
            Rectangle().fill(SonderPalette.border.opacity(0.7)).frame(height: 1)
        }
    }

    private var buttons: some View {
        HStack(spacing: 0) {
            ForEach(SonderLibraryDestination.allCases) { destination in
                Button {
                    selection = destination
                } label: {
                    VStack(spacing: 3) {
                        Image(systemName: destination.icon)
                            .font(.system(size: 19, weight: selection == destination ? .semibold : .regular))
                            .frame(width: 38, height: 26)
                            .background(selection == destination ? SonderPalette.accentMuted : .clear, in: Capsule())
                        Text(destination.title)
                            .font(.caption2.weight(selection == destination ? .semibold : .regular))
                            .lineLimit(1)
                    }
                    .frame(minWidth: dynamicTypeSize.isAccessibilitySize ? 120 : 0, maxWidth: .infinity, minHeight: 48)
                    .foregroundStyle(selection == destination ? SonderPalette.text : SonderPalette.textLight)
                    .accessibilityLabel(destination.title)
                    .accessibilityAddTraits(selection == destination ? .isSelected : [])
                }
                .buttonStyle(.plain)
            }
        }
    }
}

private struct LibraryHubTab: View {
    @ObservedObject var model: SonderClientModel
    @Binding var kind: SonderMediaKind

    var body: some View {
        VStack(spacing: 0) {
            Picker("Library type", selection: $kind) {
                Text("Audio").tag(SonderMediaKind.audiobook)
                Text("Books").tag(SonderMediaKind.ebook)
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
            .background(SonderPalette.background)

            ReadingLibraryTab(kind: kind, model: model)
                .id(kind)
        }
        .background(SonderPalette.background.ignoresSafeArea())
    }
}

private struct OfflineDownloadsTab: View {
    @ObservedObject var model: SonderClientModel
    @State private var selectedItem: SonderMediaItem?

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 14) {
                    SectionHeader(
                        title: "Downloads",
                        subtitle: "Saved titles and active transfers on this iPhone",
                        count: model.offlineDownloadedItems.count
                    )

                    HStack {
                        Label(
                            ByteCountFormatter.string(fromByteCount: model.offlineDownloadBytes, countStyle: .file),
                            systemImage: "internaldrive"
                        )
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(SonderPalette.text)
                        Spacer()
                        Text("Saved offline")
                            .font(.caption)
                            .foregroundStyle(SonderPalette.textLight)
                    }
                    .sonderPanel()

                    if model.offlineTransferItems.isEmpty == false {
                        Text("In Progress")
                            .font(.title3.bold())
                            .foregroundStyle(SonderPalette.text)

                        ForEach(model.offlineTransferItems) { item in
                            HStack(spacing: 12) {
                                PosterPlaceholder(item: item, artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                                    .frame(width: 42, height: 62)
                                VStack(alignment: .leading, spacing: 5) {
                                    Text(item.title)
                                        .font(.subheadline.weight(.semibold))
                                        .foregroundStyle(SonderPalette.text)
                                        .lineLimit(2)
                                    OfflineTransferStatus(
                                        progress: model.downloadProgress(for: item),
                                        isPaused: model.isOfflineDownloadPaused(item)
                                    )
                                }
                                Spacer()
                                if model.isOfflineDownloadPaused(item) {
                                    Button {
                                        Task { await model.downloadForOffline(item) }
                                    } label: {
                                        Image(systemName: "play.fill")
                                    }
                                    .buttonStyle(.bordered)
                                    .accessibilityLabel("Resume \(item.title)")
                                } else {
                                    Button {
                                        model.pauseOfflineDownload(for: item)
                                    } label: {
                                        Image(systemName: "pause.fill")
                                    }
                                    .buttonStyle(.bordered)
                                    .accessibilityLabel("Pause \(item.title)")
                                }
                            }
                            .sonderPanel()
                        }
                    }

                    if model.offlineDownloadedItems.isEmpty && model.offlineTransferItems.isEmpty {
                        ContentUnavailableView(
                            "No Downloads",
                            systemImage: "arrow.down.circle",
                            description: Text("Open a movie, audiobook, PDF, or EPUB and choose Download for Offline.")
                        )
                    } else {
                        ForEach(model.offlineDownloadedItems) { item in
                            HStack(spacing: 12) {
                                PosterPlaceholder(
                                    item: item,
                                    artworkURL: model.artworkURL(for: item),
                                    accessToken: model.accessToken
                                )
                                .frame(width: 48, height: 70)

                                VStack(alignment: .leading, spacing: 4) {
                                    Text(item.title)
                                        .font(.headline)
                                        .foregroundStyle(SonderPalette.text)
                                        .lineLimit(2)
                                    Text(item.kind.label)
                                        .font(.caption)
                                        .foregroundStyle(SonderPalette.textLight)
                                    if let download = model.offlineDownloads[item.id] {
                                        Text(ByteCountFormatter.string(fromByteCount: download.byteCount, countStyle: .file))
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(SonderPalette.textLight)
                                    }
                                }
                                Spacer()
                                Button { selectedItem = item } label: {
                                    Image(systemName: item.kind == .ebook ? "book" : "play.fill")
                                        .frame(minWidth: 44, minHeight: 44)
                                }
                                .accessibilityLabel("Open \(item.title)")
                                Button(role: .destructive) {
                                    model.removeOfflineDownload(for: item)
                                } label: {
                                    Image(systemName: "trash")
                                }
                                .buttonStyle(.borderless)
                                .accessibilityLabel("Remove offline copy of \(item.title)")
                            }
                            .sonderPanel()
                        }
                    }
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .navigationTitle("Downloads")
            .sheet(item: $selectedItem) { item in
                NavigationStack { MediaDetailView(model: model, item: item) }
            }
        }
    }
}
