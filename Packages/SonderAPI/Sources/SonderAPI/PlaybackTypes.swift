import Foundation

public enum SonderPlaybackTrackKind: String, Codable, Sendable, Hashable {
    case embedded
    case sidecar
}

public struct SonderPlaybackTrack: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var label: String
    public var languageCode: String?
    public var kind: SonderPlaybackTrackKind
    public var url: String?

    public init(
        id: String,
        label: String,
        languageCode: String? = nil,
        kind: SonderPlaybackTrackKind = .embedded,
        url: String? = nil
    ) {
        self.id = id
        self.label = label
        self.languageCode = languageCode
        self.kind = kind
        self.url = url
    }

    public init(
        id: String,
        label: String,
        languageCode: String? = nil,
        kind: String,
        url: String? = nil
    ) {
        self.id = id
        self.label = label
        self.languageCode = languageCode
        self.kind = SonderPlaybackTrackKind(rawValue: kind) ?? .embedded
        self.url = url
    }

    enum CodingKeys: String, CodingKey {
        case id
        case label
        case title
        case name
        case languageCode
        case language
        case kind
        case url
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? ""
        label = try container.decodeIfPresent(String.self, forKey: .label)
            ?? container.decodeIfPresent(String.self, forKey: .title)
            ?? container.decodeIfPresent(String.self, forKey: .name)
            ?? id
        languageCode = try container.decodeIfPresent(String.self, forKey: .languageCode)
            ?? container.decodeIfPresent(String.self, forKey: .language)
        if let kindValue = try container.decodeIfPresent(SonderPlaybackTrackKind.self, forKey: .kind) {
            kind = kindValue
        } else if let kindString = try container.decodeIfPresent(String.self, forKey: .kind) {
            kind = SonderPlaybackTrackKind(rawValue: kindString) ?? .embedded
        } else {
            kind = .embedded
        }
        url = try container.decodeIfPresent(String.self, forKey: .url)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(label, forKey: .label)
        try container.encodeIfPresent(languageCode, forKey: .languageCode)
        try container.encode(kind.rawValue, forKey: .kind)
        try container.encodeIfPresent(url, forKey: .url)
    }
}

public struct SonderProgress: Codable, Sendable, Identifiable, Hashable {
    public var id: UUID
    public var itemID: UUID
    public var seconds: Double
    public var duration: Double
    public var updatedAt: Date
    public var audioTrackID: String?
    public var subtitleTrackID: String?
    public var subtitlesEnabled: Bool?

    public init(
        id: UUID = UUID(),
        itemID: UUID,
        seconds: Double,
        duration: Double,
        updatedAt: Date = Date(),
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil
    ) {
        self.id = id
        self.itemID = itemID
        self.seconds = seconds
        self.duration = duration
        self.updatedAt = updatedAt
        self.audioTrackID = audioTrackID
        self.subtitleTrackID = subtitleTrackID
        self.subtitlesEnabled = subtitlesEnabled
    }

    public var percent: Double {
        guard duration > 0 else { return 0 }
        return min(max(seconds / duration, 0), 1)
    }
}

public struct SonderPlaybackStateUpdate: Codable, Sendable, Hashable {
    public var seconds: Double
    public var duration: Double
    public var audioTrackID: String?
    public var subtitleTrackID: String?
    public var subtitlesEnabled: Bool?

    public init(
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil
    ) {
        self.seconds = seconds
        self.duration = duration
        self.audioTrackID = audioTrackID
        self.subtitleTrackID = subtitleTrackID
        self.subtitlesEnabled = subtitlesEnabled
    }
}

public typealias SonderProgressUpdate = SonderPlaybackStateUpdate

public struct SonderPlaybackSessionResponse: Codable, Sendable, Hashable {
    public var itemID: UUID?
    public var streamURL: String
    public var seconds: Double
    public var duration: Double
    public var percent: Double
    public var updatedAt: Date?
    public var audioTrackID: String?
    public var subtitleTrackID: String?
    public var subtitlesEnabled: Bool?
    public var audioTracks: [SonderPlaybackTrack]
    public var subtitleTracks: [SonderPlaybackTrack]

    public init(
        itemID: UUID? = nil,
        streamURL: String,
        seconds: Double,
        duration: Double,
        percent: Double,
        updatedAt: Date? = nil,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil,
        audioTracks: [SonderPlaybackTrack] = [],
        subtitleTracks: [SonderPlaybackTrack] = []
    ) {
        self.itemID = itemID
        self.streamURL = streamURL
        self.seconds = seconds
        self.duration = duration
        self.percent = percent
        self.updatedAt = updatedAt
        self.audioTrackID = audioTrackID
        self.subtitleTrackID = subtitleTrackID
        self.subtitlesEnabled = subtitlesEnabled
        self.audioTracks = audioTracks
        self.subtitleTracks = subtitleTracks
    }

    public var sidecarSubtitleTracks: [SonderPlaybackTrack] {
        subtitleTracks.filter { $0.kind == .sidecar }
    }

    enum CodingKeys: String, CodingKey {
        case itemID
        case streamURL
        case streamUrl
        case url
        case seconds
        case duration
        case percent
        case updatedAt
        case audioTrackID
        case subtitleTrackID
        case subtitlesEnabled
        case audioTracks
        case subtitleTracks
        case sidecarSubtitleTracks
        case sidecarSubtitles
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        itemID = try container.decodeIfPresent(UUID.self, forKey: .itemID)
        streamURL = try container.decodeIfPresent(String.self, forKey: .streamURL)
            ?? container.decodeIfPresent(String.self, forKey: .streamUrl)
            ?? container.decodeIfPresent(String.self, forKey: .url)
            ?? ""
        seconds = try container.decodeIfPresent(Double.self, forKey: .seconds) ?? 0
        duration = try container.decodeIfPresent(Double.self, forKey: .duration) ?? 0
        percent = try container.decodeIfPresent(Double.self, forKey: .percent) ?? 0
        updatedAt = try container.decodeIfPresent(Date.self, forKey: .updatedAt)
        audioTrackID = try container.decodeIfPresent(String.self, forKey: .audioTrackID)
        subtitleTrackID = try container.decodeIfPresent(String.self, forKey: .subtitleTrackID)
        subtitlesEnabled = try container.decodeIfPresent(Bool.self, forKey: .subtitlesEnabled)
        audioTracks = try container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .audioTracks) ?? []
        subtitleTracks = try container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .subtitleTracks)
            ?? container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .sidecarSubtitles)
            ?? container.decodeIfPresent([SonderPlaybackTrack].self, forKey: .sidecarSubtitleTracks)
            ?? []
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encodeIfPresent(itemID, forKey: .itemID)
        try container.encode(streamURL, forKey: .streamURL)
        try container.encode(seconds, forKey: .seconds)
        try container.encode(duration, forKey: .duration)
        try container.encode(percent, forKey: .percent)
        try container.encodeIfPresent(updatedAt, forKey: .updatedAt)
        try container.encodeIfPresent(audioTrackID, forKey: .audioTrackID)
        try container.encodeIfPresent(subtitleTrackID, forKey: .subtitleTrackID)
        try container.encodeIfPresent(subtitlesEnabled, forKey: .subtitlesEnabled)
        try container.encode(audioTracks, forKey: .audioTracks)
        try container.encode(subtitleTracks, forKey: .subtitleTracks)
    }
}

/// Client-friendly alias matching prior iOS naming.
public typealias SonderPlaybackResponse = SonderPlaybackSessionResponse
