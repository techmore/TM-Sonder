import Foundation

public enum SonderMediaKind: String, Codable, Sendable, CaseIterable, Identifiable, Hashable {
    case all
    case movie
    case tvShow
    case documentary
    case audiobook
    case ebook

    public var id: String { rawValue }

    public static var mediaCases: [SonderMediaKind] {
        [.tvShow, .movie, .documentary, .audiobook, .ebook]
    }

    public static var mediaTabs: [SonderMediaKind] {
        [.movie, .tvShow, .documentary, .ebook, .audiobook]
    }

    public var label: String {
        switch self {
        case .all: "All"
        case .movie: "Movie"
        case .tvShow: "TV Show"
        case .documentary: "Documentary"
        case .audiobook: "Audiobook"
        case .ebook: "Book"
        }
    }

    public var shortLabel: String {
        switch self {
        case .all: "ALL"
        case .movie: "FILM"
        case .tvShow: "TV"
        case .documentary: "DOC"
        case .audiobook: "AUDIO"
        case .ebook: "BOOK"
        }
    }

    public var icon: String {
        switch self {
        case .all: "rectangle.stack"
        case .movie: "film"
        case .tvShow: "tv"
        case .documentary: "camera.metering.matrix"
        case .audiobook: "headphones"
        case .ebook: "book"
        }
    }
}

public enum SonderMediaFormat: String, Codable, Sendable, Hashable {
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

    public init(url: URL) {
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

    public static func isSupported(url: URL) -> Bool {
        SonderMediaFormat(url: url) != .unknown
    }

    public var contentType: String {
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

    public var isPlayableInAVPlayer: Bool {
        switch self {
        // WebM is cataloged and can be served, but AVFoundation on iOS does
        // not provide dependable WebM decoding. Keep the client Play affordance
        // honest until a server-side conversion path is available.
        case .mp4, .mov, .m4v, .m4b, .mp3, .m4a:
            return true
        default:
            return false
        }
    }

    public var isBrowserPlayable: Bool {
        switch self {
        case .mp4, .m4v, .mov, .webm: return true
        default: return false
        }
    }

    /// Client-facing guidance when the local player cannot open the original file.
    public var clientPlaybackGuidance: String {
        switch self {
        case .mkv, .avi, .mpg, .mpeg, .ts, .m2ts:
            return "\(rawValue.uppercased()) is cataloged on the server but is not decoded by the iOS player. On the Mac host, convert the title to MP4 (Library → Convert) or re-encode with H.264/AAC, then rescan."
        case .pdf:
            return "PDF opens in Sonder's built-in iOS reader."
        case .epub:
            return "EPUB is cataloged on iOS, but its built-in reader is still in development. Open it on the Mac host for now."
        case .unknown:
            return "This format is not recognized for in-app playback. Convert to MP4 on the Mac host, then rescan."
        default:
            return "This title is available from the server but is not rendered by the in-app player."
        }
    }
}
