import AppKit
import AVFoundation
import Combine
import CryptoKit
import SwiftUI
import UniformTypeIdentifiers

struct ContentView: View {
    @EnvironmentObject private var library: SonderLibrary
    @State private var selection: SonderSection? = .library
    @State private var selectedItemID: UUID?
    @State private var searchText = ""
    @State private var selectedKind: SonderMediaKind?
    @State private var selectedTag: String?
    @State private var showingImporter = false
    @State private var showingStoragePicker = false

    private var filteredItems: [SonderMediaItem] {
        library.search(searchText, kind: selectedKind, tag: selectedTag)
    }

    var body: some View {
        NavigationSplitView {
            Sidebar(selection: $selection, library: library)
                .navigationSplitViewColumnWidth(min: 220, ideal: 240, max: 280)
        } detail: {
            contentView
        }
        .tint(SonderTheme.accent)
        .foregroundStyle(SonderTheme.text)
        .frame(minWidth: 1040, idealWidth: 1240, minHeight: 820, idealHeight: 920)
        .fileImporter(
            isPresented: $showingImporter,
            allowedContentTypes: SonderMediaFormat.importTypes,
            allowsMultipleSelection: true
        ) { result in
            library.importFiles(result)
        }
        .fileImporter(
            isPresented: $showingStoragePicker,
            allowedContentTypes: [.folder],
            allowsMultipleSelection: false
        ) { result in
            library.setStorageFolder(result)
        }
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                Button {
                    NSWorkspace.shared.open(URL(string: "http://127.0.0.1:\(library.serverSettings.port)")!)
                } label: {
                    Label("Web", systemImage: "network")
                }

                Button {
                    showingImporter = true
                } label: {
                    Label("Import", systemImage: "square.and.arrow.down")
                }

                Button {
                    showingStoragePicker = true
                } label: {
                    Label("Storage", systemImage: "externaldrive")
                }

                Button {
                    library.seedDemoLibrary()
                } label: {
                    Label("Seed", systemImage: "sparkles.tv")
                }
            }
        }
    }

    @ViewBuilder
    private var contentView: some View {
        switch selection ?? .library {
        case .library:
            LibraryView(
                items: filteredItems,
                allTags: library.allTags,
                searchText: $searchText,
                selectedKind: $selectedKind,
                selectedTag: $selectedTag,
                selectedItemID: $selectedItemID,
                library: library
            )
        case .tvShows:
            TVShowsView(library: library, selectedItemID: $selectedItemID)
        case .audiobooks:
            AudiobooksView(library: library, selectedItemID: $selectedItemID)
        case .ebooks:
            BooksView(library: library, selectedItemID: $selectedItemID)
        case .continueWatching:
            ContinueWatchingView(library: library, selectedItemID: $selectedItemID)
        case .collections:
            CollectionsView(library: library, selectedItemID: $selectedItemID)
        case .server:
            ServerDashboard(library: library)
        case .about:
            AboutSonderView()
        }
    }
}

struct Sidebar: View {
    @Binding var selection: SonderSection?
    @ObservedObject var library: SonderLibrary

    var body: some View {
        List(selection: $selection) {
            Label("Library", systemImage: "rectangle.stack")
                .tag(SonderSection.library)
            Label("TV Shows", systemImage: "tv")
                .tag(SonderSection.tvShows)
            Label("Audiobooks", systemImage: "headphones")
                .tag(SonderSection.audiobooks)
            Label("Books", systemImage: "book")
                .tag(SonderSection.ebooks)
            Label("Continue Watching", systemImage: "play.rectangle")
                .tag(SonderSection.continueWatching)
            Label("Collections", systemImage: "folder")
                .tag(SonderSection.collections)
            Label("Server", systemImage: "chart.bar.xaxis")
                .tag(SonderSection.server)
            Label("About", systemImage: "info.circle")
                .tag(SonderSection.about)

            Section("Sonder") {
                StatRow(label: "Titles", value: "\(library.items.count)", icon: "film")
                StatRow(label: "Playable", value: "\(library.playableCount)", icon: "play.circle")
                StatRow(label: "In Progress", value: "\(library.inProgressItems.count)", icon: "clock")
            }
        }
        .navigationTitle("TM Sonder")
        .scrollContentBackground(.hidden)
        .background(SonderTheme.sidebar)
    }
}

struct LibraryView: View {
    let items: [SonderMediaItem]
    let allTags: [String]
    @Binding var searchText: String
    @Binding var selectedKind: SonderMediaKind?
    @Binding var selectedTag: String?
    @Binding var selectedItemID: UUID?
    @ObservedObject var library: SonderLibrary
    @State private var cardMinimumWidth = 230.0

    private var posterHeight: CGFloat {
        CGFloat(cardMinimumWidth * 1.44)
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                catalogHeader
                filterBar

                LazyVGrid(columns: [GridItem(.adaptive(minimum: cardMinimumWidth, maximum: cardMinimumWidth + 28), spacing: 18)], spacing: 18) {
                    ForEach(items) { item in
                        MediaCard(item: item, progress: library.progress(for: item), posterHeight: posterHeight) {
                            selectedItemID = item.id
                        } play: {
                            library.play(item)
                        }
                    }
                }
            }
            .padding(20)
        }
        .background(SonderTheme.background)
        .navigationTitle("Library")
    }

    private var catalogHeader: some View {
        HStack(alignment: .top, spacing: 18) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Sonder Library")
                    .font(.largeTitle.weight(.bold))
                    .foregroundStyle(SonderTheme.text)
        Text("Local movies, shows, documentaries, books, and audiobooks streamed from this Mac")
                    .foregroundStyle(SonderTheme.textLight)
            }

            Spacer(minLength: 18)

            densityControl
                .frame(minWidth: 280, idealWidth: 360, maxWidth: 430)
        }
    }

    private var filterBar: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                Image(systemName: "magnifyingglass")
                    .foregroundStyle(SonderTheme.textLight)
                TextField("Search title, studio, season, genre, or tag", text: $searchText)
                    .textFieldStyle(.plain)
            }
            .padding(12)
            .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))

            HStack {
                Picker("Kind", selection: Binding(
                    get: { selectedKind ?? .all },
                    set: { selectedKind = $0 == .all ? nil : $0 }
                )) {
                    ForEach(SonderMediaKind.allCases) { kind in
                        Text(kind.label).tag(kind)
                    }
                }
                .pickerStyle(.segmented)

                Menu {
                    Button("All Tags") { selectedTag = nil }
                    ForEach(allTags, id: \.self) { tag in
                        Button(tag) { selectedTag = tag }
                    }
                } label: {
                    Label(selectedTag ?? "All Tags", systemImage: "tag")
                }
            }
        }
    }

    private var densityControl: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Label("Poster Density", systemImage: "rectangle.grid.2x2")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(SonderTheme.textLight)
                Spacer()
                Text("\(Int(cardMinimumWidth))")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderTheme.textLight)
                    .frame(width: 34, alignment: .trailing)
            }

            Slider(value: $cardMinimumWidth, in: 180...300) {
                Text("Poster Density")
            } minimumValueLabel: {
                Image(systemName: "square.grid.3x3")
            } maximumValueLabel: {
                Image(systemName: "rectangle.grid.1x2")
            }
        }
        .padding(12)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct ContinueWatchingView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?

    var body: some View {
        List {
            Section("Up Next") {
                ForEach(library.inProgressItems) { item in
                    MediaListRow(item: item, detail: library.progressLabel(for: item)) {
                        selectedItemID = item.id
                    }
                }
            }

            Section("Recently Updated") {
                ForEach(library.progressRecords.sorted { $0.updatedAt > $1.updatedAt }.prefix(12)) { progress in
                    if let item = library.item(id: progress.itemID) {
                        ProgressRow(item: item, progress: progress)
                    }
                }
            }
        }
        .navigationTitle("Continue Watching")
        .scrollContentBackground(.hidden)
        .background(SonderTheme.background)
    }
}

enum LibraryDisplayMode: String, CaseIterable, Identifiable {
    case grid
    case spreadsheet

    var id: String { rawValue }

    var label: String {
        switch self {
        case .grid: "Grid"
        case .spreadsheet: "Spreadsheet"
        }
    }

    var icon: String {
        switch self {
        case .grid: "square.grid.2x2"
        case .spreadsheet: "tablecells"
        }
    }
}

struct LibraryDisplayModePicker: View {
    @Binding var mode: LibraryDisplayMode

    var body: some View {
        Picker("View", selection: $mode) {
            ForEach(LibraryDisplayMode.allCases) { mode in
                Label(mode.label, systemImage: mode.icon).tag(mode)
            }
        }
        .pickerStyle(.segmented)
        .frame(width: 260)
        .labelsHidden()
    }
}

struct TVShowsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid
    @State private var searchText = ""
    @State private var selectedShowName: String?

    private var selectedShow: SonderTVShowGroup? {
        guard let selectedShowName else { return nil }
        return library.tvShowGroups.first { $0.name == selectedShowName }
    }

    private var filteredShows: [SonderTVShowGroup] {
        let trimmed = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return library.tvShowGroups }
        return library.tvShowGroups.compactMap { show in
            if show.name.localizedCaseInsensitiveContains(trimmed) {
                return show
            }
            let seasons = show.seasons.compactMap { season -> SonderTVSeasonGroup? in
                let matchingEpisodes = season.episodes.filter { $0.searchText.localizedCaseInsensitiveContains(trimmed) }
                guard matchingEpisodes.isEmpty == false else { return nil }
                return SonderTVSeasonGroup(seasonNumber: season.seasonNumber, episodes: matchingEpisodes)
            }
            guard seasons.isEmpty == false else { return nil }
            return SonderTVShowGroup(name: show.name, seasons: seasons)
        }
    }

    var body: some View {
        Group {
            if let selectedShow {
                TVShowDetailPage(show: selectedShow, library: library) { item in
                    selectedItemID = item.id
                } back: {
                    selectedShowName = nil
                }
            } else {
                VStack(spacing: 0) {
                    libraryModeBar(title: "TV Shows", count: filteredShows.count, countLabel: "shows", mode: $displayMode, searchText: $searchText, searchPrompt: "Search shows, seasons, or episodes")
                    switch displayMode {
            case .grid:
                ScrollView {
                    if filteredShows.isEmpty {
                        EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add a TV Shows library folder to group episodes by show and season." : "No shows match this search.")
                            .padding(20)
                    } else {
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 240, maximum: 300), spacing: 16)], spacing: 16) {
                            ForEach(filteredShows) { show in
                                TVShowCard(show: show, library: library) {
                                    selectedShowName = show.name
                                } play: { item in
                                    library.play(item)
                                }
                            }
                        }
                        .padding(20)
                    }
                }
            case .spreadsheet:
                List {
                    ForEach(filteredShows) { show in
                        DisclosureGroup {
                            ForEach(show.seasons) { season in
                                DisclosureGroup {
                                    ForEach(season.episodes) { item in
                                        MediaListRow(item: item, detail: library.progressLabel(for: item)) {
                                            selectedItemID = item.id
                                        }
                                    }
                                } label: {
                                    HStack {
                                        Label(season.label, systemImage: "rectangle.stack")
                                        Spacer()
                                        Text("\(season.episodes.count) episodes")
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(SonderTheme.textLight)
                                    }
                                }
                            }
                        } label: {
                            HStack {
                                Label(show.name, systemImage: "tv")
                                Button {
                                    selectedShowName = show.name
                                } label: {
                                    Label("Open", systemImage: "arrow.right")
                                }
                                .buttonStyle(.borderless)
                                Spacer()
                                Text("\(show.seasons.count) seasons")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight)
                                Text("\(show.displayCount) episodes")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight)
                            }
                        }
                    }
                    if filteredShows.isEmpty {
                        EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add a TV Shows library folder to group episodes by show and season." : "No shows match this search.")
                    }
                }
                .scrollContentBackground(.hidden)
                    }
                }
            }
        }
        .navigationTitle(selectedShow?.name ?? "TV Shows")
        .background(SonderTheme.background)
    }
}

struct TVShowDetailPage: View {
    let show: SonderTVShowGroup
    @ObservedObject var library: SonderLibrary
    let selectEpisode: (SonderMediaItem) -> Void
    let back: () -> Void

    private var episodes: [SonderMediaItem] {
        show.seasons.flatMap(\.episodes)
    }

    private var realEpisodes: [SonderMediaItem] {
        episodes.filter { $0.isPlaceholder == false }
    }

    private var representativeEpisode: SonderMediaItem? {
        episodes.first { $0.localPosterPath != nil } ?? realEpisodes.first ?? episodes.first
    }

    private var playableCount: Int {
        realEpisodes.filter(\.hasFile).count
    }

    private var totalRuntime: Double {
        realEpisodes.map(\.durationSeconds).filter { $0 > 0 }.reduce(0, +)
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                Button(action: back) {
                    Label("TV Shows", systemImage: "chevron.left")
                }
                .buttonStyle(.borderless)

                HStack(alignment: .top, spacing: 22) {
                    if let representativeEpisode {
                        PosterView(item: representativeEpisode)
                            .frame(width: 180, height: 270)
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .onAppear {
                                library.prioritizeAssets(for: representativeEpisode.id)
                            }
                    } else {
                        RoundedRectangle(cornerRadius: 8)
                            .fill(SonderTheme.surfaceDeep)
                            .frame(width: 180, height: 270)
                            .overlay {
                                Image(systemName: "tv")
                                    .font(.system(size: 46, weight: .semibold))
                                    .foregroundStyle(SonderTheme.accent)
                            }
                    }

                    VStack(alignment: .leading, spacing: 14) {
                        Text(show.name)
                            .font(.largeTitle.weight(.bold))
                            .foregroundStyle(SonderTheme.text)
                            .lineLimit(2)
                        Text(show.displayCount == 0 ? "Scanning show details" : "\(show.seasons.count) season(s), \(show.displayCount) episode(s)")
                            .font(.title3)
                            .foregroundStyle(SonderTheme.textLight)

                        HStack(spacing: 10) {
                            TVShowStatTile(label: "Seasons", value: "\(show.seasons.count)", icon: "rectangle.stack")
                            TVShowStatTile(label: "Episodes", value: "\(show.displayCount)", icon: "play.rectangle")
                            TVShowStatTile(label: "Playable", value: "\(playableCount)", icon: "play.circle")
                            TVShowStatTile(label: "Runtime", value: totalRuntime > 0 ? SonderTime.format(totalRuntime) : "Pending", icon: "clock")
                        }
                    }
                }

                DashboardPanel(title: "Seasons") {
                    VStack(alignment: .leading, spacing: 14) {
                        ForEach(show.seasons) { season in
                            TVSeasonDetailSection(season: season, library: library, selectEpisode: selectEpisode)
                        }
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
    }
}

struct TVShowStatTile: View {
    let label: String
    let value: String
    let icon: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(label, systemImage: icon)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
            Text(value)
                .font(.headline.monospacedDigit())
                .foregroundStyle(SonderTheme.text)
        }
        .frame(minWidth: 116, alignment: .leading)
        .padding(12)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct TVSeasonDetailSection: View {
    let season: SonderTVSeasonGroup
    @ObservedObject var library: SonderLibrary
    let selectEpisode: (SonderMediaItem) -> Void

    private var visibleEpisodes: [SonderMediaItem] {
        let realEpisodes = season.episodes.filter { $0.isPlaceholder == false }
        return realEpisodes.isEmpty ? season.episodes : realEpisodes
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label(season.label, systemImage: "rectangle.stack")
                    .font(.headline)
                Spacer()
                Text("\(visibleEpisodes.count) episode(s)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderTheme.textLight)
            }

            ForEach(visibleEpisodes) { episode in
                HStack(spacing: 12) {
                    Button {
                        selectEpisode(episode)
                        library.prioritizeAssets(for: episode.id)
                    } label: {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(episode.episodeCode == "Not episodic" ? episode.title : "\(episode.episodeCode) - \(episode.title)")
                                .font(.body.weight(.medium))
                                .lineLimit(1)
                            Text([episode.format.rawValue.uppercased(), library.progressLabel(for: episode)].joined(separator: " - "))
                                .font(.caption)
                                .foregroundStyle(SonderTheme.textLight)
                                .lineLimit(1)
                        }
                    }
                    .buttonStyle(.plain)

                    Spacer()

                    Button {
                        library.play(episode)
                    } label: {
                        Image(systemName: "play.fill")
                    }
                    .buttonStyle(.borderless)
                    .disabled(episode.hasFile == false)
                }
                .padding(.vertical, 7)
                .padding(.horizontal, 10)
                .background(SonderTheme.surfaceDeep, in: RoundedRectangle(cornerRadius: 6))
            }
        }
    }
}

struct TVShowCard: View {
    let show: SonderTVShowGroup
    @ObservedObject var library: SonderLibrary
    let open: () -> Void
    let play: (SonderMediaItem) -> Void

    private var episodes: [SonderMediaItem] {
        show.seasons.flatMap(\.episodes)
    }

    private var representativeEpisode: SonderMediaItem? {
        episodes.first { $0.localPosterPath != nil } ?? episodes.first
    }

    private var seasonCount: Int {
        show.seasons.count
    }

    private var progressLabel: String {
        guard let episode = representativeEpisode else { return "No episodes" }
        return library.progressLabel(for: episode)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button(action: open) {
                if let representativeEpisode {
                    PosterView(item: representativeEpisode)
                        .frame(maxWidth: .infinity)
                        .frame(height: 240)
                } else {
                    Rectangle()
                        .fill(SonderTheme.surfaceDeep)
                        .overlay {
                            Image(systemName: "tv")
                                .font(.system(size: 42, weight: .semibold))
                                .foregroundStyle(SonderTheme.accent)
                        }
                        .frame(maxWidth: .infinity)
                        .frame(height: 240)
                }
            }
            .buttonStyle(.plain)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .contentShape(RoundedRectangle(cornerRadius: 8))

            VStack(alignment: .leading, spacing: 5) {
                Text(show.name)
                    .font(.headline)
                    .lineLimit(2)
                HStack(spacing: 8) {
                    Label("\(seasonCount) seasons", systemImage: "rectangle.stack")
                    Label("\(show.episodeCount) episodes", systemImage: "play.rectangle")
                }
                .font(.caption.monospacedDigit())
                .foregroundStyle(SonderTheme.textLight)
                Text(progressLabel)
                    .font(.caption)
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
            }
            .frame(minHeight: 62, alignment: .top)

            HStack {
                Button(action: open) {
                    Label("Open", systemImage: "rectangle.stack")
                }
                Spacer()
                Button {
                    if let representativeEpisode {
                        play(representativeEpisode)
                    }
                } label: {
                    Image(systemName: "play.fill")
                }
                .buttonStyle(.borderless)
                .disabled(representativeEpisode?.hasFile != true)
            }
        }
        .padding(12)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct AudiobooksView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid

    private var items: [SonderMediaItem] {
        library.items.filter { $0.kind == .audiobook }.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    var body: some View {
        MediaKindBrowser(
            title: "Audiobooks",
            emptyText: "Add audiobook files to browse them here.",
            items: items,
            detail: { library.progressLabel(for: $0) },
            library: library,
            selectedItemID: $selectedItemID,
            displayMode: $displayMode
        )
    }
}

struct BooksView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid

    private var items: [SonderMediaItem] {
        library.items.filter { $0.kind == .ebook }.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    var body: some View {
        MediaKindBrowser(
            title: "Books",
            emptyText: "Add EPUB or PDF files to browse them here.",
            items: items,
            detail: { $0.format.rawValue.uppercased() },
            library: library,
            selectedItemID: $selectedItemID,
            displayMode: $displayMode
        )
    }
}

struct MediaKindBrowser: View {
    let title: String
    let emptyText: String
    let items: [SonderMediaItem]
    let detail: (SonderMediaItem) -> String
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @Binding var displayMode: LibraryDisplayMode
    @State private var searchText = ""

    private var filteredItems: [SonderMediaItem] {
        let trimmed = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return items }
        return items.filter { $0.searchText.localizedCaseInsensitiveContains(trimmed) }
    }

    var body: some View {
        VStack(spacing: 0) {
            libraryModeBar(title: title, count: filteredItems.count, mode: $displayMode, searchText: $searchText, searchPrompt: "Search \(title.lowercased())")
            switch displayMode {
            case .grid:
                ScrollView {
                    if filteredItems.isEmpty {
                        EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? emptyText : "No titles match this search.")
                            .padding(20)
                    } else {
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 190, maximum: 230), spacing: 14)], spacing: 14) {
                            ForEach(filteredItems) { item in
                                MediaCard(item: item, progress: library.progress(for: item), posterHeight: 260) {
                                    selectedItemID = item.id
                                } play: {
                                    library.play(item)
                                }
                            }
                        }
                        .padding(20)
                    }
                }
            case .spreadsheet:
                List {
                    if filteredItems.isEmpty {
                        EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? emptyText : "No titles match this search.")
                    } else {
                        ForEach(filteredItems) { item in
                            MediaListRow(item: item, detail: detail(item)) {
                                selectedItemID = item.id
                            }
                        }
                    }
                }
                .scrollContentBackground(.hidden)
            }
        }
        .navigationTitle(title)
        .background(SonderTheme.background)
    }
}

struct SearchField: View {
    @Binding var text: String
    let prompt: String

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(SonderTheme.textLight)
            TextField(prompt, text: $text)
                .textFieldStyle(.plain)
            if text.isEmpty == false {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(SonderTheme.textLight)
                }
                .buttonStyle(.plain)
                .help("Clear search")
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(SonderTheme.background, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct EmptyLibraryMessage: View {
    let text: String

    var body: some View {
        Text(text)
            .foregroundStyle(SonderTheme.textLight)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private func libraryModeBar(
    title: String,
    count: Int,
    countLabel: String = "titles",
    mode: Binding<LibraryDisplayMode>,
    searchText: Binding<String>,
    searchPrompt: String
) -> some View {
    HStack(spacing: 14) {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
                .font(.title2.weight(.semibold))
            Text("\(count) \(countLabel)")
                .font(.caption.monospacedDigit())
                .foregroundStyle(SonderTheme.textLight)
        }
        Spacer(minLength: 14)
        SearchField(text: searchText, prompt: searchPrompt)
            .frame(minWidth: 240, idealWidth: 320, maxWidth: 420)
        LibraryDisplayModePicker(mode: mode)
    }
    .padding(.horizontal, 20)
    .padding(.vertical, 12)
    .background(SonderTheme.surface)
    .overlay(alignment: .bottom) {
        Rectangle()
            .fill(SonderTheme.border)
            .frame(height: 1)
    }
}

struct CollectionsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var newListName = ""
    @State private var newListKind: SonderCollectionKind = .playlist

    var body: some View {
        List {
            Section("Create") {
                TextField("Name", text: $newListName)
                Picker("Type", selection: $newListKind) {
                    ForEach(SonderCollectionKind.allCases) { kind in
                        Text(kind.label).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                Button {
                    library.createCollection(named: newListName, kind: newListKind)
                    newListName = ""
                } label: {
                    Label("Create", systemImage: "plus.circle")
                }
                .disabled(newListName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }

            ForEach(library.collections) { collection in
                Section("\(collection.name) - \(collection.kind.label)") {
                    ForEach(collection.itemIDs.compactMap(library.item(id:))) { item in
                        MediaListRow(item: item, detail: item.kind.label) {
                            selectedItemID = item.id
                        }
                    }
                    if collection.itemIDs.isEmpty {
                        Text("Add media from a title detail screen.")
                            .foregroundStyle(SonderTheme.textLight)
                    }
                }
            }
        }
        .navigationTitle("Collections & Playlists")
        .scrollContentBackground(.hidden)
        .background(SonderTheme.background)
    }
}

struct ServerDashboard: View {
    @ObservedObject var library: SonderLibrary

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                PageHeader(title: "Server", subtitle: "Native macOS host with a local web catalog, JSON API, video routes, and progress sync.")

                LazyVGrid(columns: [GridItem(.adaptive(minimum: 170), spacing: 12)], spacing: 12) {
                    MetricCard(title: "Titles", value: "\(library.items.count)", detail: "Indexed media")
                    MetricCard(title: "Playable", value: "\(library.playableCount)", detail: "Local files")
                    MetricCard(title: "Avg. Progress", value: "\(Int(library.averageProgress * 100))%", detail: "Across active titles")
                    MetricCard(title: "Collections", value: "\(library.collections.count)", detail: "Curated groups")
                }

                LazyVGrid(columns: [GridItem(.adaptive(minimum: 280), spacing: 14)], spacing: 14) {
                    DashboardPanel(title: "Library Mix") {
                        ForEach(SonderMediaKind.mediaCases) { kind in
                            let count = library.mediaKindCounts[kind, default: 0]
                            MeterRow(label: kind.label, value: count, max: max(1, library.items.count))
                        }
                    }

                    DashboardPanel(title: "Popular Tags") {
                        let popularTags = Array(library.tagUsage.prefix(10))
                        let maxTagCount = max(1, library.tagUsage.first?.count ?? 1)
                        ForEach(popularTags, id: \.tag) { item in
                            MeterRow(label: item.tag, value: item.count, max: maxTagCount)
                        }
                    }
                }

                DashboardPanel(title: "Endpoints") {
                    EndpointRow(label: "Web", value: "http://127.0.0.1:\(library.serverSettings.port)")
                    EndpointRow(label: "Health", value: "http://127.0.0.1:\(library.serverSettings.port)/api/health")
                    EndpointRow(label: "Library JSON", value: "http://127.0.0.1:\(library.serverSettings.port)/api/library")
                    EndpointRow(label: "Stream Route", value: "http://127.0.0.1:\(library.serverSettings.port)/stream/{id}")
                }

                DashboardPanel(title: "Web Server") {
                    Toggle("Enable local web server", isOn: Binding(
                        get: { library.serverSettings.isEnabled },
                        set: { library.updateServerSettings(isEnabled: $0) }
                    ))
                    Toggle("Allow LAN sharing", isOn: Binding(
                        get: { library.serverSettings.allowLAN },
                        set: { library.updateServerSettings(allowLAN: $0) }
                    ))
                    .disabled(library.serverSettings.isEnabled == false)
                    Text(library.serverSettings.allowLAN ? "LAN mode advertises Sonder with Bonjour and accepts non-local browser requests." : "Local-only mode rejects non-local browser requests and does not advertise Bonjour.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                }

                DashboardPanel(title: "Theme") {
                    Picker("Palette", selection: Binding(
                        get: { SonderThemePreset(rawValue: library.serverSettings.themePreset) ?? .earthy },
                        set: { library.updateTheme($0) }
                    )) {
                        ForEach(SonderThemePreset.allCases) { preset in
                            Text(preset.label).tag(preset)
                        }
                    }
                    .pickerStyle(.segmented)

                    Text(SonderThemePreset(rawValue: library.serverSettings.themePreset)?.description ?? SonderThemePreset.earthy.description)
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                }

                DashboardPanel(title: "Storage") {
                    EndpointRow(label: "Media", value: library.storagePath)
                    Text("Use the Storage toolbar button to choose a mounted NAS, external disk, or synced folder. New imports copy there; the app database remains in Application Support.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                }

                DashboardPanel(title: "Plex Import") {
                    HStack {
                        Label("Import cached Plex context into show roots", systemImage: "externaldrive.badge.icloud")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        Button {
                            library.importPlexContextIfAvailable(force: true)
                        } label: {
                            Label("Import Now", systemImage: "arrow.down.doc")
                        }
                        .disabled(library.scanProgress != nil)
                    }
                    Text("Sonder can import Plex summaries, art, and identifiers into per-show `.sonder/sonder-context.json` caches so later boots read local context first.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                    HStack {
                        Text(library.plexImportStatus.lastMessage)
                            .font(.caption)
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        if let lastRunAt = library.plexImportStatus.lastRunAt {
                            Text("Last run \(lastRunAt.formatted(date: .abbreviated, time: .shortened))")
                                .font(.caption2.monospacedDigit())
                                .foregroundStyle(SonderTheme.textLight.opacity(0.85))
                        }
                    }
                }

                DashboardPanel(title: "Audiobook Import") {
                    HStack {
                        Label("Build Sonder's native audiobook index and chapter cache", systemImage: "headphones")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        Button {
                            library.importAudiobookContextIfAvailable(force: true)
                        } label: {
                            Label("Refresh Now", systemImage: "arrow.clockwise")
                        }
                        .disabled(library.scanProgress != nil)
                    }
                    Text("Sonder stores audiobook metadata in its own local cache so boots can load books, chapters, and artwork without rescanning everything.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                    HStack {
                        Text(library.audiobookImportStatus.lastMessage)
                            .font(.caption)
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        if let lastRunAt = library.audiobookImportStatus.lastRunAt {
                            Text("Last run \(lastRunAt.formatted(date: .abbreviated, time: .shortened))")
                                .font(.caption2.monospacedDigit())
                                .foregroundStyle(SonderTheme.textLight.opacity(0.85))
                        }
                    }
                }

                DashboardPanel(title: "Remote Libraries") {
                    HStack {
                        Label("\(library.libraryDefinitions.count) libraries / \(library.mediaDirectories.count) folders", systemImage: "network")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        Button {
                            library.chooseMediaLibraryRoot()
                        } label: {
                            Label("Media Library", systemImage: "externaldrive.badge.plus")
                        }
                        Button {
                            library.chooseMediaDirectories(kind: .movies)
                        } label: {
                            Label("Movies Library", systemImage: "film")
                        }
                        Button {
                            library.chooseMediaDirectories(kind: .tvShows)
                        } label: {
                            Label("TV Library", systemImage: "tv")
                        }
                        Button {
                            library.chooseMediaDirectories(kind: .audiobooks)
                        } label: {
                            Label("Audio Library", systemImage: "headphones")
                        }
                        Button {
                            library.chooseMediaDirectories(kind: .ebooks)
                        } label: {
                            Label("Books Library", systemImage: "book")
                        }
                        Button {
                            library.rescanMediaDirectories()
                        } label: {
                            Label("Rescan", systemImage: "arrow.clockwise")
                        }
                        .disabled(library.mediaDirectories.isEmpty)
                    }

                    Text("Select a Media Library root to auto-detect TV Shows, Movies, Books, and Audiobooks folders, or add individual SMB/NFS/NAS folders manually. Sonder stores security-scoped bookmarks and only scans directories the user grants.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)

                    if let progress = library.scanProgress {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Label(progress.title, systemImage: "magnifyingglass")
                                Spacer()
                                Text(progress.phase.label)
                                    .font(.caption.weight(.semibold))
                                    .foregroundStyle(SonderTheme.accent)
                                Text("\(progress.filesSeen) files")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight)
                            }
                            ProgressView(value: progress.fraction)
                                .tint(SonderTheme.accentStrong)
                            HStack {
                                Label("\(progress.mediaFound) media", systemImage: "doc.richtext")
                                Label("\(progress.indexedCount) indexed", systemImage: "tray.and.arrow.down")
                                Spacer()
                                Text("\(progress.directoriesDone)/\(progress.directoriesTotal) folders")
                            }
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(SonderTheme.textLight)
                            Text(progress.detail)
                                .font(.caption)
                                .foregroundStyle(SonderTheme.textLight)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }

                    ForEach(library.libraryDefinitions) { definition in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Label(definition.name, systemImage: definition.kind.icon)
                                    .font(.headline)
                                Spacer()
                                Text("\(library.mediaDirectories.filter { $0.libraryID == definition.id }.count) folders")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight)
                            }
                            ForEach(library.mediaDirectories.filter { $0.libraryID == definition.id }) { directory in
                                HStack {
                                    Image(systemName: "folder")
                                        .foregroundStyle(SonderTheme.accentStrong)
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(directory.name)
                                            .font(.subheadline.weight(.semibold))
                                        Text(directory.path)
                                            .font(.caption)
                                            .foregroundStyle(SonderTheme.textLight)
                                            .lineLimit(1)
                                            .truncationMode(.middle)
                                        Text(directory.scanSummary)
                                            .font(.caption2)
                                            .foregroundStyle(SonderTheme.textLight.opacity(0.85))
                                    }
                                    Spacer()
                                    VStack(alignment: .trailing, spacing: 2) {
                                        Text("\(directory.lastIndexedCount) media")
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(SonderTheme.textLight)
                                        Text("\(directory.lastScannedFileCount) files")
                                            .font(.caption2.monospacedDigit())
                                            .foregroundStyle(SonderTheme.textLight.opacity(0.8))
                                    }
                                    Button {
                                        library.rescanMediaDirectory(directory.id)
                                    } label: {
                                        Image(systemName: "arrow.clockwise")
                                    }
                                    .buttonStyle(.borderless)
                                    .help("Rescan \(directory.name)")
                                    .disabled(library.scanProgress != nil)
                                }
                            }
                        }
                    }
                }

                DashboardPanel(title: "Conversion Queue") {
                    HStack {
                        Label("App Store-safe AVFoundation export", systemImage: "checkmark.circle")
                            .foregroundStyle(SonderTheme.accent)
                        Spacer()
                        Button {
                            library.convertAllPlayableToMP4()
                        } label: {
                            Label("Convert Playable to MP4", systemImage: "arrow.triangle.2.circlepath")
                        }
                    }
                    Text("Sonder only converts formats Apple frameworks can read and export. For MKV or uncommon codecs, keep this app as the catalog/server and use a companion workflow with recommended HandBrake settings.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                    ForEach(library.conversionJobs.prefix(8)) { job in
                        HStack {
                            Image(systemName: job.status.icon)
                                .foregroundStyle(SonderTheme.accentStrong)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(job.title)
                                    .font(.subheadline.weight(.semibold))
                                Text(job.detail)
                                    .font(.caption)
                                    .foregroundStyle(SonderTheme.textLight)
                            }
                            Spacer()
                            Text(job.status.label)
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(SonderTheme.textLight)
                        }
                    }
                }

                DashboardPanel(title: "Recent Activity") {
                    ForEach(library.activity.prefix(12)) { event in
                        HStack {
                            Image(systemName: event.icon)
                                .foregroundStyle(SonderTheme.accentStrong)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(event.title)
                                    .font(.subheadline.weight(.semibold))
                                Text(event.detail)
                                    .font(.caption)
                                    .foregroundStyle(SonderTheme.textLight)
                            }
                            Spacer()
                            Text(event.date, style: .time)
                                .font(.caption)
                                .foregroundStyle(SonderTheme.textLight)
                        }
                        Divider()
                    }
                }
            }
            .padding(20)
        }
        .background(SonderTheme.background)
        .navigationTitle("Server")
    }
}

struct AboutSonderView: View {
    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                PageHeader(
                    title: "About TM Sonder",
                    subtitle: "A private, self-hosted media library for local video collections."
                )

                AboutPanel(
                    title: "Native Host",
                    icon: "macwindow",
                    bodyText: "Sonder runs as a macOS app, manages imported media in Application Support, keeps a menu-bar presence, and exposes a browser interface on the local network service."
                )

                AboutPanel(
                    title: "Plex-Like Basics",
                    icon: "play.rectangle.on.rectangle",
                    bodyText: "The app catalogs movies, television, documentaries, books, and audiobooks, tracks watched position, groups titles into collections, and serves playable files through HTTP routes."
                )

                AboutPanel(
                    title: "TV Show Workflow",
                    icon: "tv",
                    bodyText: "TV libraries are documented as Show -> Season -> Episode. Opening a show should expose show metrics, series information, season artwork, season drill-down, and episode playback. Unknown seasons are kept in an Unsorted Season bucket until filenames or folders provide a season number."
                )

                AboutPanel(
                    title: "Server and iOS Contract",
                    icon: "iphone.and.arrow.forward",
                    bodyText: "Client workflows should use the server library JSON, group TV episodes by showTitle and seasonNumber, open /stream/{id} for playback, and POST progress to /api/progress/{id}. Browser-playable files stream directly; catalog-only formats should open in a native player or use a future transcoding route."
                )

                AboutPanel(
                    title: "Transcoding and Cache Policy",
                    icon: "externaldrive.badge.icloud",
                    bodyText: "Direct play is preferred. On-demand conversion, local cache, and pre-cache controls should be explicit user choices with visible storage impact, rather than automatic background work during browsing."
                )

                AboutPanel(
                    title: "Alexandria-Inspired Interface",
                    icon: "rectangle.3.group",
                    bodyText: "The layout mirrors TM Alexandria's three-column macOS structure, dense poster grid, management dashboard, muted olive theme, and focused title detail panes."
                )
            }
            .padding(20)
        }
        .background(SonderTheme.background)
        .navigationTitle("About")
    }
}

struct MediaDetailView: View {
    let item: SonderMediaItem
    @ObservedObject var library: SonderLibrary
    @State private var seconds: Double
    @State private var duration: Double

    init(item: SonderMediaItem, library: SonderLibrary) {
        self.item = item
        self.library = library
        let progress = library.progressRecord(for: item)
        _seconds = State(initialValue: progress?.seconds ?? item.progressSeconds)
        _duration = State(initialValue: progress?.duration ?? item.durationSeconds)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack(alignment: .top, spacing: 20) {
                    PosterView(item: item)
                        .frame(width: 170, height: 250)
                        .clipShape(RoundedRectangle(cornerRadius: 8))

                    VStack(alignment: .leading, spacing: 10) {
                        Text(item.title)
                            .font(.largeTitle.weight(.bold))
                            .foregroundStyle(SonderTheme.text)
                        Text(item.subtitle)
                            .font(.title3)
                            .foregroundStyle(SonderTheme.textLight)
                        Text(item.summary)
                            .foregroundStyle(SonderTheme.text)
                        TagCloud(tags: item.tags)

                        HStack {
                            Button {
                                library.play(item)
                            } label: {
                                Label(item.hasFile ? "Play" : "Locate File", systemImage: item.hasFile ? "play.fill" : "folder")
                            }
                            .buttonStyle(.borderedProminent)

                            Button {
                                NSWorkspace.shared.open(URL(string: "http://127.0.0.1:\(library.serverSettings.port)/stream/\(item.id.uuidString)")!)
                            } label: {
                                Label("Stream URL", systemImage: "network")
                            }
                            .disabled(item.hasFile == false)
                        }

                        if item.kind == .movie || item.kind == .tvShow || item.kind == .documentary {
                            HStack {
                                Button {
                                    library.renameForPlex(item)
                                } label: {
                                    Label("Rename for Plex", systemImage: "textformat")
                                }
                                .disabled(item.hasFile == false)

                                Button {
                                    library.convertToMP4(item)
                                } label: {
                                    Label("Convert to MP4", systemImage: "arrow.triangle.2.circlepath")
                                }
                                .disabled(item.hasFile == false)
                            }
                        }
                    }
                }

                DashboardPanel(title: "Watch Progress") {
                    VStack(alignment: .leading, spacing: 12) {
                        ProgressView(value: duration <= 0 ? 0 : seconds / duration)
                            .tint(SonderTheme.accentStrong)

                        HStack {
                            Stepper("Position: \(SonderTime.format(seconds))", value: $seconds, in: 0...max(duration, 1), step: 60)
                            Spacer()
                            Text("\(Int((duration <= 0 ? 0 : seconds / duration) * 100))%")
                                .font(.body.monospacedDigit())
                                .foregroundStyle(SonderTheme.textLight)
                        }

                        Button {
                            library.updateProgress(itemID: item.id, seconds: seconds, duration: duration)
                        } label: {
                            Label("Save Progress", systemImage: "checkmark")
                        }
                    }
                }

                DashboardPanel(title: "Metadata") {
                    MetadataRow(label: "Kind", value: item.kind.label)
                    MetadataRow(label: "Studio", value: item.studio)
                    MetadataRow(label: "Year", value: "\(item.year)")
                    MetadataRow(label: "Runtime", value: item.durationSeconds > 0 ? SonderTime.format(item.durationSeconds) : "Not probed")
                    MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
                    if let width = item.probedWidth, let height = item.probedHeight {
                        MetadataRow(label: "Resolution", value: "\(width)×\(height)")
                    } else {
                        MetadataRow(label: "Resolution", value: "Not probed")
                    }
                    MetadataRow(label: "Codec", value: item.probedCodec ?? "Not probed")
                    if let bitrate = item.probedBitrate {
                        MetadataRow(label: "Bitrate", value: "\(bitrate / 1_000_000) Mbps")
                    }
                    MetadataRow(label: "Browser Play", value: item.isBrowserPlayable ? "Yes" : "Catalog only")
                    if item.kind == .tvShow {
                        MetadataRow(label: "Show", value: item.showTitle ?? item.title)
                        MetadataRow(label: "Episode", value: item.episodeCode)
                    } else if item.kind == .ebook {
                        MetadataRow(label: "Reader", value: "Books / Preview")
                    } else if item.kind == .audiobook {
                        MetadataRow(label: "Player", value: "Books / Music")
                    }
                    MetadataRow(label: "Plex Name", value: item.plexFileName)
                }

                DashboardPanel(title: "Add to Collection") {
                    FlowLayout {
                        ForEach(library.collections) { collection in
                            Button(collection.name) {
                                library.add(item, to: collection)
                            }
                            .buttonStyle(.bordered)
                        }
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
        .navigationTitle("Title")
        .onAppear {
            library.prioritizeAssets(for: item.id)
        }
        .onChange(of: item.id) { _, _ in
            library.prioritizeAssets(for: item.id)
            let progress = library.progressRecord(for: item)
            seconds = progress?.seconds ?? item.progressSeconds
            duration = progress?.duration ?? item.durationSeconds
        }
    }
}

struct MediaCard: View {
    let item: SonderMediaItem
    let progress: Double
    let posterHeight: CGFloat
    let action: () -> Void
    let play: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button(action: action) {
                PosterView(item: item)
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity)
            .frame(height: posterHeight)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .contentShape(RoundedRectangle(cornerRadius: 8))
            .clipped()

            VStack(alignment: .leading, spacing: 4) {
                Text(item.title)
                    .font(.headline)
                    .lineLimit(2)
                Text(item.subtitle)
                    .font(.caption)
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
                Text(item.kind.label)
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
            }
            .frame(minHeight: 56, alignment: .top)

            Text(item.summary)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
                .lineLimit(5)
                .fixedSize(horizontal: false, vertical: true)
                .frame(minHeight: 70, alignment: .top)

            ProgressView(value: progress)
                .tint(SonderTheme.accentStrong)

            Spacer(minLength: 0)

            Button(action: play) {
                Label(item.hasFile ? "Play" : "Locate", systemImage: item.hasFile ? "play.fill" : "folder")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
        .padding(12)
        .frame(maxWidth: .infinity, minHeight: posterHeight + 272, maxHeight: posterHeight + 272, alignment: .top)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }
}

struct PosterView: View {
    let item: SonderMediaItem

    var body: some View {
        ZStack {
            if let posterImage = item.localPosterImage {
                Image(nsImage: posterImage)
                    .resizable()
                    .scaledToFill()
            } else {
                LinearGradient(colors: item.kind.gradient, startPoint: .topLeading, endPoint: .bottomTrailing)
                Image(systemName: item.kind.icon)
                    .font(.system(size: 52, weight: .semibold))
                    .foregroundStyle(SonderTheme.accent.opacity(0.55))
            }
        }
        .overlay(alignment: .topLeading) {
            Text(item.kind.shortLabel)
                .font(.caption2.weight(.heavy))
                .padding(.horizontal, 7)
                .padding(.vertical, 5)
                .foregroundStyle(SonderTheme.darkText)
                .background(SonderTheme.accent.opacity(0.92), in: RoundedRectangle(cornerRadius: 4))
                .padding(8)
        }
        .overlay(alignment: .bottomLeading) {
            VStack(alignment: .leading, spacing: 4) {
                Text(item.title)
                    .font(.caption.weight(.bold))
                    .lineLimit(3)
                Text(item.format.rawValue.uppercased())
                    .font(.system(size: 9, weight: .heavy))
            }
            .foregroundStyle(SonderTheme.darkText)
            .padding(8)
            .background(SonderTheme.accent.opacity(0.92), in: RoundedRectangle(cornerRadius: 6))
            .padding(8)
        }
    }
}

struct EmptyDetailView: View {
    let title: String
    let subtitle: String
    let icon: String

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: icon)
                .font(.system(size: 42, weight: .semibold))
                .foregroundStyle(SonderTheme.accentStrong)
            Text(title)
                .font(.title2.weight(.bold))
                .foregroundStyle(SonderTheme.text)
            Text(subtitle)
                .multilineTextAlignment(.center)
                .foregroundStyle(SonderTheme.textLight)
                .frame(maxWidth: 360)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(SonderTheme.background)
    }
}

struct MediaListRow: View {
    let item: SonderMediaItem
    let detail: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: 12) {
                PosterView(item: item)
                    .frame(width: 38, height: 56)
                    .clipShape(RoundedRectangle(cornerRadius: 6))
                VStack(alignment: .leading, spacing: 3) {
                    Text(item.title)
                        .foregroundStyle(SonderTheme.text)
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                }
                Spacer()
            }
        }
    }
}

struct ProgressRow: View {
    let item: SonderMediaItem
    let progress: SonderProgress

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(item.title)
                Spacer()
                Text("\(Int(progress.percent * 100))%")
                    .foregroundStyle(SonderTheme.textLight)
            }
            ProgressView(value: progress.percent)
                .tint(SonderTheme.accentStrong)
        }
    }
}

struct MetricCard: View {
    let title: String
    let value: String
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(SonderTheme.textLight)
            Text(value)
                .font(.system(size: 32, weight: .bold, design: .rounded))
                .foregroundStyle(SonderTheme.text)
            Text(detail)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct DashboardPanel<Content: View>: View {
    let title: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(title)
                .font(.headline)
            content
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .padding(14)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct MeterRow: View {
    let label: String
    let value: Int
    let max: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(label)
                Spacer()
                Text("\(value)")
                    .foregroundStyle(SonderTheme.textLight)
            }
            GeometryReader { proxy in
                RoundedRectangle(cornerRadius: 3)
                    .fill(SonderTheme.accentMuted)
                    .overlay(alignment: .leading) {
                        RoundedRectangle(cornerRadius: 3)
                            .fill(SonderTheme.accentStrong)
                            .frame(width: proxy.size.width * CGFloat(value) / CGFloat(max))
                    }
            }
            .frame(height: 8)
        }
        .font(.caption)
    }
}

struct EndpointRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(SonderTheme.textLight)
                .frame(width: 92, alignment: .leading)
            Text(value)
                .font(.body.monospaced())
                .textSelection(.enabled)
        }
    }
}

struct MetadataRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(SonderTheme.textLight)
                .frame(width: 86, alignment: .leading)
            Text(value)
        }
    }
}

struct PageHeader: View {
    let title: String
    let subtitle: String

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(title)
                .font(.largeTitle.weight(.bold))
                .foregroundStyle(SonderTheme.text)
            Text(subtitle)
                .foregroundStyle(SonderTheme.textLight)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct StatRow: View {
    let label: String
    let value: String
    let icon: String

    var body: some View {
        HStack {
            Label(label, systemImage: icon)
            Spacer()
            Text(value)
                .foregroundStyle(SonderTheme.textLight)
        }
    }
}

struct AboutPanel: View {
    let title: String
    let icon: String
    let bodyText: String

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            Image(systemName: icon)
                .font(.title2.weight(.semibold))
                .foregroundStyle(SonderTheme.accentStrong)
                .frame(width: 28)

            VStack(alignment: .leading, spacing: 8) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(SonderTheme.text)
                Text(bodyText)
                    .font(.body)
                    .foregroundStyle(SonderTheme.textLight)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct TagCloud: View {
    let tags: [String]

    var body: some View {
        FlowLayout {
            ForEach(tags, id: \.self) { tag in
                Text(tag)
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(SonderTheme.accentMuted, in: Capsule())
                    .foregroundStyle(SonderTheme.accent)
            }
        }
    }
}

struct FlowLayout<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        WrappingLayout(horizontalSpacing: 8, verticalSpacing: 8) {
            content
        }
    }
}

struct WrappingLayout: Layout {
    var horizontalSpacing: CGFloat = 8
    var verticalSpacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxWidth = proposal.width ?? 600
        let rows = rows(in: maxWidth, subviews: subviews)
        return CGSize(width: maxWidth, height: rows.last.map { $0.y + $0.height } ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for item in rows(in: bounds.width, subviews: subviews) {
            subviews[item.index].place(
                at: CGPoint(x: bounds.minX + item.x, y: bounds.minY + item.y),
                proposal: ProposedViewSize(width: item.width, height: item.height)
            )
        }
    }

    private func rows(in maxWidth: CGFloat, subviews: Subviews) -> [(index: Int, x: CGFloat, y: CGFloat, width: CGFloat, height: CGFloat)] {
        var result: [(Int, CGFloat, CGFloat, CGFloat, CGFloat)] = []
        var x: CGFloat = 0
        var y: CGFloat = 0
        var rowHeight: CGFloat = 0

        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            if x > 0, x + size.width > maxWidth {
                x = 0
                y += rowHeight + verticalSpacing
                rowHeight = 0
            }
            result.append((index, x, y, min(size.width, maxWidth), size.height))
            x += size.width + horizontalSpacing
            rowHeight = max(rowHeight, size.height)
        }
        return result
    }
}

/// Thread-safe cache of pre-serialized HTTP responses.  The library invalidates
/// this whenever its state changes, so the HTTP server never needs to hop to the
/// main actor for reads.  Encoding happens at most once between mutations.
nonisolated final class SonderHTTPCache: @unchecked Sendable {
    private let lock = NSLock()
    private var libraryData: Data?
    private var discoveryData: Data?
    /// Monotonically increasing generation that lets the server detect when a
    /// cached response it intends to store was built against stale library state.
    /// Bumped inside `invalidate()` so every library mutation pushes the
    /// generation forward.
    private var generation: UInt64 = 0

    func invalidate() {
        lock.withLock {
            libraryData = nil
            discoveryData = nil
            generation &+= 1
        }
    }

    /// Returns the cached library response and the generation it was stored at.
    /// The caller should capture the generation and pass it back to
    /// `setLibrary(_, generation:)`. If the generation does not match the
    /// current value, the set is a no-op (the caller built stale data).
    func cachedLibrary() -> (data: Data?, generation: UInt64) {
        lock.withLock {
            (libraryData, generation)
        }
    }

    func setLibrary(_ data: Data, generation: UInt64) {
        lock.withLock {
            guard generation == self.generation else { return }
            libraryData = data
        }
    }

    func cachedDiscovery() -> (data: Data?, generation: UInt64) {
        lock.withLock {
            (discoveryData, generation)
        }
    }

    func setDiscovery(_ data: Data, generation: UInt64) {
        lock.withLock {
            guard generation == self.generation else { return }
            discoveryData = data
        }
    }
}

@MainActor
final class SonderLibrary: ObservableObject {
    @Published private(set) var items: [SonderMediaItem]
    @Published private(set) var progressRecords: [SonderProgress]
    @Published private(set) var collections: [SonderCollection]
    @Published private(set) var activity: [SonderActivityEvent]
    @Published private(set) var conversionJobs: [SonderConversionJob]
    @Published private(set) var libraryDefinitions: [SonderLibraryDefinition]
    @Published private(set) var mediaDirectories: [SonderMediaDirectory]
    @Published private(set) var scanProgress: SonderScanProgress?
    @Published private(set) var serverSettings: SonderServerSettings
    @Published private(set) var storagePath: String
    private var storageBookmark: Data?

    /// The cache is itself thread-safe and Sendable, so the stored reference
    /// is implicitly nonisolated and safely accessible from the HTTP server's
    /// background queue without main-actor hops.
    private let httpCacheStorage = SonderHTTPCache()
    nonisolated var httpCache: SonderHTTPCache { httpCacheStorage }

    private let store: SonderStore
    private let saveQueue = DispatchQueue(label: "tm.sonder.save", qos: .utility)
    private var saveWorkItem: DispatchWorkItem?
    private var pendingIndexedItems: [SonderMediaItem] = []
    private var pendingScanProgress: SonderScanProgress?
    private var pendingIndexFlushTask: Task<Void, Never>?
    private var prioritizedAssetRefreshIDs = Set<UUID>()
    private var didAttemptPlexImport = false
    private var didAttemptAudiobookImport = false
    @Published private(set) var plexImportStatus = PlexImportStatus()
    @Published private(set) var audiobookImportStatus = SonderAudiobookImportStatus()

    static let defaultCollections = [
        SonderCollection(name: "Saturday Feature Queue", kind: .playlist, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .movie }.prefix(4).map(\.id))),
        SonderCollection(name: "Documentary Shelf", kind: .collection, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .documentary }.map(\.id))),
        SonderCollection(name: "Shows in Rotation", kind: .playlist, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .tvShow }.map(\.id)))
    ]

    init(store: SonderStore? = nil) {
        let resolvedStore = store ?? SonderStore()
        self.store = resolvedStore
        let initialSnapshot = SonderSnapshot.startupPlaceholder
        items = initialSnapshot.items
        progressRecords = initialSnapshot.progress
        collections = initialSnapshot.collections
        activity = initialSnapshot.activity
        conversionJobs = initialSnapshot.conversionJobs ?? []
        libraryDefinitions = initialSnapshot.libraryDefinitions ?? SonderLibraryDefinition.defaults
        mediaDirectories = initialSnapshot.mediaDirectories ?? []
        serverSettings = initialSnapshot.serverSettings ?? .default
        storageBookmark = initialSnapshot.storageBookmark
        storagePath = initialSnapshot.storagePath ?? resolvedStore.uploadsURL.path
        rebuildDerivedData()
        loadPersistedLibrary(from: resolvedStore)
    }

    func importPlexContextIfAvailable(force: Bool = false) {
        guard force || didAttemptPlexImport == false else { return }
        didAttemptPlexImport = true
        Task.detached { [store] in
            let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            let plexRoot = support?.appendingPathComponent("Plex Media Server", isDirectory: true)
            let dbURL = plexRoot?.appendingPathComponent("Plug-in Support/Databases/com.plexapp.plugins.library.db")
            let metadataRoot = plexRoot?.appendingPathComponent("Metadata/TV Shows", isDirectory: true)
            guard let dbURL, let metadataRoot,
                  FileManager.default.fileExists(atPath: dbURL.path),
                  FileManager.default.fileExists(atPath: metadataRoot.path) else { return }
            let importer = PlexImporter(dbURL: dbURL, bundleRootURL: metadataRoot)
            let summary = await importer.importShowContexts()
            guard summary.importedCount > 0 || summary.unchangedCount > 0 || summary.skippedCount > 0 else {
                await MainActor.run {
                    self.plexImportStatus = PlexImportStatus(lastRunAt: Date(), importedCount: 0, unchangedCount: 0, skippedCount: 0, lastMessage: "No Plex show metadata found.")
                }
                return
            }
            let cacheRoot = store.rootURL.appendingPathComponent("PlexImport", isDirectory: true)
            try? FileManager.default.createDirectory(at: cacheRoot, withIntermediateDirectories: true)
            let payload = [
                "imported": summary.importedCount,
                "unchanged": summary.unchangedCount,
                "skipped": summary.skippedCount,
                "capturedAt": ISO8601DateFormatter().string(from: Date())
            ] as [String: Any]
            if let data = try? JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys]) {
                try? data.write(to: cacheRoot.appendingPathComponent("plex-import-index.json"), options: .atomic)
            }
            await MainActor.run {
                let message = summary.importedCount > 0 ? "Imported Plex context for \(summary.importedCount) show(s)." : "Plex context already up to date."
                self.plexImportStatus = PlexImportStatus(lastRunAt: Date(), importedCount: summary.importedCount, unchangedCount: summary.unchangedCount, skippedCount: summary.skippedCount, lastMessage: message)
                self.addActivity("Imported Plex context", detail: "\(summary.importedCount) imported, \(summary.unchangedCount) unchanged, \(summary.skippedCount) skipped.", icon: "externaldrive.badge.icloud")
            }
        }
    }

    func importAudiobookContextIfAvailable(force: Bool = false) {
        guard force || didAttemptAudiobookImport == false else { return }
        didAttemptAudiobookImport = true
        Task.detached { [store] in
            let importer = SonderAudiobookImporter(store: store)
            let summary = await importer.refreshIndex(items: await MainActor.run { self.items }, mediaDirectories: await MainActor.run { self.mediaDirectories })
            await MainActor.run {
                self.audiobookImportStatus = SonderAudiobookImportStatus(
                    lastRunAt: Date(),
                    importedCount: summary.importedCount,
                    updatedCount: summary.updatedCount,
                    unchangedCount: summary.unchangedCount,
                    skippedCount: summary.skippedCount,
                    lastMessage: summary.message
                )
                self.addActivity("Refreshed audiobook index", detail: summary.message, icon: "headphones")
            }
        }
    }

    @Published private(set) var playableCount = 0
    @Published private(set) var inProgressItems: [SonderMediaItem] = []
    @Published private(set) var averageProgress = 0.0
    @Published private(set) var allTags: [String] = []
    @Published private(set) var tagUsage: [(tag: String, count: Int)] = []
    @Published private(set) var tvShowGroups: [SonderTVShowGroup] = []
    @Published private(set) var mediaKindCounts: [SonderMediaKind: Int] = [:]
    @Published private(set) var isLoadingPersistedLibrary = false

    private nonisolated static let scanProgressUpdateStride = 100
    private nonisolated static let scanIndexBatchSize = 12
    private nonisolated static let maxConcurrentAssetRefreshes = 4
    private nonisolated static let indexUICommitIntervalNanoseconds: UInt64 = 450_000_000
    private nonisolated static let maxPendingIndexedItemsBeforeCommit = 96
    private nonisolated static let maxPriorityAssetRefreshItems = 24

    private func loadPersistedLibrary(from store: SonderStore) {
        isLoadingPersistedLibrary = true
        Task.detached {
            let snapshot = store.load()
            let derivedData = Self.makeDerivedData(items: snapshot.items, progressRecords: snapshot.progress)
            await MainActor.run {
                self.applyLoadedSnapshot(snapshot, derivedData: derivedData, from: store)
            }
        }
    }

    private func applyLoadedSnapshot(_ snapshot: SonderSnapshot, derivedData: SonderDerivedData, from store: SonderStore) {
        items = snapshot.items
        progressRecords = snapshot.progress
        collections = snapshot.collections
        activity = snapshot.activity
        conversionJobs = snapshot.conversionJobs ?? []
        let loadedLibraryDefinitions = snapshot.libraryDefinitions ?? SonderLibraryDefinition.defaults
        libraryDefinitions = loadedLibraryDefinitions
        mediaDirectories = Self.migrateDirectories(snapshot.mediaDirectories ?? [], libraries: loadedLibraryDefinitions)
        let loadedServerSettings = snapshot.serverSettings ?? .default
        let serverSettingsChanged = loadedServerSettings != serverSettings
        serverSettings = loadedServerSettings
        storageBookmark = snapshot.storageBookmark
        storagePath = snapshot.storagePath ?? store.uploadsURL.path
        applyDerivedData(derivedData)
        isLoadingPersistedLibrary = false
        if serverSettingsChanged {
            NotificationCenter.default.post(name: .sonderServerSettingsDidChange, object: nil)
        }
    }

    private func enqueueScanProgress(_ progress: SonderScanProgress) {
        pendingScanProgress = progress
        scheduleIndexFlush()
    }

    private func enqueueIndexedItems(_ newItems: [SonderMediaItem], progress: SonderScanProgress?) {
        pendingIndexedItems.append(contentsOf: newItems)
        if let progress {
            pendingScanProgress = progress
        }
        if pendingIndexedItems.count >= Self.maxPendingIndexedItemsBeforeCommit {
            flushPendingIndexUpdates()
        } else {
            scheduleIndexFlush()
        }
    }

    private func scheduleIndexFlush() {
        guard pendingIndexFlushTask == nil else { return }
        pendingIndexFlushTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: Self.indexUICommitIntervalNanoseconds)
            await MainActor.run {
                self?.flushPendingIndexUpdates()
            }
        }
    }

    private func flushPendingIndexUpdates() {
        pendingIndexFlushTask?.cancel()
        pendingIndexFlushTask = nil
        let indexedItems = pendingIndexedItems
        let progress = pendingScanProgress
        pendingIndexedItems.removeAll(keepingCapacity: true)
        pendingScanProgress = nil

        if indexedItems.isEmpty == false {
            items.append(contentsOf: indexedItems)
            removeResolvedPlaceholders()
            commitLibraryMutation(persist: false)
        }
        if let progress {
            scanProgress = progress
        }
    }

    private func removeResolvedPlaceholders() {
        let realKeys = Set(items.filter { $0.isPlaceholder == false }.map(placeholderKey(for:)))
        items.removeAll { item in
            item.isPlaceholder && realKeys.contains(placeholderKey(for: item))
        }
    }

    private func placeholderKey(for item: SonderMediaItem) -> String {
        let rootTitle = (item.kind == .tvShow ? item.showTitle : nil) ?? item.title
        return "\(item.kind.rawValue)|\(rootTitle.cleanedMediaTitle.lowercased())"
    }

    private func rebuildDerivedData() {
        applyDerivedData(Self.makeDerivedData(items: items, progressRecords: progressRecords))
    }

    private func applyDerivedData(_ derivedData: SonderDerivedData) {
        playableCount = derivedData.playableCount
        averageProgress = derivedData.averageProgress
        allTags = derivedData.allTags
        tagUsage = derivedData.tagUsage
        mediaKindCounts = derivedData.mediaKindCounts
        inProgressItems = derivedData.inProgressItems
        tvShowGroups = derivedData.tvShowGroups
    }

    nonisolated private static func makeDerivedData(items: [SonderMediaItem], progressRecords: [SonderProgress]) -> SonderDerivedData {
        let playableCount = items.filter(\.hasFile).count
        let averageProgress = progressRecords.isEmpty ? 0 : progressRecords.map(\.percent).reduce(0, +) / Double(progressRecords.count)
        let allTags = Array(Set(items.flatMap(\.tags))).sorted()
        let tagUsage: [(tag: String, count: Int)] = Dictionary(grouping: items.flatMap(\.tags), by: { $0 })
            .map { (tag: $0.key, count: $0.value.count) }
            .sorted { lhs, rhs in lhs.count > rhs.count }
        let mediaKindCounts = Dictionary(grouping: items, by: \.kind).mapValues(\.count)

        let itemsByID = Dictionary(items.map { ($0.id, $0) }, uniquingKeysWith: { current, _ in current })
        let inProgressItems = progressRecords
            .filter { $0.seconds > 0 && $0.percent < 0.96 }
            .sorted { $0.updatedAt > $1.updatedAt }
            .compactMap { itemsByID[$0.itemID] }

        let episodes = items
            .filter { $0.kind == .tvShow }
            .sorted { lhs, rhs in
                let lhsKey = lhs.showTitle ?? lhs.title
                let rhsKey = rhs.showTitle ?? rhs.title
                if lhsKey != rhsKey { return lhsKey < rhsKey }
                if lhs.seasonNumber != rhs.seasonNumber { return (lhs.seasonNumber ?? 0) < (rhs.seasonNumber ?? 0) }
                if lhs.episodeNumber != rhs.episodeNumber { return (lhs.episodeNumber ?? 0) < (rhs.episodeNumber ?? 0) }
                return lhs.title < rhs.title
            }
        let tvShowGroups = Dictionary(grouping: episodes, by: { $0.showTitle ?? $0.title })
            .map { showName, showEpisodes in
                let seasons = Dictionary(grouping: showEpisodes, by: { $0.seasonNumber ?? SonderTVSeasonGroup.unknownSeasonNumber })
                    .map { seasonNumber, seasonEpisodes in
                        SonderTVSeasonGroup(
                            seasonNumber: seasonNumber,
                            episodes: seasonEpisodes.sorted { ($0.episodeNumber ?? 0, $0.title) < ($1.episodeNumber ?? 0, $1.title) }
                        )
                    }
                    .sorted { $0.sortOrder < $1.sortOrder }
                return SonderTVShowGroup(name: showName, seasons: seasons)
            }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }

        return SonderDerivedData(
            playableCount: playableCount,
            averageProgress: averageProgress,
            allTags: allTags,
            tagUsage: tagUsage,
            mediaKindCounts: mediaKindCounts,
            inProgressItems: inProgressItems,
            tvShowGroups: tvShowGroups
        )
    }

    func seedDemoLibrary() {
        items = SonderSeed.catalog
        progressRecords = [
            SonderProgress(itemID: SonderSeed.catalog[0].id, seconds: 2380, duration: SonderSeed.catalog[0].durationSeconds),
            SonderProgress(itemID: SonderSeed.catalog[3].id, seconds: 1120, duration: SonderSeed.catalog[3].durationSeconds)
        ]
        collections = Self.defaultCollections
        conversionJobs = []
        libraryDefinitions = SonderLibraryDefinition.defaults
        mediaDirectories = []
        scanProgress = nil
        serverSettings = .default
        addActivity("Seeded demo library", detail: "Loaded movies, shows, and documentaries for layout testing.", icon: "sparkles.tv")
        commitLibraryMutation()
    }

    func updateServerSettings(isEnabled: Bool? = nil, allowLAN: Bool? = nil, port: Int? = nil, pairingToken: String? = nil) {
        if let isEnabled {
            serverSettings.isEnabled = isEnabled
        }
        if let allowLAN {
            serverSettings.allowLAN = allowLAN
            // Auto-generate a pairing token the first time LAN is enabled, so the
            // default is secure rather than open. The user can rotate it in settings.
            if allowLAN && serverSettings.pairingToken.isEmpty {
                serverSettings.pairingToken = SonderServerSettings.generateToken()
            }
        }
        if let port {
            serverSettings.port = min(max(port, 1024), 65535)
        }
        if let pairingToken {
            serverSettings.pairingToken = pairingToken
        }
        SonderTheme.apply(preset: SonderThemePreset(rawValue: serverSettings.themePreset) ?? .earthy)
        addActivity("Updated server settings", detail: serverSettings.statusLabel, icon: "network")
        commitLibraryMutation()
        NotificationCenter.default.post(name: .sonderServerSettingsDidChange, object: nil)
    }

    func updateTheme(_ preset: SonderThemePreset) {
        serverSettings.themePreset = preset.rawValue
        SonderTheme.apply(preset: preset)
        addActivity("Updated theme", detail: preset.label, icon: "paintpalette")
        commitLibraryMutation()
    }

    /// Convenience: regenerates the LAN pairing token, invalidating any previously
    /// paired clients.
    func regeneratePairingToken() {
        serverSettings.pairingToken = SonderServerSettings.generateToken()
        addActivity("Rotated LAN pairing token", detail: "Previously paired clients must re-pair.", icon: "key.fill")
        commitLibraryMutation()
        NotificationCenter.default.post(name: .sonderServerSettingsDidChange, object: nil)
    }

    func setStorageFolder(_ result: Result<[URL], Error>) {
        guard case .success(let urls) = result, let url = urls.first else {
            addActivity("Storage unchanged", detail: "The selected folder could not be read.", icon: "exclamationmark.triangle")
            return
        }
        do {
            storageBookmark = try store.bookmark(for: url)
            storagePath = url.path
            addActivity("Updated storage", detail: url.path, icon: "externaldrive")
            commitLibraryMutation()
        } catch {
            addActivity("Storage unchanged", detail: error.localizedDescription, icon: "exclamationmark.triangle")
        }
    }

    func chooseMediaLibraryRoot() {
        let panel = NSOpenPanel()
        panel.title = "Add Media Library"
        panel.prompt = "Add Library"
        panel.message = "Choose a folder that contains Movies, TV Shows, Books, or Audiobooks subfolders. Other folders will be ignored."
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        panel.resolvesAliases = true

        guard panel.runModal() == .OK, let rootURL = panel.urls.first else {
            return
        }
        addMediaLibraryRoot(rootURL)
    }

    func chooseMediaDirectories(kind: SonderLibraryImportKind) {
        let panel = NSOpenPanel()
        panel.title = "Add \(kind.label) Directories"
        panel.prompt = "Add"
        panel.message = "Choose mounted SMB, NFS, NAS, or external \(kind.label.lowercased()) folders. Sonder will scan subfolders and remember the index locally."
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = true
        panel.canCreateDirectories = false
        panel.resolvesAliases = true

        guard panel.runModal() == .OK else {
            return
        }
        addMediaDirectories(panel.urls, kind: kind)
    }

    func addMediaLibraryRoot(_ rootURL: URL) {
        let matchedDirectories = detectMediaLibraryDirectories(in: rootURL)
        guard matchedDirectories.isEmpty == false else {
            addActivity("No media folders found", detail: "\(rootURL.lastPathComponent) did not contain Movies, TV Shows, Books, or Audiobooks folders.", icon: "folder.badge.questionmark")
            return
        }

        let addedDirectories = matchedDirectories.flatMap { match in
            addMediaDirectories([match.url], kind: match.kind, scansAfterAdd: false)
        }

        if addedDirectories.isEmpty == false {
            let summary = matchedDirectories.map { "\($0.kind.label): \($0.url.lastPathComponent)" }.joined(separator: ", ")
            addActivity("Added media library", detail: summary, icon: "externaldrive.badge.plus")
            scanMediaDirectories(addedDirectories)
        }
    }

    @discardableResult
    func addMediaDirectories(_ urls: [URL], kind: SonderLibraryImportKind, scansAfterAdd: Bool = true) -> [SonderMediaDirectory] {
        var addedDirectories: [SonderMediaDirectory] = []
        for url in urls {
            do {
                let bookmark = try store.bookmark(for: url)
                let directory = SonderMediaDirectory(name: url.lastPathComponent, path: url.path, bookmark: bookmark, kind: kind, libraryID: libraryID(for: kind))
                if let existingIndex = mediaDirectories.firstIndex(where: { $0.path == url.path && $0.kind == kind }) {
                    mediaDirectories[existingIndex].bookmark = bookmark
                    mediaDirectories[existingIndex].name = url.lastPathComponent
                    mediaDirectories[existingIndex].libraryID = libraryID(for: kind)
                    addedDirectories.append(mediaDirectories[existingIndex])
                } else {
                    mediaDirectories.append(directory)
                    addedDirectories.append(directory)
                }
            } catch {
                addActivity("Directory add failed", detail: "\(url.lastPathComponent): \(error.localizedDescription)", icon: "exclamationmark.triangle")
            }
        }

        if addedDirectories.isEmpty == false {
            addActivity("Added media directories", detail: "\(addedDirectories.count) folder(s) ready to scan.", icon: "folder.badge.plus")
            if scansAfterAdd {
                scanMediaDirectories(addedDirectories)
            }
        }
        return addedDirectories
    }

    private func detectMediaLibraryDirectories(in rootURL: URL) -> [(kind: SonderLibraryImportKind, url: URL)] {
        let didAccess = rootURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                rootURL.stopAccessingSecurityScopedResource()
            }
        }

        guard let children = try? FileManager.default.contentsOfDirectory(
            at: rootURL,
            includingPropertiesForKeys: [.isDirectoryKey],
            options: [.skipsHiddenFiles]
        ) else { return [] }

        let directories = children.filter { url in
            (try? url.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) == true
        }
        var matches: [SonderLibraryImportKind: URL] = [:]
        for url in directories {
            guard let kind = Self.mediaLibraryKind(forFolderName: url.lastPathComponent) else { continue }
            if matches[kind] == nil {
                matches[kind] = url
            }
        }

        return SonderLibraryImportKind.mediaLibraryDetectionPriority.compactMap { kind in
            matches[kind].map { (kind: kind, url: $0) }
        }
    }

    nonisolated private static func mediaLibraryKind(forFolderName folderName: String) -> SonderLibraryImportKind? {
        let normalized = folderName.lowercased().filter(\.isLetter)
        switch normalized {
        case "tvshows", "tvshow", "television", "series":
            return .tvShows
        case "movies", "movie", "films", "film":
            return .movies
        case "ebooks", "ebook", "books", "book":
            return .ebooks
        case "audiobooks", "audiobook", "audibooks", "audibook", "audio":
            return .audiobooks
        default:
            return nil
        }
    }

    func rescanMediaDirectories() {
        scanMediaDirectories(mediaDirectories)
    }

    func rescanMediaDirectory(_ id: UUID) {
        scanMediaDirectories(mediaDirectories.filter { $0.id == id })
    }

    private func scanMediaDirectories(_ directories: [SonderMediaDirectory]) {
        guard scanProgress == nil else { return }
        guard directories.isEmpty == false else { return }
        let existingPaths = Set(items.compactMap(\.sourcePath))
        let existingMaterialKeys = Set(items.map(placeholderKey(for:)))
        scanProgress = SonderScanProgress(phase: .fastTitles, title: "Preparing scan", detail: "Starting fast title index.", filesSeen: 0, mediaFound: 0, indexedCount: 0, directoriesDone: 0, directoriesTotal: directories.count)

        Task.detached { [store] in
            var directoryUpdates: [UUID: SonderScanDiagnostics] = [:]
            var totalFilesSeen = 0
            var totalMediaFound = 0
            let scanIndexState = SonderScanIndexState(existingPaths: existingPaths, existingMaterialKeys: existingMaterialKeys)

            for (offset, directory) in directories.enumerated() {
                await scanIndexState.beginDirectory()
                let startFilesSeen = totalFilesSeen
                let startMediaFound = totalMediaFound
                let currentIndexedCount = 0
                await MainActor.run {
                    self.scanProgress = SonderScanProgress(
                        phase: .fastTitles,
                        title: "Indexing \(directory.kind.label)",
                        detail: directory.path,
                        filesSeen: startFilesSeen,
                        mediaFound: startMediaFound,
                        indexedCount: currentIndexedCount,
                        directoriesDone: offset,
                        directoriesTotal: directories.count
                    )
                }

                let rootMaterials = store.rootMaterials(in: directory.path, bookmark: directory.bookmark, kind: directory.kind)
                let placeholders = await scanIndexState.placeholders(from: rootMaterials, directory: directory)
                if placeholders.isEmpty == false {
                    let progress = SonderScanProgress(
                        phase: .fastTitles,
                        title: "Indexing \(directory.kind.label)",
                        detail: "Discovered \(placeholders.count) root title(s). Deep scan continues.",
                        filesSeen: startFilesSeen,
                        mediaFound: startMediaFound,
                        indexedCount: await scanIndexState.currentDirectoryIndexedCount(),
                        directoriesDone: offset,
                        directoriesTotal: directories.count
                    )
                    await MainActor.run {
                        self.enqueueIndexedItems(placeholders, progress: progress)
                    }
                }

                do {
                    let scanResult = try await store.mediaFiles(
                        in: directory.path,
                        bookmark: directory.bookmark,
                        progressStride: Self.scanProgressUpdateStride,
                        discoveryBatchSize: Self.scanIndexBatchSize
                    ) { filesSeen, mediaFound, currentPath in
                        let progressFilesSeen = startFilesSeen + filesSeen
                        let progressMediaFound = startMediaFound + mediaFound
                        let indexedSoFar = await scanIndexState.currentDirectoryIndexedCount()
                        let progressIndexedCount = currentIndexedCount + indexedSoFar
                        let progress = SonderScanProgress(
                            phase: .fastTitles,
                            title: "Indexing \(directory.kind.label)",
                            detail: currentPath,
                            filesSeen: progressFilesSeen,
                            mediaFound: progressMediaFound,
                            indexedCount: progressIndexedCount,
                            directoriesDone: offset,
                            directoriesTotal: directories.count
                        )
                        await MainActor.run {
                            self.enqueueScanProgress(progress)
                        }
                    } discovered: { scannedFiles in
                        let chunk = await scanIndexState.index(scannedFiles, directory: directory)
                        guard chunk.isEmpty == false else { return }
                        let indexedSoFar = await scanIndexState.currentDirectoryIndexedCount()
                        let lastPath = scannedFiles.last?.url.path ?? directory.path
                        await MainActor.run {
                            let currentProgress = self.pendingScanProgress ?? self.scanProgress
                            let progress = SonderScanProgress(
                                phase: .fastTitles,
                                title: "Indexing \(directory.kind.label)",
                                detail: lastPath,
                                filesSeen: currentProgress?.filesSeen ?? startFilesSeen,
                                mediaFound: max(currentProgress?.mediaFound ?? startMediaFound, startMediaFound + indexedSoFar),
                                indexedCount: indexedSoFar,
                                directoriesDone: offset,
                                directoriesTotal: directories.count
                            )
                            self.enqueueIndexedItems(chunk, progress: progress)
                        }
                    }

                    totalFilesSeen += scanResult.filesSeen
                    totalMediaFound += scanResult.mediaFound
                    directoryUpdates[directory.id] = SonderScanDiagnostics(
                        mediaCount: scanResult.mediaFound,
                        fileCount: scanResult.filesSeen,
                        unsupportedCount: scanResult.unsupportedMediaCount,
                        skippedDuplicateCount: await scanIndexState.currentDirectorySkippedDuplicateCount(),
                        parseFailureCount: 0,
                        scannedAt: Date()
                    )
                } catch {
                    await MainActor.run {
                        self.addActivity("Directory scan failed", detail: "\(directory.name): \(error.localizedDescription)", icon: "exclamationmark.triangle")
                    }
                }
            }

            let discoveredIDsSnapshot = await scanIndexState.discoveredItemIDs()
            let finalDirectoryUpdates = directoryUpdates
            await MainActor.run {
                self.flushPendingIndexUpdates()
                for index in self.mediaDirectories.indices {
                    if let update = finalDirectoryUpdates[self.mediaDirectories[index].id] {
                        self.mediaDirectories[index].lastIndexedCount = update.mediaCount
                        self.mediaDirectories[index].lastScannedFileCount = update.fileCount
                        self.mediaDirectories[index].lastUnsupportedCount = update.unsupportedCount
                        self.mediaDirectories[index].lastSkippedDuplicateCount = update.skippedDuplicateCount
                        self.mediaDirectories[index].lastParseFailureCount = update.parseFailureCount
                        self.mediaDirectories[index].lastScannedAt = update.scannedAt
                    }
                }
                self.addActivity("Indexed media titles", detail: "\(self.items.count) title(s) indexed. Local assets will refresh next.", icon: "arrow.clockwise")
                self.commitLibraryMutation()
                self.refreshLocalAssets(for: discoveredIDsSnapshot, directoriesTotal: directories.count)
            }
        }
    }

    private func refreshLocalAssets(for itemIDs: [UUID]? = nil, directoriesTotal: Int? = nil, showsProgress: Bool = true) {
        let targetIDs = Set(itemIDs ?? items.map(\.id))
        let targets = items.filter { targetIDs.contains($0.id) && $0.sourcePath != nil }
        guard targets.isEmpty == false else {
            if showsProgress {
                scanProgress = nil
            }
            return
        }

        if showsProgress {
            scanProgress = SonderScanProgress(phase: .localAssets, title: "Finding local artwork", detail: "Checking posters and subtitles beside indexed media.", filesSeen: 0, mediaFound: 0, indexedCount: 0, directoriesDone: 0, directoriesTotal: max(directoriesTotal ?? targets.count, 1))
        }

        // Capture Sendable snapshots of each target's id + resolved URL so the detached
        // task can probe off the main actor without touching the @MainActor model.
        let probeTargets: [(id: UUID, url: URL)] = targets.compactMap { item in
            guard let url = item.playableURL else { return nil }
            return (item.id, url)
        }

        Task.detached {
            let collector = SonderAssetRefreshCollector()

            await Self.runWithConcurrencyLimit(Self.maxConcurrentAssetRefreshes, over: probeTargets) { target in
                let didAccess = target.url.startAccessingSecurityScopedResource()
                defer { if didAccess { target.url.stopAccessingSecurityScopedResource() } }
                let assets = SonderMediaParser.localAssets(near: target.url)
                let probe = await SonderMediaProbe.probe(url: target.url)
                await collector.append(SonderAssetRefreshResult(
                    itemID: target.id,
                    assetUpdate: SonderLocalAssetUpdate(
                        posterPath: assets.poster?.path,
                        backdropPath: assets.backdrop?.path,
                        subtitlePaths: assets.subtitles.map(\.path)
                    ),
                    probe: probe
                ))
            }

            let results = await collector.all()
            let finalAssetUpdates = Dictionary(results.map { ($0.itemID, $0.assetUpdate) }, uniquingKeysWith: { _, latest in latest })
            let finalProbeResults = Dictionary(results.map { ($0.itemID, $0.probe) }, uniquingKeysWith: { _, latest in latest })
            let finalUpdateCount = results.count
            await MainActor.run {
                for index in self.items.indices {
                    let id = self.items[index].id
                    if let update = finalAssetUpdates[id] {
                        self.items[index].localPosterPath = update.posterPath
                        self.items[index].localBackdropPath = update.backdropPath
                        self.items[index].subtitlePaths = update.subtitlePaths
                    }
                    if let probe = finalProbeResults[id] {
                        if probe.durationSeconds > 0 {
                            self.items[index].durationSeconds = probe.durationSeconds
                        }
                        self.items[index].probedWidth = probe.width
                        self.items[index].probedHeight = probe.height
                        self.items[index].probedCodec = probe.codec
                        self.items[index].probedBitrate = probe.bitrate
                    }
                }
                if showsProgress {
                    self.scanProgress = nil
                    self.addActivity("Refreshed local assets", detail: "\(finalUpdateCount) title(s) checked for posters, subtitles, and runtime.", icon: "photo")
                } else {
                    self.prioritizedAssetRefreshIDs.subtract(probeTargets.map(\.id))
                }
                self.commitLibraryMutation()
                if showsProgress {
                    self.refreshMetadata(for: targets.map(\.id))
                }
            }
        }
    }

    func importFiles(_ result: Result<[URL], Error>) {
        guard case .success(let urls) = result else {
            addActivity("Import failed", detail: "The selected files could not be read.", icon: "exclamationmark.triangle")
            return
        }

        var importedCount = 0
        var importedIDs: [UUID] = []
        for url in urls {
            do {
                let managedURL = try store.copyIntoManagedStorage(url, storagePath: storagePath, storageBookmark: storageBookmark)
                let format = SonderMediaFormat(url: managedURL)
                let parsed = SonderMediaParser.parse(url: managedURL)
                let item = SonderMediaItem(
                    title: parsed.title,
                    subtitle: parsed.subtitle,
                    kind: parsed.kind,
                    studio: "Local",
                    year: parsed.year ?? Calendar.current.component(.year, from: Date()),
                    durationSeconds: parsed.kind == .ebook ? 0 : 5400,
                    format: format,
                    tags: ["imported", format.rawValue.lowercased()],
                    summary: parsed.kind == .ebook ? "Imported book copied into Sonder's managed library storage." : parsed.kind == .audiobook ? "Imported audiobook copied into Sonder's managed library storage." : "Imported video copied into Sonder's managed library storage.",
                    sourcePath: managedURL.path,
                    showTitle: parsed.showTitle,
                    seasonNumber: parsed.season,
                    episodeNumber: parsed.episode
                )
                items.insert(item, at: 0)
                importedIDs.append(item.id)
                importedCount += 1
                // Probe real duration/dimensions off the main actor; merge by id when done.
                let itemID = item.id
                Task { [weak self] in
                    guard let self else { return }
                    let didAccess = managedURL.startAccessingSecurityScopedResource()
                    defer { if didAccess { managedURL.stopAccessingSecurityScopedResource() } }
                    let probe = await SonderMediaProbe.probe(url: managedURL)
                    await MainActor.run {
                        self.applyProbe(probe, toItemID: itemID)
                    }
                }
            } catch {
                addActivity("Import failed", detail: "\(url.lastPathComponent): \(error.localizedDescription)", icon: "exclamationmark.triangle")
            }
        }
        if importedCount > 0 {
            addActivity("Imported media", detail: "\(importedCount) item(s) copied into managed storage.", icon: "square.and.arrow.down")
            commitLibraryMutation()
            refreshMetadata(for: importedIDs)
        }
    }

    /// Merges a probed media result into the item identified by `itemID`, preserving
    /// the duration stored by the user's watch progress when one exists. Looks up by id
    /// so it stays correct even if the array reorders between probe start and finish.
    private func applyProbe(_ probe: SonderMediaProbe.Result, toItemID itemID: UUID) {
        guard let index = items.firstIndex(where: { $0.id == itemID }) else { return }
        items[index].durationSeconds = probe.durationSeconds > 0 ? probe.durationSeconds : items[index].durationSeconds
        items[index].probedWidth = probe.width
        items[index].probedHeight = probe.height
        items[index].probedCodec = probe.codec
        items[index].probedBitrate = probe.bitrate
        if let progressIndex = progressRecords.firstIndex(where: { $0.itemID == itemID }), probe.durationSeconds > 0 {
            progressRecords[progressIndex].duration = probe.durationSeconds
        }
        commitLibraryMutation()
    }

    func refreshMetadata(for itemIDs: [UUID]? = nil) {
        let targetIDs = itemIDs.map(Set.init)
        let targets = items.filter { item in
            let matchesTarget = targetIDs?.contains(item.id) ?? true
            return matchesTarget && item.isPlaceholder == false && item.needsMetadataRefresh
        }
        guard targets.isEmpty == false else { return }
        Task.detached { [store] in
            let enricher = SonderMetadataEnricher(cacheRoot: store.rootURL.appendingPathComponent("MetadataCache", isDirectory: true))
            for item in targets {
                if let enrichment = await enricher.enrich(item: item) {
                    await MainActor.run {
                        self.applyMetadata(enrichment, toItemID: item.id)
                    }
                }
            }
        }
    }

    private func applyMetadata(_ enrichment: SonderMetadataEnrichment, toItemID itemID: UUID) {
        guard let index = items.firstIndex(where: { $0.id == itemID }) else { return }
        if let summary = enrichment.summary, items[index].summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || items[index].summary.contains("Imported") {
            items[index].summary = summary
        }
        if let studio = enrichment.publisher, items[index].studio == "Local" || items[index].studio == "Remote Library" {
            items[index].studio = studio
        }
        if let posterPath = enrichment.posterPath {
            items[index].localPosterPath = posterPath
        }
        if let backdropPath = enrichment.backdropPath {
            items[index].localBackdropPath = backdropPath
        }
        if let tags = enrichment.tags, tags.isEmpty == false {
            items[index].tags = Array(Set(items[index].tags + tags)).sorted()
        }
        commitLibraryMutation()
    }

    func search(_ query: String, kind: SonderMediaKind?, tag: String?) -> [SonderMediaItem] {
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        return items.filter { item in
            let kindMatch = kind == nil || item.kind == kind
            let tagMatch = tag == nil || item.tags.contains(tag ?? "")
            let queryMatch = trimmed.isEmpty || item.searchText.localizedCaseInsensitiveContains(trimmed)
            return kindMatch && tagMatch && queryMatch
        }
    }

    func item(id: UUID) -> SonderMediaItem? {
        items.first { $0.id == id }
    }

    func audiobookItems() -> [SonderMediaItem] {
        items.filter { $0.kind == .audiobook }
    }

    func audiobookItem(id: UUID) -> SonderMediaItem? {
        items.first { $0.id == id && $0.kind == .audiobook }
    }

    func audiobookMetadata(for itemID: UUID) -> (author: String?, series: String?, narrator: String?)? {
        guard let item = audiobookItem(id: itemID), let sourcePath = item.sourcePath else { return nil }
        let mediaURL = URL(fileURLWithPath: sourcePath)
        return SonderAudiobookImporter.metadata(for: item, mediaURL: mediaURL)
    }

    func audiobookChapters(for itemID: UUID) -> [SonderAudiobookChapterRecord] {
        guard let item = audiobookItem(id: itemID), let sourcePath = item.sourcePath else { return [] }
        if let cached = loadCachedAudiobookChapters(for: itemID), cached.isEmpty == false {
            return cached
        }
        let mediaURL = URL(fileURLWithPath: sourcePath)
        let folderCandidates = [
            mediaURL.deletingLastPathComponent(),
            mediaURL.deletingLastPathComponent().deletingLastPathComponent()
        ]
        let fileNames = [
            mediaURL.deletingPathExtension().lastPathComponent + ".chapters.json",
            "chapters.json"
        ]
        for folder in folderCandidates {
            for name in fileNames {
                let candidate = folder.appendingPathComponent(name)
                guard FileManager.default.fileExists(atPath: candidate.path),
                      let data = try? Data(contentsOf: candidate),
                      let decoded = try? JSONDecoder.sonder.decode(SonderAudiobookChapterFile.self, from: data) else { continue }
                return decoded.chapters
            }
        }
        return []
    }

    private func loadCachedAudiobookChapters(for itemID: UUID) -> [SonderAudiobookChapterRecord]? {
        let cacheURL = store.rootURL.appendingPathComponent("AudiobookImport", isDirectory: true).appendingPathComponent("audiobook-index.json")
        guard let data = try? Data(contentsOf: cacheURL),
              let index = try? JSONDecoder.sonder.decode(SonderAudiobookIndex.self, from: data),
              let entry = index.items.first(where: { $0.itemID == itemID }) else {
            return nil
        }
        return entry.chapters
    }

    func prioritizeAssets(for itemID: UUID) {
        let candidateIDs = priorityAssetTargetIDs(for: itemID)
        let newIDs = candidateIDs.filter { prioritizedAssetRefreshIDs.insert($0).inserted }
        guard newIDs.isEmpty == false else { return }
        let limitedIDs = Array(newIDs.prefix(Self.maxPriorityAssetRefreshItems))
        refreshLocalAssets(for: limitedIDs, showsProgress: false)
    }

    private func priorityAssetTargetIDs(for itemID: UUID) -> [UUID] {
        guard let item = item(id: itemID) else { return [] }
        if item.kind == .tvShow {
            let showTitle = (item.showTitle ?? item.title).cleanedMediaTitle.lowercased()
            return items
                .filter { candidate in
                    candidate.kind == .tvShow &&
                    candidate.sourcePath != nil &&
                    ((candidate.showTitle ?? candidate.title).cleanedMediaTitle.lowercased() == showTitle) &&
                    (candidate.localPosterPath == nil || candidate.localBackdropPath == nil || candidate.probedWidth == nil)
                }
                .prefix(Self.maxPriorityAssetRefreshItems)
                .map(\.id)
        }
        guard item.sourcePath != nil else { return [] }
        guard item.localPosterPath == nil || item.localBackdropPath == nil || item.probedWidth == nil else { return [] }
        return [item.id]
    }

    /// Returns a `Sendable` snapshot of everything the HTTP server needs to stream a
    /// title, so file I/O and socket writes can run entirely off the main actor.
    func streamTarget(id: UUID) -> SonderStreamTarget? {
        guard let item = item(id: id) else { return nil }
        prioritizeAssets(for: id)
        return SonderStreamTarget(contentType: item.contentType, sourceBookmark: item.sourceBookmark, sourcePath: item.sourcePath)
    }

    /// Snapshot of auth/server state the background HTTP queue reads to enforce the
    /// pairing gate on LAN requests. `Sendable` so it can cross actor boundaries.
    func serverAuthSnapshot() -> SonderAuthSnapshot {
        SonderAuthSnapshot(
            allowLAN: serverSettings.allowLAN,
            pairingToken: serverSettings.pairingToken
        )
    }

    func play(_ item: SonderMediaItem) {
        prioritizeAssets(for: item.id)
        guard let url = item.playableURL else {
            addActivity("Play unavailable", detail: "\(item.title) needs a local media file.", icon: "exclamationmark.triangle")
            return
        }
        _ = url.startAccessingSecurityScopedResource()
        NSWorkspace.shared.open(url)
        addActivity("Opened \(item.title)", detail: url.lastPathComponent, icon: "play.fill")
    }

    func progress(for item: SonderMediaItem) -> Double {
        progressRecord(for: item)?.percent ?? item.progress
    }

    func progressRecord(for item: SonderMediaItem) -> SonderProgress? {
        progressRecords.first { $0.itemID == item.id }
    }

    func progressLabel(for item: SonderMediaItem) -> String {
        let record = progressRecord(for: item)
        let seconds = record?.seconds ?? item.progressSeconds
        let duration = record?.duration ?? item.durationSeconds
        return "\(SonderTime.format(seconds)) of \(SonderTime.format(duration))"
    }

    func updateProgress(itemID: UUID, seconds: Double, duration: Double) {
        let sanitizedDuration = max(duration, 1)
        let sanitizedSeconds = min(max(seconds, 0), sanitizedDuration)
        if let index = progressRecords.firstIndex(where: { $0.itemID == itemID }) {
            progressRecords[index].seconds = sanitizedSeconds
            progressRecords[index].duration = sanitizedDuration
            progressRecords[index].updatedAt = Date()
        } else {
            progressRecords.append(SonderProgress(itemID: itemID, seconds: sanitizedSeconds, duration: sanitizedDuration))
        }
        if let item = item(id: itemID) {
            addActivity("Updated progress", detail: "\(item.title) is now \(Int((sanitizedSeconds / sanitizedDuration) * 100))% watched.", icon: "chart.line.uptrend.xyaxis")
        }
        commitLibraryMutation()
    }

    func createCollection(named name: String, kind: SonderCollectionKind = .collection) {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return }
        collections.insert(SonderCollection(name: trimmed, kind: kind, itemIDs: []), at: 0)
        addActivity("Created \(kind.label.lowercased())", detail: trimmed, icon: kind.icon)
        commitLibraryMutation()
    }

    func renameForPlex(_ item: SonderMediaItem) {
        guard let index = items.firstIndex(where: { $0.id == item.id }),
              let sourceURL = item.playableURL else {
            addActivity("Rename failed", detail: "\(item.title) has no local file.", icon: "exclamationmark.triangle")
            return
        }

        let destination = sourceURL.deletingLastPathComponent().appendingPathComponent(item.plexFileName)
        do {
            let finalURL = try store.moveAvoidingCollision(from: sourceURL, to: destination)
            items[index].sourcePath = finalURL.path
            items[index].format = SonderMediaFormat(url: finalURL)
            addActivity("Renamed for Plex", detail: finalURL.lastPathComponent, icon: "textformat")
            commitLibraryMutation()
        } catch {
            addActivity("Rename failed", detail: error.localizedDescription, icon: "exclamationmark.triangle")
        }
    }

    /// Maximum number of `AVAssetExportSession`s to run concurrently. Without a cap,
    /// `convertAllPlayableToMP4` used to fan out one Task per title (200 in a large
    /// library), which exhausts GPU/memory and stalls the system.
    private static let maxConcurrentConversions = 2

    func convertAllPlayableToMP4() {
        let candidates = items.filter { $0.hasFile && $0.format != .mp4 }
        guard candidates.isEmpty == false else {
            addActivity("Conversion skipped", detail: "No playable non-MP4 titles to convert.", icon: "checkmark.circle")
            return
        }
        addActivity("Queueing conversions", detail: "\(candidates.count) title(s), \(Self.maxConcurrentConversions) at a time.", icon: "arrow.triangle.2.circlepath")
        Task { [weak self] in
            guard let self else { return }
            await Self.runWithConcurrencyLimit(Self.maxConcurrentConversions, over: candidates) { item in
                await MainActor.run { self.convertToMP4(item) }
            }
        }
    }

    /// Runs `work` over `items` with at most `limit` concurrent invocations. Used to
    /// bound export-session fan-out. Each work closure awaits completion before the
    /// slot is released.
    private nonisolated static func runWithConcurrencyLimit<Item>(
        _ limit: Int,
        over items: [Item],
        work: @escaping @Sendable (Item) async -> Void
    ) async {
        precondition(limit > 0)
        await withTaskGroup(of: Void.self) { group in
            var iterator = items.makeIterator()
            for _ in 0..<min(limit, items.count) {
                guard let item = iterator.next() else { break }
                group.addTask { await work(item) }
            }
            while await group.next() != nil {
                guard let item = iterator.next() else { continue }
                group.addTask { await work(item) }
            }
        }
    }

    func convertToMP4(_ item: SonderMediaItem) {
        guard let sourceURL = item.playableURL else {
            addActivity("Conversion unavailable", detail: "Select a playable file first.", icon: "exclamationmark.triangle")
            return
        }
        guard item.format != .mp4 else {
            addActivity("Conversion skipped", detail: "\(item.title) is already MP4.", icon: "checkmark.circle")
            return
        }

        let outputURL = store.availableConversionURL(for: sourceURL)
        let job = SonderConversionJob(title: item.title, detail: "\(sourceURL.lastPathComponent) -> \(outputURL.lastPathComponent)", status: .running)
        conversionJobs.insert(job, at: 0)
        commitLibraryMutation()

        Task {
            let result = await store.convertToMP4(sourceURL: sourceURL, outputURL: outputURL)
            await MainActor.run {
                if let jobIndex = self.conversionJobs.firstIndex(where: { $0.id == job.id }) {
                    self.conversionJobs[jobIndex].status = result ? .completed : .failed
                }
                if result, let itemIndex = self.items.firstIndex(where: { $0.id == item.id }) {
                    self.items[itemIndex].sourcePath = outputURL.path
                    self.items[itemIndex].format = .mp4
                    self.addActivity("Converted to MP4", detail: outputURL.lastPathComponent, icon: "checkmark.circle")
                } else if result == false {
                    self.addActivity("Conversion failed", detail: "\(sourceURL.lastPathComponent). Use a companion HandBrake workflow for unsupported codecs.", icon: "exclamationmark.triangle")
                }
                self.commitLibraryMutation()
            }
        }
    }

    func add(_ item: SonderMediaItem, to collection: SonderCollection) {
        guard let index = collections.firstIndex(where: { $0.id == collection.id }) else { return }
        if collections[index].itemIDs.contains(item.id) == false {
            collections[index].itemIDs.append(item.id)
            addActivity("Added to collection", detail: "\(item.title) -> \(collection.name)", icon: "plus.circle")
            commitLibraryMutation()
        }
    }

    private func addActivity(_ title: String, detail: String, icon: String) {
        activity.insert(SonderActivityEvent(title: title, detail: detail, icon: icon), at: 0)
        activity = Array(activity.prefix(60))
    }

    /// Invalidates the HTTP cache so the next request rebuilds the serialised
    /// library/discovery response.  Called by every mutation path.
    private func invalidateHTTPCache() {
        httpCache.invalidate()
    }

    /// Central mutation hook for library state. It refreshes derived data, invalidates
    /// HTTP response caches, and optionally schedules a coalesced background persist.
    private func commitLibraryMutation(persist: Bool = true) {
        rebuildDerivedData()
        invalidateHTTPCache()
        guard persist else { return }
        saveWorkItem?.cancel()
        let snapshot = SonderSnapshot(
            items: items,
            progress: progressRecords,
            collections: collections,
            activity: activity,
            storagePath: storagePath,
            storageBookmark: storageBookmark,
            conversionJobs: conversionJobs,
            libraryDefinitions: libraryDefinitions,
            mediaDirectories: mediaDirectories,
            serverSettings: serverSettings
        )
        let workItem = DispatchWorkItem { [store = self.store] in
            store.save(snapshot)
        }
        saveWorkItem = workItem
        // 500 ms coalescing window — plenty for rapid progress updates without I/O pressure
        saveQueue.asyncAfter(deadline: .now() + 0.5, execute: workItem)
    }

    private func libraryID(for kind: SonderLibraryImportKind) -> UUID {
        libraryDefinitions.first { $0.kind == kind }?.id ?? kind.defaultLibraryID
    }

    private static func migrateDirectories(_ directories: [SonderMediaDirectory], libraries: [SonderLibraryDefinition]) -> [SonderMediaDirectory] {
        directories.map { directory in
            var copy = directory
            if libraries.contains(where: { $0.id == copy.libraryID }) == false {
                copy.libraryID = libraries.first { $0.kind == copy.kind }?.id ?? copy.kind.defaultLibraryID
            }
            return copy
        }
    }
}

nonisolated private struct SonderDerivedData {
    var playableCount: Int
    var averageProgress: Double
    var allTags: [String]
    var tagUsage: [(tag: String, count: Int)]
    var mediaKindCounts: [SonderMediaKind: Int]
    var inProgressItems: [SonderMediaItem]
    var tvShowGroups: [SonderTVShowGroup]
}

nonisolated struct SonderSnapshot: Codable {
    var items: [SonderMediaItem]
    var progress: [SonderProgress]
    var collections: [SonderCollection]
    var activity: [SonderActivityEvent]
    var storagePath: String?
    var storageBookmark: Data?
    var conversionJobs: [SonderConversionJob]?
    var libraryDefinitions: [SonderLibraryDefinition]?
    var mediaDirectories: [SonderMediaDirectory]?
    var serverSettings: SonderServerSettings?

    static let startupPlaceholder = SonderSnapshot(
        items: [],
        progress: [],
        collections: [],
        activity: [],
        storagePath: nil,
        storageBookmark: nil,
        conversionJobs: [],
        libraryDefinitions: nil,
        mediaDirectories: [],
        serverSettings: nil
    )

    static let seeded = startupPlaceholder
}

#Preview {
    ContentView()
        .environmentObject(SonderLibrary(store: SonderStore(rootURL: FileManager.default.temporaryDirectory.appendingPathComponent("SonderPreview"))))
}
