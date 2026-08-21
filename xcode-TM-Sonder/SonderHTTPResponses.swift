import Foundation
import SonderAPI

// MARK: - Mapping host domain models → shared wire DTOs (SonderAPI)

extension SonderPublicMediaItem {
    nonisolated init(_ item: SonderMediaItem) {
        self.init(
            id: item.id,
            title: item.title,
            subtitle: item.subtitle,
            kind: SonderAPI.SonderMediaKind(rawValue: item.kind.rawValue) ?? .movie,
            studio: item.studio,
            year: item.year,
            durationSeconds: item.durationSeconds,
            format: SonderAPI.SonderMediaFormat(rawValue: item.format.rawValue) ?? .unknown,
            libraryID: item.libraryID,
            tags: item.tags,
            summary: item.summary,
            progressSeconds: item.progressSeconds,
            showTitle: item.showTitle,
            seasonNumber: item.seasonNumber,
            episodeNumber: item.episodeNumber,
            metadataIDSource: item.metadataIDSource,
            metadataID: item.metadataID,
            edition: item.edition,
            splitPart: item.splitPart,
            isPlaceholder: item.isPlaceholder,
            posterURL: item.posterURL,
            backdropURL: item.backdropURL,
            embeddedAudioTracks: item.embeddedAudioTracks.map(SonderAPI.SonderPlaybackTrack.init(host:)),
            embeddedSubtitleTracks: item.embeddedSubtitleTracks.map(SonderAPI.SonderPlaybackTrack.init(host:)),
            trackProbeUpdatedAt: item.trackProbeUpdatedAt,
            probedWidth: item.probedWidth,
            probedHeight: item.probedHeight,
            probedCodec: item.probedCodec,
            probedBitrate: item.probedBitrate,
            bookValidation: item.bookValidation,
            coverSource: item.coverSource
        )
    }
}

extension SonderPublicServerSettings {
    nonisolated init(_ settings: SonderServerSettings) {
        self.init(
            isEnabled: settings.isEnabled,
            allowLAN: settings.allowLAN,
            port: settings.port,
            themePreset: settings.themePreset,
            requiresPairing: settings.requiresPairing
        )
    }
}

extension SonderPublicMediaDirectory {
    nonisolated init(_ directory: SonderMediaDirectory) {
        self.init(
            id: directory.id,
            name: directory.name,
            kind: directory.kind.rawValue,
            libraryID: directory.libraryID,
            lastIndexedCount: directory.lastIndexedCount,
            lastScannedFileCount: directory.lastScannedFileCount,
            lastScannedAt: directory.lastScannedAt
        )
    }
}

extension SonderActivityEventDTO {
    nonisolated init(_ event: SonderActivityEvent) {
        self.init(id: event.id, title: event.title, detail: event.detail, icon: event.icon, date: event.date)
    }
}

extension SonderAPI.SonderProgress {
    nonisolated init(host progress: SonderProgress) {
        self.init(
            id: progress.id,
            itemID: progress.itemID,
            seconds: progress.seconds,
            duration: progress.duration,
            updatedAt: progress.updatedAt,
            audioTrackID: progress.audioTrackID,
            subtitleTrackID: progress.subtitleTrackID,
            subtitlesEnabled: progress.subtitlesEnabled
        )
    }
}

extension SonderAPI.SonderPlaybackTrack {
    nonisolated init(host track: SonderPlaybackTrack) {
        self.init(
            id: track.id,
            label: track.label,
            languageCode: track.languageCode,
            kind: track.kind,
            url: track.url
        )
    }
}

extension SonderAPI.SonderPlaybackSessionResponse {
    nonisolated init(
        itemID: UUID,
        streamURL: String,
        seconds: Double,
        duration: Double,
        percent: Double,
        updatedAt: Date?,
        audioTrackID: String?,
        subtitleTrackID: String?,
        subtitlesEnabled: Bool,
        audioTracks: [SonderPlaybackTrack],
        subtitleTracks: [SonderPlaybackTrack]
    ) {
        self.init(
            itemID: itemID,
            streamURL: streamURL,
            seconds: seconds,
            duration: duration,
            percent: percent,
            updatedAt: updatedAt,
            audioTrackID: audioTrackID,
            subtitleTrackID: subtitleTrackID,
            subtitlesEnabled: subtitlesEnabled,
            audioTracks: audioTracks.map(SonderAPI.SonderPlaybackTrack.init(host:)),
            subtitleTracks: subtitleTracks.map(SonderAPI.SonderPlaybackTrack.init(host:))
        )
    }
}

// Theme snapshot lives in SonderAPI; host code can construct it directly.

// MARK: - Host-only HTTP payloads (not yet shared)

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

// Host domain track / progress update types (library persistence).

nonisolated struct SonderPlaybackTrack: Codable, Sendable, Hashable {
    var id: String
    var label: String
    var languageCode: String?
    var kind: String
    var url: String?
}

nonisolated struct SonderProgressUpdate: Codable, Sendable {
    var seconds: Double
    var duration: Double
    var audioTrackID: String?
    var subtitleTrackID: String?
    var subtitlesEnabled: Bool?
}

extension SonderProgressUpdate {
    init(_ update: SonderAPI.SonderPlaybackStateUpdate) {
        self.init(
            seconds: update.seconds,
            duration: update.duration,
            audioTrackID: update.audioTrackID,
            subtitleTrackID: update.subtitleTrackID,
            subtitlesEnabled: update.subtitlesEnabled
        )
    }
}
