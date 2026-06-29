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
                    SonderSystemServices.shared.openLocalWebInterface(port: UInt16(clamping: library.serverSettings.port))
                } label: {
                    Label("Web", systemImage: "network")
                }
                .help("Open Sonder's local web interface in your browser.")

                Button {
                    showingImporter = true
                } label: {
                    Label("Import", systemImage: "square.and.arrow.down")
                }
                .help("Import media files into Sonder's library.")

                Button {
                    showingStoragePicker = true
                } label: {
                    Label("Storage", systemImage: "externaldrive")
                }
                .help("Choose where imported media is stored.")
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
        case .movies:
            MoviesView(library: library, selectedItemID: $selectedItemID)
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
        case .log:
            ActivityLogView(library: library)
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
            Label("Movies", systemImage: "film")
                .tag(SonderSection.movies)
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
            Label("Log", systemImage: "text.bubble")
                .tag(SonderSection.log)
            Label("About", systemImage: "info.circle")
                .tag(SonderSection.about)

            Section {
                HStack(spacing: 10) {
                    if library.isBusy {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Image(systemName: "checkmark.circle")
                            .foregroundStyle(SonderTheme.textLight)
                    }
                    VStack(alignment: .leading, spacing: 2) {
                        Text(library.isBusy ? "Working" : "Idle")
                        Text(library.isBusy ? "Discovery, import, or scan in progress" : "Ready for import and discovery")
                            .font(.caption2)
                            .foregroundStyle(SonderTheme.textLight)
                    }
                }
                .padding(.vertical, 2)
            }

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

struct ActivityLogView: View {
    @ObservedObject var library: SonderLibrary

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                PageHeader(
                    title: "Log",
                    subtitle: "A running feed of imports, discovery, conversions, and library mutations."
                )

                DashboardPanel(title: "Recent Activity") {
                    if library.activity.isEmpty {
                        Text("No activity yet. Start an import, scan, or discovery pass and it will appear here.")
                            .foregroundStyle(SonderTheme.textLight)
                            .fixedSize(horizontal: false, vertical: true)
                    } else {
                        LazyVStack(alignment: .leading, spacing: 10) {
                            ForEach(library.activity) { event in
                                HStack(alignment: .top, spacing: 12) {
                                    Image(systemName: event.icon)
                                        .frame(width: 18)
                                        .foregroundStyle(SonderTheme.accentStrong)
                                    VStack(alignment: .leading, spacing: 3) {
                                        HStack {
                                            Text(event.title)
                                                .font(.headline)
                                            Spacer()
                                            Text(event.date.formatted(date: .abbreviated, time: .shortened))
                                                .font(.caption2.monospacedDigit())
                                                .foregroundStyle(SonderTheme.textLight)
                                        }
                                        Text(event.detail)
                                            .foregroundStyle(SonderTheme.textLight)
                                            .fixedSize(horizontal: false, vertical: true)
                                    }
                                }
                                .padding(.vertical, 4)
                                if event.id != library.activity.last?.id {
                                    Divider()
                                }
                            }
                        }
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
    }
}
