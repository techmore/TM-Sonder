import AppKit
import AVFoundation
import Combine
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
        } content: {
            contentView
                .navigationSplitViewColumnWidth(min: 780, ideal: 900, max: 1120)
        } detail: {
            detailView
                .navigationSplitViewColumnWidth(min: 430, ideal: 520, max: 680)
        }
        .tint(SonderTheme.accent)
        .foregroundStyle(SonderTheme.text)
        .frame(minWidth: 1320, idealWidth: 1480, minHeight: 820, idealHeight: 920)
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
                    NSWorkspace.shared.open(URL(string: "http://127.0.0.1:8797")!)
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

    @ViewBuilder
    private var detailView: some View {
        switch selection ?? .library {
        case .server:
            EmptyDetailView(title: "Local Server", subtitle: "The web interface and API are hosted from this Mac on port 8797.", icon: "network")
        case .about:
            EmptyDetailView(title: "TM Sonder", subtitle: "A private macOS media server for movies, shows, documentaries, and watched progress.", icon: "play.tv")
        default:
            if let selectedItemID, let item = library.item(id: selectedItemID) {
                MediaDetailView(item: item, library: library)
            } else {
                EmptyDetailView(title: "Select Media", subtitle: "Choose a title to inspect metadata, progress, stream URL, and collection options.", icon: "film")
            }
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
                Text("Local movies, shows, and documentaries streamed from this Mac")
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

struct TVShowsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?

    var body: some View {
        List {
            ForEach(library.tvShowGroups) { show in
                Section {
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
                } header: {
                    HStack {
                        Text(show.name)
                        Spacer()
                        Text("\(show.episodeCount) episodes")
                            .font(.caption.monospacedDigit())
                    }
                }
            }
            if library.tvShowGroups.isEmpty {
                Text("Add a TV Shows library folder to group episodes by show and season.")
                    .foregroundStyle(SonderTheme.textLight)
            }
        }
        .navigationTitle("TV Shows")
        .scrollContentBackground(.hidden)
        .background(SonderTheme.background)
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
                            let count = library.items.filter { $0.kind == kind }.count
                            MeterRow(label: kind.label, value: count, max: max(1, library.items.count))
                        }
                    }

                    DashboardPanel(title: "Popular Tags") {
                        ForEach(library.tagUsage.prefix(10), id: \.tag) { item in
                            MeterRow(label: item.tag, value: item.count, max: max(1, library.tagUsage.first?.count ?? 1))
                        }
                    }
                }

                DashboardPanel(title: "Endpoints") {
                    EndpointRow(label: "Web", value: "http://127.0.0.1:8797")
                    EndpointRow(label: "Health", value: "http://127.0.0.1:8797/api/health")
                    EndpointRow(label: "Library JSON", value: "http://127.0.0.1:8797/api/library")
                    EndpointRow(label: "Stream Route", value: "http://127.0.0.1:8797/stream/{id}")
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

                DashboardPanel(title: "Storage") {
                    EndpointRow(label: "Media", value: library.storagePath)
                    Text("Use the Storage toolbar button to choose a mounted NAS, external disk, or synced folder. New imports copy there; the app database remains in Application Support.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)
                }

                DashboardPanel(title: "Remote Libraries") {
                    HStack {
                        Label("\(library.libraryDefinitions.count) libraries / \(library.mediaDirectories.count) folders", systemImage: "network")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
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
                            library.rescanMediaDirectories()
                        } label: {
                            Label("Rescan", systemImage: "arrow.clockwise")
                        }
                        .disabled(library.mediaDirectories.isEmpty)
                    }

                    Text("Select mounted SMB/NFS/NAS folders through the system picker. Sonder stores security-scoped bookmarks and only scans directories the user grants.")
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
                    bodyText: "The app catalogs movies, television, and documentaries, tracks watched position, groups titles into collections, and serves playable files through HTTP routes."
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
                                NSWorkspace.shared.open(URL(string: "http://127.0.0.1:8797/stream/\(item.id.uuidString)")!)
                            } label: {
                                Label("Stream URL", systemImage: "network")
                            }
                            .disabled(item.hasFile == false)
                        }

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
                    MetadataRow(label: "Runtime", value: SonderTime.format(item.durationSeconds))
                    MetadataRow(label: "Format", value: item.format.rawValue.uppercased())
                    if item.kind == .tvShow {
                        MetadataRow(label: "Show", value: item.showTitle ?? item.title)
                        MetadataRow(label: "Episode", value: item.episodeCode)
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
        .onChange(of: item.id) { _, _ in
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

    private let store: SonderStore

    static let defaultCollections = [
        SonderCollection(name: "Saturday Feature Queue", kind: .playlist, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .movie }.prefix(4).map(\.id))),
        SonderCollection(name: "Documentary Shelf", kind: .collection, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .documentary }.map(\.id))),
        SonderCollection(name: "Shows in Rotation", kind: .playlist, itemIDs: Array(SonderSeed.catalog.filter { $0.kind == .tvShow }.map(\.id)))
    ]

    init(store: SonderStore? = nil) {
        let resolvedStore = store ?? SonderStore()
        self.store = resolvedStore
        let snapshot = resolvedStore.load()
        items = snapshot.items
        progressRecords = snapshot.progress
        collections = snapshot.collections
        activity = snapshot.activity
        conversionJobs = snapshot.conversionJobs ?? []
        let loadedLibraryDefinitions = snapshot.libraryDefinitions ?? SonderLibraryDefinition.defaults
        libraryDefinitions = loadedLibraryDefinitions
        mediaDirectories = Self.migrateDirectories(snapshot.mediaDirectories ?? [], libraries: loadedLibraryDefinitions)
        serverSettings = snapshot.serverSettings ?? .default
        storageBookmark = snapshot.storageBookmark
        storagePath = resolvedStore.storageURL(from: snapshot.storageBookmark)?.path ?? snapshot.storagePath ?? resolvedStore.uploadsURL.path
    }

    var playableCount: Int {
        items.filter(\.hasFile).count
    }

    var inProgressItems: [SonderMediaItem] {
        progressRecords
            .filter { $0.seconds > 0 && $0.percent < 0.96 }
            .sorted { $0.updatedAt > $1.updatedAt }
            .compactMap { item(id: $0.itemID) }
    }

    var averageProgress: Double {
        guard progressRecords.isEmpty == false else { return 0 }
        return progressRecords.map(\.percent).reduce(0, +) / Double(progressRecords.count)
    }

    var allTags: [String] {
        Array(Set(items.flatMap(\.tags))).sorted()
    }

    var tagUsage: [(tag: String, count: Int)] {
        Dictionary(grouping: items.flatMap(\.tags), by: { $0 })
            .map { ($0.key, $0.value.count) }
            .sorted { $0.count > $1.count }
    }

    var tvShowGroups: [SonderTVShowGroup] {
        let episodes = items
            .filter { $0.kind == .tvShow }
            .sorted {
                (($0.showTitle ?? $0.title), $0.seasonNumber ?? 0, $0.episodeNumber ?? 0, $0.title)
                    < (($1.showTitle ?? $1.title), $1.seasonNumber ?? 0, $1.episodeNumber ?? 0, $1.title)
            }
        return Dictionary(grouping: episodes, by: { $0.showTitle ?? $0.title })
            .map { showName, showEpisodes in
                let seasons = Dictionary(grouping: showEpisodes, by: { $0.seasonNumber ?? 0 })
                    .map { seasonNumber, seasonEpisodes in
                        SonderTVSeasonGroup(
                            seasonNumber: seasonNumber,
                            episodes: seasonEpisodes.sorted { ($0.episodeNumber ?? 0, $0.title) < ($1.episodeNumber ?? 0, $1.title) }
                        )
                    }
                    .sorted { $0.seasonNumber < $1.seasonNumber }
                return SonderTVShowGroup(name: showName, seasons: seasons)
            }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
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
        save()
    }

    func updateServerSettings(isEnabled: Bool? = nil, allowLAN: Bool? = nil, port: Int? = nil) {
        if let isEnabled {
            serverSettings.isEnabled = isEnabled
        }
        if let allowLAN {
            serverSettings.allowLAN = allowLAN
        }
        if let port {
            serverSettings.port = min(max(port, 1024), 65535)
        }
        addActivity("Updated server settings", detail: serverSettings.statusLabel, icon: "network")
        save()
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
            save()
        } catch {
            addActivity("Storage unchanged", detail: error.localizedDescription, icon: "exclamationmark.triangle")
        }
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

    func addMediaDirectories(_ urls: [URL], kind: SonderLibraryImportKind) {
        var added = 0
        for url in urls {
            do {
                let bookmark = try store.bookmark(for: url)
                if let existingIndex = mediaDirectories.firstIndex(where: { $0.path == url.path && $0.kind == kind }) {
                    mediaDirectories[existingIndex].bookmark = bookmark
                    mediaDirectories[existingIndex].name = url.lastPathComponent
                    mediaDirectories[existingIndex].libraryID = libraryID(for: kind)
                    added += 1
                } else {
                    mediaDirectories.append(SonderMediaDirectory(name: url.lastPathComponent, path: url.path, bookmark: bookmark, kind: kind, libraryID: libraryID(for: kind)))
                    added += 1
                }
            } catch {
                addActivity("Directory add failed", detail: "\(url.lastPathComponent): \(error.localizedDescription)", icon: "exclamationmark.triangle")
            }
        }

        if added > 0 {
            addActivity("Added media directories", detail: "\(added) folder(s) ready to scan.", icon: "folder.badge.plus")
            rescanMediaDirectories()
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
        scanProgress = SonderScanProgress(phase: .fastTitles, title: "Preparing scan", detail: "Starting fast title index.", filesSeen: 0, mediaFound: 0, indexedCount: 0, directoriesDone: 0, directoriesTotal: directories.count)

        Task.detached { [store] in
            var indexedItems: [SonderMediaItem] = []
            var directoryUpdates: [UUID: SonderScanDiagnostics] = [:]
            var totalFilesSeen = 0
            var totalMediaFound = 0
            var scannedPaths = existingPaths

            for (offset, directory) in directories.enumerated() {
                let startFilesSeen = totalFilesSeen
                let startMediaFound = totalMediaFound
                let currentIndexedCount = indexedItems.count
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

                do {
                    let scanResult = try await store.mediaFiles(in: directory.path, bookmark: directory.bookmark) { filesSeen, mediaFound, currentPath in
                        let progressFilesSeen = startFilesSeen + filesSeen
                        let progressMediaFound = startMediaFound + mediaFound
                        let progressIndexedCount = currentIndexedCount
                        await MainActor.run {
                            self.scanProgress = SonderScanProgress(
                                phase: .fastTitles,
                                title: "Indexing \(directory.kind.label)",
                                detail: currentPath,
                                filesSeen: progressFilesSeen,
                                mediaFound: progressMediaFound,
                                indexedCount: progressIndexedCount,
                                directoriesDone: offset,
                                directoriesTotal: directories.count
                            )
                        }
                    }

                    totalFilesSeen += scanResult.filesSeen
                    totalMediaFound += scanResult.mediaFound
                    var directoryIndexedCount = 0
                    var skippedDuplicates = 0
                    for scannedFile in scanResult.files {
                        guard scannedPaths.contains(scannedFile.url.path) == false else {
                            skippedDuplicates += 1
                            continue
                        }
                        scannedPaths.insert(scannedFile.url.path)
                        let parsed = SonderMediaParser.parseTitle(url: scannedFile.url, libraryKind: directory.kind)
                        let format = SonderMediaFormat(url: scannedFile.url)
                        indexedItems.append(
                            SonderMediaItem(
                                title: parsed.title,
                                subtitle: parsed.subtitle,
                                kind: parsed.kind,
                                studio: "Remote Library",
                                year: parsed.year ?? Calendar.current.component(.year, from: Date()),
                                durationSeconds: 5400,
                                format: format,
                                tags: ["remote", directory.kind.tag, format.rawValue.lowercased()],
                                summary: "Indexed from a user-selected \(directory.kind.label.lowercased()) directory.",
                                sourcePath: scannedFile.url.path,
                                sourceBookmark: scannedFile.bookmark,
                                showTitle: parsed.showTitle,
                                seasonNumber: parsed.season,
                                episodeNumber: parsed.episode,
                                metadataIDSource: parsed.metadataIDSource,
                                metadataID: parsed.metadataID,
                                edition: parsed.edition,
                                splitPart: parsed.splitPart,
                                localPosterPath: nil,
                                localBackdropPath: nil,
                                subtitlePaths: []
                            )
                        )
                        directoryIndexedCount += 1
                    }
                    directoryUpdates[directory.id] = SonderScanDiagnostics(
                        mediaCount: scanResult.files.count,
                        fileCount: scanResult.filesSeen,
                        unsupportedCount: scanResult.unsupportedMediaCount,
                        skippedDuplicateCount: skippedDuplicates,
                        parseFailureCount: 0,
                        scannedAt: Date()
                    )
                } catch {
                    await MainActor.run {
                        self.addActivity("Directory scan failed", detail: "\(directory.name): \(error.localizedDescription)", icon: "exclamationmark.triangle")
                    }
                }
            }

            let finalIndexedItems = indexedItems
            let finalDirectoryUpdates = directoryUpdates
            let finalIndexedCount = indexedItems.count
            await MainActor.run {
                self.items.append(contentsOf: finalIndexedItems)
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
                self.addActivity("Indexed media titles", detail: "\(finalIndexedCount) new title(s) indexed. Local assets will refresh next.", icon: "arrow.clockwise")
                self.save()
                self.refreshLocalAssets(for: finalIndexedItems.map(\.id), directoriesTotal: directories.count)
            }
        }
    }

    private func refreshLocalAssets(for itemIDs: [UUID]? = nil, directoriesTotal: Int? = nil) {
        let targetIDs = Set(itemIDs ?? items.map(\.id))
        let targets = items.filter { targetIDs.contains($0.id) && $0.sourcePath != nil }
        guard targets.isEmpty == false else {
            scanProgress = nil
            return
        }

        scanProgress = SonderScanProgress(phase: .localAssets, title: "Finding local artwork", detail: "Checking posters and subtitles beside indexed media.", filesSeen: 0, mediaFound: 0, indexedCount: 0, directoriesDone: 0, directoriesTotal: max(directoriesTotal ?? targets.count, 1))

        Task.detached {
            var updates: [UUID: SonderLocalAssetUpdate] = [:]
            for (offset, item) in targets.enumerated() {
                guard let sourcePath = item.sourcePath else { continue }
                let assets = SonderMediaParser.localAssets(near: URL(fileURLWithPath: sourcePath))
                updates[item.id] = SonderLocalAssetUpdate(
                    posterPath: assets.poster?.path,
                    backdropPath: assets.backdrop?.path,
                    subtitlePaths: assets.subtitles.map(\.path)
                )
                if offset.isMultiple(of: 25) {
                    let updateCount = updates.count
                    await MainActor.run {
                        self.scanProgress = SonderScanProgress(
                            phase: .localAssets,
                            title: "Finding local artwork",
                            detail: sourcePath,
                            filesSeen: offset + 1,
                            mediaFound: updateCount,
                            indexedCount: updateCount,
                            directoriesDone: min(offset + 1, targets.count),
                            directoriesTotal: targets.count
                        )
                    }
                }
            }

            let finalUpdates = updates
            let finalUpdateCount = updates.count
            await MainActor.run {
                for index in self.items.indices {
                    if let update = finalUpdates[self.items[index].id] {
                        self.items[index].localPosterPath = update.posterPath
                        self.items[index].localBackdropPath = update.backdropPath
                        self.items[index].subtitlePaths = update.subtitlePaths
                    }
                }
                self.scanProgress = nil
                self.addActivity("Refreshed local assets", detail: "\(finalUpdateCount) title(s) checked for posters and subtitles.", icon: "photo")
                self.save()
            }
        }
    }

    func importFiles(_ result: Result<[URL], Error>) {
        guard case .success(let urls) = result else {
            addActivity("Import failed", detail: "The selected files could not be read.", icon: "exclamationmark.triangle")
            return
        }

        var importedCount = 0
        for url in urls {
            do {
                let managedURL = try store.copyIntoManagedStorage(url, storagePath: storagePath, storageBookmark: storageBookmark)
                let format = SonderMediaFormat(url: managedURL)
                let parsed = SonderMediaParser.parse(url: managedURL)
                items.insert(
                    SonderMediaItem(
                        title: parsed.title,
                        subtitle: parsed.subtitle,
                        kind: parsed.kind,
                        studio: "Local",
                        year: parsed.year ?? Calendar.current.component(.year, from: Date()),
                        durationSeconds: 5400,
                        format: format,
                        tags: ["imported", format.rawValue.lowercased()],
                        summary: "Imported video copied into Sonder's managed library storage.",
                        sourcePath: managedURL.path,
                        showTitle: parsed.showTitle,
                        seasonNumber: parsed.season,
                        episodeNumber: parsed.episode
                    ),
                    at: 0
                )
                importedCount += 1
            } catch {
                addActivity("Import failed", detail: "\(url.lastPathComponent): \(error.localizedDescription)", icon: "exclamationmark.triangle")
            }
        }
        if importedCount > 0 {
            addActivity("Imported media", detail: "\(importedCount) item(s) copied into managed storage.", icon: "square.and.arrow.down")
            save()
        }
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

    func play(_ item: SonderMediaItem) {
        guard let url = item.playableURL else {
            addActivity("Play unavailable", detail: "\(item.title) needs a local media file.", icon: "exclamationmark.triangle")
            return
        }
        _ = url.startAccessingSecurityScopedResource()
        NSWorkspace.shared.open(url)
        addActivity("Opened \(item.title)", detail: url.lastPathComponent, icon: "play.fill")
        save()
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
        save()
    }

    func createCollection(named name: String, kind: SonderCollectionKind = .collection) {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return }
        collections.insert(SonderCollection(name: trimmed, kind: kind, itemIDs: []), at: 0)
        addActivity("Created \(kind.label.lowercased())", detail: trimmed, icon: kind.icon)
        save()
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
            save()
        } catch {
            addActivity("Rename failed", detail: error.localizedDescription, icon: "exclamationmark.triangle")
        }
    }

    func convertAllPlayableToMP4() {
        for item in items where item.hasFile {
            convertToMP4(item)
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
        save()

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
                self.save()
            }
        }
    }

    func add(_ item: SonderMediaItem, to collection: SonderCollection) {
        guard let index = collections.firstIndex(where: { $0.id == collection.id }) else { return }
        if collections[index].itemIDs.contains(item.id) == false {
            collections[index].itemIDs.append(item.id)
            addActivity("Added to collection", detail: "\(item.title) -> \(collection.name)", icon: "plus.circle")
            save()
        }
    }

    private func addActivity(_ title: String, detail: String, icon: String) {
        activity.insert(SonderActivityEvent(title: title, detail: detail, icon: icon), at: 0)
        activity = Array(activity.prefix(60))
    }

    private func save() {
        store.save(SonderSnapshot(items: items, progress: progressRecords, collections: collections, activity: activity, storagePath: storagePath, storageBookmark: storageBookmark, conversionJobs: conversionJobs, libraryDefinitions: libraryDefinitions, mediaDirectories: mediaDirectories, serverSettings: serverSettings))
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

struct SonderSnapshot: Codable {
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

    static let seeded = SonderSnapshot(
        items: SonderSeed.catalog,
        progress: [],
        collections: SonderLibrary.defaultCollections,
        activity: [],
        storagePath: nil,
        storageBookmark: nil,
        conversionJobs: [],
        libraryDefinitions: SonderLibraryDefinition.defaults,
        mediaDirectories: [],
        serverSettings: .default
    )
}

final class SonderStore {
    let rootURL: URL

    init(rootURL: URL? = nil) {
        if let rootURL {
            self.rootURL = rootURL
        } else {
            let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory
            self.rootURL = support.appendingPathComponent("TM Sonder", isDirectory: true)
        }
    }

    var databaseURL: URL {
        rootURL.appendingPathComponent("library.json")
    }

    var uploadsURL: URL {
        rootURL.appendingPathComponent("Media", isDirectory: true)
    }

    func load() -> SonderSnapshot {
        do {
            let data = try Data(contentsOf: databaseURL)
            return sanitize(try JSONDecoder.sonder.decode(SonderSnapshot.self, from: data))
        } catch {
            return .seeded
        }
    }

    func save(_ snapshot: SonderSnapshot) {
        do {
            try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
            let data = try JSONEncoder.sonder.encode(snapshot)
            try data.write(to: databaseURL, options: .atomic)
        } catch {
            assertionFailure("Could not save Sonder library: \(error)")
        }
    }

    func storageURL(from bookmark: Data?) -> URL? {
        guard let bookmark else { return nil }
        var isStale = false
        return try? URL(resolvingBookmarkData: bookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale)
    }

    func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    func copyIntoManagedStorage(_ sourceURL: URL, storagePath: String, storageBookmark: Data?) throws -> URL {
        let mediaURL = storageURL(from: storageBookmark) ?? URL(fileURLWithPath: storagePath, isDirectory: true)
        let didAccessMedia = mediaURL.startAccessingSecurityScopedResource()
        defer {
            if didAccessMedia {
                mediaURL.stopAccessingSecurityScopedResource()
            }
        }
        try FileManager.default.createDirectory(at: mediaURL, withIntermediateDirectories: true)
        let destination = uniqueDestination(for: sourceURL.lastPathComponent, in: mediaURL)
        let didAccessSource = sourceURL.startAccessingSecurityScopedResource()
        if didAccessSource {
            defer { sourceURL.stopAccessingSecurityScopedResource() }
            try FileManager.default.copyItem(at: sourceURL, to: destination)
        } else {
            try FileManager.default.copyItem(at: sourceURL, to: destination)
        }
        return destination
    }

    nonisolated func mediaFiles(
        in directoryPath: String,
        bookmark: Data?,
        progress: @escaping @Sendable (_ filesSeenDelta: Int, _ mediaFoundDelta: Int, _ currentPath: String) async -> Void
    ) async throws -> SonderMediaScanResult {
        let scopedURL = Self.storageURL(from: bookmark) ?? URL(fileURLWithPath: directoryPath, isDirectory: true)
        let didAccess = scopedURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                scopedURL.stopAccessingSecurityScopedResource()
            }
        }

        guard let enumerator = FileManager.default.enumerator(
            at: scopedURL,
            includingPropertiesForKeys: [.isRegularFileKey],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]
        ) else {
            return SonderMediaScanResult(files: [], filesSeen: 0, mediaFound: 0, unsupportedMediaCount: 0)
        }

        var files: [SonderScannedMediaFile] = []
        var filesSeen = 0
        var mediaFound = 0
        var unsupportedMediaCount = 0
        while let url = enumerator.nextObject() as? URL {
            filesSeen += 1
            guard SonderMediaFormat.isSupported(url: url) else {
                if Self.looksLikeUnsupportedVideo(url) {
                    unsupportedMediaCount += 1
                }
                if filesSeen.isMultiple(of: 100) {
                    await progress(filesSeen, mediaFound, url.path)
                }
                continue
            }
            let values = try? url.resourceValues(forKeys: [.isRegularFileKey])
            if values?.isRegularFile == true {
                mediaFound += 1
                files.append(SonderScannedMediaFile(url: url, bookmark: try? Self.bookmark(for: url)))
            }
            if filesSeen.isMultiple(of: 100) {
                await progress(filesSeen, mediaFound, url.path)
            }
        }
        if filesSeen > 0 || mediaFound > 0 {
            await progress(filesSeen, mediaFound, scopedURL.path)
        }
        return SonderMediaScanResult(
            files: files.sorted { $0.url.path.localizedStandardCompare($1.url.path) == .orderedAscending },
            filesSeen: filesSeen,
            mediaFound: mediaFound,
            unsupportedMediaCount: unsupportedMediaCount
        )
    }

    nonisolated static func storageURL(from bookmark: Data?) -> URL? {
        guard let bookmark else { return nil }
        var isStale = false
        return try? URL(resolvingBookmarkData: bookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale)
    }

    nonisolated static func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    nonisolated private static func looksLikeUnsupportedVideo(_ url: URL) -> Bool {
        ["flv", "wmv", "divx", "vob", "ogm", "rm", "rmvb"].contains(url.pathExtension.lowercased())
    }

    private func sanitize(_ snapshot: SonderSnapshot) -> SonderSnapshot {
        var sanitized = snapshot
        let itemIDs = Set(sanitized.items.map(\.id))
        sanitized.progress.removeAll { itemIDs.contains($0.itemID) == false }
        sanitized.collections = sanitized.collections.map { collection in
            var copy = collection
            copy.itemIDs = copy.itemIDs.filter { itemIDs.contains($0) }
            return copy
        }
        return sanitized
    }

    func moveAvoidingCollision(from sourceURL: URL, to destinationURL: URL) throws -> URL {
        let destination = uniqueDestination(for: destinationURL.lastPathComponent, in: destinationURL.deletingLastPathComponent())
        if sourceURL.path == destination.path {
            return sourceURL
        }
        try FileManager.default.moveItem(at: sourceURL, to: destination)
        return destination
    }

    func convertToMP4(sourceURL: URL, outputURL: URL) async -> Bool {
        let didAccess = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                sourceURL.stopAccessingSecurityScopedResource()
            }
        }
        let asset = AVURLAsset(url: sourceURL)
        guard let session = AVAssetExportSession(asset: asset, presetName: AVAssetExportPresetHighestQuality),
              session.supportedFileTypes.contains(.mp4) else {
            return false
        }
        do {
            session.outputURL = outputURL
            session.outputFileType = .mp4
            try await session.export(to: outputURL, as: .mp4)
            return FileManager.default.fileExists(atPath: outputURL.path)
        } catch {
            return false
        }
    }

    func availableConversionURL(for sourceURL: URL) -> URL {
        uniqueDestination(
            for: sourceURL.deletingPathExtension().appendingPathExtension("mp4").lastPathComponent,
            in: sourceURL.deletingLastPathComponent()
        )
    }

    private func uniqueDestination(for fileName: String, in folderURL: URL) -> URL {
        let base = URL(fileURLWithPath: fileName).deletingPathExtension().lastPathComponent
        let ext = URL(fileURLWithPath: fileName).pathExtension
        var candidate = folderURL.appendingPathComponent(fileName)
        var counter = 2
        while FileManager.default.fileExists(atPath: candidate.path) {
            let nextName = ext.isEmpty ? "\(base)-\(counter)" : "\(base)-\(counter).\(ext)"
            candidate = folderURL.appendingPathComponent(nextName)
            counter += 1
        }
        return candidate
    }
}

struct SonderMediaItem: Codable, Identifiable, Hashable {
    var id = UUID()
    var title: String
    var subtitle: String
    var kind: SonderMediaKind
    var studio: String
    var year: Int
    var durationSeconds: Double
    var format: SonderMediaFormat
    var tags: [String]
    var summary: String
    var sourcePath: String?
    var sourceBookmark: Data?
    var progressSeconds: Double = 0
    var showTitle: String?
    var seasonNumber: Int?
    var episodeNumber: Int?
    var metadataIDSource: String?
    var metadataID: String?
    var edition: String?
    var splitPart: String?
    var localPosterPath: String?
    var localBackdropPath: String?
    var subtitlePaths: [String] = []

    var hasFile: Bool {
        playableURL != nil
    }

    var playableURL: URL? {
        if let sourceBookmark {
            var isStale = false
            if let url = try? URL(resolvingBookmarkData: sourceBookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale),
               FileManager.default.fileExists(atPath: url.path) {
                return url
            }
        }
        guard let sourcePath, FileManager.default.fileExists(atPath: sourcePath) else { return nil }
        return URL(fileURLWithPath: sourcePath)
    }

    var localPosterImage: NSImage? {
        guard let localPosterPath else { return nil }
        return NSImage(contentsOfFile: localPosterPath)
    }

    var contentType: String {
        format.contentType
    }

    var progress: Double {
        guard durationSeconds > 0 else { return 0 }
        return min(max(progressSeconds / durationSeconds, 0), 1)
    }

    var searchText: String {
        ([title, subtitle, studio, kind.label, summary, showTitle ?? "", episodeCode] + tags).joined(separator: " ")
    }

    var episodeCode: String {
        guard let seasonNumber, let episodeNumber else { return "Not episodic" }
        return String(format: "S%02dE%02d", seasonNumber, episodeNumber)
    }

    var plexFileName: String {
        let ext = format == .unknown ? "mp4" : format.rawValue
        switch kind {
        case .tvShow:
            let show = (showTitle ?? title).plexSafeName
            let episodeTitle = title.plexSafeName
            let code = episodeCode == "Not episodic" ? "S01E01" : episodeCode
            return "\(show) - \(code) - \(episodeTitle).\(ext)"
        case .movie, .documentary:
            let editionTag = edition.map { " {edition-\($0.plexSafeName)}" } ?? ""
            return "\(title.plexSafeName) (\(year))\(editionTag).\(ext)"
        case .all:
            return "\(title.plexSafeName).\(ext)"
        }
    }
}

enum SonderSection: String, CaseIterable, Identifiable {
    case library
    case tvShows
    case continueWatching
    case collections
    case server
    case about

    var id: String { rawValue }
}

enum SonderMediaKind: String, Codable, CaseIterable, Identifiable {
    case all
    case movie
    case tvShow
    case documentary

    var id: String { rawValue }

    static var mediaCases: [SonderMediaKind] {
        [.movie, .tvShow, .documentary]
    }

    var label: String {
        switch self {
        case .all: "All"
        case .movie: "Movie"
        case .tvShow: "TV Show"
        case .documentary: "Documentary"
        }
    }

    var shortLabel: String {
        switch self {
        case .all: "ALL"
        case .movie: "FILM"
        case .tvShow: "TV"
        case .documentary: "DOC"
        }
    }

    var icon: String {
        switch self {
        case .all: "rectangle.stack"
        case .movie: "film"
        case .tvShow: "tv"
        case .documentary: "camera.metering.matrix"
        }
    }

    var gradient: [Color] {
        switch self {
        case .all: [SonderTheme.accentMuted, SonderTheme.surfaceDeep]
        case .movie: [Color(red: 0.27, green: 0.32, blue: 0.18), SonderTheme.surfaceDeep]
        case .tvShow: [Color(red: 0.19, green: 0.29, blue: 0.31), SonderTheme.surfaceDeep]
        case .documentary: [Color(red: 0.31, green: 0.24, blue: 0.18), SonderTheme.surfaceDeep]
        }
    }
}

enum SonderMediaFormat: String, Codable {
    case mp4
    case mov
    case m4v
    case mkv
    case webm
    case avi
    case mpg
    case mpeg
    case ts
    case m2ts
    case unknown

    static var importTypes: [UTType] {
        [
            .movie,
            .mpeg4Movie,
            .quickTimeMovie,
            UTType(filenameExtension: "mkv") ?? .data,
            UTType(filenameExtension: "webm") ?? .data,
            UTType(filenameExtension: "avi") ?? .data,
            UTType(filenameExtension: "mpg") ?? .data,
            UTType(filenameExtension: "mpeg") ?? .data,
            UTType(filenameExtension: "ts") ?? .data,
            UTType(filenameExtension: "m2ts") ?? .data
        ]
    }

    nonisolated init(url: URL) {
        switch url.pathExtension.lowercased() {
        case "mp4": self = .mp4
        case "mov": self = .mov
        case "m4v": self = .m4v
        case "mkv": self = .mkv
        case "webm": self = .webm
        case "avi": self = .avi
        case "mpg": self = .mpg
        case "mpeg": self = .mpeg
        case "ts": self = .ts
        case "m2ts": self = .m2ts
        default: self = .unknown
        }
    }

    nonisolated static func isSupported(url: URL) -> Bool {
        switch url.pathExtension.lowercased() {
        case "mp4", "mov", "m4v", "mkv", "webm", "avi", "mpg", "mpeg", "ts", "m2ts":
            return true
        default:
            return false
        }
    }

    nonisolated var contentType: String {
        switch self {
        case .mp4, .m4v: "video/mp4"
        case .mov: "video/quicktime"
        case .mkv: "video/x-matroska"
        case .webm: "video/webm"
        case .avi: "video/x-msvideo"
        case .mpg, .mpeg: "video/mpeg"
        case .ts, .m2ts: "video/mp2t"
        case .unknown: "application/octet-stream"
        }
    }
}

struct SonderProgress: Codable, Identifiable, Hashable {
    var id = UUID()
    var itemID: UUID
    var seconds: Double
    var duration: Double
    var updatedAt = Date()

    var percent: Double {
        guard duration > 0 else { return 0 }
        return min(max(seconds / duration, 0), 1)
    }
}

struct SonderCollection: Codable, Identifiable, Hashable {
    var id = UUID()
    var name: String
    var kind: SonderCollectionKind = .collection
    var itemIDs: [UUID]
}

struct SonderMediaDirectory: Codable, Identifiable, Hashable {
    var id = UUID()
    var name: String
    var path: String
    var bookmark: Data
    var kind: SonderLibraryImportKind = .movies
    var libraryID: UUID = SonderLibraryImportKind.movies.defaultLibraryID
    var lastIndexedCount = 0
    var lastScannedFileCount = 0
    var lastUnsupportedCount = 0
    var lastSkippedDuplicateCount = 0
    var lastParseFailureCount = 0
    var lastScannedAt: Date?

    var scanSummary: String {
        "unsupported \(lastUnsupportedCount) - duplicates \(lastSkippedDuplicateCount) - parse issues \(lastParseFailureCount)"
    }
}

struct SonderTVShowGroup: Identifiable, Hashable {
    var id: String { name }
    var name: String
    var seasons: [SonderTVSeasonGroup]
    var episodeCount: Int {
        seasons.map(\.episodes.count).reduce(0, +)
    }
}

struct SonderTVSeasonGroup: Identifiable, Hashable {
    var id: Int { seasonNumber }
    var seasonNumber: Int
    var episodes: [SonderMediaItem]

    var label: String {
        seasonNumber == 0 ? "Specials" : "Season \(seasonNumber)"
    }
}

struct SonderLibraryDefinition: Codable, Identifiable, Hashable {
    var id: UUID
    var name: String
    var kind: SonderLibraryImportKind
    var createdAt = Date()

    static let defaults = [
        SonderLibraryDefinition(id: SonderLibraryImportKind.movies.defaultLibraryID, name: "Movies", kind: .movies),
        SonderLibraryDefinition(id: SonderLibraryImportKind.tvShows.defaultLibraryID, name: "TV Shows", kind: .tvShows)
    ]
}

enum SonderLibraryImportKind: String, Codable, CaseIterable, Identifiable {
    case movies
    case tvShows

    var id: String { rawValue }

    nonisolated var label: String {
        switch self {
        case .movies: "Movies"
        case .tvShows: "TV Shows"
        }
    }

    nonisolated var tag: String {
        switch self {
        case .movies: "movies"
        case .tvShows: "tv"
        }
    }

    nonisolated var icon: String {
        switch self {
        case .movies: "film"
        case .tvShows: "tv"
        }
    }

    nonisolated var defaultLibraryID: UUID {
        switch self {
        case .movies: UUID(uuidString: "11111111-1111-4111-8111-111111111111")!
        case .tvShows: UUID(uuidString: "22222222-2222-4222-8222-222222222222")!
        }
    }
}

struct SonderScanDiagnostics: Hashable {
    var mediaCount: Int
    var fileCount: Int
    var unsupportedCount: Int
    var skippedDuplicateCount: Int
    var parseFailureCount: Int
    var scannedAt: Date
}

struct SonderScanProgress: Identifiable, Hashable {
    var id = UUID()
    var phase: SonderScanPhase = .fastTitles
    var title: String
    var detail: String
    var filesSeen: Int
    var mediaFound: Int
    var indexedCount: Int
    var directoriesDone: Int
    var directoriesTotal: Int

    var fraction: Double {
        guard directoriesTotal > 0 else { return 0 }
        return min(Double(directoriesDone) / Double(directoriesTotal), 0.98)
    }
}

enum SonderScanPhase: String, Codable, Hashable {
    case fastTitles
    case localAssets
    case metadata

    var label: String {
        switch self {
        case .fastTitles: "Indexing titles"
        case .localAssets: "Finding artwork"
        case .metadata: "Refreshing metadata"
        }
    }
}

struct SonderServerSettings: Codable, Hashable {
    var isEnabled: Bool
    var allowLAN: Bool
    var port: Int

    static let `default` = SonderServerSettings(isEnabled: false, allowLAN: false, port: 8797)

    var statusLabel: String {
        guard isEnabled else { return "Web server is off." }
        return allowLAN ? "Web server is available on the LAN." : "Web server is local-only."
    }
}

extension Notification.Name {
    static let sonderServerSettingsDidChange = Notification.Name("SonderServerSettingsDidChange")
}

struct SonderMediaScanResult: Sendable {
    var files: [SonderScannedMediaFile]
    var filesSeen: Int
    var mediaFound: Int
    var unsupportedMediaCount: Int
}

struct SonderLocalAssetUpdate: Sendable {
    var posterPath: String?
    var backdropPath: String?
    var subtitlePaths: [String]
}

struct SonderScannedMediaFile: Sendable {
    var url: URL
    var bookmark: Data?
}

enum SonderCollectionKind: String, Codable, CaseIterable, Identifiable {
    case collection
    case playlist

    var id: String { rawValue }

    var label: String {
        switch self {
        case .collection: "Collection"
        case .playlist: "Playlist"
        }
    }

    var icon: String {
        switch self {
        case .collection: "folder.badge.plus"
        case .playlist: "music.note.list"
        }
    }
}

struct SonderConversionJob: Codable, Identifiable, Hashable {
    var id = UUID()
    var title: String
    var detail: String
    var status: SonderConversionStatus
    var createdAt = Date()
}

enum SonderConversionStatus: String, Codable {
    case running
    case completed
    case failed

    var label: String {
        switch self {
        case .running: "Running"
        case .completed: "Done"
        case .failed: "Failed"
        }
    }

    var icon: String {
        switch self {
        case .running: "arrow.triangle.2.circlepath"
        case .completed: "checkmark.circle"
        case .failed: "exclamationmark.triangle"
        }
    }
}

struct SonderActivityEvent: Codable, Identifiable, Hashable {
    var id = UUID()
    var title: String
    var detail: String
    var icon: String
    var date = Date()
}

enum SonderSeed {
    static let catalog: [SonderMediaItem] = [
        SonderMediaItem(
            title: "Northstar",
            subtitle: "Feature Film - 2022",
            kind: .movie,
            studio: "Local Archive",
            year: 2022,
            durationSeconds: 6840,
            format: .mp4,
            tags: ["feature", "drama", "4k"],
            summary: "A placeholder feature entry for validating poster density, filtering, detail metadata, and watch progress."
        ),
        SonderMediaItem(
            title: "The Last Signal",
            subtitle: "Feature Film - 2019",
            kind: .movie,
            studio: "Local Archive",
            year: 2019,
            durationSeconds: 6120,
            format: .m4v,
            tags: ["sci-fi", "feature"],
            summary: "A science-fiction library sample that demonstrates Sonder's movie catalog flow and local streaming route."
        ),
        SonderMediaItem(
            title: "Workshop Sessions",
            subtitle: "Season 1 - 8 episodes",
            kind: .tvShow,
            studio: "Home Studio",
            year: 2024,
            durationSeconds: 2880,
            format: .mp4,
            tags: ["series", "education", "workshop"],
            summary: "A TV-style series entry for grouping episodic media and keeping active titles in Continue Watching."
        ),
        SonderMediaItem(
            title: "Archive Road",
            subtitle: "Documentary - 2021",
            kind: .documentary,
            studio: "Field Notes",
            year: 2021,
            durationSeconds: 5340,
            format: .mov,
            tags: ["documentary", "history", "travel"],
            summary: "A documentary sample used to tune the Alexandria-inspired management dashboard and tag analytics."
        ),
        SonderMediaItem(
            title: "Deep Workbench",
            subtitle: "Documentary Series - 3 parts",
            kind: .documentary,
            studio: "Independent",
            year: 2023,
            durationSeconds: 4020,
            format: .mp4,
            tags: ["documentary", "craft", "series"],
            summary: "A multi-part documentary concept showing how collections can separate films, shows, and documentary shelves."
        ),
        SonderMediaItem(
            title: "Night Harbor",
            subtitle: "Feature Film - 2020",
            kind: .movie,
            studio: "Local Archive",
            year: 2020,
            durationSeconds: 7020,
            format: .mkv,
            tags: ["thriller", "feature"],
            summary: "A long-form feature sample with MKV metadata for testing non-MP4 catalog entries and detail state."
        )
    ]
}

struct SonderParsedMedia {
    var title: String
    var subtitle: String
    var kind: SonderMediaKind
    var year: Int?
    var showTitle: String?
    var season: Int?
    var episode: Int?
    var metadataIDSource: String?
    var metadataID: String?
    var edition: String?
    var splitPart: String?
    var localPosterPath: String?
    var localBackdropPath: String?
    var subtitlePaths: [String] = []
}

enum SonderMediaParser {
    nonisolated static func parseTitle(url: URL, libraryKind: SonderLibraryImportKind? = nil) -> SonderParsedMedia {
        parse(url: url, libraryKind: libraryKind, includeLocalAssets: false)
    }

    nonisolated static func parse(url: URL, libraryKind: SonderLibraryImportKind? = nil) -> SonderParsedMedia {
        parse(url: url, libraryKind: libraryKind, includeLocalAssets: true)
    }

    nonisolated private static func parse(url: URL, libraryKind: SonderLibraryImportKind? = nil, includeLocalAssets: Bool) -> SonderParsedMedia {
        let raw = url.deletingPathExtension().lastPathComponent.cleanedMediaTitle
        let parent = url.deletingLastPathComponent().lastPathComponent.cleanedMediaTitle
        let grandparent = url.deletingLastPathComponent().deletingLastPathComponent().lastPathComponent.cleanedMediaTitle
        let assets = includeLocalAssets ? localAssets(near: url) : (poster: nil, backdrop: nil, subtitles: [])
        let metadataTag = extractMetadataTag(from: raw) ?? extractMetadataTag(from: parent)
        let edition = extractEdition(from: raw) ?? extractEdition(from: parent)
        let splitPart = extractSplitPart(from: raw)
        let movieFolderCandidate = extractMovieNameAndYear(from: parent)
        let movieFileCandidate = extractMovieNameAndYear(from: raw)
        let nsRange = NSRange(raw.startIndex..<raw.endIndex, in: raw)
        let patterns = [
            #"(?i)^(.+?)[\s._-]+S(\d{1,2})E(\d{1,2})(?:[\s._-]*E?(\d{1,2}))?[\s._-]*(.*)$"#,
            #"(?i)^S(\d{1,2})E(\d{1,2})(?:[\s._-]*E?(\d{1,2}))?[\s._-]*(.*)$"#,
            #"(?i)^(.+?)[\s._-]+(\d{1,2})x(\d{1,2})[\s._-]*(.*)$"#
        ]

        for pattern in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern),
                  let match = regex.firstMatch(in: raw, range: nsRange) else {
                continue
            }
            let hasShowPrefix = match.numberOfRanges >= 5
            let showRangeIndex = hasShowPrefix ? 1 : nil
            let seasonIndex = hasShowPrefix ? 2 : 1
            let episodeIndex = hasShowPrefix ? 3 : 2
            let titleIndex = match.numberOfRanges - 1
            guard let seasonRange = Range(match.range(at: seasonIndex), in: raw),
                  let episodeRange = Range(match.range(at: episodeIndex), in: raw) else {
                continue
            }
            let episodeTitle = Range(match.range(at: titleIndex), in: raw).map { String(raw[$0]).cleanedMediaTitle } ?? ""
            let folderShow = parent.localizedCaseInsensitiveContains("season") ? grandparent : parent
            let show = showRangeIndex.flatMap { Range(match.range(at: $0), in: raw).map { String(raw[$0]).cleanedMediaTitle } } ?? folderShow
            let season = Int(raw[seasonRange]) ?? 1
            let episode = Int(raw[episodeRange]) ?? 1
            return SonderParsedMedia(
                title: episodeTitle.isEmpty ? "Episode \(episode)" : episodeTitle,
                subtitle: "\(show) - S\(String(format: "%02d", season))E\(String(format: "%02d", episode))",
                kind: .tvShow,
                year: extractYear(from: raw) ?? extractYear(from: show),
                showTitle: show,
                season: season,
                episode: episode,
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        if libraryKind == .tvShows {
            let show = parent.localizedCaseInsensitiveContains("season") ? grandparent : parent
            let season = extractSeason(from: parent)
            return SonderParsedMedia(
                title: raw,
                subtitle: season.map { "\(show) - Season \($0)" } ?? show,
                kind: .tvShow,
                year: extractYear(from: raw) ?? extractYear(from: show),
                showTitle: show,
                season: season,
                episode: nil,
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        if libraryKind == .movies, let movie = movieFolderCandidate ?? movieFileCandidate {
            return SonderParsedMedia(
                title: movie.title,
                subtitle: movie.year.map { "Movie - \($0)" } ?? "Movie",
                kind: .movie,
                year: movie.year,
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                splitPart: splitPart,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        let lower = raw.lowercased()
        let kind: SonderMediaKind = lower.contains("documentary") || lower.contains("docu") ? .documentary : .movie
        return SonderParsedMedia(
            title: movieFileCandidate?.title ?? raw.removingPlexTags.cleanedMediaTitle,
            subtitle: "Imported local media",
            kind: kind,
            year: extractYear(from: raw),
            metadataIDSource: metadataTag?.source,
            metadataID: metadataTag?.id,
            edition: edition,
            splitPart: splitPart,
            localPosterPath: assets.poster?.path,
            localBackdropPath: assets.backdrop?.path,
            subtitlePaths: assets.subtitles.map(\.path)
        )
    }

    nonisolated private static func extractMovieNameAndYear(from value: String) -> (title: String, year: Int?)? {
        guard let year = extractYear(from: value) else { return nil }
        let title = value
            .replacingOccurrences(of: #"\(\d{4}\)"#, with: "", options: .regularExpression)
            .replacingOccurrences(of: #"\b\d{4}\b"#, with: "", options: .regularExpression)
            .removingPlexTags
            .removingSplitSuffix
            .cleanedMediaTitle
        return (title.isEmpty ? value : title, year)
    }

    nonisolated private static func extractMetadataTag(from value: String) -> (source: String, id: String)? {
        guard let match = value.firstMatch(pattern: #"\{(imdb|tmdb)-([^}]+)\}"#) else { return nil }
        return (match[0], match[1])
    }

    nonisolated private static func extractEdition(from value: String) -> String? {
        value.firstMatch(pattern: #"\{edition-([^}]{1,32})\}"#)?.first
    }

    nonisolated private static func extractSplitPart(from value: String) -> String? {
        value.firstMatch(pattern: #"(?i)(?:^|[\s._-])(cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$"#)?.first
    }

    nonisolated static func localAssets(near url: URL) -> (poster: URL?, backdrop: URL?, subtitles: [URL]) {
        let folder = url.deletingLastPathComponent()
        let base = url.deletingPathExtension().lastPathComponent
        let posterNames = ["poster.jpg", "poster.png", "folder.jpg", "\(base).jpg", "\(base).png"]
        let backdropNames = ["background.jpg", "background.png", "fanart.jpg", "fanart.png", "backdrop.jpg", "backdrop.png"]
        let poster = posterNames.map { folder.appendingPathComponent($0) }.first { FileManager.default.fileExists(atPath: $0.path) }
        let backdrop = backdropNames.map { folder.appendingPathComponent($0) }.first { FileManager.default.fileExists(atPath: $0.path) }
        let subtitles = (try? FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil))
            .map { urls in urls.filter { ["srt", "vtt", "ass", "ssa"].contains($0.pathExtension.lowercased()) && $0.deletingPathExtension().lastPathComponent.hasPrefix(base) } } ?? []
        return (poster, backdrop, subtitles)
    }

    nonisolated private static func extractYear(from value: String) -> Int? {
        guard let range = value.range(of: #"(19|20)\d{2}"#, options: .regularExpression) else { return nil }
        return Int(value[range])
    }

    nonisolated private static func extractSeason(from value: String) -> Int? {
        guard let range = value.range(of: #"(?i)season[\s._-]*(\d{1,2})"#, options: .regularExpression) else { return nil }
        let digits = value[range].filter(\.isNumber)
        return Int(String(digits))
    }
}

enum SonderTheme {
    static let background = Color(red: 0.06, green: 0.075, blue: 0.055)
    static let sidebar = Color(red: 0.08, green: 0.10, blue: 0.07)
    static let surface = Color(red: 0.10, green: 0.13, blue: 0.085)
    static let surfaceDeep = Color(red: 0.045, green: 0.055, blue: 0.04)
    static let border = Color(red: 0.20, green: 0.25, blue: 0.16)
    static let accent = Color(red: 0.71, green: 0.84, blue: 0.43)
    static let accentStrong = Color(red: 0.55, green: 0.69, blue: 0.28)
    static let accentMuted = Color(red: 0.18, green: 0.25, blue: 0.12)
    static let text = Color(red: 0.92, green: 0.95, blue: 0.88)
    static let textLight = Color(red: 0.66, green: 0.70, blue: 0.61)
    static let darkText = Color(red: 0.06, green: 0.08, blue: 0.05)
}

enum SonderTime {
    static func format(_ seconds: Double) -> String {
        let safeSeconds = max(0, Int(seconds))
        let hours = safeSeconds / 3600
        let minutes = (safeSeconds % 3600) / 60
        if hours > 0 {
            return "\(hours)h \(minutes)m"
        }
        return "\(minutes)m"
    }
}

extension JSONEncoder {
    static var sonder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}

extension JSONDecoder {
    static var sonder: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}

private extension String {
    nonisolated var cleanedMediaTitle: String {
        replacingOccurrences(of: ".", with: " ")
            .replacingOccurrences(of: "_", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    nonisolated var plexSafeName: String {
        let illegal = CharacterSet(charactersIn: "/:\\?%*|\"<>")
        return components(separatedBy: illegal)
            .joined(separator: " ")
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    nonisolated var removingPlexTags: String {
        replacingOccurrences(of: #"\{(?:imdb|tmdb)-[^}]+\}"#, with: "", options: [.regularExpression, .caseInsensitive])
            .replacingOccurrences(of: #"\{edition-[^}]+\}"#, with: "", options: [.regularExpression, .caseInsensitive])
    }

    nonisolated var removingSplitSuffix: String {
        replacingOccurrences(of: #"(?i)[\s._-]+(?:cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$"#, with: "", options: .regularExpression)
    }

    nonisolated func firstMatch(pattern: String) -> [String]? {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]) else { return nil }
        let nsRange = NSRange(startIndex..<endIndex, in: self)
        guard let match = regex.firstMatch(in: self, range: nsRange), match.numberOfRanges > 1 else { return nil }
        return (1..<match.numberOfRanges).compactMap { index in
            guard let range = Range(match.range(at: index), in: self) else { return nil }
            return String(self[range]).cleanedMediaTitle
        }
    }
}

#Preview {
    ContentView()
        .environmentObject(SonderLibrary(store: SonderStore(rootURL: FileManager.default.temporaryDirectory.appendingPathComponent("SonderPreview"))))
}
