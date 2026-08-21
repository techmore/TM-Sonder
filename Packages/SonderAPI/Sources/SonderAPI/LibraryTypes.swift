import Foundation

/// Catalog item safe for remote clients. Relative artwork URLs only; no host paths.
/// Clients may `typealias SonderMediaItem = SonderPublicMediaItem`.
public struct SonderPublicMediaItem: Codable, Sendable, Identifiable, Hashable {
    public var id: UUID
    public var title: String
    public var subtitle: String
    public var kind: SonderMediaKind
    public var studio: String
    public var year: Int
    public var durationSeconds: Double
    public var format: SonderMediaFormat
    public var libraryID: UUID?
    public var tags: [String]
    public var summary: String
    public var progressSeconds: Double
    public var showTitle: String?
    public var seasonNumber: Int?
    public var episodeNumber: Int?
    public var metadataIDSource: String?
    public var metadataID: String?
    public var edition: String?
    public var splitPart: String?
    public var isPlaceholder: Bool
    public var posterURL: String?
    public var backdropURL: String?
    public var embeddedAudioTracks: [SonderPlaybackTrack]
    public var embeddedSubtitleTracks: [SonderPlaybackTrack]
    public var trackProbeUpdatedAt: Date?
    public var probedWidth: Int?
    public var probedHeight: Int?
    public var probedCodec: String?
    public var probedBitrate: Int?
    public var bookValidation: String?
    public var coverSource: String?

    public init(
        id: UUID = UUID(),
        title: String,
        subtitle: String,
        kind: SonderMediaKind,
        studio: String,
        year: Int,
        durationSeconds: Double,
        format: SonderMediaFormat,
        libraryID: UUID? = nil,
        tags: [String] = [],
        summary: String,
        progressSeconds: Double = 0,
        showTitle: String? = nil,
        seasonNumber: Int? = nil,
        episodeNumber: Int? = nil,
        metadataIDSource: String? = nil,
        metadataID: String? = nil,
        edition: String? = nil,
        splitPart: String? = nil,
        isPlaceholder: Bool = false,
        posterURL: String? = nil,
        backdropURL: String? = nil,
        embeddedAudioTracks: [SonderPlaybackTrack] = [],
        embeddedSubtitleTracks: [SonderPlaybackTrack] = [],
        trackProbeUpdatedAt: Date? = nil,
        probedWidth: Int? = nil,
        probedHeight: Int? = nil,
        probedCodec: String? = nil,
        probedBitrate: Int? = nil,
        bookValidation: String? = nil,
        coverSource: String? = nil
    ) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.kind = kind
        self.studio = studio
        self.year = year
        self.durationSeconds = durationSeconds
        self.format = format
        self.libraryID = libraryID
        self.tags = tags
        self.summary = summary
        self.progressSeconds = progressSeconds
        self.showTitle = showTitle
        self.seasonNumber = seasonNumber
        self.episodeNumber = episodeNumber
        self.metadataIDSource = metadataIDSource
        self.metadataID = metadataID
        self.edition = edition
        self.splitPart = splitPart
        self.isPlaceholder = isPlaceholder
        self.posterURL = posterURL
        self.backdropURL = backdropURL
        self.embeddedAudioTracks = embeddedAudioTracks
        self.embeddedSubtitleTracks = embeddedSubtitleTracks
        self.trackProbeUpdatedAt = trackProbeUpdatedAt
        self.probedWidth = probedWidth
        self.probedHeight = probedHeight
        self.probedCodec = probedCodec
        self.probedBitrate = probedBitrate
        self.bookValidation = bookValidation
        self.coverSource = coverSource
    }

    public var episodeCode: String {
        guard let seasonNumber, let episodeNumber else { return "Not episodic" }
        return String(format: "S%02dE%02d", seasonNumber, episodeNumber)
    }

    public var progress: Double {
        guard durationSeconds > 0 else { return 0 }
        return min(max(progressSeconds / durationSeconds, 0), 1)
    }

    public var resolutionLabel: String {
        guard let probedHeight else { return "Unknown" }
        switch probedHeight {
        case 2000...: return "\(probedWidth ?? 3840)x\(probedHeight)"
        case 1000..<2000: return "1080p"
        case 700..<1000: return "720p"
        case 400..<700: return "480p"
        default: return "\(probedHeight)p"
        }
    }

    public var searchableText: String {
        ([title, subtitle, studio, kind.rawValue, kind.label, summary, showTitle ?? "", episodeCode] + tags)
            .joined(separator: " ")
    }

    public var isPlayableInBrowser: Bool {
        format.isBrowserPlayable
    }
}

/// Server settings exposed to clients. Never includes the pairing token.
public struct SonderPublicServerSettings: Codable, Sendable, Hashable {
    public var isEnabled: Bool
    public var allowLAN: Bool
    public var port: Int
    public var themePreset: String
    public var requiresPairing: Bool
    /// Legacy field some older servers leaked. Clients should prefer `requiresPairing`.
    public var pairingToken: String?

    public init(
        isEnabled: Bool,
        allowLAN: Bool,
        port: Int,
        themePreset: String,
        requiresPairing: Bool,
        pairingToken: String? = nil
    ) {
        self.isEnabled = isEnabled
        self.allowLAN = allowLAN
        self.port = port
        self.themePreset = themePreset
        self.requiresPairing = requiresPairing
        self.pairingToken = pairingToken
    }

    public var isPairingRequired: Bool {
        if requiresPairing { return true }
        return pairingToken?.isEmpty == false
    }

    enum CodingKeys: String, CodingKey {
        case isEnabled, allowLAN, port, themePreset, requiresPairing, pairingToken
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        isEnabled = try container.decodeIfPresent(Bool.self, forKey: .isEnabled) ?? true
        allowLAN = try container.decodeIfPresent(Bool.self, forKey: .allowLAN) ?? false
        port = try container.decodeIfPresent(Int.self, forKey: .port) ?? 8797
        themePreset = try container.decodeIfPresent(String.self, forKey: .themePreset) ?? "earthy"
        pairingToken = try container.decodeIfPresent(String.self, forKey: .pairingToken)
        if let requires = try container.decodeIfPresent(Bool.self, forKey: .requiresPairing) {
            requiresPairing = requires
        } else {
            requiresPairing = pairingToken?.isEmpty == false
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(isEnabled, forKey: .isEnabled)
        try container.encode(allowLAN, forKey: .allowLAN)
        try container.encode(port, forKey: .port)
        try container.encode(themePreset, forKey: .themePreset)
        try container.encode(requiresPairing, forKey: .requiresPairing)
        // Never encode pairingToken on the wire from well-behaved servers.
    }
}

public typealias SonderServerSettingsDTO = SonderPublicServerSettings

public struct SonderPublicMediaDirectory: Codable, Sendable, Identifiable, Hashable {
    public var id: UUID
    public var name: String
    public var kind: String
    public var libraryID: UUID
    public var lastIndexedCount: Int
    public var lastScannedFileCount: Int
    public var lastScannedAt: Date?

    public init(
        id: UUID,
        name: String,
        kind: String,
        libraryID: UUID,
        lastIndexedCount: Int = 0,
        lastScannedFileCount: Int = 0,
        lastScannedAt: Date? = nil
    ) {
        self.id = id
        self.name = name
        self.kind = kind
        self.libraryID = libraryID
        self.lastIndexedCount = lastIndexedCount
        self.lastScannedFileCount = lastScannedFileCount
        self.lastScannedAt = lastScannedAt
    }
}

public struct SonderActivityEventDTO: Codable, Sendable, Identifiable, Hashable {
    public var id: UUID
    public var title: String
    public var detail: String
    public var icon: String
    public var date: Date

    public init(id: UUID = UUID(), title: String, detail: String, icon: String, date: Date = Date()) {
        self.id = id
        self.title = title
        self.detail = detail
        self.icon = icon
        self.date = date
    }
}

public struct SonderThemeSnapshot: Codable, Sendable, Hashable {
    public var preset: String
    public var background: String
    public var sidebar: String
    public var surface: String
    public var border: String
    public var accent: String
    public var text: String

    public init(
        preset: String,
        background: String,
        sidebar: String,
        surface: String,
        border: String,
        accent: String,
        text: String
    ) {
        self.preset = preset
        self.background = background
        self.sidebar = sidebar
        self.surface = surface
        self.border = border
        self.accent = accent
        self.text = text
    }
}

public struct SonderLibraryResponse: Codable, Sendable, Hashable {
    public var items: [SonderPublicMediaItem]
    public var progress: [SonderProgress]
    public var mediaDirectories: [SonderPublicMediaDirectory]
    public var activity: [SonderActivityEventDTO]
    public var serverSettings: SonderPublicServerSettings?
    public var theme: SonderThemeSnapshot?

    public init(
        items: [SonderPublicMediaItem],
        progress: [SonderProgress],
        mediaDirectories: [SonderPublicMediaDirectory] = [],
        activity: [SonderActivityEventDTO] = [],
        serverSettings: SonderPublicServerSettings? = nil,
        theme: SonderThemeSnapshot? = nil
    ) {
        self.items = items
        self.progress = progress
        self.mediaDirectories = mediaDirectories
        self.activity = activity
        self.serverSettings = serverSettings
        self.theme = theme
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        items = try container.decodeIfPresent([SonderPublicMediaItem].self, forKey: .items) ?? []
        progress = try container.decodeIfPresent([SonderProgress].self, forKey: .progress) ?? []
        mediaDirectories = try container.decodeIfPresent([SonderPublicMediaDirectory].self, forKey: .mediaDirectories) ?? []
        activity = try container.decodeIfPresent([SonderActivityEventDTO].self, forKey: .activity) ?? []
        serverSettings = try container.decodeIfPresent(SonderPublicServerSettings.self, forKey: .serverSettings)
        theme = try container.decodeIfPresent(SonderThemeSnapshot.self, forKey: .theme)
    }
}
