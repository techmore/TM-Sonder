import AppKit
import AVFoundation
import Combine
import CryptoKit
import Foundation
import SwiftUI
import UniformTypeIdentifiers

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
    private let systemServices: SonderSystemServicing
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
    var isBusy: Bool {
        scanProgress != nil || plexImportStatus.isRunning || audiobookImportStatus.isRunning
    }

    static let defaultCollections: [SonderCollection] = []

    init(store: SonderStore? = nil, systemServices: SonderSystemServicing? = nil) {
        let resolvedStore = store ?? SonderStore()
        self.store = resolvedStore
        self.systemServices = systemServices ?? SonderSystemServices.shared
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
        Task { @MainActor in
            await Task.yield()
            loadPersistedLibrary(from: resolvedStore)
        }
    }

    func importPlexContextIfAvailable(force: Bool = false) {
        guard plexImportStatus.isRunning == false else { return }
        guard force || didAttemptPlexImport == false else { return }
        didAttemptPlexImport = true
        plexImportStatus = PlexImportStatus(
            lastRunAt: plexImportStatus.lastRunAt,
            importedCount: 0,
            unchangedCount: 0,
            skippedCount: 0,
            lastMessage: "Preparing Plex import...",
            isRunning: true,
            processedCount: 0,
            totalCount: 0,
            currentTitle: nil
        )
        Task.detached { [store] in
            let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            let plexRoot = support?.appendingPathComponent("Plex Media Server", isDirectory: true)
            let dbURL = plexRoot?.appendingPathComponent("Plug-in Support/Databases/com.plexapp.plugins.library.db")
            let metadataRoot = plexRoot?.appendingPathComponent("Metadata/TV Shows", isDirectory: true)
            guard let dbURL, let metadataRoot,
                  FileManager.default.fileExists(atPath: dbURL.path),
                  FileManager.default.fileExists(atPath: metadataRoot.path) else {
                await MainActor.run {
                    self.plexImportStatus = PlexImportStatus(
                        lastRunAt: Date(),
                        importedCount: 0,
                        unchangedCount: 0,
                        skippedCount: 0,
                        lastMessage: "Plex database or TV metadata cache was not found on this Mac."
                    )
                }
                return
            }
            let importer = PlexImporter(dbURL: dbURL, bundleRootURL: metadataRoot)
            let summary = await importer.importShowContexts { progress in
                await MainActor.run {
                    self.plexImportStatus = PlexImportStatus(
                        lastRunAt: self.plexImportStatus.lastRunAt,
                        importedCount: progress.importedCount,
                        unchangedCount: progress.unchangedCount,
                        skippedCount: progress.skippedCount,
                        lastMessage: progress.totalCount > 0 ? "Importing Plex context..." : "No Plex show metadata found.",
                        isRunning: true,
                        processedCount: progress.processedCount,
                        totalCount: progress.totalCount,
                        currentTitle: progress.currentTitle
                    )
                }
            }
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
                self.plexImportStatus = PlexImportStatus(
                    lastRunAt: Date(),
                    importedCount: summary.importedCount,
                    unchangedCount: summary.unchangedCount,
                    skippedCount: summary.skippedCount,
                    lastMessage: message,
                    isRunning: false,
                    processedCount: summary.importedCount + summary.unchangedCount + summary.skippedCount,
                    totalCount: summary.importedCount + summary.unchangedCount + summary.skippedCount,
                    currentTitle: nil
                )
                self.addActivity("Imported Plex context", detail: "\(summary.importedCount) imported, \(summary.unchangedCount) unchanged, \(summary.skippedCount) skipped.", icon: "externaldrive.badge.icloud")
            }
        }
    }

    func importAudiobookContextIfAvailable(force: Bool = false) {
        guard force || didAttemptAudiobookImport == false else { return }
        didAttemptAudiobookImport = true
        audiobookImportStatus.isRunning = true
        audiobookImportStatus.lastMessage = "Preparing audiobook index..."
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
                    lastMessage: summary.message,
                    isRunning: false
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
            let derivedData = SonderDerivedData.make(items: snapshot.items, progressRecords: snapshot.progress)
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
        applyDerivedData(SonderDerivedData.make(items: items, progressRecords: progressRecords))
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
        guard let rootURL = systemServices.chooseMediaLibraryRoot() else { return }
        addMediaLibraryRoot(rootURL)
    }

    func chooseMediaDirectories(kind: SonderLibraryImportKind) {
        let urls = systemServices.chooseMediaDirectories(kind: kind)
        guard urls.isEmpty == false else { return }
        addMediaDirectories(urls, kind: kind)
    }

    func addMediaLibraryRoot(_ rootURL: URL) {
        let matchedDirectories = SonderMediaLibraryDirectoryScanner.detect(in: rootURL)
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
        scanProgress = SonderScanProgressFactory.preparing(directoriesTotal: directories.count)

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
                    self.scanProgress = SonderScanProgressFactory.indexing(
                        directory: directory,
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
                    let progress = SonderScanProgressFactory.discoveredRootTitles(
                        count: placeholders.count,
                        directory: directory,
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
                        let progress = SonderScanProgressFactory.indexing(
                            directory: directory,
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
                            let progress = SonderScanProgressFactory.indexing(
                                directory: directory,
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
            scanProgress = SonderScanProgressFactory.localAssets(directoriesTotal: directoriesTotal ?? targets.count)
        }

        // Capture Sendable snapshots of each target's id + resolved URL so the detached
        // task can probe off the main actor without touching the @MainActor model.
        let probeTargets: [(id: UUID, url: URL)] = targets.compactMap { item in
            guard let url = item.playableURL else { return nil }
            return (item.id, url)
        }

        Task.detached {
            let collector = SonderAssetRefreshCollector()

            await SonderConcurrencyLimiter.run(limit: Self.maxConcurrentAssetRefreshes, over: probeTargets) { target in
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
                let parsed = SonderMediaParser.parse(url: managedURL)
                let item = SonderManagedImportFactory.makeItem(for: managedURL, parsed: parsed)
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
        SonderLibraryQueries.search(items: items, query: query, kind: kind, tag: tag)
    }

    func item(id: UUID) -> SonderMediaItem? {
        SonderLibraryQueries.item(id: id, in: items)
    }

    func audiobookItems() -> [SonderMediaItem] {
        SonderLibraryQueries.audiobookItems(in: items)
    }

    func audiobookItem(id: UUID) -> SonderMediaItem? {
        SonderLibraryQueries.audiobookItem(id: id, in: items)
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
        SonderLibraryQueries.priorityAssetTargetIDs(
            for: itemID,
            in: items,
            maxItems: Self.maxPriorityAssetRefreshItems
        )
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
        systemServices.openSecurityScoped(url)
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
        let update = SonderProgressRecords.upserting(
            itemID: itemID,
            seconds: seconds,
            duration: duration,
            in: progressRecords
        )
        progressRecords = update.records
        if let item = item(id: itemID) {
            addActivity("Updated progress", detail: "\(item.title) is now \(Int((update.seconds / update.duration) * 100))% watched.", icon: "chart.line.uptrend.xyaxis")
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
        let candidates = SonderConversionPlanner.candidates(from: items)
        guard candidates.isEmpty == false else {
            addActivity("Conversion skipped", detail: "No playable non-MP4 titles to convert.", icon: "checkmark.circle")
            return
        }
        addActivity("Queueing conversions", detail: "\(candidates.count) title(s), \(Self.maxConcurrentConversions) at a time.", icon: "arrow.triangle.2.circlepath")
        Task { [weak self] in
            guard let self else { return }
            await SonderConcurrencyLimiter.run(limit: Self.maxConcurrentConversions, over: candidates) { item in
                await MainActor.run { self.convertToMP4(item) }
            }
        }
    }

    func convertToMP4(_ item: SonderMediaItem) {
        guard let sourceURL = item.playableURL else {
            addActivity("Conversion unavailable", detail: "Select a playable file first.", icon: "exclamationmark.triangle")
            return
        }
        let outputURL = store.availableConversionURL(for: sourceURL)
        guard let plan = SonderConversionPlanner.plan(for: item, outputURL: outputURL) else {
            addActivity("Conversion skipped", detail: "\(item.title) is already MP4.", icon: "checkmark.circle")
            return
        }

        conversionJobs.insert(plan.job, at: 0)
        commitLibraryMutation()

        Task {
            let result = await store.convertToMP4(sourceURL: plan.sourceURL, outputURL: plan.outputURL)
            await MainActor.run {
                if let jobIndex = self.conversionJobs.firstIndex(where: { $0.id == plan.job.id }) {
                    self.conversionJobs[jobIndex].status = result ? .completed : .failed
                }
                if result, let itemIndex = self.items.firstIndex(where: { $0.id == plan.itemID }) {
                    self.items[itemIndex].sourcePath = plan.outputURL.path
                    self.items[itemIndex].format = .mp4
                    self.addActivity("Converted to MP4", detail: plan.outputURL.lastPathComponent, icon: "checkmark.circle")
                } else if result == false {
                    self.addActivity("Conversion failed", detail: "\(plan.sourceURL.lastPathComponent). Use a companion HandBrake workflow for unsupported codecs.", icon: "exclamationmark.triangle")
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
