import Foundation

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
            directoriesTotal: directoriesTotal
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
            directoriesTotal: directoriesTotal
        )
    }

    static func discoveredRootTitles(
        count: Int,
        directory: SonderMediaDirectory,
        filesSeen: Int,
        mediaFound: Int,
        indexedCount: Int,
        directoriesDone: Int,
        directoriesTotal: Int
    ) -> SonderScanProgress {
        indexing(
            directory: directory,
            detail: "Discovered \(count) root title(s). Deep scan continues.",
            filesSeen: filesSeen,
            mediaFound: mediaFound,
            indexedCount: indexedCount,
            directoriesDone: directoriesDone,
            directoriesTotal: directoriesTotal
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
            directoriesTotal: max(directoriesTotal, 1)
        )
    }
}

struct SonderServerSettings: Codable, Hashable {
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

    nonisolated var placeholderSubtitle: String {
        switch kind {
        case .movies: "Movie placeholder"
        case .tvShows: "Show placeholder"
        case .audiobooks: "Audiobook placeholder"
        case .ebooks: "Book placeholder"
        }
    }
}

struct SonderLocalAssetUpdate: Sendable {
    var posterPath: String?
    var backdropPath: String?
    var subtitlePaths: [String]
}

nonisolated struct SonderAssetRefreshResult: Sendable {
    var itemID: UUID
    var assetUpdate: SonderLocalAssetUpdate
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

actor SonderScanIndexState {
    private var scannedPaths: Set<String>
    private var materialKeys: Set<String>
    private var allDiscoveredIDs = Set<UUID>()
    private var directoryIndexedCount = 0
    private var directorySkippedDuplicateCount = 0

    init(existingPaths: Set<String>, existingMaterialKeys: Set<String>) {
        self.scannedPaths = existingPaths
        self.materialKeys = existingMaterialKeys
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

    func placeholders(from materials: [SonderRootMaterial], directory: SonderMediaDirectory) -> [SonderMediaItem] {
        var placeholders: [SonderMediaItem] = []
        placeholders.reserveCapacity(materials.count)

        for material in materials {
            let key = Self.materialKey(kind: material.mediaKind, title: material.name)
            guard materialKeys.contains(key) == false else { continue }
            materialKeys.insert(key)

            let localAssets = SonderMediaParser.localAssets(inMaterialFolder: material.url)
            let item = SonderMediaItem(
                title: material.name,
                subtitle: material.placeholderSubtitle,
                kind: material.mediaKind,
                studio: "Remote Library",
                year: Calendar.current.component(.year, from: Date()),
                durationSeconds: 0,
                format: .unknown,
                tags: ["remote", directory.kind.tag, "placeholder"],
                summary: "Indexed from the root folder. Detailed metadata, artwork, and playable files are still scanning.",
                sourcePath: nil,
                showTitle: material.mediaKind == .tvShow ? material.name : nil,
                seasonNumber: nil,
                episodeNumber: nil,
                localPosterPath: localAssets.poster?.path,
                localBackdropPath: localAssets.backdrop?.path,
                isPlaceholder: true
            )
            placeholders.append(item)
            directoryIndexedCount += 1
        }

        return placeholders
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
            materialKeys.insert(Self.materialKey(kind: parsed.kind, title: (parsed.kind == .tvShow ? parsed.showTitle : nil) ?? parsed.title))
            let item = SonderMediaItem(
                title: parsed.title,
                subtitle: parsed.subtitle,
                kind: parsed.kind,
                studio: "Remote Library",
                year: parsed.year ?? Calendar.current.component(.year, from: Date()),
                durationSeconds: parsed.kind == .ebook ? 0 : 5400,
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
            newItems.append(item)
            allDiscoveredIDs.insert(item.id)
            directoryIndexedCount += 1
        }

        return newItems
    }

    private static func materialKey(kind: SonderMediaKind, title: String) -> String {
        "\(kind.rawValue)|\(title.cleanedMediaTitle.lowercased())"
    }
}
