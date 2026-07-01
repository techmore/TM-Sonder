import Foundation

nonisolated enum SonderSection: String, CaseIterable, Identifiable {
    case library
    case movies
    case tvShows
    case audiobooks
    case ebooks
    case continueWatching
    case collections
    case server
    case log
    case about

    var id: String { rawValue }
}

nonisolated struct SonderCollection: Codable, Identifiable, Hashable {
    var id = UUID()
    var name: String
    var kind: SonderCollectionKind = .collection
    var itemIDs: [UUID]
}

nonisolated enum SonderCollectionKind: String, Codable, CaseIterable, Identifiable {
    case collection
    case playlist

    var id: String { rawValue }

    var label: String {
        switch self {
        case .collection: "Collection"
        case .playlist: "Playlist"
        }
    }

    var icon: String {
        switch self {
        case .collection: "folder.badge.plus"
        case .playlist: "music.note.list"
        }
    }
}

nonisolated struct SonderMediaDirectory: Codable, Identifiable, Hashable {
    var id = UUID()
    var name: String
    var path: String
    var bookmark: Data
    var kind: SonderLibraryImportKind = .movies
    var libraryID: UUID = SonderLibraryImportKind.movies.defaultLibraryID
    var lastIndexedCount = 0
    var lastScannedFileCount = 0
    var lastUnsupportedCount = 0
    var lastSkippedDuplicateCount = 0
    var lastParseFailureCount = 0
    var lastScannedAt: Date?

    var scanSummary: String {
        "unsupported \(lastUnsupportedCount) - duplicates \(lastSkippedDuplicateCount) - parse issues \(lastParseFailureCount)"
    }
}

nonisolated struct SonderLibraryDefinition: Codable, Identifiable, Hashable, Sendable {
    var id: UUID
    var name: String
    var kind: SonderLibraryImportKind
    var createdAt = Date()

    static let defaults = [
        SonderLibraryDefinition(id: SonderLibraryImportKind.movies.defaultLibraryID, name: "Movies", kind: .movies),
        SonderLibraryDefinition(id: SonderLibraryImportKind.tvShows.defaultLibraryID, name: "TV Shows", kind: .tvShows),
        SonderLibraryDefinition(id: SonderLibraryImportKind.audiobooks.defaultLibraryID, name: "Audiobooks", kind: .audiobooks),
        SonderLibraryDefinition(id: SonderLibraryImportKind.ebooks.defaultLibraryID, name: "Books", kind: .ebooks)
    ]

    static let defaultIDs = Set(defaults.map(\.id))

    var isDefault: Bool {
        Self.defaultIDs.contains(id)
    }
}

nonisolated enum SonderLibraryImportKind: String, Codable, CaseIterable, Identifiable {
    case movies
    case tvShows
    case audiobooks
    case ebooks

    var id: String { rawValue }

    nonisolated static let mediaLibraryDetectionPriority: [SonderLibraryImportKind] = [.tvShows, .movies, .ebooks, .audiobooks]

    nonisolated var label: String {
        switch self {
        case .movies: "Movies"
        case .tvShows: "TV Shows"
        case .audiobooks: "Audiobooks"
        case .ebooks: "Books"
        }
    }

    nonisolated var tag: String {
        switch self {
        case .movies: "movies"
        case .tvShows: "tv"
        case .audiobooks: "audiobooks"
        case .ebooks: "books"
        }
    }

    nonisolated var icon: String {
        switch self {
        case .movies: "film"
        case .tvShows: "tv"
        case .audiobooks: "headphones"
        case .ebooks: "book"
        }
    }

    nonisolated var defaultLibraryID: UUID {
        switch self {
        case .movies: UUID(uuidString: "11111111-1111-4111-8111-111111111111")!
        case .tvShows: UUID(uuidString: "22222222-2222-4222-8222-222222222222")!
        case .audiobooks: UUID(uuidString: "33333333-3333-4333-8333-333333333333")!
        case .ebooks: UUID(uuidString: "44444444-4444-4444-8444-444444444444")!
        }
    }
}

nonisolated struct SonderActivityEvent: Codable, Identifiable, Hashable, Sendable {
    var id = UUID()
    var title: String
    var detail: String
    var icon: String
    var date = Date()
}
