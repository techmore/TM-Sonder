import SwiftUI

struct MoviesView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid

    private var items: [SonderMediaItem] {
        library.items
            .filter { $0.kind == .movie && ($0.libraryID == nil || $0.libraryID == SonderLibraryImportKind.movies.defaultLibraryID) }
            .sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    var body: some View {
        MediaKindBrowser(
            title: "Movies",
            emptyText: "Add movie files or a Movies remote library to browse them here.",
            items: items,
            detail: { library.progressLabel(for: $0) },
            library: library,
            selectedItemID: $selectedItemID,
            displayMode: $displayMode
        )
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

struct CustomLibraryView: View {
    @ObservedObject var library: SonderLibrary
    let definition: SonderLibraryDefinition
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid
    @State private var selectedShowName: String?

    private var items: [SonderMediaItem] {
        library.items
            .filter { $0.libraryID == definition.id }
            .sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    private var showGroups: [SonderTVShowGroup] {
        SonderDerivedData.make(items: items, progressRecords: library.progressRecords).tvShowGroups
    }

    private var selectedShow: SonderTVShowGroup? {
        guard let selectedShowName else { return nil }
        return showGroups.first { $0.name == selectedShowName }
    }

    var body: some View {
        if definition.kind == .tvShows {
            CustomTVLibraryView(
                title: definition.name,
                showGroups: showGroups,
                selectedShow: selectedShow,
                selectedShowName: $selectedShowName,
                selectedItemID: $selectedItemID,
                displayMode: $displayMode,
                library: library
            )
        } else {
            MediaKindBrowser(
                title: definition.name,
                emptyText: "Add a folder to this custom library to browse it here.",
                items: items,
                detail: { library.progressLabel(for: $0) },
                library: library,
                selectedItemID: $selectedItemID,
                displayMode: $displayMode
            )
        }
    }
}

struct CustomTVLibraryView: View {
    let title: String
    let showGroups: [SonderTVShowGroup]
    let selectedShow: SonderTVShowGroup?
    @Binding var selectedShowName: String?
    @Binding var selectedItemID: UUID?
    @Binding var displayMode: LibraryDisplayMode
    @ObservedObject var library: SonderLibrary
    @State private var searchText = ""

    private var filteredShows: [SonderTVShowGroup] {
        let trimmed = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return showGroups }
        return showGroups.compactMap { show in
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
                    libraryModeBar(title: title, count: filteredShows.count, countLabel: "shows", mode: $displayMode, searchText: $searchText, searchPrompt: "Search \(title.lowercased())")
                    switch displayMode {
                    case .grid:
                        ScrollView {
                            if filteredShows.isEmpty {
                                EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add TV-style folders to browse shows here." : "No shows match this search.")
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
                                EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add TV-style folders to browse shows here." : "No shows match this search.")
                            }
                        }
                        .scrollContentBackground(.hidden)
                    }
                }
            }
        }
        .navigationTitle(selectedShow?.name ?? title)
        .background(SonderTheme.background)
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

func libraryModeBar(
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
