import SwiftUI

struct LibraryView: View {
    let items: [SonderMediaItem]
    let allTags: [String]
    @Binding var searchText: String
    @Binding var selectedKind: SonderMediaKind?
    @Binding var selectedTag: String?
    @Binding var selectedItemID: UUID?
    @ObservedObject var library: SonderLibrary
    @State private var cardMinimumWidth = 230.0
    @State private var selectedShowName: String?

    private var posterHeight: CGFloat {
        CGFloat(cardMinimumWidth * 1.44)
    }

    private var nonTVItems: [SonderMediaItem] {
        items.filter { $0.kind != .tvShow }
    }

    private var tvShowGroups: [SonderTVShowGroup] {
        SonderDerivedData.make(items: items, progressRecords: library.progressRecords).tvShowGroups
    }

    private var selectedShow: SonderTVShowGroup? {
        guard let selectedShowName else { return nil }
        return tvShowGroups.first { $0.name == selectedShowName }
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
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 18) {
                        catalogHeader
                        filterBar

                        if tvShowGroups.isEmpty && nonTVItems.isEmpty {
                            EmptyLibraryMessage(text: "No titles match the current filters.")
                        }

                        if tvShowGroups.isEmpty == false {
                            LibrarySectionHeader(title: "TV Shows", detail: "\(tvShowGroups.count) shows")
                            LazyVGrid(columns: [GridItem(.adaptive(minimum: cardMinimumWidth, maximum: cardMinimumWidth + 28), spacing: 18)], spacing: 18) {
                                ForEach(tvShowGroups) { show in
                                    TVShowCard(show: show, library: library) {
                                        selectedShowName = show.name
                                    } play: { item in
                                        library.play(item)
                                    }
                                }
                            }
                        }

                        if nonTVItems.isEmpty == false {
                            LibrarySectionHeader(title: selectedKind == nil ? "Titles" : selectedKind?.label ?? "Titles", detail: "\(nonTVItems.count) titles")
                            LazyVGrid(columns: [GridItem(.adaptive(minimum: cardMinimumWidth, maximum: cardMinimumWidth + 28), spacing: 18)], spacing: 18) {
                                ForEach(nonTVItems) { item in
                                    MediaCard(item: item, progress: library.progress(for: item), posterHeight: posterHeight) {
                                        selectedItemID = item.id
                                    } play: {
                                        library.play(item)
                                    }
                                }
                            }
                        }
                    }
                    .padding(20)
                }
            }
        }
        .background(SonderTheme.background)
        .navigationTitle(selectedShow?.name ?? "Library")
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

private struct LibrarySectionHeader: View {
    let title: String
    let detail: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(title)
                .font(.title3.weight(.semibold))
                .foregroundStyle(SonderTheme.text)
            Spacer()
            Text(detail)
                .font(.caption.monospacedDigit())
                .foregroundStyle(SonderTheme.textLight)
        }
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
