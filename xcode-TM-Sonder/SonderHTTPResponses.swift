import Foundation

nonisolated struct SonderLibraryResponse: Codable, Sendable {
    var items: [SonderMediaItem]
    var progress: [SonderProgress]
    var mediaDirectories: [SonderMediaDirectory]
    var scanProgress: SonderScanProgress?
    var activity: [SonderActivityEvent]
    var serverSettings: SonderServerSettings?
    var theme: SonderThemeSnapshot?
}

struct SonderThemeSnapshot: Codable, Sendable {
    var preset: String
    var background: String
    var sidebar: String
    var surface: String
    var border: String
    var accent: String
    var text: String
}

nonisolated struct SonderStatusResponse: Codable, Sendable {
    var itemCount: Int
    var playableCount: Int
    var mediaDirectoryCount: Int
    var scanProgress: SonderScanProgress?
    var activeScanDirectoryPath: String?
    var activeScanStartedAt: Date?
    var activeScanUpdatedAt: Date?
    var queuedScanDirectoryCount: Int
    var queuedScanDirectorySummaries: [String]
    var isLoadingPersistedLibrary: Bool
    var isBusy: Bool
}

struct SonderDiscoveryResponse: Codable, Sendable {
    var app: String
    var name: String
    var version: String
    var build: String
    var isEnabled: Bool
    var allowLAN: Bool
    var port: UInt16
    var localURL: String
    var lanURL: String?
    var discoveryMethods: [String]
    var tailscaleHint: String
    var capabilities: SonderDiscoveryCapabilities
    var endpoints: SonderDiscoveryEndpoints
    var theme: SonderThemeSnapshot
}

struct SonderDiscoveryCapabilities: Codable, Sendable {
    var books: Bool
    var audiobooks: Bool
    var themes: Bool
    var progressSync: Bool
    var mediaStreaming: Bool
    var remoteCatalog: Bool
}

struct SonderDiscoveryEndpoints: Codable, Sendable {
    var health: String
    var library: String
    var audiobooks: String
    var audiobookBrowser: String
    var discovery: String
    var progress: String
    var playback: String
    var stream: String
}

struct SonderAudiobookResponse: Codable, Sendable {
    var items: [SonderAudiobookItem]
    var count: Int
    var theme: SonderThemeSnapshot
    var generatedAt: Date
}

struct SonderAudiobookDetail: Codable, Sendable {
    var item: SonderAudiobookItem
    var chapters: [SonderAudiobookChapter]
}

struct SonderAudiobookChapter: Codable, Sendable {
    var index: Int
    var title: String
    var startSeconds: Double
    var endSeconds: Double?
}

struct SonderAudiobookItem: Codable, Sendable, Identifiable {
    var id: UUID
    var title: String
    var subtitle: String
    var author: String?
    var series: String?
    var narrator: String?
    var summary: String
    var studio: String
    var year: Int
    var durationSeconds: Double
    var chapterCount: Int
    var posterURL: String?
    var backdropURL: String?
    var sourcePath: String?
    var tags: [String]
    var playback: SonderAudiobookPlaybackSnapshot?

    var searchText: String {
        ([title, subtitle, author ?? "", series ?? "", narrator ?? "", summary, studio] + tags).joined(separator: " ")
    }

    init(_ item: SonderMediaItem) {
        id = item.id
        title = item.title
        subtitle = item.subtitle
        author = nil
        series = item.showTitle
        narrator = nil
        summary = item.summary
        studio = item.studio
        year = item.year
        durationSeconds = item.durationSeconds
        chapterCount = 0
        posterURL = item.posterURL
        backdropURL = item.backdropURL
        sourcePath = item.sourcePath
        tags = item.tags
        playback = nil
    }

    init(_ item: SonderMediaItem, chapterCount: Int, author: String?, series: String?, narrator: String?, playback: SonderAudiobookPlaybackSnapshot?) {
        self.init(item)
        self.chapterCount = chapterCount
        self.author = author
        self.series = series
        self.narrator = narrator
        self.playback = playback
    }
}

nonisolated struct SonderPlaybackTrack: Codable, Sendable, Hashable {
    var id: String
    var label: String
    var languageCode: String?
    var kind: String
    var url: String?
}

nonisolated struct SonderPlaybackSessionResponse: Codable, Sendable {
    var itemID: UUID
    var streamURL: String
    var seconds: Double
    var duration: Double
    var percent: Double
    var updatedAt: Date?
    var audioTrackID: String?
    var subtitleTrackID: String?
    var subtitlesEnabled: Bool
    var audioTracks: [SonderPlaybackTrack]
    var subtitleTracks: [SonderPlaybackTrack]
}

nonisolated struct SonderProgressUpdate: Codable, Sendable {
    var seconds: Double
    var duration: Double
    var audioTrackID: String?
    var subtitleTrackID: String?
    var subtitlesEnabled: Bool?
}
