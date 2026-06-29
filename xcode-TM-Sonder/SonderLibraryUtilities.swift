import Foundation

nonisolated struct SonderDetectedMediaLibraryDirectory: Hashable, Sendable {
    var kind: SonderLibraryImportKind
    var url: URL
}

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

nonisolated enum SonderMediaLibraryDirectoryScanner {
    static func detect(in rootURL: URL, fileManager: FileManager = .default) -> [SonderDetectedMediaLibraryDirectory] {
        let didAccess = rootURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                rootURL.stopAccessingSecurityScopedResource()
            }
        }

        guard let children = try? fileManager.contentsOfDirectory(
            at: rootURL,
            includingPropertiesForKeys: [.isDirectoryKey],
            options: [.skipsHiddenFiles]
        ) else { return [] }

        let directories = children.filter { url in
            (try? url.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) == true
        }
        var matches: [SonderLibraryImportKind: URL] = [:]
        for url in directories {
            guard let kind = SonderMediaLibraryDetector.kind(forFolderName: url.lastPathComponent) else { continue }
            if matches[kind] == nil {
                matches[kind] = url
            }
        }

        return SonderLibraryImportKind.mediaLibraryDetectionPriority.compactMap { kind in
            matches[kind].map { SonderDetectedMediaLibraryDirectory(kind: kind, url: $0) }
        }
    }
}

nonisolated enum SonderProgressRecords {
    static func upserting(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        in records: [SonderProgress],
        now: Date = Date()
    ) -> (records: [SonderProgress], seconds: Double, duration: Double) {
        let sanitizedDuration = max(duration, 1)
        let sanitizedSeconds = min(max(seconds, 0), sanitizedDuration)
        var updatedRecords = records

        if let index = updatedRecords.firstIndex(where: { $0.itemID == itemID }) {
            updatedRecords[index].seconds = sanitizedSeconds
            updatedRecords[index].duration = sanitizedDuration
            updatedRecords[index].updatedAt = now
        } else {
            updatedRecords.append(SonderProgress(itemID: itemID, seconds: sanitizedSeconds, duration: sanitizedDuration, updatedAt: now))
        }

        return (updatedRecords, sanitizedSeconds, sanitizedDuration)
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
