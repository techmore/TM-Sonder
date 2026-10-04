import Foundation
import SonderAPI

// Shared wire types live in the SonderAPI package. Client-only models and thin
// convenience aliases stay here.

typealias SonderMediaItem = SonderPublicMediaItem
typealias SonderServerSettings = SonderPublicServerSettings
typealias SonderPlaybackResponse = SonderPlaybackSessionResponse
typealias SonderProgressUpdate = SonderPlaybackStateUpdate

nonisolated struct SonderLibraryCache: Codable, Sendable {
    var response: SonderLibraryResponse
    var cachedAt: Date
    var serverURL: URL
    var etag: String?

    init(response: SonderLibraryResponse, cachedAt: Date, serverURL: URL, etag: String? = nil) {
        self.response = response
        self.cachedAt = cachedAt
        self.serverURL = serverURL
        self.etag = etag
    }
}

struct SonderClientEvent: Identifiable, Hashable {
    var id = UUID()
    var date = Date()
    var title: String
    var detail: String
}

enum SonderConnectionMode: String, Codable, Hashable, Identifiable {
    case local
    case lan
    case tailscale
    case manual
    case offline
    case stale
    case unauthorized

    var id: String { rawValue }

    var label: String {
        switch self {
        case .local: "Local"
        case .lan: "LAN"
        case .tailscale: "Tailscale"
        case .manual: "Manual"
        case .offline: "Offline"
        case .stale: "Stale"
        case .unauthorized: "Restricted"
        }
    }
}

extension SonderMediaKind {
    /// Labels tuned for iOS tab chrome.
    var tabLabel: String {
        switch self {
        case .all: "All"
        case .movie: "Movies"
        case .tvShow: "TV Shows"
        case .documentary: "Docs"
        case .audiobook: "Audiobooks"
        case .ebook: "Books"
        }
    }
}

extension JSONDecoder {
    nonisolated static var sonder: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let value = try decoder.singleValueContainer().decode(String.self)
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = formatter.date(from: value) { return date }
            formatter.formatOptions = [.withInternetDateTime]
            if let date = formatter.date(from: value) { return date }
            throw DecodingError.dataCorruptedError(in: try decoder.singleValueContainer(), debugDescription: "Invalid ISO 8601 date")
        }
        return decoder
    }
}

extension JSONEncoder {
    nonisolated static var sonder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}
