import CryptoKit
import Foundation

nonisolated struct SonderMetadataEnrichment: Sendable {
    var summary: String?
    var publisher: String?
    var posterPath: String?
    var backdropPath: String?
    var tags: [String]?
}

actor SonderMetadataEnricher {
    private let cacheRoot: URL
    private let session: URLSession

    init(cacheRoot: URL) {
        self.cacheRoot = cacheRoot
        self.session = URLSession(configuration: .ephemeral)
        try? FileManager.default.createDirectory(at: cacheRoot, withIntermediateDirectories: true)
    }

    func enrich(item: SonderMediaItem) async -> SonderMetadataEnrichment? {
        let query = makeQuery(for: item)
        guard query.isEmpty == false else { return nil }
        let cacheKey = SHA256.hash(data: Data(query.utf8)).map { String(format: "%02x", $0) }.joined()
        let cachedJSON = cacheRoot.appendingPathComponent("\(cacheKey).json")
        let cachedPoster = cacheRoot.appendingPathComponent("\(cacheKey).jpg")
        let cachedBackdrop = cacheRoot.appendingPathComponent("\(cacheKey)-backdrop.jpg")
        if let data = try? Data(contentsOf: cachedJSON),
           let decoded = try? JSONDecoder().decode(SonderCachedMetadata.self, from: data) {
            return decoded.toEnrichment(posterPath: FileManager.default.fileExists(atPath: cachedPoster.path) ? cachedPoster.path : nil, backdropPath: FileManager.default.fileExists(atPath: cachedBackdrop.path) ? cachedBackdrop.path : nil)
        }

        if item.kind == .audiobook,
           let result = await audnexusLookup(item: item, cachedJSON: cachedJSON, cachedPoster: cachedPoster) {
            return result
        }

        if item.kind == .ebook,
           let result = await openLibraryLookup(item: item, cachedJSON: cachedJSON, cachedPoster: cachedPoster) {
            return result
        }

        guard let result = await wikipediaSearch(query: query) else { return nil }
        if let imageURL = result.thumbnailURL, let imageData = await download(url: imageURL) {
            try? imageData.write(to: cachedPoster, options: .atomic)
        }
        if let backdropURL = result.originalImageURL, let imageData = await download(url: backdropURL) {
            try? imageData.write(to: cachedBackdrop, options: .atomic)
        }
        let payload = SonderCachedMetadata(summary: result.extract, publisher: result.publisher, tags: result.tags)
        if let encoded = try? JSONEncoder().encode(payload) {
            try? encoded.write(to: cachedJSON, options: .atomic)
        }
        return payload.toEnrichment(
            posterPath: FileManager.default.fileExists(atPath: cachedPoster.path) ? cachedPoster.path : nil,
            backdropPath: FileManager.default.fileExists(atPath: cachedBackdrop.path) ? cachedBackdrop.path : nil
        )
    }

    private func makeQuery(for item: SonderMediaItem) -> String {
        let year = item.year == 0 ? nil : String(item.year)
        switch item.kind {
        case .movie:
            return [item.title, item.edition, year, "film", "Wikipedia"].compactMap { $0 }.joined(separator: " ")
        case .documentary:
            return [item.title, item.edition, year, "documentary", "Wikipedia"].compactMap { $0 }.joined(separator: " ")
        case .tvShow:
            let show = item.showTitle ?? item.title
            if let season = item.seasonNumber, let episode = item.episodeNumber {
                let code = String(format: "S%02dE%02d", season, episode)
                return [show, code, item.title, "episode", "Wikipedia"].joined(separator: " ")
            }
            return [show, "television series", "Wikipedia"].joined(separator: " ")
        case .ebook:
            return [item.title, item.edition, item.studio, year, "book", "Wikipedia"].compactMap { $0 }.joined(separator: " ")
        case .audiobook:
            return [item.title, item.edition, item.studio, year, "audiobook", "Wikipedia"].compactMap { $0 }.joined(separator: " ")
        case .all:
            return ""
        }
    }

    private func audnexusLookup(item: SonderMediaItem, cachedJSON: URL, cachedPoster: URL) async -> SonderMetadataEnrichment? {
        guard let source = item.metadataIDSource?.lowercased(),
              ["audible", "audnexus"].contains(source),
              let asin = item.metadataID?.trimmingCharacters(in: .whitespacesAndNewlines),
              asin.isEmpty == false else { return nil }

        var components = URLComponents(string: "https://api.audnex.us/books/\(asin)")
        components?.queryItems = [URLQueryItem(name: "region", value: "us")]
        guard let url = components?.url,
              let (data, response) = try? await session.data(from: url),
              (response as? HTTPURLResponse)?.statusCode ?? 500 < 400,
              let book = try? JSONDecoder().decode(SonderAudnexusBook.self, from: data) else { return nil }

        if let imageURL = book.image, let imageData = await download(url: imageURL) {
            try? imageData.write(to: cachedPoster, options: .atomic)
        }

        let payload = SonderCachedMetadata(
            summary: book.description,
            publisher: book.publisher,
            tags: book.genreNames + book.authorNames + book.narratorNames
        )
        if let encoded = try? JSONEncoder().encode(payload) {
            try? encoded.write(to: cachedJSON, options: .atomic)
        }
        return payload.toEnrichment(
            posterPath: FileManager.default.fileExists(atPath: cachedPoster.path) ? cachedPoster.path : nil,
            backdropPath: nil
        )
    }

    /// Open Library's public search and Covers endpoints require neither an API
    /// key nor an account. We only accept an exact normalized title match so a
    /// plausible-but-wrong cover is never preferred over a missing cover.
    private func openLibraryLookup(item: SonderMediaItem, cachedJSON: URL, cachedPoster: URL) async -> SonderMetadataEnrichment? {
        var components = URLComponents(string: "https://openlibrary.org/search.json")
        components?.queryItems = [
            URLQueryItem(name: "title", value: item.title),
            URLQueryItem(name: "limit", value: "5"),
            URLQueryItem(name: "fields", value: "title,author_name,first_publish_year,cover_i,subject")
        ]
        guard let url = components?.url,
              let (data, response) = try? await session.data(from: url),
              (response as? HTTPURLResponse)?.statusCode ?? 500 < 400,
              let response = try? JSONDecoder().decode(SonderOpenLibrarySearch.self, from: data),
              let match = response.docs.first(where: { normalizeBookTitle($0.title) == normalizeBookTitle(item.title) }) else { return nil }

        if let coverID = match.coverID,
           let coverURL = URL(string: "https://covers.openlibrary.org/b/id/\(coverID)-L.jpg"),
           let imageData = await download(url: coverURL) {
            try? imageData.write(to: cachedPoster, options: .atomic)
        }
        let tags = (match.subjects ?? []).prefix(8).map { $0.lowercased() }
            + (match.authorNames ?? []).map { $0.lowercased() }
            + ["open-library"]
        let payload = SonderCachedMetadata(summary: nil, publisher: match.authorNames?.first, tags: tags)
        if let encoded = try? JSONEncoder().encode(payload) {
            try? encoded.write(to: cachedJSON, options: .atomic)
        }
        return payload.toEnrichment(
            posterPath: FileManager.default.fileExists(atPath: cachedPoster.path) ? cachedPoster.path : nil,
            backdropPath: nil
        )
    }

    private func normalizeBookTitle(_ value: String) -> String {
        value.folding(options: [.diacriticInsensitive, .caseInsensitive], locale: .current)
            .components(separatedBy: CharacterSet.alphanumerics.inverted)
            .joined()
    }

    private func wikipediaSearch(query: String) async -> SonderWikipediaLookupResult? {
        var components = URLComponents(string: "https://en.wikipedia.org/w/api.php")
        components?.queryItems = [
            URLQueryItem(name: "action", value: "query"),
            URLQueryItem(name: "generator", value: "search"),
            URLQueryItem(name: "gsrsearch", value: query),
            URLQueryItem(name: "gsrlimit", value: "1"),
            URLQueryItem(name: "prop", value: "extracts|pageimages|info|categories"),
            URLQueryItem(name: "exintro", value: "1"),
            URLQueryItem(name: "explaintext", value: "1"),
            URLQueryItem(name: "inprop", value: "url"),
            URLQueryItem(name: "piprop", value: "thumbnail|original"),
            URLQueryItem(name: "pithumbsize", value: "800"),
            URLQueryItem(name: "cllimit", value: "10"),
            URLQueryItem(name: "format", value: "json"),
            URLQueryItem(name: "origin", value: "*")
        ]
        guard let url = components?.url,
              let (data, _) = try? await session.data(from: url),
              let decoded = try? JSONDecoder().decode(SonderWikipediaResponse.self, from: data),
              let page = decoded.query.pages.values.first else { return nil }
        return SonderWikipediaLookupResult(
            extract: page.extract,
            publisher: page.description,
            tags: page.categories?.compactMap { $0.title.split(separator: ":").last.map(String.init) }.prefix(6).map { $0.lowercased() },
            thumbnailURL: page.thumbnail?.source,
            originalImageURL: page.originalimage?.source
        )
    }

    private func download(url: URL) async -> Data? {
        guard let (data, response) = try? await session.data(from: url),
              (response as? HTTPURLResponse)?.statusCode ?? 200 < 400 else { return nil }
        return data
    }
}

nonisolated private struct SonderCachedMetadata: Codable {
    var summary: String?
    var publisher: String?
    var tags: [String]?

    func toEnrichment(posterPath: String?, backdropPath: String?) -> SonderMetadataEnrichment {
        SonderMetadataEnrichment(summary: summary, publisher: publisher, posterPath: posterPath, backdropPath: backdropPath, tags: tags)
    }
}

nonisolated private struct SonderAudnexusBook: Codable {
    var asin: String?
    var title: String?
    var subtitle: String?
    var description: String?
    var publisher: String?
    var image: URL?
    var authors: [SonderAudnexusPerson]?
    var narrators: [SonderAudnexusPerson]?
    var genres: [SonderAudnexusGenre]?

    var authorNames: [String] {
        authors?.compactMap(\.name).filter { $0.isEmpty == false } ?? []
    }

    var narratorNames: [String] {
        narrators?.compactMap(\.name).filter { $0.isEmpty == false } ?? []
    }

    var genreNames: [String] {
        genres?.compactMap(\.name).filter { $0.isEmpty == false }.map { $0.lowercased() } ?? []
    }
}

nonisolated private struct SonderAudnexusPerson: Codable {
    var asin: String?
    var name: String?
}

nonisolated private struct SonderAudnexusGenre: Codable {
    var asin: String?
    var name: String?
    var type: String?
}

nonisolated private struct SonderWikipediaResponse: Codable {
    var query: SonderWikipediaQuery
}

nonisolated private struct SonderWikipediaQuery: Codable {
    var pages: [String: SonderWikipediaPage]
}

nonisolated private struct SonderWikipediaPage: Codable {
    var extract: String?
    var description: String?
    var thumbnail: SonderWikipediaImage?
    var originalimage: SonderWikipediaImage?
    var categories: [SonderWikipediaCategory]?
}

nonisolated private struct SonderWikipediaImage: Codable {
    var source: URL?
}

nonisolated private struct SonderWikipediaCategory: Codable {
    var title: String
}

nonisolated private struct SonderOpenLibrarySearch: Codable {
    var docs: [SonderOpenLibraryDocument]
}

nonisolated private struct SonderOpenLibraryDocument: Codable {
    var title: String
    var authorNames: [String]?
    var coverID: Int?
    var subjects: [String]?

    enum CodingKeys: String, CodingKey {
        case title
        case authorNames = "author_name"
        case coverID = "cover_i"
        case subjects = "subject"
    }
}

nonisolated private struct SonderWikipediaLookupResult {
    var extract: String?
    var publisher: String?
    var tags: [String]?
    var thumbnailURL: URL?
    var originalImageURL: URL?
}
