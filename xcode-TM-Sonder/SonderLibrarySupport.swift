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
