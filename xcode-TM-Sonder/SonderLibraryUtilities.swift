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

extension SonderLibraryImportKind {
    nonisolated var mediaKind: SonderMediaKind {
        switch self {
        case .movies: .movie
        case .tvShows: .tvShow
        case .audiobooks: .audiobook
        case .ebooks: .ebook
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

nonisolated struct SonderMediaDirectoryAddFailure: Hashable {
    var url: URL
    var message: String
}

nonisolated struct SonderMediaDirectoryMutation: Hashable {
    var directories: [SonderMediaDirectory]
    var addedDirectories: [SonderMediaDirectory]
    var failures: [SonderMediaDirectoryAddFailure]
}

nonisolated enum SonderMediaDirectoryPlanner {
    static func upserting(
        urls: [URL],
        kind: SonderLibraryImportKind,
        libraryID: UUID,
        in existingDirectories: [SonderMediaDirectory],
        bookmarkProvider: (URL) throws -> Data
    ) -> SonderMediaDirectoryMutation {
        var directories = existingDirectories
        var addedDirectories: [SonderMediaDirectory] = []
        var failures: [SonderMediaDirectoryAddFailure] = []

        for url in urls {
            do {
                let bookmark = try bookmarkProvider(url)
                let directory = SonderMediaDirectory(
                    name: url.lastPathComponent,
                    path: url.path,
                    bookmark: bookmark,
                    kind: kind,
                    libraryID: libraryID
                )
                if let existingIndex = directories.firstIndex(where: { $0.path == url.path && $0.kind == kind && $0.libraryID == libraryID }) {
                    directories[existingIndex].bookmark = bookmark
                    directories[existingIndex].name = url.lastPathComponent
                    directories[existingIndex].libraryID = libraryID
                    addedDirectories.append(directories[existingIndex])
                } else {
                    directories.append(directory)
                    addedDirectories.append(directory)
                }
            } catch {
                failures.append(SonderMediaDirectoryAddFailure(url: url, message: error.localizedDescription))
            }
        }

        return SonderMediaDirectoryMutation(
            directories: directories,
            addedDirectories: addedDirectories,
            failures: failures
        )
    }

    static func migrated(_ directories: [SonderMediaDirectory], libraries: [SonderLibraryDefinition]) -> [SonderMediaDirectory] {
        directories.map { directory in
            var copy = directory
            if libraries.contains(where: { $0.id == copy.libraryID }) == false {
                copy.libraryID = libraries.first { $0.kind == copy.kind }?.id ?? copy.kind.defaultLibraryID
            }
            return copy
        }
    }
}

nonisolated struct SonderCustomLibraryPlan: Hashable, Sendable {
    var definition: SonderLibraryDefinition
    var activity: SonderLibraryActivityDraft
}

nonisolated enum SonderCustomLibraryPlanner {
    static func plan(
        named name: String,
        kind: SonderLibraryImportKind,
        idProvider: () -> UUID = UUID.init
    ) -> SonderCustomLibraryPlan? {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        guard isCustomLibraryKind(kind) else { return nil }
        let definition = SonderLibraryDefinition(id: idProvider(), name: trimmed, kind: kind)
        return SonderCustomLibraryPlan(
            definition: definition,
            activity: SonderLibraryActivityDraft(
                title: "Added custom library",
                detail: "\(trimmed) scans as \(kind.label.lowercased()).",
                icon: "folder.badge.plus"
            )
        )
    }

    static func isCustomLibraryKind(_ kind: SonderLibraryImportKind) -> Bool {
        kind == .movies || kind == .tvShows
    }
}

nonisolated struct SonderServerSettingsMutation: Hashable, Sendable {
    var settings: SonderServerSettings
    var activity: SonderLibraryActivityDraft
    var themePreset: SonderThemePreset
    var postsServerNotification: Bool
}

nonisolated enum SonderServerSettingsPlanner {
    static func updating(
        _ settings: SonderServerSettings,
        isEnabled: Bool? = nil,
        allowLAN: Bool? = nil,
        port: Int? = nil,
        pairingToken: String? = nil,
        tokenGenerator: () -> String = SonderServerSettings.generateToken
    ) -> SonderServerSettingsMutation {
        var updated = settings
        if let isEnabled {
            updated.isEnabled = isEnabled
        }
        if let allowLAN {
            updated.allowLAN = allowLAN
            if allowLAN && updated.pairingToken.isEmpty {
                updated.pairingToken = tokenGenerator()
            }
        }
        if let port {
            updated.port = clampedPort(port)
        }
        if let pairingToken {
            updated.pairingToken = pairingToken
        }
        return SonderServerSettingsMutation(
            settings: updated,
            activity: SonderLibraryActivityDraft(
                title: "Updated server settings",
                detail: updated.statusLabel,
                icon: "network"
            ),
            themePreset: themePreset(for: updated),
            postsServerNotification: true
        )
    }

    static func applyingTheme(_ preset: SonderThemePreset, to settings: SonderServerSettings) -> SonderServerSettingsMutation {
        var updated = settings
        updated.themePreset = preset.rawValue
        return SonderServerSettingsMutation(
            settings: updated,
            activity: SonderLibraryActivityDraft(
                title: "Updated theme",
                detail: preset.label,
                icon: "paintpalette"
            ),
            themePreset: preset,
            postsServerNotification: false
        )
    }

    static func rotatingPairingToken(
        in settings: SonderServerSettings,
        tokenGenerator: () -> String = SonderServerSettings.generateToken
    ) -> SonderServerSettingsMutation {
        var updated = settings
        updated.pairingToken = tokenGenerator()
        return SonderServerSettingsMutation(
            settings: updated,
            activity: SonderLibraryActivityDraft(
                title: "Rotated LAN pairing token",
                detail: "Previously paired clients must re-pair.",
                icon: "key.fill"
            ),
            themePreset: themePreset(for: updated),
            postsServerNotification: true
        )
    }

    static func clampedPort(_ port: Int) -> Int {
        min(max(port, 1024), 65535)
    }

    private static func themePreset(for settings: SonderServerSettings) -> SonderThemePreset {
        SonderThemePreset(rawValue: settings.themePreset) ?? .earthy
    }
}

nonisolated struct SonderCollectionMutation: Hashable, Sendable {
    var collections: [SonderCollection]
    var activity: SonderLibraryActivityDraft?
    var didMutate: Bool
}

nonisolated enum SonderCollectionPlanner {
    static func creating(
        named name: String,
        kind: SonderCollectionKind,
        in collections: [SonderCollection]
    ) -> SonderCollectionMutation {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            return SonderCollectionMutation(collections: collections, activity: nil, didMutate: false)
        }

        var updatedCollections = collections
        updatedCollections.insert(SonderCollection(name: trimmed, kind: kind, itemIDs: []), at: 0)
        return SonderCollectionMutation(
            collections: updatedCollections,
            activity: SonderLibraryActivityDraft(
                title: "Created \(kind.label.lowercased())",
                detail: trimmed,
                icon: kind.icon
            ),
            didMutate: true
        )
    }

    static func adding(
        item: SonderMediaItem,
        to collection: SonderCollection,
        in collections: [SonderCollection]
    ) -> SonderCollectionMutation {
        guard let index = collections.firstIndex(where: { $0.id == collection.id }) else {
            return SonderCollectionMutation(collections: collections, activity: nil, didMutate: false)
        }
        guard collections[index].itemIDs.contains(item.id) == false else {
            return SonderCollectionMutation(collections: collections, activity: nil, didMutate: false)
        }

        var updatedCollections = collections
        updatedCollections[index].itemIDs.append(item.id)
        return SonderCollectionMutation(
            collections: updatedCollections,
            activity: SonderLibraryActivityDraft(
                title: "Added to collection",
                detail: "\(item.title) -> \(collection.name)",
                icon: "plus.circle"
            ),
            didMutate: true
        )
    }
}

nonisolated enum SonderProgressRecords {
    static func upserting(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil,
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
            if let audioTrackID {
                updatedRecords[index].audioTrackID = audioTrackID
            }
            if let subtitleTrackID {
                updatedRecords[index].subtitleTrackID = subtitleTrackID
            }
            if let subtitlesEnabled {
                updatedRecords[index].subtitlesEnabled = subtitlesEnabled
            }
        } else {
            updatedRecords.append(SonderProgress(
                itemID: itemID,
                seconds: sanitizedSeconds,
                duration: sanitizedDuration,
                updatedAt: now,
                audioTrackID: audioTrackID,
                subtitleTrackID: subtitleTrackID,
                subtitlesEnabled: subtitlesEnabled
            ))
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
