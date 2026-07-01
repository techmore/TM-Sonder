import Foundation

/// Thread-safe cache of pre-serialized HTTP responses. The library invalidates
/// this whenever its state changes, so the HTTP server never needs to hop to the
/// main actor for reads. Encoding happens at most once between mutations.
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

nonisolated struct SonderPreparedLibraryState {
    var items: [SonderMediaItem]
    var progressRecords: [SonderProgress]
    var collections: [SonderCollection]
    var activity: [SonderActivityEvent]
    var conversionJobs: [SonderConversionJob]
    var libraryDefinitions: [SonderLibraryDefinition]
    var mediaDirectories: [SonderMediaDirectory]
    var serverSettings: SonderServerSettings
    var storagePath: String
    var storageBookmark: Data?
    var removedPlaceholderCount: Int
    var derivedData: SonderDerivedData

    static func prepare(snapshot: SonderSnapshot, store: SonderStore) -> SonderPreparedLibraryState {
        let loadedItems = snapshot.items.filter { $0.isPlaceholder == false }
        let itemIDs = Set(loadedItems.map(\.id))
        let progressRecords = snapshot.progress.filter { itemIDs.contains($0.itemID) }
        let collections = snapshot.collections.map { collection in
            var copy = collection
            copy.itemIDs.removeAll { itemIDs.contains($0) == false }
            return copy
        }.filter { $0.itemIDs.isEmpty == false }
        let libraryDefinitions = snapshot.libraryDefinitions ?? SonderLibraryDefinition.defaults
        let mediaDirectories = SonderMediaDirectoryPlanner.migrated(snapshot.mediaDirectories ?? [], libraries: libraryDefinitions)
        let derivedData = SonderDerivedData.make(items: loadedItems, progressRecords: progressRecords)

        return SonderPreparedLibraryState(
            items: loadedItems,
            progressRecords: progressRecords,
            collections: collections,
            activity: snapshot.activity,
            conversionJobs: snapshot.conversionJobs ?? [],
            libraryDefinitions: libraryDefinitions,
            mediaDirectories: mediaDirectories,
            serverSettings: snapshot.serverSettings ?? .default,
            storagePath: snapshot.storagePath ?? store.uploadsURL.path,
            storageBookmark: snapshot.storageBookmark,
            removedPlaceholderCount: snapshot.items.count - loadedItems.count,
            derivedData: derivedData
        )
    }
}

nonisolated final class SonderLibraryPersistenceCoordinator: @unchecked Sendable {
    private let saveQueue = DispatchQueue(label: "tm.sonder.save", qos: .utility)
    private var saveWorkItem: DispatchWorkItem?
    private let coalescingDelay: DispatchTimeInterval

    init(coalescingDelay: DispatchTimeInterval = .milliseconds(500)) {
        self.coalescingDelay = coalescingDelay
    }

    func scheduleSave(snapshot: SonderSnapshot, store: SonderStore) {
        saveWorkItem?.cancel()
        let workItem = DispatchWorkItem {
            store.save(snapshot)
        }
        saveWorkItem = workItem
        saveQueue.asyncAfter(deadline: .now() + coalescingDelay, execute: workItem)
    }
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

    static func libraryState(
        items: [SonderMediaItem],
        progressRecords: [SonderProgress],
        collections: [SonderCollection],
        activity: [SonderActivityEvent],
        storagePath: String,
        storageBookmark: Data?,
        conversionJobs: [SonderConversionJob],
        libraryDefinitions: [SonderLibraryDefinition],
        mediaDirectories: [SonderMediaDirectory],
        serverSettings: SonderServerSettings
    ) -> SonderSnapshot {
        SonderSnapshot(
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
    }
}
