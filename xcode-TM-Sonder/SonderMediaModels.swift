import Foundation
import SwiftUI
import UniformTypeIdentifiers

nonisolated struct SonderMediaItem: Codable, Identifiable, Hashable {
    var id = UUID()
    var title: String
    var subtitle: String
    var kind: SonderMediaKind
    var studio: String
    var year: Int
    var durationSeconds: Double
    var format: SonderMediaFormat
    var libraryID: UUID? = nil
    var tags: [String]
    var summary: String
    var sourcePath: String?
    var sourceBookmark: Data?
    var progressSeconds: Double = 0
    var showTitle: String?
    var seasonNumber: Int?
    var episodeNumber: Int?
    var metadataIDSource: String?
    var metadataID: String?
    var edition: String?
    var splitPart: String?
    var localPosterPath: String?
    var localBackdropPath: String?
    var isPlaceholder: Bool = false
    var posterURL: String? {
        localPosterPath == nil ? nil : "/artwork/poster/\(id.uuidString)"
    }
    var backdropURL: String? {
        localBackdropPath == nil ? nil : "/artwork/backdrop/\(id.uuidString)"
    }
    var subtitlePaths: [String] = []
    var embeddedAudioTracks: [SonderPlaybackTrack] = []
    var embeddedSubtitleTracks: [SonderPlaybackTrack] = []
    var trackProbeUpdatedAt: Date?
    /// Probed from the actual media file at scan/import time (zero until probed).
    var probedWidth: Int?
    var probedHeight: Int?
    var probedCodec: String?
    var probedBitrate: Int?
    var bookValidation: String?
    var coverSource: String?

    var hasFile: Bool {
        playableURL != nil
    }

    /// Resolution label derived from probed dimensions, e.g. "4K", "1080p", "720p".
    var resolutionLabel: String {
        guard let height = probedHeight else { return "Unknown" }
        switch height {
        case 2000...: return "\(probedWidth ?? 3840)×\(height)"
        case 1000..<2000: return "1080p"
        case 700..<1000: return "720p"
        case 400..<700: return "480p"
        default: return "\(height)p"
        }
    }

    /// True for formats the major browsers can render directly in an HTML5 `<video>`
    /// element without server-side transcoding. MKV/AVI/MPG/TS are catalog-only over
    /// HTTP even though they may play fine in a native desktop player.
    var isBrowserPlayable: Bool {
        switch format {
        case .mp4, .m4v, .mov, .webm: return true
        case .mkv, .avi, .mpg, .mpeg, .ts, .m2ts, .epub, .pdf, .m4b, .mp3, .m4a, .unknown: return false
        }
    }

    var playableURL: URL? {
        if let sourceBookmark {
            var isStale = false
            if let url = try? URL(resolvingBookmarkData: sourceBookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale),
               FileManager.default.fileExists(atPath: url.path) {
                return url
            }
        }
        guard let sourcePath, FileManager.default.fileExists(atPath: sourcePath) else { return nil }
        return URL(fileURLWithPath: sourcePath)
    }

    var contentType: String {
        format.contentType
    }

    var progress: Double {
        guard durationSeconds > 0 else { return 0 }
        return min(max(progressSeconds / durationSeconds, 0), 1)
    }

    var searchText: String {
        ([title, subtitle, studio, kind.label, summary, showTitle ?? "", episodeCode] + tags).joined(separator: " ")
    }

    var needsMetadataRefresh: Bool {
        localPosterPath == nil || localBackdropPath == nil || summary.isSonderPlaceholderSummary || studio == "Local" || studio == "Remote Library"
    }

    var episodeCode: String {
        guard let seasonNumber, let episodeNumber else { return "Not episodic" }
        return String(format: "S%02dE%02d", seasonNumber, episodeNumber)
    }

    var plexFileName: String {
        let ext = format == .unknown ? "mp4" : format.rawValue
        switch kind {
        case .tvShow:
            let show = (showTitle ?? title).plexSafeName
            let episodeTitle = title.plexSafeName
            let code = episodeCode == "Not episodic" ? "S01E01" : episodeCode
            return "\(show) - \(code) - \(episodeTitle).\(ext)"
        case .movie, .documentary:
            let editionTag = edition.map { " {edition-\($0.plexSafeName)}" } ?? ""
            return "\(title.plexSafeName) (\(year))\(editionTag).\(ext)"
        case .audiobook, .ebook:
            let editionTag = edition.map { " {edition-\($0.plexSafeName)}" } ?? ""
            return "\(title.plexSafeName) (\(year))\(editionTag).\(ext)"
        case .all:
            return "\(title.plexSafeName).\(ext)"
        }
    }
}

nonisolated extension SonderMediaItem {
    enum CodingKeys: String, CodingKey {
        case id
        case title
        case subtitle
        case kind
        case studio
        case year
        case durationSeconds
        case format
        case libraryID
        case tags
        case summary
        case sourcePath
        case sourceBookmark
        case progressSeconds
        case showTitle
        case seasonNumber
        case episodeNumber
        case metadataIDSource
        case metadataID
        case edition
        case splitPart
        case localPosterPath
        case localBackdropPath
        case isPlaceholder
        case subtitlePaths
        case embeddedAudioTracks
        case embeddedSubtitleTracks
        case trackProbeUpdatedAt
        case probedWidth
        case probedHeight
        case probedCodec
        case probedBitrate
        case bookValidation
        case coverSource
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(UUID.self, forKey: .id) ?? UUID()
        title = try container.decode(String.self, forKey: .title)
        subtitle = try container.decode(String.self, forKey: .subtitle)
        kind = try container.decode(SonderMediaKind.self, forKey: .kind)
        studio = try container.decode(String.self, forKey: .studio)
        year = try container.decode(Int.self, forKey: .year)
        durationSeconds = try container.decode(Double.self, forKey: .durationSeconds)
        format = try container.decode(SonderMediaFormat.self, forKey: .format)
        libraryID = try container.decodeIfPresent(UUID.self, forKey: .libraryID)
        tags = try container.decode([String].self, forKey: .tags)
        summary = try container.decode(String.self, forKey: .summary)
        sourcePath = try container.decodeIfPresent(String.self, forKey: .sourcePath)
        sourceBookmark = try container.decodeIfPresent(Data.self, forKey: .sourceBookmark)
        progressSeconds = try container.decodeIfPresent(Double.self, forKey: .progressSeconds) ?? 0
        showTitle = try container.decodeIfPresent(String.self, forKey: .showTitle)
        seasonNumber = try container.decodeIfPresent(Int.self, forKey: .seasonNumber)
        episodeNumber = try container.decodeIfPresent(Int.self, forKey: .episodeNumber)
        metadataIDSource = try container.decodeIfPresent(String.self, forKey: .metadataIDSource)
        metadataID = try container.decodeIfPresent(String.self, forKey: .metadataID)
        edition = try container.decodeIfPresent(String.self, forKey: .edition)
        splitPart = try container.decodeIfPresent(String.self, forKey: .splitPart)
        localPosterPath = try container.decodeIfPresent(String.self, forKey: .localPosterPath)
        localBackdropPath = try container.decodeIfPresent(String.self, forKey: .localBackdropPath)
        isPlaceholder = try container.decodeIfPresent(Bool.self, forKey: .isPlaceholder) ?? false
        subtitlePaths = try container.decodeIfPresent([String].self, forKey: .subtitlePaths) ?? []
        embeddedAudioTracks = try container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .embeddedAudioTracks) ?? []
        embeddedSubtitleTracks = try container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .embeddedSubtitleTracks) ?? []
        trackProbeUpdatedAt = try container.decodeIfPresent(Date.self, forKey: .trackProbeUpdatedAt)
        probedWidth = try container.decodeIfPresent(Int.self, forKey: .probedWidth)
        probedHeight = try container.decodeIfPresent(Int.self, forKey: .probedHeight)
        probedCodec = try container.decodeIfPresent(String.self, forKey: .probedCodec)
        probedBitrate = try container.decodeIfPresent(Int.self, forKey: .probedBitrate)
        bookValidation = try container.decodeIfPresent(String.self, forKey: .bookValidation)
        coverSource = try container.decodeIfPresent(String.self, forKey: .coverSource)
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(title, forKey: .title)
        try container.encode(subtitle, forKey: .subtitle)
        try container.encode(kind, forKey: .kind)
        try container.encode(studio, forKey: .studio)
        try container.encode(year, forKey: .year)
        try container.encode(durationSeconds, forKey: .durationSeconds)
        try container.encode(format, forKey: .format)
        try container.encodeIfPresent(libraryID, forKey: .libraryID)
        try container.encode(tags, forKey: .tags)
        try container.encode(summary, forKey: .summary)
        try container.encodeIfPresent(sourcePath, forKey: .sourcePath)
        try container.encodeIfPresent(sourceBookmark, forKey: .sourceBookmark)
        try container.encode(progressSeconds, forKey: .progressSeconds)
        try container.encodeIfPresent(showTitle, forKey: .showTitle)
        try container.encodeIfPresent(seasonNumber, forKey: .seasonNumber)
        try container.encodeIfPresent(episodeNumber, forKey: .episodeNumber)
        try container.encodeIfPresent(metadataIDSource, forKey: .metadataIDSource)
        try container.encodeIfPresent(metadataID, forKey: .metadataID)
        try container.encodeIfPresent(edition, forKey: .edition)
        try container.encodeIfPresent(splitPart, forKey: .splitPart)
        try container.encodeIfPresent(localPosterPath, forKey: .localPosterPath)
        try container.encodeIfPresent(localBackdropPath, forKey: .localBackdropPath)
        try container.encode(isPlaceholder, forKey: .isPlaceholder)
        try container.encode(subtitlePaths, forKey: .subtitlePaths)
        try container.encode(embeddedAudioTracks, forKey: .embeddedAudioTracks)
        try container.encode(embeddedSubtitleTracks, forKey: .embeddedSubtitleTracks)
        try container.encodeIfPresent(trackProbeUpdatedAt, forKey: .trackProbeUpdatedAt)
        try container.encodeIfPresent(probedWidth, forKey: .probedWidth)
        try container.encodeIfPresent(probedHeight, forKey: .probedHeight)
        try container.encodeIfPresent(probedCodec, forKey: .probedCodec)
        try container.encodeIfPresent(probedBitrate, forKey: .probedBitrate)
        try container.encodeIfPresent(bookValidation, forKey: .bookValidation)
        try container.encodeIfPresent(coverSource, forKey: .coverSource)
    }
}

nonisolated enum SonderMediaKind: String, Codable, CaseIterable, Identifiable {
    case all
    case movie
    case tvShow
    case documentary
    case audiobook
    case ebook

    var id: String { rawValue }

    static var mediaCases: [SonderMediaKind] {
        [.tvShow, .movie, .documentary, .audiobook, .ebook]
    }

    var label: String {
        switch self {
        case .all: "All"
        case .movie: "Movie"
        case .tvShow: "TV Show"
        case .documentary: "Documentary"
        case .audiobook: "Audiobook"
        case .ebook: "Book"
        }
    }

    var shortLabel: String {
        switch self {
        case .all: "ALL"
        case .movie: "FILM"
        case .tvShow: "TV"
        case .documentary: "DOC"
        case .audiobook: "AUDIO"
        case .ebook: "BOOK"
        }
    }

    var icon: String {
        switch self {
        case .all: "rectangle.stack"
        case .movie: "film"
        case .tvShow: "tv"
        case .documentary: "camera.metering.matrix"
        case .audiobook: "headphones"
        case .ebook: "book"
        }
    }

    var importKind: SonderLibraryImportKind? {
        switch self {
        case .movie, .documentary:
            return .movies
        case .tvShow:
            return .tvShows
        case .audiobook:
            return .audiobooks
        case .ebook:
            return .ebooks
        case .all:
            return nil
        }
    }

    @MainActor var gradient: [Color] {
        switch self {
        case .all: [SonderTheme.accentMuted, SonderTheme.surfaceDeep]
        case .movie: [Color(red: 0.27, green: 0.32, blue: 0.18), SonderTheme.surfaceDeep]
        case .tvShow: [Color(red: 0.19, green: 0.29, blue: 0.31), SonderTheme.surfaceDeep]
        case .documentary: [Color(red: 0.31, green: 0.24, blue: 0.18), SonderTheme.surfaceDeep]
        case .audiobook: [Color(red: 0.23, green: 0.22, blue: 0.33), SonderTheme.surfaceDeep]
        case .ebook: [Color(red: 0.22, green: 0.30, blue: 0.20), SonderTheme.surfaceDeep]
        }
    }
}

nonisolated enum SonderMediaFormat: String, Codable {
    case mp4
    case mov
    case m4v
    case mkv
    case webm
    case avi
    case mpg
    case mpeg
    case ts
    case m2ts
    case epub
    case pdf
    case m4b
    case mp3
    case m4a
    case unknown

    static var importTypes: [UTType] {
        [
            .movie,
            .mpeg4Movie,
            .quickTimeMovie,
            .audio,
            .mp3,
            .pdf,
            .epub,
            UTType(filenameExtension: "mkv") ?? .data,
            UTType(filenameExtension: "webm") ?? .data,
            UTType(filenameExtension: "avi") ?? .data,
            UTType(filenameExtension: "mpg") ?? .data,
            UTType(filenameExtension: "mpeg") ?? .data,
            UTType(filenameExtension: "ts") ?? .data,
            UTType(filenameExtension: "m2ts") ?? .data,
            UTType(filenameExtension: "m4b") ?? .data,
            UTType(filenameExtension: "m4a") ?? .data
        ]
    }

    nonisolated init(url: URL) {
        switch url.pathExtension.lowercased() {
        case "mp4": self = .mp4
        case "mov": self = .mov
        case "m4v": self = .m4v
        case "mkv": self = .mkv
        case "webm": self = .webm
        case "avi": self = .avi
        case "mpg": self = .mpg
        case "mpeg": self = .mpeg
        case "ts": self = .ts
        case "m2ts": self = .m2ts
        case "epub": self = .epub
        case "pdf": self = .pdf
        case "m4b": self = .m4b
        case "mp3": self = .mp3
        case "m4a": self = .m4a
        default: self = .unknown
        }
    }

    nonisolated static func isSupported(url: URL) -> Bool {
        switch url.pathExtension.lowercased() {
        case "mp4", "mov", "m4v", "mkv", "webm", "avi", "mpg", "mpeg", "ts", "m2ts", "epub", "pdf", "m4b", "mp3", "m4a":
            return true
        default:
            return false
        }
    }

    nonisolated var contentType: String {
        switch self {
        case .mp4, .m4v: "video/mp4"
        case .mov: "video/quicktime"
        case .mkv: "video/x-matroska"
        case .webm: "video/webm"
        case .avi: "video/x-msvideo"
        case .mpg, .mpeg: "video/mpeg"
        case .ts, .m2ts: "video/mp2t"
        case .epub: "application/epub+zip"
        case .pdf: "application/pdf"
        case .m4b: "audio/mp4"
        case .mp3: "audio/mpeg"
        case .m4a: "audio/mp4"
        case .unknown: "application/octet-stream"
        }
    }
}

nonisolated struct SonderTVShowGroup: Identifiable, Hashable {
    var id: String { name }
    var name: String
    var seasons: [SonderTVSeasonGroup]
    var episodeCount: Int {
        seasons.map { $0.episodes.filter { $0.isPlaceholder == false }.count }.reduce(0, +)
    }

    var displayCount: Int {
        episodeCount
    }
}

nonisolated struct SonderTVSeasonGroup: Identifiable, Hashable {
    static let unknownSeasonNumber = -1

    var id: Int { seasonNumber }
    var seasonNumber: Int
    var episodes: [SonderMediaItem]

    var label: String {
        if seasonNumber == Self.unknownSeasonNumber { return "Unsorted Season" }
        return seasonNumber == 0 ? "Specials" : "Season \(seasonNumber)"
    }

    var sortOrder: Int {
        seasonNumber == Self.unknownSeasonNumber ? Int.max : seasonNumber
    }
}

nonisolated struct SonderScannedMediaFile: Sendable {
    var url: URL
    var bookmark: Data?
}
