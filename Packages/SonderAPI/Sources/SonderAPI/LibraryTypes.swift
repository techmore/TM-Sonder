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
    public var libraryID: String?
    @DefaultEmptyArray public var tags: [String]
    /// Curated genres, distinct from free-form `tags` (provider keywords and
    /// people names). Older servers may omit it; decodes to an empty list.
    @DefaultEmptyArray public var genres: [String]
    public var summary: String
    public var progressSeconds: Double
    public var showTitle: String?
    public var showGroupID: String? = nil
    public var showGroupTitle: String? = nil
    public var seasonNumber: Int?
    public var episodeNumber: Int?
    public var metadataIDSource: String?
    public var metadataID: String?
    public var edition: String?
    public var splitPart: String?
    public var isPlaceholder: Bool
    public var posterURL: String?
    public var backdropURL: String?
    @DefaultEmptyArray public var embeddedAudioTracks: [SonderPlaybackTrack]
    @DefaultEmptyArray public var embeddedSubtitleTracks: [SonderPlaybackTrack]
    public var trackProbeUpdatedAt: Date?
    public var probedWidth: Int?
    public var probedHeight: Int?
    public var probedCodec: String?
    /// Audio stream codecs reported by the server probe (for example, "aac" or "opus").
    @DefaultEmptyArray public var probedAudioCodecs: [String]
    public var probedBitrate: Int?
    public var bookValidation: String?
    public var coverSource: String?
    /// Populated for audiobooks and ebooks.
    public var author: String?
    public var narrator: String?

    // Book structure. The server has always sent these; until now no Swift
    // client declared them, so every native player decoded the item and threw
    // the book's shape away. A book delivered as many files then looked like N
    // unrelated titles: each file's own runtime on its own card, and playback
    // that stopped at the end of the first one.
    //
    // All optional, deliberately. The wire format omits zero and empty values
    // (`omitempty`), and a non-optional property would make decoding fail
    // outright on a perfectly ordinary single-file book.
    public var bookGroupID: String?
    public var bookGroupTitle: String?
    public var bookPartIndex: Int?
    public var bookPartCount: Int?

    // Ordering keys. SortTitle is the A-Z key (the raw title may start with a
    // year or embed a narrator credit), and the series fields carry what the
    // sort key removed, so ordering on the cleaner key loses nothing.
    public var sortTitle: String?
    public var seriesName: String?
    public var seriesPosition: String?
    public var seriesNumber: Double?
    public var publicationYear: Int?

    /// True when this file is one part of a multi-file book.
    ///
    /// A client must key its "one card per book" grouping on ``bookGroupID`` and
    /// must represent the book with the *head* part -- bookPartIndex 1 -- because
    /// a trailing part is only reachable through the head. Promoting a trailing
    /// part to be the card would orphan the parts before it.
    public var isPartOfMultiFileBook: Bool {
        guard kind == .audiobook, let count = bookPartCount else { return false }
        return count > 1 && bookGroupID != nil
    }

    /// The file that represents its book, given every file of that book.
    ///
    /// This is the one place the head-part rule should live, so four players
    /// cannot each invent a different answer.
    public func bookRepresentative(in siblings: [SonderPublicMediaItem]) -> SonderPublicMediaItem? {
        guard let groupID = bookGroupID else { return nil }
        let parts = siblings.filter { $0.bookGroupID == groupID }
        guard !parts.isEmpty else { return nil }
        return parts.min { lhs, rhs in
            let li = lhs.bookPartIndex ?? Int.max
            let ri = rhs.bookPartIndex ?? Int.max
            if li != ri { return li < ri }
            return lhs.id.uuidString < rhs.id.uuidString
        }
    }

    /// Total runtime of the book this file belongs to, across every file.
    ///
    /// A single file's own duration is the whole book's length only when the
    /// book *is* one file; otherwise the card reports one fragment of a long
    /// recording as if it were the entire work.
    public func bookDuration(in siblings: [SonderPublicMediaItem]) -> Double {
        guard let groupID = bookGroupID else { return durationSeconds }
        let parts = siblings.filter { $0.bookGroupID == groupID }
        guard parts.count > 1 else { return durationSeconds }
        return parts.reduce(0) { $0 + $1.durationSeconds }
    }

    /// Listening position as a fraction of the whole book, 0...1.
    ///
    /// One part's seconds divided by the book's full runtime reports a finished
    /// recording as barely started, so the two must be measured over the same
    /// span: this part's seconds over this part's duration.
    public func bookProgressFraction(in siblings: [SonderPublicMediaItem]) -> Double {
        guard let groupID = bookGroupID else {
            guard durationSeconds > 0 else { return 0 }
            return min(max(progressSeconds / durationSeconds, 0), 1)
        }
        let parts = siblings.filter { $0.bookGroupID == groupID }
        guard parts.count > 1 else { return 0 }
        let total = parts.reduce(0) { $0 + $1.durationSeconds }
        guard total > 0 else { return 0 }
        let done = parts.reduce(0) { $0 + $1.progressSeconds }
        return min(max(done / total, 0), 1)
    }

    /// True only when every file of the book has been listened to.
    public func isBookFinished(in siblings: [SonderPublicMediaItem]) -> Bool {
        guard let groupID = bookGroupID else {
            guard durationSeconds > 0 else { return false }
            return progressSeconds / durationSeconds >= 0.96
        }
        let parts = siblings.filter { $0.bookGroupID == groupID }
        guard !parts.isEmpty else { return false }
        return parts.allSatisfy { part in
            let dur = part.durationSeconds
            guard dur > 0 else { return false }
            return part.progressSeconds / dur >= 0.96
        }
    }

    public init(
        id: UUID = UUID(),
        title: String,
        subtitle: String,
        kind: SonderMediaKind,
        studio: String,
        year: Int,
        durationSeconds: Double,
        format: SonderMediaFormat,
        libraryID: String? = nil,
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
        probedAudioCodecs: [String] = [],
        probedBitrate: Int? = nil,
        bookValidation: String? = nil,
        coverSource: String? = nil,
        genres: [String] = [],
        author: String? = nil,
        narrator: String? = nil,
        bookGroupID: String? = nil,
        bookGroupTitle: String? = nil,
        bookPartIndex: Int? = nil,
        bookPartCount: Int? = nil,
        sortTitle: String? = nil,
        seriesName: String? = nil,
        seriesPosition: String? = nil,
        seriesNumber: Double? = nil,
        publicationYear: Int? = nil
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
        self.probedAudioCodecs = probedAudioCodecs
        self.probedBitrate = probedBitrate
        self.bookValidation = bookValidation
        self.coverSource = coverSource
        self.genres = genres
        self.author = author
        self.narrator = narrator
        self.bookGroupID = bookGroupID
        self.bookGroupTitle = bookGroupTitle
        self.bookPartIndex = bookPartIndex
        self.bookPartCount = bookPartCount
        self.sortTitle = sortTitle
        self.seriesName = seriesName
        self.seriesPosition = seriesPosition
        self.seriesNumber = seriesNumber
        self.publicationYear = publicationYear
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
        port = try container.decodeIfPresent(Int.self, forKey: .port) ?? 8096
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
    public var id: String
    public var name: String
    public var kind: String
    public var libraryID: String
    public var lastIndexedCount: Int
    public var lastScannedFileCount: Int
    public var lastScannedAt: Date?

    public init(
        id: String,
        name: String,
        kind: String,
        libraryID: String,
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
