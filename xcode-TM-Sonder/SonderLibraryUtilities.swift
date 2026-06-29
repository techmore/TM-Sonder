import Foundation

nonisolated enum SonderMediaLibraryDetector {
    static func kind(forFolderName folderName: String) -> SonderLibraryImportKind? {
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
}

nonisolated enum SonderConcurrencyLimiter {
    /// Runs `work` over `items` with at most `limit` concurrent invocations. Each work
    /// closure awaits completion before the slot is released.
    static func run<Item>(
        limit: Int,
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
}
