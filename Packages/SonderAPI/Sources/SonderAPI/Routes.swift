import Foundation

/// Canonical path templates advertised by `/api/discovery`.
public enum SonderAPIRoutes {
    public static let health = "/api/health"
    public static let discovery = "/api/discovery"
    public static let library = "/api/library"
    public static let status = "/api/status"
    public static let audiobooks = "/api/audiobooks"
    public static let audiobookBrowser = "/audiobooks"
    public static let progress = "/api/progress/{id}"
    public static let playback = "/api/playback/{id}"
    public static let playbackTrackRefresh = "/api/playback/{id}/refresh-tracks"
    public static let stream = "/stream/{id}"
    public static let subtitles = "/subtitles/{id}/{index}"
    public static let poster = "/artwork/poster/{id}"
    public static let backdrop = "/artwork/backdrop/{id}"

    public static func progress(itemID: UUID) -> String {
        "/api/progress/\(itemID.uuidString)"
    }

    public static func playback(itemID: UUID) -> String {
        "/api/playback/\(itemID.uuidString)"
    }

    public static func playbackTrackRefresh(itemID: UUID) -> String {
        "/api/playback/\(itemID.uuidString)/refresh-tracks"
    }

    public static func stream(itemID: UUID) -> String {
        "/stream/\(itemID.uuidString)"
    }

    public static func poster(itemID: UUID) -> String {
        "/artwork/poster/\(itemID.uuidString)"
    }

    public static func backdrop(itemID: UUID) -> String {
        "/artwork/backdrop/\(itemID.uuidString)"
    }
}
