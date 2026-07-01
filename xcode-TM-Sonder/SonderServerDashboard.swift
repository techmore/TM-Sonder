import SwiftUI

struct ServerDashboard: View {
    @ObservedObject var library: SonderLibrary
    @State private var customLibraryName = ""
    @State private var customLibraryStyle: SonderLibraryImportKind = .movies
    @State private var showingResetDatabaseConfirmation = false

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

                DashboardPanel(title: "Testing Tools") {
                    HStack {
                        Label("Reset imported library database", systemImage: "trash")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        Button(role: .destructive) {
                            showingResetDatabaseConfirmation = true
                        } label: {
                            Label("Reset Database", systemImage: "trash")
                        }
                        .disabled(library.isBusy || library.isLoadingPersistedLibrary)
                    }
                    Text("Clears imported titles, watched progress, collections, scanned folders, queued scans, and conversion jobs for testing. It does not delete media files from disk.")
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
                            Label(library.plexImportStatus.isRunning ? "Importing" : "Import Now", systemImage: library.plexImportStatus.isRunning ? "hourglass" : "arrow.down.doc")
                        }
                        .disabled(library.plexImportStatus.isRunning)
                    }
                    Text("Workflow: install/run Plex Media Server on this Mac, let Plex scan your TV library, then use Import Now. Sonder reads Plex's local database and TV metadata cache, then writes per-show `.sonder/sonder-context.json` caches beside matched show roots so later boots read local context first.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)

                    if library.plexImportStatus.isRunning {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Label(library.plexImportStatus.lastMessage, systemImage: "arrow.down.doc")
                                Spacer()
                                if library.plexImportStatus.totalCount > 0 {
                                    Text("\(library.plexImportStatus.processedCount)/\(library.plexImportStatus.totalCount) shows")
                                        .font(.caption.monospacedDigit())
                                        .foregroundStyle(SonderTheme.textLight)
                                }
                            }
                            if library.plexImportStatus.totalCount > 0 {
                                ProgressView(value: library.plexImportStatus.progressFraction)
                                    .tint(SonderTheme.accentStrong)
                            } else {
                                ProgressView()
                                    .controlSize(.small)
                            }
                            HStack {
                                Label("\(library.plexImportStatus.importedCount) imported", systemImage: "tray.and.arrow.down")
                                Label("\(library.plexImportStatus.unchangedCount) unchanged", systemImage: "checkmark.circle")
                                Label("\(library.plexImportStatus.skippedCount) skipped", systemImage: "forward.end")
                                Spacer()
                            }
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(SonderTheme.textLight)
                            if let currentTitle = library.plexImportStatus.currentTitle {
                                Text(currentTitle)
                                    .font(.caption)
                                    .foregroundStyle(SonderTheme.textLight)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                    }

                    HStack {
                        Text(library.plexImportStatus.lastMessage)
                            .font(.caption)
                            .foregroundStyle(library.plexImportStatus.isRunning ? SonderTheme.accent : SonderTheme.textLight)
                        Spacer()
                        if library.plexImportStatus.isRunning == false {
                            Text("\(library.plexImportStatus.importedCount) imported / \(library.plexImportStatus.unchangedCount) unchanged / \(library.plexImportStatus.skippedCount) skipped")
                                .font(.caption2.monospacedDigit())
                                .foregroundStyle(SonderTheme.textLight.opacity(0.85))
                        }
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
                        let pickerDisabled = library.isBusy || library.isLoadingPersistedLibrary
                        Label("\(library.libraryDefinitions.count) libraries / \(library.mediaDirectories.count) folders", systemImage: "network")
                            .foregroundStyle(SonderTheme.textLight)
                        Spacer()
                        Button {
                            library.chooseMediaLibraryRoot()
                        } label: {
                            Label("Media Library", systemImage: "externaldrive.badge.plus")
                        }
                        .disabled(pickerDisabled)
                        Button {
                            library.chooseMediaDirectories(kind: .movies)
                        } label: {
                            Label("Movies Library", systemImage: "film")
                        }
                        .disabled(pickerDisabled)
                        Button {
                            library.chooseMediaDirectories(kind: .tvShows)
                        } label: {
                            Label("TV Library", systemImage: "tv")
                        }
                        .disabled(pickerDisabled)
                        Button {
                            library.chooseMediaDirectories(kind: .audiobooks)
                        } label: {
                            Label("Audio Library", systemImage: "headphones")
                        }
                        .disabled(pickerDisabled)
                        Button {
                            library.chooseMediaDirectories(kind: .ebooks)
                        } label: {
                            Label("Books Library", systemImage: "book")
                        }
                        .disabled(pickerDisabled)
                        Button {
                            library.rescanMediaDirectories()
                        } label: {
                            Label("Rescan", systemImage: "arrow.clockwise")
                        }
                        .disabled(library.mediaDirectories.isEmpty || pickerDisabled)
                    }

                    Text("Select a Media Library root to auto-detect TV Shows, Movies, Books, and Audiobooks folders, or add individual SMB/NFS/NAS folders manually. Sonder stores security-scoped bookmarks and only scans directories the user grants.")
                        .font(.caption)
                        .foregroundStyle(SonderTheme.textLight)
                        .fixedSize(horizontal: false, vertical: true)

                    DashboardPanel(title: "Other Import") {
                        HStack(spacing: 10) {
                            TextField("Category name", text: $customLibraryName)
                                .textFieldStyle(.roundedBorder)
                                .frame(minWidth: 180)
                            Picker("Style", selection: $customLibraryStyle) {
                                Text("Movie styled").tag(SonderLibraryImportKind.movies)
                                Text("TV Show styled").tag(SonderLibraryImportKind.tvShows)
                            }
                            .pickerStyle(.segmented)
                            .frame(width: 260)
                            Button {
                                library.chooseCustomMediaDirectory(named: customLibraryName, styledAs: customLibraryStyle)
                                customLibraryName = ""
                                customLibraryStyle = .movies
                            } label: {
                                Label("Choose Folder", systemImage: "folder.badge.plus")
                            }
                            .disabled(customLibraryName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || library.scanProgress != nil)
                        }
                    }

                    if let progress = library.scanProgress {
                        DashboardPanel(title: "Import Monitor") {
                            VStack(alignment: .leading, spacing: 10) {
                                VStack(alignment: .leading, spacing: 5) {
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
                                    HStack(spacing: 10) {
                                        if let startedAt = library.activeScanStartedAt {
                                            HStack(spacing: 4) {
                                                Image(systemName: "timer")
                                                Text("Started")
                                                Text(startedAt, style: .relative)
                                            }
                                        }
                                        if let updatedAt = library.activeScanUpdatedAt {
                                            HStack(spacing: 4) {
                                                Image(systemName: "dot.radiowaves.left.and.right")
                                                Text("Updated")
                                                Text(updatedAt, style: .relative)
                                            }
                                        }
                                    }
                                    .font(.caption2.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight.opacity(0.9))
                                    ProgressView(value: progress.fraction)
                                        .tint(SonderTheme.accentStrong)
                                    HStack {
                                        Label("\(progress.mediaFound) media", systemImage: "doc.richtext")
                                        Label("\(progress.indexedCount) indexed", systemImage: "tray.and.arrow.down")
                                        if library.queuedScanDirectoryCount > 0 {
                                            Label("\(library.queuedScanDirectoryCount) queued", systemImage: "tray")
                                        }
                                        Spacer()
                                        Text("\(progress.directoriesDone)/\(progress.directoriesTotal) folders")
                                    }
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(SonderTheme.textLight)
                                    if let activePath = library.activeScanDirectoryPath {
                                        Text("Active folder: \(activePath)")
                                            .font(.caption)
                                            .foregroundStyle(SonderTheme.textLight)
                                            .lineLimit(1)
                                            .truncationMode(.middle)
                                    }
                                    Text("Current: \(progress.detail)")
                                        .font(.caption)
                                        .foregroundStyle(SonderTheme.textLight)
                                        .lineLimit(1)
                                        .truncationMode(.middle)
                                    if library.queuedScanDirectorySummaries.isEmpty == false {
                                        VStack(alignment: .leading, spacing: 3) {
                                            Text("Queued next")
                                                .font(.caption2.weight(.semibold))
                                                .foregroundStyle(SonderTheme.textLight)
                                            ForEach(Array(library.queuedScanDirectorySummaries.prefix(4)), id: \.self) { summary in
                                                Label(summary, systemImage: "tray")
                                                    .font(.caption2)
                                                    .foregroundStyle(SonderTheme.textLight)
                                                    .lineLimit(1)
                                                    .truncationMode(.middle)
                                            }
                                            if library.queuedScanDirectorySummaries.count > 4 {
                                                Text("+\(library.queuedScanDirectorySummaries.count - 4) more queued")
                                                    .font(.caption2.monospacedDigit())
                                                    .foregroundStyle(SonderTheme.textLight.opacity(0.85))
                                            }
                                        }
                                        .padding(8)
                                        .background(SonderTheme.surfaceDeep, in: RoundedRectangle(cornerRadius: 6))
                                    }
                                }

                                ScrollView(.horizontal, showsIndicators: false) {
                                    HStack(alignment: .top, spacing: 10) {
                                        ForEach(Array(SonderMediaKind.mediaCases.enumerated()), id: \.element) { index, kind in
                                            let count = library.mediaKindCounts[kind, default: 0]
                                            let stage = library.scanStageState(for: kind)
                                            let expected = library.expectedCount(for: kind)
                                            let importedLabel = count == 1 ? "1 item" : "\(count) items"
                                            let folderLabel = expected == 1 ? "1 folder" : "\(expected) folders"
                                            let stateLabel = stage.isComplete ? "Discovered" : (stage.isActive ? "Discovering" : "Queued")
                                            let subtitle = expected > 0
                                                ? "Phase \(index + 1): \(importedLabel) from \(folderLabel)."
                                                : (count > 0
                                                    ? "Phase \(index + 1): \(importedLabel) already indexed."
                                                    : "Phase \(index + 1): waiting for this media type.")
                                            let detail = "\(count)/\(max(expected, count)) media folders"
                                            MonitorPhaseCard(
                                                title: kind.label,
                                                subtitle: subtitle,
                                                detail: detail,
                                                stateLabel: stateLabel,
                                                isActive: stage.isActive,
                                                isComplete: stage.isComplete
                                            )
                                        }
                                        MonitorPhaseCard(
                                            title: "Indexing",
                                            subtitle: "Phase \(SonderMediaKind.mediaCases.count + 1): build the catalog from discovered titles.",
                                            detail: "Playable media records are being finalized now.",
                                            stateLabel: library.scanProgress?.phase == .fastTitles ? "Indexing" : (library.scanProgress?.phase == .localAssets ? "Ready" : "Queued"),
                                            isActive: library.scanProgress?.phase == .fastTitles,
                                            isComplete: library.scanProgress?.phase == .localAssets
                                        )
                                        MonitorPhaseCard(
                                            title: "Artwork",
                                            subtitle: "Phase \(SonderMediaKind.mediaCases.count + 2): refresh posters, subtitles, and runtime details.",
                                            detail: library.scanProgress?.phase == .localAssets ? "\(library.scanProgress?.indexedCount ?? 0)/\(library.scanProgress?.mediaFound ?? 0) titles" : "Waiting for indexing to finish.",
                                            stateLabel: library.scanProgress?.phase == .localAssets ? "Refreshing art" : (library.scanProgress == nil ? "Ready" : "Queued"),
                                            isActive: library.scanProgress?.phase == .localAssets,
                                            isComplete: library.scanProgress == nil && library.items.isEmpty == false
                                        )
                                    }
                                }
                                Text("Import runs in stages: Sonder discovers media folders first, then indexes the catalog, then refreshes local artwork and runtime details.")
                                    .font(.caption2)
                                    .foregroundStyle(SonderTheme.textLight)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
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
                    Text("Best browser/server target: MP4 container, H.264 video for maximum compatibility or H.265/HEVC for smaller files on newer Apple devices, AAC stereo/5.1 audio, web-optimized start metadata, and SRT/VTT sidecar subtitles. Keep source files if you want archival quality; convert MKV/ASS/FLAC/DTS-heavy releases to MP4 for smooth in-browser playback and Plex-style naming like Show Name/Season 01/Show Name - S01E01 - Episode Title.mp4.")
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
        .confirmationDialog(
            "Reset Sonder database?",
            isPresented: $showingResetDatabaseConfirmation,
            titleVisibility: .visible
        ) {
            Button("Reset Database", role: .destructive) {
                library.resetLibraryDatabaseForTesting()
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This clears imported titles, watched progress, collections, scanned folders, queued scans, and conversion jobs. Your media files remain untouched.")
        }
    }
}
