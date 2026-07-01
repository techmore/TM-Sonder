import Combine
import Foundation

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
    @Published private(set) var activeScanKind: SonderMediaKind?
    @Published private(set) var activeScanDirectoryPath: String?
    @Published private(set) var activeScanStartedAt: Date?
    @Published private(set) var activeScanUpdatedAt: Date?
    @Published private(set) var queuedScanDirectoryCount = 0
    @Published private(set) var queuedScanDirectorySummaries: [String] = []
    @Published private(set) var serverSettings: SonderServerSettings
    @Published private(set) var nowPlayingItem: SonderMediaItem?
    @Published private(set) var storagePath: String
    private var storageBookmark: Data?

    /// The cache is itself thread-safe and Sendable, so the stored reference
    /// is implicitly nonisolated and safely accessible from the HTTP server's
    /// background queue without main-actor hops.
    private let httpCacheStorage = SonderHTTPCache()
    nonisolated var httpCache: SonderHTTPCache { httpCacheStorage }

    private let store: SonderStore
    private let systemServices: SonderSystemServicing
    private let persistenceCoordinator = SonderLibraryPersistenceCoordinator()
    private var pendingIndexedItems: [SonderMediaItem] = []
    private var pendingScanProgress: SonderScanProgress?
    private var pendingIndexFlushTask: Task<Void, Never>?
    private var queuedScanDirectories: [SonderMediaDirectory] = []
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
        nowPlayingItem = nil
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
        plexImportStatus = SonderPlexImportService.preparingStatus(previous: plexImportStatus)
        let importService = SonderPlexImportService(store: store)
        Task.detached {
            let result = await importService.importContexts { status in
                await MainActor.run {
                    self.plexImportStatus = PlexImportStatus(
                        lastRunAt: self.plexImportStatus.lastRunAt,
                        importedCount: status.importedCount,
                        unchangedCount: status.unchangedCount,
                        skippedCount: status.skippedCount,
                        lastMessage: status.lastMessage,
                        isRunning: status.isRunning,
                        processedCount: status.processedCount,
                        totalCount: status.totalCount,
                        currentTitle: status.currentTitle
                    )
                }
            }
            await MainActor.run {
                self.plexImportStatus = result.status
                if let activity = result.activity {
                    self.addActivity(activity.title, detail: activity.detail, icon: activity.icon)
                }
            }
        }
    }

    func importAudiobookContextIfAvailable(force: Bool = false) {
        guard force || didAttemptAudiobookImport == false else { return }
        didAttemptAudiobookImport = true
        audiobookImportStatus.isRunning = true
        audiobookImportStatus.lastMessage = "Preparing audiobook index..."
        let itemsSnapshot = items
        let mediaDirectoriesSnapshot = mediaDirectories
        let importService = SonderAudiobookImportService(store: store)
        Task.detached {
            let result = await importService.refreshIndex(items: itemsSnapshot, mediaDirectories: mediaDirectoriesSnapshot)
            await MainActor.run {
                self.audiobookImportStatus = result.status
                self.addActivity(result.activity.title, detail: result.activity.detail, icon: result.activity.icon)
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
    private nonisolated static let maxConcurrentAssetRefreshes = 2
    private nonisolated static let indexUICommitIntervalNanoseconds: UInt64 = 650_000_000
    private nonisolated static let minScanProgressPublishIntervalNanoseconds: UInt64 = 600_000_000
    private nonisolated static let minAssetProgressPublishInterval: TimeInterval = 0.8
    private nonisolated static let maxPendingIndexedItemsBeforeCommit = 96
    private nonisolated static let maxPriorityAssetRefreshItems = 24
    private var lastScanProgressPublishAt = Date.distantPast

    private func loadPersistedLibrary(from store: SonderStore) {
        isLoadingPersistedLibrary = true
        Task.detached {
            let preparedState = SonderPreparedLibraryState.prepare(snapshot: store.load(), store: store)
            await MainActor.run {
                self.applyPreparedLibraryState(preparedState)
            }
        }
    }

    private func applyPreparedLibraryState(_ preparedState: SonderPreparedLibraryState) {
        let serverSettingsChanged = preparedState.serverSettings != serverSettings
        items = preparedState.items
        progressRecords = preparedState.progressRecords
        collections = preparedState.collections
        activity = preparedState.activity
        conversionJobs = preparedState.conversionJobs
        libraryDefinitions = preparedState.libraryDefinitions
        mediaDirectories = preparedState.mediaDirectories
        serverSettings = preparedState.serverSettings
        storageBookmark = preparedState.storageBookmark
        storagePath = preparedState.storagePath
        applyDerivedData(preparedState.derivedData)
        isLoadingPersistedLibrary = false
        if preparedState.removedPlaceholderCount > 0 {
            addActivity("Removed placeholder records", detail: "Deleted \(preparedState.removedPlaceholderCount) non-playable placeholder item(s) from the saved library.", icon: "trash")
            commitLibraryMutation()
        }
        if serverSettingsChanged {
            NotificationCenter.default.post(name: .sonderServerSettingsDidChange, object: nil)
        }
    }

    private func enqueueScanProgress(_ progress: SonderScanProgress) {
        let now = Date()
        markScanProgressUpdated(at: now)
        guard now.timeIntervalSince(lastScanProgressPublishAt) >= Double(Self.minScanProgressPublishIntervalNanoseconds) / 1_000_000_000 else {
            pendingScanProgress = progress
            scheduleIndexFlush()
            return
        }
        lastScanProgressPublishAt = now
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
            invalidateHTTPCache()
        }
        if let progress {
            scanProgress = progress
            lastScanProgressPublishAt = Date()
        }
    }

    private func finishScanProgress(title: String, detail: String) {
        scanProgress = SonderScanProgressFactory.completed(title: title, detail: detail)
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 700_000_000)
            if self.scanProgress?.title == title && self.scanProgress?.detail == detail {
                self.scanProgress = nil
                self.activeScanStartedAt = nil
                self.activeScanUpdatedAt = nil
                self.activeScanDirectoryPath = nil
                self.startQueuedScanIfAvailable()
            }
        }
    }

    func scanStageProgress(for kind: SonderMediaKind) -> Double {
        let count = mediaKindCounts[kind, default: 0]
        let expected = expectedCount(for: kind)
        guard expected > 0 else { return count > 0 ? 1 : 0 }
        return min(1, Double(count) / Double(expected))
    }

    func scanStageState(for kind: SonderMediaKind) -> (isActive: Bool, isComplete: Bool, isQueued: Bool) {
        let expected = expectedCount(for: kind)
        let count = mediaKindCounts[kind, default: 0]
        let complete = expected > 0 ? count >= expected : count > 0
        let activeKind = activeScanKind
        let queued = complete == false && activeKind != nil && activeKind != kind
        return (activeKind == kind, complete, queued)
    }

    func expectedCount(for kind: SonderMediaKind) -> Int {
        mediaDirectories.filter { $0.kind == kind.importKind }.count
    }

    @discardableResult
    private func purgePlaceholders() -> Int {
        let placeholderIDs = Set(items.filter { $0.isPlaceholder }.map(\.id))
        guard placeholderIDs.isEmpty == false else { return 0 }

        items.removeAll { placeholderIDs.contains($0.id) }
        progressRecords.removeAll { placeholderIDs.contains($0.itemID) }
        collections = collections.map { collection in
            var copy = collection
            copy.itemIDs.removeAll { placeholderIDs.contains($0) }
            return copy
        }.filter { $0.itemIDs.isEmpty == false }
        rebuildDerivedData()
        return placeholderIDs.count
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
        applyServerSettingsMutation(
            SonderServerSettingsPlanner.updating(
                serverSettings,
                isEnabled: isEnabled,
                allowLAN: allowLAN,
                port: port,
                pairingToken: serverSettings.pairingToken
            )
        )
    }

    func updateTheme(_ preset: SonderThemePreset) {
        applyServerSettingsMutation(
            SonderServerSettingsPlanner.applyingTheme(preset, to: serverSettings)
        )
    }

    /// Convenience: regenerates the LAN pairing token, invalidating any previously
    /// paired clients.
    func regeneratePairingToken() {
        applyServerSettingsMutation(
            SonderServerSettingsPlanner.rotatingPairingToken(in: serverSettings)
        )
    }

    private func applyServerSettingsMutation(_ mutation: SonderServerSettingsMutation) {
        serverSettings = mutation.settings
        SonderTheme.apply(preset: mutation.themePreset)
        addActivity(mutation.activity.title, detail: mutation.activity.detail, icon: mutation.activity.icon)
        commitLibraryMutation()
        if mutation.postsServerNotification {
            NotificationCenter.default.post(name: .sonderServerSettingsDidChange, object: nil)
        }
    }

    func setStorageFolder(_ result: Result<[URL], Error>) {
        applyStorageUpdate(
            SonderStorageCommandService(store: store).storageUpdate(from: result)
        )
    }

    private func applyStorageUpdate(_ result: SonderStorageUpdateResult) {
        if result.didUpdate,
           let storagePath = result.storagePath,
           let storageBookmark = result.storageBookmark {
            self.storagePath = storagePath
            self.storageBookmark = storageBookmark
        }
        addActivity(result.activity.title, detail: result.activity.detail, icon: result.activity.icon)
        if result.didUpdate {
            commitLibraryMutation()
        }
    }

    func chooseMediaLibraryRoot() {
        guard isBusy == false && isLoadingPersistedLibrary == false else {
            addActivity("Library picker unavailable", detail: "Wait for the current load, scan, or import to finish before adding folders.", icon: "hourglass")
            return
        }
        guard let rootURL = systemServices.chooseMediaLibraryRoot() else { return }
        addMediaLibraryRoot(rootURL)
    }

    func chooseMediaDirectories(kind: SonderLibraryImportKind) {
        guard isBusy == false && isLoadingPersistedLibrary == false else {
            addActivity("Library picker unavailable", detail: "Wait for the current load, scan, or import to finish before adding \(kind.label.lowercased()) folders.", icon: "hourglass")
            return
        }
        let urls = systemServices.chooseMediaDirectories(kind: kind)
        guard urls.isEmpty == false else { return }
        addMediaDirectories(urls, kind: kind)
    }

    func chooseCustomMediaDirectory(named name: String, styledAs kind: SonderLibraryImportKind) {
        guard isBusy == false && isLoadingPersistedLibrary == false else {
            addActivity("Library picker unavailable", detail: "Wait for the current load, scan, or import to finish before adding folders.", icon: "hourglass")
            return
        }
        guard let plan = SonderCustomLibraryPlanner.plan(named: name, kind: kind) else { return }
        guard let url = systemServices.chooseCustomMediaDirectory(name: plan.definition.name, kind: plan.definition.kind) else { return }
        libraryDefinitions.append(plan.definition)
        let addedDirectories = addMediaDirectories([url], kind: plan.definition.kind, scansAfterAdd: true, libraryID: plan.definition.id)
        guard addedDirectories.isEmpty == false else {
            libraryDefinitions.removeAll { $0.id == plan.definition.id }
            return
        }
        addActivity(plan.activity.title, detail: plan.activity.detail, icon: plan.activity.icon)
        commitLibraryMutation()
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
    func addMediaDirectories(_ urls: [URL], kind: SonderLibraryImportKind, scansAfterAdd: Bool = true, libraryID: UUID? = nil) -> [SonderMediaDirectory] {
        let resolvedLibraryID = libraryID ?? self.libraryID(for: kind)
        let mutation = SonderMediaDirectoryPlanner.upserting(
            urls: urls,
            kind: kind,
            libraryID: resolvedLibraryID,
            in: mediaDirectories
        ) { url in
            try store.bookmark(for: url)
        }
        mediaDirectories = mutation.directories
        for failure in mutation.failures {
            addActivity("Directory add failed", detail: "\(failure.url.lastPathComponent): \(failure.message)", icon: "exclamationmark.triangle")
        }

        if mutation.addedDirectories.isEmpty == false {
            addActivity("Added media directories", detail: "\(mutation.addedDirectories.count) folder(s) ready to scan.", icon: "folder.badge.plus")
            if scansAfterAdd {
                scanMediaDirectories(mutation.addedDirectories)
            }
        }
        return mutation.addedDirectories
    }

    func rescanMediaDirectories() {
        scanMediaDirectories(mediaDirectories)
    }

    func rescanMediaDirectory(_ id: UUID) {
        scanMediaDirectories(mediaDirectories.filter { $0.id == id })
    }

    private func enqueueScanDirectories(_ directories: [SonderMediaDirectory]) {
        var queuedIDs = Set(queuedScanDirectories.map(\.id))
        let newDirectories = directories.filter { queuedIDs.insert($0.id).inserted }
        guard newDirectories.isEmpty == false else { return }
        queuedScanDirectories.append(contentsOf: newDirectories)
        updateQueuedScanState()
        addActivity(
            "Queued index scan",
            detail: "\(newDirectories.count) folder(s) will scan after the current scan finishes.",
            icon: "tray.and.arrow.down"
        )
    }

    private func startQueuedScanIfAvailable() {
        guard scanProgress == nil else { return }
        guard queuedScanDirectories.isEmpty == false else { return }
        let directories = queuedScanDirectories
        queuedScanDirectories.removeAll(keepingCapacity: true)
        updateQueuedScanState()
        scanMediaDirectories(directories)
    }

    private func updateQueuedScanState() {
        queuedScanDirectoryCount = queuedScanDirectories.count
        queuedScanDirectorySummaries = queuedScanDirectories.map { directory in
            "\(directory.kind.label): \(directory.name)"
        }
    }

    private func markScanProgressUpdated(at date: Date = Date()) {
        activeScanUpdatedAt = date
    }

    private func scanMediaDirectories(_ directories: [SonderMediaDirectory]) {
        guard directories.isEmpty == false else { return }
        guard scanProgress == nil else {
            enqueueScanDirectories(directories)
            return
        }
        let purgedCount = purgePlaceholders()
        if purgedCount > 0 {
            addActivity("Removed placeholder records", detail: "Deleted \(purgedCount) non-playable placeholder item(s) before scanning.", icon: "trash")
            commitLibraryMutation()
        }
        let existingPaths = Set(items.compactMap(\.sourcePath))
        let startedAt = Date()
        activeScanStartedAt = startedAt
        activeScanUpdatedAt = startedAt
        scanProgress = SonderScanProgressFactory.preparing(directoriesTotal: directories.count)
        addActivity("Started index scan", detail: "\(directories.count) media folder(s) queued.", icon: "arrow.triangle.2.circlepath")

        let coordinator = SonderLibraryScanCoordinator(
            store: store,
            progressStride: Self.scanProgressUpdateStride,
            discoveryBatchSize: Self.scanIndexBatchSize
        )

        Task.detached {
            let result = await coordinator.scan(
                directories: directories,
                existingPaths: existingPaths
            ) { directory, offset, startFilesSeen, startMediaFound, currentIndexedCount in
                await MainActor.run {
                    self.activeScanKind = directory.kind.mediaKind
                    self.activeScanDirectoryPath = directory.path
                    self.markScanProgressUpdated()
                    self.addActivity("Scanning \(directory.kind.label)", detail: directory.path, icon: directory.kind.icon)
                    self.scanProgress = SonderScanProgressFactory.discovering(
                        directory: directory,
                        detail: directory.path,
                        filesSeen: startFilesSeen,
                        mediaFound: startMediaFound,
                        indexedCount: currentIndexedCount,
                        directoriesDone: offset + 1,
                        directoriesTotal: directories.count
                    )
                }
            } onProgress: { progress in
                await MainActor.run {
                    self.enqueueScanProgress(progress)
                }
            } onDiscoveredChunk: { chunk, directory, lastPath, startFilesSeen, startMediaFound, indexedSoFar, offset, directoriesTotal in
                await MainActor.run {
                    let currentProgress = self.pendingScanProgress ?? self.scanProgress
                    let progress = SonderScanProgressFactory.indexing(
                        directory: directory,
                        detail: lastPath,
                        filesSeen: currentProgress?.filesSeen ?? startFilesSeen,
                        mediaFound: max(currentProgress?.mediaFound ?? startMediaFound, startMediaFound + indexedSoFar),
                        indexedCount: indexedSoFar,
                        directoriesDone: offset + 1,
                        directoriesTotal: directoriesTotal
                    )
                    self.enqueueIndexedItems(chunk, progress: progress)
                }
            } onDirectoryFailure: { directory, error in
                await MainActor.run {
                    self.addActivity("Directory scan failed", detail: "\(directory.name): \(error.localizedDescription)", icon: "exclamationmark.triangle")
                }
            }

            await MainActor.run {
                self.flushPendingIndexUpdates()
                for index in self.mediaDirectories.indices {
                    if let update = result.directoryUpdates[self.mediaDirectories[index].id] {
                        self.mediaDirectories[index].lastIndexedCount = update.mediaCount
                        self.mediaDirectories[index].lastScannedFileCount = update.fileCount
                        self.mediaDirectories[index].lastUnsupportedCount = update.unsupportedCount
                        self.mediaDirectories[index].lastSkippedDuplicateCount = update.skippedDuplicateCount
                        self.mediaDirectories[index].lastParseFailureCount = update.parseFailureCount
                        self.mediaDirectories[index].lastScannedAt = update.scannedAt
                        self.addActivity(
                            "Scanned \(self.mediaDirectories[index].name)",
                            detail: "\(update.mediaCount) media / \(update.fileCount) files / \(update.unsupportedCount) unsupported / \(update.skippedDuplicateCount) duplicates.",
                            icon: self.mediaDirectories[index].kind.icon
                        )
                    }
                }
                self.scanProgress = SonderScanProgressFactory.completed(
                    title: "Indexing complete",
                    detail: "\(self.items.count) title(s) indexed. Refreshing local artwork next."
                )
                self.finishScanProgress(title: "Indexing complete", detail: "\(self.items.count) title(s) indexed. Refreshing local artwork next.")
                self.addActivity("Index scan complete", detail: "\(self.items.count) title(s) indexed across \(self.mediaDirectories.count) folder(s).", icon: "checkmark.circle")
                self.activeScanKind = nil
                self.activeScanDirectoryPath = nil
                self.commitLibraryMutation()
                self.refreshLocalAssets(for: result.discoveredItemIDs, directoriesTotal: directories.count)
            }
        }
    }

    private func refreshLocalAssets(for itemIDs: [UUID]? = nil, directoriesTotal: Int? = nil, showsProgress: Bool = true) {
        let targetIDs = Set(itemIDs ?? items.map(\.id))
        let targets = items.filter { targetIDs.contains($0.id) && $0.sourcePath != nil }
        guard targets.isEmpty == false else {
            if showsProgress {
                scanProgress = nil
                activeScanStartedAt = nil
                activeScanUpdatedAt = nil
                activeScanDirectoryPath = nil
                startQueuedScanIfAvailable()
            }
            return
        }

        if showsProgress {
            activeScanDirectoryPath = "Local asset refresh"
            markScanProgressUpdated()
            scanProgress = SonderScanProgressFactory.localAssets(
                completed: 0,
                total: targets.count,
                detail: "Checking posters and subtitles beside indexed media."
            )
            addActivity("Refreshing local assets", detail: "\(targets.count) title(s) queued for poster and subtitle discovery.", icon: "photo")
        }

        // Capture Sendable snapshots of each target's id + resolved URL so the detached
        // task can probe off the main actor without touching the @MainActor model.
        let refreshTargets = targets.compactMap { item -> SonderAssetRefreshTarget? in
            guard let url = item.playableURL else { return nil }
            return SonderAssetRefreshTarget(
                id: item.id,
                url: url,
                kind: item.kind,
                title: item.title,
                showTitle: item.showTitle,
                seasonNumber: item.seasonNumber,
                episodeNumber: item.episodeNumber
            )
        }
        let refreshService = SonderLocalAssetRefreshService(
            maxConcurrentRefreshes: Self.maxConcurrentAssetRefreshes,
            minProgressPublishInterval: Self.minAssetProgressPublishInterval
        )

        Task.detached {
            let summary = await refreshService.refresh(targets: refreshTargets) { progress in
                await MainActor.run {
                    if showsProgress {
                        self.markScanProgressUpdated()
                        self.scanProgress = SonderScanProgressFactory.localAssets(
                            completed: progress.completed,
                            total: progress.total,
                            detail: "Checked \(progress.completed) of \(progress.total) title(s) for posters, subtitles, and runtime."
                        )
                    }
                }
            }
            await MainActor.run {
                for index in self.items.indices {
                    let id = self.items[index].id
                    if let update = summary.assetUpdates[id] {
                        self.items[index].localPosterPath = update.posterPath
                        self.items[index].localBackdropPath = update.backdropPath
                        self.items[index].subtitlePaths = update.subtitlePaths
                    }
                    if let contextUpdate = summary.contextUpdates[id] {
                        self.applyContextUpdate(contextUpdate, toItemAt: index)
                    }
                    if let probe = summary.probeResults[id] {
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
                    let detail = "\(summary.updateCount) title(s) checked for posters, subtitles, and runtime."
                    if self.queuedScanDirectories.isEmpty {
                        self.finishScanProgress(title: "Artwork refresh complete", detail: detail)
                    } else {
                        self.scanProgress = nil
                    }
                    self.addActivity("Refreshed local assets", detail: detail, icon: "photo")
                } else {
                    self.prioritizedAssetRefreshIDs.subtract(refreshTargets.map(\.id))
                }
                self.commitLibraryMutation()
                if showsProgress {
                    self.refreshMetadata(for: targets.map(\.id))
                    self.startQueuedScanIfAvailable()
                }
            }
        }
    }

    func importFiles(_ result: Result<[URL], Error>) {
        let importResult = SonderManagedImportService(store: store).importFiles(
            result,
            storagePath: storagePath,
            storageBookmark: storageBookmark
        )
        for importedItem in importResult.importedItems {
            items.insert(importedItem.item, at: 0)
            probeImportedItem(importedItem)
        }
        for activity in importResult.activities {
            addActivity(activity.title, detail: activity.detail, icon: activity.icon)
        }
        if importResult.importedCount > 0 {
            commitLibraryMutation()
            refreshMetadata(for: importResult.importedIDs)
        }
    }

    private func probeImportedItem(_ importedItem: SonderManagedImportItem) {
        let itemID = importedItem.item.id
        let managedURL = importedItem.managedURL
        Task { [weak self] in
            guard let self else { return }
            let didAccess = managedURL.startAccessingSecurityScopedResource()
            defer { if didAccess { managedURL.stopAccessingSecurityScopedResource() } }
            let probe = await SonderMediaProbe.probe(url: managedURL)
            await MainActor.run {
                self.applyProbe(probe, toItemID: itemID)
            }
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
        let refreshService = SonderMetadataRefreshService(
            cacheRoot: store.rootURL.appendingPathComponent("MetadataCache", isDirectory: true)
        )
        Task.detached {
            let enrichments = await refreshService.refresh(items: targets)
            await MainActor.run {
                for (itemID, enrichment) in enrichments {
                    self.applyMetadata(enrichment, toItemID: itemID)
                }
            }
        }
    }

    private func applyContextUpdate(_ update: SonderContextMetadataUpdate, toItemAt index: Int) {
        if items[index].kind == .tvShow {
            if let showTitle = update.showTitle, showTitle.isEmpty == false {
                items[index].showTitle = showTitle
            }
            if let title = update.title, title.isEmpty == false {
                items[index].title = title
            }
            if let subtitle = update.subtitle, subtitle.isEmpty == false {
                items[index].subtitle = subtitle
            }
        }
        if let summary = update.summary, items[index].summary.isSonderPlaceholderSummary {
            items[index].summary = summary
        }
        if let studio = update.studio, items[index].studio == "Local" || items[index].studio == "Remote Library" {
            items[index].studio = studio
        }
        if let year = update.year, items[index].year == Calendar.current.component(.year, from: Date()) {
            items[index].year = year
        }
        if update.tags.isEmpty == false {
            items[index].tags = Array(Set(items[index].tags + update.tags)).sorted()
        }
        if items[index].metadataIDSource == nil {
            items[index].metadataIDSource = update.metadataIDSource
        }
        if items[index].metadataID == nil {
            items[index].metadataID = update.metadataID
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
        guard let item = audiobookItem(id: itemID) else { return [] }
        return SonderAudiobookChapterService(storeRootURL: store.rootURL).chapters(for: item)
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

    func libraryResponseSnapshot(theme: SonderThemeSnapshot) -> SonderLibraryResponse {
        SonderLibraryResponse(
            items: items,
            progress: progressRecords,
            mediaDirectories: mediaDirectories,
            scanProgress: scanProgress,
            activity: Array(activity.prefix(40)),
            serverSettings: serverSettings,
            theme: theme
        )
    }

    func statusSnapshot() -> SonderStatusResponse {
        SonderStatusResponse(
            itemCount: items.count,
            playableCount: playableCount,
            mediaDirectoryCount: mediaDirectories.count,
            scanProgress: scanProgress,
            activeScanDirectoryPath: activeScanDirectoryPath,
            activeScanStartedAt: activeScanStartedAt,
            activeScanUpdatedAt: activeScanUpdatedAt,
            queuedScanDirectoryCount: queuedScanDirectoryCount,
            queuedScanDirectorySummaries: queuedScanDirectorySummaries,
            isLoadingPersistedLibrary: isLoadingPersistedLibrary,
            isBusy: isBusy
        )
    }

    func playbackSessionSnapshot(itemID: UUID, audioTracks: [SonderPlaybackTrack], subtitleTracks: [SonderPlaybackTrack]) -> SonderPlaybackSessionResponse? {
        guard let item = item(id: itemID), item.hasFile else { return nil }
        let progress = progressRecord(for: item)
        let duration = max(progress?.duration ?? item.durationSeconds, item.durationSeconds, 1)
        let seconds = min(max(progress?.seconds ?? item.progressSeconds, 0), duration)
        return SonderPlaybackSessionResponse(
            itemID: item.id,
            streamURL: "/stream/\(item.id.uuidString)",
            seconds: seconds,
            duration: duration,
            percent: duration > 0 ? min(max(seconds / duration, 0), 1) : 0,
            updatedAt: progress?.updatedAt,
            audioTrackID: progress?.audioTrackID,
            subtitleTrackID: progress?.subtitleTrackID,
            subtitlesEnabled: progress?.subtitlesEnabled ?? false,
            audioTracks: audioTracks,
            subtitleTracks: subtitleTracks
        )
    }

    func updatePlaybackSession(itemID: UUID, update: SonderProgressUpdate) {
        applyProgressUpdate(
            itemID: itemID,
            seconds: update.seconds,
            duration: update.duration,
            audioTrackID: update.audioTrackID,
            subtitleTrackID: update.subtitleTrackID,
            subtitlesEnabled: update.subtitlesEnabled,
            logsActivity: false
        )
    }

    func play(_ item: SonderMediaItem) {
        prioritizeAssets(for: item.id)
        let playbackItem = item.sourceBookmark == nil ? itemRepairingSourceBookmarkIfPossible(item) : item
        guard let playableURL = playbackItem.playableURL else {
            addActivity("Play unavailable", detail: "\(item.title) needs a local media file or refreshed folder permission.", icon: "exclamationmark.triangle")
            NSLog("TM Sonder play unavailable: %@ has no playable URL", item.title)
            return
        }
        NSLog("TM Sonder play request: %@ format=%@ path=%@", playbackItem.title, playbackItem.format.rawValue, playableURL.path)
        if playbackItem.kind == .movie || playbackItem.kind == .tvShow || playbackItem.kind == .documentary {
            nowPlayingItem = playbackItem
            addActivity("Playing \(playbackItem.title)", detail: "Opening \(playableURL.lastPathComponent) in Sonder's built-in player.", icon: "play.fill")
            return
        }
        guard let activity = SonderPlaybackCommands.open(playbackItem, using: systemServices) else {
            addActivity("Play unavailable", detail: "\(playbackItem.title) could not be opened.", icon: "exclamationmark.triangle")
            return
        }
        addActivity(activity.title, detail: activity.detail, icon: activity.icon)
    }

    private func itemRepairingSourceBookmarkIfPossible(_ item: SonderMediaItem) -> SonderMediaItem {
        guard let sourcePath = item.sourcePath else { return item }
        guard let directory = mediaDirectories.first(where: { sourcePath.hasPrefix($0.path) }) else { return item }
        let directoryURL = SonderStore.storageURL(from: directory.bookmark) ?? URL(fileURLWithPath: directory.path, isDirectory: true)
        let didAccessDirectory = directoryURL.startAccessingSecurityScopedResource()
        defer {
            if didAccessDirectory {
                directoryURL.stopAccessingSecurityScopedResource()
            }
        }

        let fileURL = URL(fileURLWithPath: sourcePath)
        guard FileManager.default.fileExists(atPath: fileURL.path), let bookmark = try? SonderStore.bookmark(for: fileURL) else {
            return item
        }

        var repairedItem = item
        repairedItem.sourceBookmark = bookmark
        if let index = items.firstIndex(where: { $0.id == item.id }) {
            items[index].sourceBookmark = bookmark
            commitLibraryMutation()
            addActivity("Repaired media permission", detail: fileURL.lastPathComponent, icon: "lock.open")
        }
        return repairedItem
    }

    func closePlayer() {
        nowPlayingItem = nil
    }

    func reportPlaybackIssue(title: String, detail: String) {
        NSLog("TM Sonder playback issue: %@ - %@", title, detail)
        addActivity(title, detail: detail, icon: "exclamationmark.triangle")
    }

    func resetLibraryDatabaseForTesting() {
        guard isBusy == false && isLoadingPersistedLibrary == false else {
            addActivity("Reset unavailable", detail: "Wait for the current load, scan, or import to finish before resetting the database.", icon: "hourglass")
            return
        }

        pendingIndexFlushTask?.cancel()
        pendingIndexFlushTask = nil
        pendingIndexedItems.removeAll(keepingCapacity: true)
        pendingScanProgress = nil
        items.removeAll()
        progressRecords.removeAll()
        collections.removeAll()
        activity.removeAll()
        conversionJobs.removeAll()
        libraryDefinitions = SonderLibraryDefinition.defaults
        mediaDirectories.removeAll()
        scanProgress = nil
        activeScanKind = nil
        activeScanDirectoryPath = nil
        activeScanStartedAt = nil
        activeScanUpdatedAt = nil
        queuedScanDirectories.removeAll(keepingCapacity: true)
        updateQueuedScanState()
        prioritizedAssetRefreshIDs.removeAll()
        nowPlayingItem = nil
        didAttemptPlexImport = false
        didAttemptAudiobookImport = false
        plexImportStatus = PlexImportStatus()
        audiobookImportStatus = SonderAudiobookImportStatus()

        addActivity("Reset library database", detail: "Cleared imported titles, progress, collections, scanned folders, and conversion jobs. Media files were not deleted.", icon: "trash")
        commitLibraryMutation()
    }

    func progress(for item: SonderMediaItem) -> Double {
        SonderPlaybackCommands.progress(for: item, record: progressRecord(for: item))
    }

    func progressRecord(for item: SonderMediaItem) -> SonderProgress? {
        progressRecords.first { $0.itemID == item.id }
    }

    func progressLabel(for item: SonderMediaItem) -> String {
        SonderPlaybackCommands.progressLabel(for: item, record: progressRecord(for: item))
    }

    func updateProgress(itemID: UUID, seconds: Double, duration: Double) {
        applyProgressUpdate(itemID: itemID, seconds: seconds, duration: duration, logsActivity: true)
    }

    func savePlaybackProgress(itemID: UUID, seconds: Double, duration: Double) {
        applyProgressUpdate(itemID: itemID, seconds: seconds, duration: duration, logsActivity: false)
    }

    private func applyProgressUpdate(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil,
        logsActivity: Bool
    ) {
        let update = SonderPlaybackCommands.progressUpdate(
            itemID: itemID,
            seconds: seconds,
            duration: duration,
            audioTrackID: audioTrackID,
            subtitleTrackID: subtitleTrackID,
            subtitlesEnabled: subtitlesEnabled,
            records: progressRecords
        )
        progressRecords = update.records
        if logsActivity, let item = item(id: itemID) {
            addActivity("Updated progress", detail: "\(item.title) is now \(Int((update.seconds / update.duration) * 100))% watched.", icon: "chart.line.uptrend.xyaxis")
        }
        commitLibraryMutation()
    }

    func createCollection(named name: String, kind: SonderCollectionKind = .collection) {
        applyCollectionMutation(
            SonderCollectionPlanner.creating(named: name, kind: kind, in: collections)
        )
    }

    func renameForPlex(_ item: SonderMediaItem) {
        let result = SonderFileCommandService(store: store).renameForPlex(item)
        if result.didRename,
           let itemID = result.itemID,
           let index = items.firstIndex(where: { $0.id == itemID }),
           let sourcePath = result.sourcePath,
           let format = result.format {
            items[index].sourcePath = sourcePath
            items[index].format = format
        }
        addActivity(result.activity.title, detail: result.activity.detail, icon: result.activity.icon)
        if result.didRename {
            commitLibraryMutation()
        }
    }

    func convertAllPlayableToMP4() {
        let candidates = SonderConversionCommands.candidates(from: items)
        guard candidates.isEmpty == false else {
            addActivity("Conversion skipped", detail: "No playable non-MP4 titles to convert.", icon: "checkmark.circle")
            return
        }
        addActivity("Queueing conversions", detail: "\(candidates.count) title(s), \(SonderConversionCommands.maxConcurrentConversions) at a time.", icon: "arrow.triangle.2.circlepath")
        Task { [weak self] in
            guard let self else { return }
            await SonderConcurrencyLimiter.run(limit: SonderConversionCommands.maxConcurrentConversions, over: candidates) { item in
                await MainActor.run { self.convertToMP4(item) }
            }
        }
    }

    func convertToMP4(_ item: SonderMediaItem) {
        let conversionService = SonderFileCommandService(store: store)
        guard item.playableURL != nil else {
            addActivity("Conversion unavailable", detail: "Select a playable file first.", icon: "exclamationmark.triangle")
            return
        }
        guard let plan = conversionService.conversionPlan(for: item) else {
            addActivity("Conversion skipped", detail: "\(item.title) is already MP4.", icon: "checkmark.circle")
            return
        }

        conversionJobs.insert(plan.job, at: 0)
        commitLibraryMutation()

        Task {
            let completion = await conversionService.convert(plan)
            await MainActor.run {
                self.applyConversionCompletion(completion)
            }
        }
    }

    private func applyConversionCompletion(_ completion: SonderConversionCompletion) {
        if let jobIndex = conversionJobs.firstIndex(where: { $0.id == completion.jobID }) {
            conversionJobs[jobIndex].status = completion.didConvert ? .completed : .failed
        }
        if completion.didConvert, let itemIndex = items.firstIndex(where: { $0.id == completion.itemID }) {
            items[itemIndex].sourcePath = completion.outputPath
            items[itemIndex].format = .mp4
            addActivity("Converted to MP4", detail: completion.outputFileName, icon: "checkmark.circle")
        } else if completion.didConvert == false {
            addActivity("Conversion failed", detail: "\(completion.sourceFileName). Use a companion HandBrake workflow for unsupported codecs.", icon: "exclamationmark.triangle")
        }
        commitLibraryMutation()
    }

    func add(_ item: SonderMediaItem, to collection: SonderCollection) {
        applyCollectionMutation(
            SonderCollectionPlanner.adding(item: item, to: collection, in: collections)
        )
    }

    private func applyCollectionMutation(_ mutation: SonderCollectionMutation) {
        guard mutation.didMutate else { return }
        collections = mutation.collections
        if let activity = mutation.activity {
            addActivity(activity.title, detail: activity.detail, icon: activity.icon)
        }
        commitLibraryMutation()
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
        let snapshot = SonderSnapshot.libraryState(
            items: items,
            progressRecords: progressRecords,
            collections: collections,
            activity: activity,
            storagePath: storagePath,
            storageBookmark: storageBookmark,
            conversionJobs: conversionJobs,
            libraryDefinitions: libraryDefinitions,
            mediaDirectories: mediaDirectories,
            serverSettings: serverSettings
        )
        persistenceCoordinator.scheduleSave(snapshot: snapshot, store: store)
    }

    private func libraryID(for kind: SonderLibraryImportKind) -> UUID {
        libraryDefinitions.first { $0.kind == kind }?.id ?? kind.defaultLibraryID
    }

}
