import Foundation

nonisolated struct SonderScanDiagnostics: Codable, Hashable, Sendable {
    var mediaCount: Int
    var fileCount: Int
    var unsupportedCount: Int
    var skippedDuplicateCount: Int
    var parseFailureCount: Int
    var scannedAt: Date
}

nonisolated struct SonderScanProgress: Codable, Identifiable, Hashable, Sendable {
    var id = UUID()
    var phase: SonderScanPhase = .fastTitles
    var title: String
    var detail: String
    var filesSeen: Int
    var mediaFound: Int
    var indexedCount: Int
    var directoriesDone: Int
    var directoriesTotal: Int
    var workDone: Int
    var workTotal: Int

    var fraction: Double {
        guard workTotal > 0 else { return 0 }
        return min(Double(workDone) / Double(workTotal), 0.98)
    }
}

nonisolated enum SonderScanPhase: String, Codable, Hashable {
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

nonisolated enum SonderScanProgressFactory {
    static func preparing(directoriesTotal: Int) -> SonderScanProgress {
        SonderScanProgress(
            phase: .fastTitles,
            title: "Preparing scan",
            detail: "Starting fast title index.",
            filesSeen: 0,
            mediaFound: 0,
            indexedCount: 0,
            directoriesDone: 0,
            directoriesTotal: directoriesTotal,
            workDone: 0,
            workTotal: max(directoriesTotal, 1)
        )
    }

    static func discovering(
        directory: SonderMediaDirectory,
        detail: String,
        filesSeen: Int,
        mediaFound: Int,
        indexedCount: Int,
        directoriesDone: Int,
        directoriesTotal: Int
    ) -> SonderScanProgress {
        SonderScanProgress(
            phase: .fastTitles,
            title: "Discovering \(directory.kind.label)",
            detail: detail,
            filesSeen: filesSeen,
            mediaFound: mediaFound,
            indexedCount: indexedCount,
            directoriesDone: directoriesDone,
            directoriesTotal: directoriesTotal,
            workDone: max(directoriesDone - 1, 0),
            workTotal: max(directoriesTotal, 1)
        )
    }

    static func indexing(
        directory: SonderMediaDirectory,
        detail: String,
        filesSeen: Int,
        mediaFound: Int,
        indexedCount: Int,
        directoriesDone: Int,
        directoriesTotal: Int
    ) -> SonderScanProgress {
        SonderScanProgress(
            phase: .fastTitles,
            title: "Indexing \(directory.kind.label)",
            detail: detail,
            filesSeen: filesSeen,
            mediaFound: mediaFound,
            indexedCount: indexedCount,
            directoriesDone: directoriesDone,
            directoriesTotal: directoriesTotal,
            workDone: max(directoriesDone - 1, 0),
            workTotal: max(directoriesTotal, 1)
        )
    }

    static func localAssets(directoriesTotal: Int) -> SonderScanProgress {
        SonderScanProgress(
            phase: .localAssets,
            title: "Finding local artwork",
            detail: "Checking posters and subtitles beside indexed media.",
            filesSeen: 0,
            mediaFound: 0,
            indexedCount: 0,
            directoriesDone: 0,
            directoriesTotal: max(directoriesTotal, 1),
            workDone: 0,
            workTotal: max(directoriesTotal, 1)
        )
    }

    static func localAssets(
        completed: Int,
        total: Int,
        detail: String
    ) -> SonderScanProgress {
        SonderScanProgress(
            phase: .localAssets,
            title: "Finding local artwork",
            detail: detail,
            filesSeen: completed,
            mediaFound: total,
            indexedCount: completed,
            directoriesDone: 0,
            directoriesTotal: max(total, 1),
            workDone: completed,
            workTotal: max(total, 1)
        )
    }

    static func completed(title: String, detail: String) -> SonderScanProgress {
        SonderScanProgress(
            phase: .metadata,
            title: title,
            detail: detail,
            filesSeen: 0,
            mediaFound: 0,
            indexedCount: 0,
            directoriesDone: 1,
            directoriesTotal: 1,
            workDone: 1,
            workTotal: 1
        )
    }
}

nonisolated struct SonderServerSettings: Codable, Hashable, Sendable {
    var isEnabled: Bool
    var allowLAN: Bool
    var port: Int
    var themePreset: String
    /// Bearer token required for all non-localhost requests when LAN sharing is on.
    /// Localhost (loopback) requests are always exempt so the host Mac keeps working
    /// without pairing. Empty means LAN sharing is open by default.
    var pairingToken: String

    static let `default` = SonderServerSettings(isEnabled: true, allowLAN: true, port: 8797, themePreset: SonderThemePreset.earthy.rawValue, pairingToken: "")

    init(isEnabled: Bool, allowLAN: Bool, port: Int, themePreset: String, pairingToken: String) {
        self.isEnabled = isEnabled
        self.allowLAN = allowLAN
        self.port = port
        self.themePreset = themePreset
        self.pairingToken = pairingToken
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        isEnabled = try container.decode(Bool.self, forKey: .isEnabled)
        allowLAN = try container.decode(Bool.self, forKey: .allowLAN)
        port = try container.decode(Int.self, forKey: .port)
        themePreset = try container.decodeIfPresent(String.self, forKey: .themePreset) ?? SonderThemePreset.earthy.rawValue
        pairingToken = try container.decode(String.self, forKey: .pairingToken)
    }

    /// Generates a fresh random pairing token. Used when the user first enables LAN.
    static func generateToken() -> String {
        let bytes = (0..<24).map { _ in UInt8.random(in: 0...255) }
        return Data(bytes).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    var statusLabel: String {
        guard isEnabled else { return "Web server is off." }
        if allowLAN == false { return "Web server is local-only." }
        return pairingToken.isEmpty ? "Web server is available on the LAN (open by default)." : "Web server is available on the LAN (pairing required)."
    }
}

extension Notification.Name {
    static let sonderServerSettingsDidChange = Notification.Name("SonderServerSettingsDidChange")
}

struct SonderMediaScanResult: Sendable {
    var filesSeen: Int
    var mediaFound: Int
    var unsupportedMediaCount: Int
}

struct SonderRootMaterial: Sendable {
    var url: URL
    var name: String
    var kind: SonderLibraryImportKind

    nonisolated var mediaKind: SonderMediaKind {
        switch kind {
        case .movies: .movie
        case .tvShows: .tvShow
        case .audiobooks: .audiobook
        case .ebooks: .ebook
        }
    }
}

nonisolated struct SonderLocalAssetUpdate: Sendable {
    var posterPath: String?
    var backdropPath: String?
    var subtitlePaths: [String]
}

nonisolated struct SonderContextMetadataUpdate: Sendable, Hashable {
    var title: String?
    var subtitle: String?
    var showTitle: String?
    var summary: String?
    var studio: String?
    var year: Int?
    var tags: [String]
    var metadataIDSource: String?
    var metadataID: String?
    var posterPath: String?
    var backdropPath: String?
}

nonisolated struct SonderAssetRefreshResult: Sendable {
    var itemID: UUID
    var assetUpdate: SonderLocalAssetUpdate
    var contextUpdate: SonderContextMetadataUpdate?
    var probe: SonderMediaProbe.Result
}

actor SonderAssetRefreshCollector {
    private var results: [SonderAssetRefreshResult] = []

    func append(_ result: SonderAssetRefreshResult) {
        results.append(result)
    }

    func all() -> [SonderAssetRefreshResult] {
        results
    }
}

actor SonderConcurrentCounter {
    private var value = 0
    private var lastPublishAt = Date.distantPast

    func increment() -> Int {
        value += 1
        return value
    }

    func incrementAndShouldPublish(total: Int, minInterval: TimeInterval) -> (value: Int, shouldPublish: Bool) {
        value += 1
        let now = Date()
        let isFinished = value >= total
        let shouldPublish = isFinished || now.timeIntervalSince(lastPublishAt) >= minInterval
        if shouldPublish {
            lastPublishAt = now
        }
        return (value, shouldPublish)
    }
}

actor SonderScanIndexState {
    private var scannedPaths: Set<String>
    private var allDiscoveredIDs = Set<UUID>()
    private var directoryIndexedCount = 0
    private var directorySkippedDuplicateCount = 0

    init(existingPaths: Set<String>) {
        self.scannedPaths = existingPaths
    }

    func beginDirectory() {
        directoryIndexedCount = 0
        directorySkippedDuplicateCount = 0
    }

    func currentDirectoryIndexedCount() -> Int {
        directoryIndexedCount
    }

    func currentDirectorySkippedDuplicateCount() -> Int {
        directorySkippedDuplicateCount
    }

    func discoveredItemIDs() -> [UUID] {
        Array(allDiscoveredIDs)
    }

    func index(_ scannedFiles: [SonderScannedMediaFile], directory: SonderMediaDirectory) -> [SonderMediaItem] {
        var newItems: [SonderMediaItem] = []
        newItems.reserveCapacity(scannedFiles.count)

        for scannedFile in scannedFiles {
            guard scannedPaths.contains(scannedFile.url.path) == false else {
                directorySkippedDuplicateCount += 1
                continue
            }
            scannedPaths.insert(scannedFile.url.path)
            let parsed = SonderMediaParser.parseTitle(url: scannedFile.url, libraryKind: directory.kind)
            let format = SonderMediaFormat(url: scannedFile.url)
            let item = SonderMediaItem(
                title: parsed.title,
                subtitle: parsed.subtitle,
                kind: parsed.kind,
                studio: "Remote Library",
                year: parsed.year ?? Calendar.current.component(.year, from: Date()),
                durationSeconds: parsed.kind == .ebook ? 0 : 5400,
                format: format,
                libraryID: directory.libraryID,
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
            newItems.append(item)
            allDiscoveredIDs.insert(item.id)
            directoryIndexedCount += 1
        }

        return newItems
    }
}
