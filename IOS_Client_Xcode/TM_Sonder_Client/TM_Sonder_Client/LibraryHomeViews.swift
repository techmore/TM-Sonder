import SwiftUI
import SonderAPI

struct LibraryHomeTab: View {
    @ObservedObject var model: SonderClientModel
    @State private var selectedItem: SonderMediaItem?

    private var inProgressItems: [SonderMediaItem] {
        Array(model.inProgressItems.prefix(12))
    }

    private var recentlyAdded: [SonderMediaItem] {
        Array(model.items.prefix(16))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 24) {
                    HeroLibraryHeader(model: model, inProgressCount: inProgressItems.count)
                    LibrarySearchBar(model: model)

                    if model.filteredItems.isEmpty && model.isLoading == false {
                        EmptyLibraryState()
                    } else {
                        if inProgressItems.isEmpty == false {
                            MediaShelf(title: "Continue Watching", subtitle: "Pick up where you left off", items: Array(inProgressItems.prefix(12)), model: model, selectedItem: $selectedItem)
                        }

                        MediaShelf(title: "Recently Added", subtitle: "Fresh from the server", items: recentlyAdded, model: model, selectedItem: $selectedItem)

                        ForEach(SonderMediaKind.mediaTabs) { kind in
                            let allItems = model.filteredItems(for: kind)
                            let items = Array(allItems.prefix(24))
                            if items.isEmpty == false {
                                MediaShelf(title: kind.label, subtitle: shelfSubtitle(for: kind, count: allItems.count), items: items, model: model, selectedItem: $selectedItem)
                            }
                        }
                    }
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .refreshable {
                await model.reload()
            }
            .navigationTitle("Sonder")
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        Task { await model.reload() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .disabled(model.isLoading)
                }
            }
            .sheet(item: $selectedItem) { item in
                NavigationStack {
                    MediaDetailView(model: model, item: item)
                }
            }
        }
    }

    private func shelfSubtitle(for kind: SonderMediaKind, count: Int) -> String {
        switch kind {
        case .movie:
            return "\(count) films in your catalog"
        case .tvShow:
            return "Shows and episodes from your server"
        case .documentary:
            return "Docs and nonfiction video"
        case .audiobook:
            return "Listen and resume across devices"
        case .ebook:
            return "Books and documents from Sonder"
        case .all:
            return "Everything"
        }
    }
}

struct MediaKindTab: View {
    let title: String
    let subtitle: String
    let kind: SonderMediaKind
    @ObservedObject var model: SonderClientModel
    @State private var selectedItem: SonderMediaItem?

    private var items: [SonderMediaItem] {
        model.filteredItems(for: kind)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 18) {
                    SectionHeader(title: title, subtitle: subtitle, count: items.count)
                    LibrarySearchBar(model: model)

                    if items.isEmpty {
                        EmptyLibraryState()
                    } else {
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 126, maximum: 150), spacing: 16)], spacing: 18) {
                            ForEach(items) { item in
                                Button {
                                    selectedItem = item
                                } label: {
                                    MediaPosterCard(item: item, progress: model.progress(for: item), artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .refreshable {
                await model.reload()
            }
            .navigationTitle(title)
            .sheet(item: $selectedItem) { item in
                NavigationStack {
                    MediaDetailView(model: model, item: item)
                }
            }
        }
    }
}

struct HeroLibraryHeader: View {
    @ObservedObject var model: SonderClientModel
    let inProgressCount: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .top, spacing: 14) {
                ZStack {
                    RoundedRectangle(cornerRadius: 8)
                        .fill(SonderPalette.accent)
                    Image(systemName: "play.tv.fill")
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(SonderPalette.darkText)
                }
                .frame(width: 52, height: 52)

                VStack(alignment: .leading, spacing: 3) {
                    Text(model.discovery?.name ?? model.health?.app ?? "Sonder Library")
                        .font(.title.bold())
                        .foregroundStyle(SonderPalette.text)
                    Text("Your private collection, streamed from Sonder")
                        .font(.subheadline)
                        .foregroundStyle(SonderPalette.textLight)
                }
                Spacer()
            }

            HStack(spacing: 10) {
                StatPill(value: "\(model.items.count)", label: "Titles")
                StatPill(value: "\(inProgressCount)", label: "In Progress")
                if model.pendingProgressCount > 0 {
                    StatPill(value: "\(model.pendingProgressCount)", label: "Queued")
                }
                if let lastSyncedAt = model.lastSyncedAt {
                    Label(lastSyncedAt.formatted(date: .omitted, time: .shortened), systemImage: "clock")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(SonderPalette.textLight)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 7)
                        .background(SonderPalette.surfaceDeep, in: Capsule())
                }
            }
        }
        .sonderPanel()
    }
}

struct LibrarySearchBar: View {
    @ObservedObject var model: SonderClientModel

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(SonderPalette.textLight)
            TextField("Search titles, shows, studios, tags", text: $model.searchText)
                .textFieldStyle(.plain)
                .autocorrectionDisabled()
        }
        .sonderInput()
    }
}

struct SectionHeader: View {
    let title: String
    let subtitle: String
    let count: Int?

    var body: some View {
        HStack(alignment: .lastTextBaseline) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(SonderPalette.text)
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(SonderPalette.textLight)
            }
            Spacer()
            if let count {
                Text("\(count)")
                    .font(.headline.monospacedDigit())
                    .foregroundStyle(SonderPalette.textLight)
            }
        }
    }
}

struct EmptyLibraryState: View {
    var body: some View {
        ContentUnavailableView("No Titles", systemImage: "rectangle.stack", description: Text("Check the server URL or adjust the current filters."))
            .foregroundStyle(SonderPalette.text)
            .frame(maxWidth: .infinity, minHeight: 260)
    }
}

struct MediaShelf: View {
    let title: String
    let subtitle: String
    let items: [SonderMediaItem]
    @ObservedObject var model: SonderClientModel
    @Binding var selectedItem: SonderMediaItem?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .lastTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.title3.bold())
                        .foregroundStyle(SonderPalette.text)
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(SonderPalette.textLight)
                }
                Spacer()
                Text("\(items.count)")
                    .font(.caption.monospacedDigit().weight(.semibold))
                    .foregroundStyle(SonderPalette.textLight)
            }

            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: 14) {
                    ForEach(items) { item in
                        Button {
                            selectedItem = item
                        } label: {
                            MediaPosterCard(item: item, progress: model.progress(for: item), artworkURL: model.artworkURL(for: item), accessToken: model.accessToken)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.vertical, 2)
            }
        }
    }
}

struct MediaPosterCard: View {
    static let width: CGFloat = 126
    private static let coverHeight: CGFloat = 186

    let item: SonderMediaItem
    let progress: SonderProgress
    let artworkURL: URL?
    var accessToken: String = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            PosterPlaceholder(item: item, artworkURL: artworkURL, accessToken: accessToken)
                .frame(width: Self.width, height: Self.coverHeight)
                .overlay(alignment: .bottom) {
                    if progress.percent > 0 {
                        ProgressView(value: progress.percent)
                            .tint(SonderPalette.ironGrey)
                            .padding(.horizontal, 8)
                            .padding(.bottom, 8)
                    }
                }

            VStack(alignment: .leading, spacing: 2) {
                Text(item.title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(SonderPalette.text)
                    .lineLimit(2, reservesSpace: true)
                    .truncationMode(.tail)
                    .frame(width: Self.width, alignment: .topLeading)
                    .fixedSize(horizontal: false, vertical: true)
                Text(cardSubtitle)
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
                    .lineLimit(1)
                    .truncationMode(.tail)
                    .frame(width: Self.width, alignment: .topLeading)
            }
        }
        .frame(width: Self.width, alignment: .topLeading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(item.title)
        .accessibilityValue(cardSubtitle)
    }

    private var cardSubtitle: String {
        if item.kind == .tvShow {
            return item.episodeCode
        }
        if item.year > 0 {
            return "\(item.year) - \(item.format.rawValue.uppercased())"
        }
        return item.format.rawValue.uppercased()
    }
}
