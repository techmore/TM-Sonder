import Foundation

nonisolated struct SonderParsedMedia {
    var title: String
    var subtitle: String
    var kind: SonderMediaKind
    var year: Int?
    var showTitle: String?
    var season: Int?
    var episode: Int?
    var metadataIDSource: String?
    var metadataID: String?
    var edition: String?
    var splitPart: String?
    var localPosterPath: String?
    var localBackdropPath: String?
    var subtitlePaths: [String] = []
}

enum SonderMediaParser {
    nonisolated static func parseTitle(url: URL, libraryKind: SonderLibraryImportKind? = nil) -> SonderParsedMedia {
        parse(url: url, libraryKind: libraryKind, includeLocalAssets: false)
    }

    nonisolated static func parse(url: URL, libraryKind: SonderLibraryImportKind? = nil) -> SonderParsedMedia {
        parse(url: url, libraryKind: libraryKind, includeLocalAssets: true)
    }

    nonisolated private static func parse(url: URL, libraryKind: SonderLibraryImportKind? = nil, includeLocalAssets: Bool) -> SonderParsedMedia {
        let raw = url.deletingPathExtension().lastPathComponent.cleanedMediaTitle
        let parent = url.deletingLastPathComponent().lastPathComponent.cleanedMediaTitle
        let grandparent = url.deletingLastPathComponent().deletingLastPathComponent().lastPathComponent.cleanedMediaTitle
        let assets = includeLocalAssets ? localAssets(near: url) : (poster: nil, backdrop: nil, subtitles: [])
        let metadataTag = extractMetadataTag(from: raw) ?? extractMetadataTag(from: parent)
        let edition = extractEdition(from: raw) ?? extractEdition(from: parent)
        let splitPart = extractSplitPart(from: raw)
        let movieFolderCandidate = extractMovieNameAndYear(from: parent)
        let movieFileCandidate = extractMovieNameAndYear(from: raw)
        let nsRange = NSRange(raw.startIndex..<raw.endIndex, in: raw)
        let patterns = [
            #"(?i)^(.+?)[\s._-]+S(\d{1,2})E(\d{1,2})(?:[\s._-]*E?(\d{1,2}))?[\s._-]*(.*)$"#,
            #"(?i)^S(\d{1,2})E(\d{1,2})(?:[\s._-]*E?(\d{1,2}))?[\s._-]*(.*)$"#,
            #"(?i)^(.+?)[\s._-]+(\d{1,2})x(\d{1,2})[\s._-]*(.*)$"#
        ]

        for pattern in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern),
                  let match = regex.firstMatch(in: raw, range: nsRange) else {
                continue
            }
            let hasShowPrefix = match.numberOfRanges >= 5
            let showRangeIndex = hasShowPrefix ? 1 : nil
            let seasonIndex = hasShowPrefix ? 2 : 1
            let episodeIndex = hasShowPrefix ? 3 : 2
            let titleIndex = match.numberOfRanges - 1
            guard let seasonRange = Range(match.range(at: seasonIndex), in: raw),
                  let episodeRange = Range(match.range(at: episodeIndex), in: raw) else {
                continue
            }
            let episodeTitle = Range(match.range(at: titleIndex), in: raw).map { cleanEpisodeTitle(String(raw[$0])) } ?? ""
            let folderShow = tvShowName(parent: parent, grandparent: grandparent)
            let parsedShow = showRangeIndex.flatMap { Range(match.range(at: $0), in: raw).map { cleanShowTitle(String(raw[$0])) } }
            let show = libraryKind == .tvShows ? folderShow : (parsedShow?.isEmpty == false ? parsedShow! : folderShow)
            let season = Int(raw[seasonRange]) ?? 1
            let episode = Int(raw[episodeRange]) ?? 1
            return tvParsedMedia(
                show: show,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                raw: raw,
                metadataTag: metadataTag,
                edition: edition,
                assets: assets
            )
        }

        if libraryKind == .tvShows, let absoluteEpisode = parseAbsoluteEpisode(raw: raw, parent: parent, grandparent: grandparent) {
            return tvParsedMedia(
                show: absoluteEpisode.show,
                season: absoluteEpisode.season,
                episode: absoluteEpisode.episode,
                episodeTitle: absoluteEpisode.title,
                raw: raw,
                metadataTag: metadataTag,
                edition: edition,
                assets: assets
            )
        }

        if libraryKind == .tvShows {
            let show = parent.localizedCaseInsensitiveContains("season") ? grandparent : parent
            let season = extractSeason(from: parent)
            return SonderParsedMedia(
                title: raw,
                subtitle: season.map { "\(show) - Season \($0)" } ?? show,
                kind: .tvShow,
                year: extractYear(from: raw) ?? extractYear(from: show),
                showTitle: show,
                season: season,
                episode: nil,
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        if libraryKind == .movies, let movie = movieFolderCandidate ?? movieFileCandidate {
            return SonderParsedMedia(
                title: movie.title,
                subtitle: movie.year.map { "Movie - \($0)" } ?? "Movie",
                kind: .movie,
                year: movie.year,
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                splitPart: splitPart,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        if libraryKind == .audiobooks {
            return SonderParsedMedia(
                title: movieFileCandidate?.title ?? raw.removingPlexTags.cleanedMediaTitle,
                subtitle: extractYear(from: raw).map { "Audiobook - \($0)" } ?? "Audiobook",
                kind: .audiobook,
                year: extractYear(from: raw),
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                splitPart: splitPart,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        if libraryKind == .ebooks {
            return SonderParsedMedia(
                title: movieFileCandidate?.title ?? raw.removingPlexTags.cleanedMediaTitle,
                subtitle: extractYear(from: raw).map { "Book - \($0)" } ?? "Book",
                kind: .ebook,
                year: extractYear(from: raw),
                metadataIDSource: metadataTag?.source,
                metadataID: metadataTag?.id,
                edition: edition,
                splitPart: splitPart,
                localPosterPath: assets.poster?.path,
                localBackdropPath: assets.backdrop?.path,
                subtitlePaths: assets.subtitles.map(\.path)
            )
        }

        let lower = raw.lowercased()
        let kind: SonderMediaKind
        if lower.contains("audiobook") {
            kind = .audiobook
        } else if lower.contains("book") || lower.contains("ebook") || ["epub", "pdf", "m4b", "mp3", "m4a"].contains(url.pathExtension.lowercased()) {
            kind = .ebook
        } else {
            kind = .movie
        }
        return SonderParsedMedia(
            title: movieFileCandidate?.title ?? raw.removingPlexTags.cleanedMediaTitle,
            subtitle: "Imported local media",
            kind: kind,
            year: extractYear(from: raw),
            metadataIDSource: metadataTag?.source,
            metadataID: metadataTag?.id,
            edition: edition,
            splitPart: splitPart,
            localPosterPath: assets.poster?.path,
            localBackdropPath: assets.backdrop?.path,
            subtitlePaths: assets.subtitles.map(\.path)
        )
    }

    private struct AbsoluteEpisodeMatch {
        var show: String
        var season: Int
        var episode: Int
        var title: String
    }

    nonisolated private static func tvParsedMedia(
        show: String,
        season: Int,
        episode: Int,
        episodeTitle: String,
        raw: String,
        metadataTag: (source: String, id: String)?,
        edition: String?,
        assets: (poster: URL?, backdrop: URL?, subtitles: [URL])
    ) -> SonderParsedMedia {
        SonderParsedMedia(
            title: episodeTitle.isEmpty ? "Episode \(episode)" : episodeTitle,
            subtitle: "\(show) - S\(String(format: "%02d", season))E\(String(format: "%02d", episode))",
            kind: .tvShow,
            year: extractYear(from: raw) ?? extractYear(from: show),
            showTitle: show,
            season: season,
            episode: episode,
            metadataIDSource: metadataTag?.source,
            metadataID: metadataTag?.id,
            edition: edition,
            localPosterPath: assets.poster?.path,
            localBackdropPath: assets.backdrop?.path,
            subtitlePaths: assets.subtitles.map(\.path)
        )
    }

    nonisolated private static func parseAbsoluteEpisode(raw: String, parent: String, grandparent: String) -> AbsoluteEpisodeMatch? {
        let folderShow = tvShowName(parent: parent, grandparent: grandparent)
        let season = extractSeason(from: parent) ?? 1
        let normalized = raw
            .replacingOccurrences(of: #"^\[[^\]]+\][\s._-]*"#, with: "", options: .regularExpression)
            .cleanedMediaTitle
        let patterns = [
            #"(?i)^(.+?)[\s._-]+(?:episode|ep)?[\s._-]*(\d{1,3})(?:v\d+)?(?:[\s._-]+(.+))?$"#,
            #"(?i)^(?:episode|ep)?[\s._-]*(\d{1,3})(?:v\d+)?(?:[\s._-]+(.+))?$"#
        ]

        for pattern in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern),
                  let match = regex.firstMatch(in: normalized, range: NSRange(normalized.startIndex..<normalized.endIndex, in: normalized)) else { continue }
            let hasShowPrefix = match.numberOfRanges == 4
            let episodeIndex = hasShowPrefix ? 2 : 1
            let titleIndex = hasShowPrefix ? 3 : 2
            guard let episodeRange = Range(match.range(at: episodeIndex), in: normalized),
                  let episode = Int(normalized[episodeRange]),
                  isPlausibleAbsoluteEpisode(episode) else { continue }
            let parsedShow = hasShowPrefix ? Range(match.range(at: 1), in: normalized).map { cleanShowTitle(String(normalized[$0])) } : nil
            let title = Range(match.range(at: titleIndex), in: normalized).map { cleanEpisodeTitle(String(normalized[$0])) } ?? ""
            return AbsoluteEpisodeMatch(
                show: parsedShow?.isEmpty == false ? parsedShow! : folderShow,
                season: season,
                episode: episode,
                title: title
            )
        }
        return nil
    }

    nonisolated private static func isPlausibleAbsoluteEpisode(_ value: Int) -> Bool {
        (1...200).contains(value)
    }

    nonisolated private static func tvShowName(parent: String, grandparent: String) -> String {
        parent.localizedCaseInsensitiveContains("season") ? grandparent : parent
    }

    nonisolated private static func cleanShowTitle(_ value: String) -> String {
        value
            .replacingOccurrences(of: #"^\[[^\]]+\][\s._-]*"#, with: "", options: .regularExpression)
            .removingPlexTags
            .cleanedMediaTitle
    }

    nonisolated private static func cleanEpisodeTitle(_ value: String) -> String {
        let cleaned = value
            .replacingOccurrences(of: #"\[[^\]]*(?:720|1080|2160|x264|x265|h\.264|h\.265|hevc|aac|flac|bd|bluray|web|webrip|dvd|dual audio)[^\]]*\]"#, with: "", options: [.regularExpression, .caseInsensitive])
            .replacingOccurrences(of: #"\([^)]*(?:720|1080|2160|x264|x265|h\.264|h\.265|hevc|aac|flac|bd|bluray|web|webrip|dvd|dual audio)[^)]*\)"#, with: "", options: [.regularExpression, .caseInsensitive])
            .replacingOccurrences(of: #"\[[A-F0-9]{8}\]"#, with: "", options: [.regularExpression, .caseInsensitive])
            .removingPlexTags
            .cleanedMediaTitle
            .trimmingCharacters(in: CharacterSet(charactersIn: "- "))
        return cleaned
    }

    nonisolated private static func extractMovieNameAndYear(from value: String) -> (title: String, year: Int?)? {
        guard let year = extractYear(from: value) else { return nil }
        let title = value
            .replacingOccurrences(of: #"\(\d{4}\)"#, with: "", options: .regularExpression)
            .replacingOccurrences(of: #"\b\d{4}\b"#, with: "", options: .regularExpression)
            .removingPlexTags
            .removingSplitSuffix
            .cleanedMediaTitle
        return (title.isEmpty ? value : title, year)
    }

    nonisolated private static func extractMetadataTag(from value: String) -> (source: String, id: String)? {
        guard let match = value.firstMatch(pattern: #"\{(imdb|tmdb|audible|audnexus)-([^}]+)\}"#) else { return nil }
        return (match[0], match[1])
    }

    nonisolated private static func extractEdition(from value: String) -> String? {
        value.firstMatch(pattern: #"\{edition-([^}]{1,32})\}"#)?.first
    }

    nonisolated private static func extractSplitPart(from value: String) -> String? {
        value.firstMatch(pattern: #"(?i)(?:^|[\s._-])(cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$"#)?.first
    }

    nonisolated static func localAssets(near url: URL) -> (poster: URL?, backdrop: URL?, subtitles: [URL]) {
        let folder = url.deletingLastPathComponent()
        let parentFolder = folder.deletingLastPathComponent()
        let base = url.deletingPathExtension().lastPathComponent
        let assetFolders = uniqueFolders([folder, parentFolder])
        let posterNames = ["poster.jpg", "poster.png", "cover.jpg", "cover.png", "folder.jpg", "folder.png", "\(base).jpg", "\(base).png"]
        let backdropNames = ["background.jpg", "background.png", "fanart.jpg", "fanart.png", "backdrop.jpg", "backdrop.png"]
        let poster = firstExistingAsset(named: posterNames, in: assetFolders)
        let backdrop = firstExistingAsset(named: backdropNames, in: assetFolders)
        let subtitles = (try? FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil))
            .map { urls in urls.filter { ["srt", "vtt", "ass", "ssa"].contains($0.pathExtension.lowercased()) && $0.deletingPathExtension().lastPathComponent.hasPrefix(base) } } ?? []
        return (poster, backdrop, subtitles)
    }

    nonisolated static func localAssets(inMaterialFolder folder: URL) -> (poster: URL?, backdrop: URL?) {
        let posterNames = ["poster.jpg", "poster.png", "cover.jpg", "cover.png", "folder.jpg", "folder.png"]
        let backdropNames = ["background.jpg", "background.png", "fanart.jpg", "fanart.png", "backdrop.jpg", "backdrop.png"]
        return (
            poster: firstExistingAsset(named: posterNames, in: [folder]),
            backdrop: firstExistingAsset(named: backdropNames, in: [folder])
        )
    }

    nonisolated private static func uniqueFolders(_ folders: [URL]) -> [URL] {
        var seen = Set<String>()
        return folders.filter { seen.insert($0.path).inserted }
    }

    nonisolated private static func firstExistingAsset(named names: [String], in folders: [URL]) -> URL? {
        for folder in folders {
            for name in names {
                let url = folder.appendingPathComponent(name)
                if FileManager.default.fileExists(atPath: url.path) {
                    return url
                }
            }
        }
        return nil
    }

    nonisolated private static func extractYear(from value: String) -> Int? {
        guard let range = value.range(of: #"(19|20)\d{2}"#, options: .regularExpression) else { return nil }
        return Int(value[range])
    }

    nonisolated private static func extractSeason(from value: String) -> Int? {
        let normalized = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if normalized.range(of: #"(?i)^(specials|season[\s._-]*0|s0{1,2})$"#, options: .regularExpression) != nil {
            return 0
        }
        let patterns = [
            #"(?i)season[\s._-]*(\d{1,2})"#,
            #"(?i)^s(\d{1,2})$"#,
            #"(?i)^series[\s._-]*(\d{1,2})$"#
        ]
        for pattern in patterns {
            guard let range = normalized.range(of: pattern, options: .regularExpression) else { continue }
            let digits = normalized[range].filter(\.isNumber)
            return Int(String(digits))
        }
        return nil
    }
}
