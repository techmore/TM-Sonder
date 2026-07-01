import Foundation

nonisolated struct SonderAssetRefreshTarget: Sendable, Hashable {
    var id: UUID
    var url: URL
    var kind: SonderMediaKind
    var title: String
    var showTitle: String?
    var seasonNumber: Int?
    var episodeNumber: Int?
}

nonisolated struct SonderAssetRefreshProgress: Sendable, Hashable {
    var completed: Int
    var total: Int
}

nonisolated struct SonderAssetRefreshSummary: Sendable {
    var assetUpdates: [UUID: SonderLocalAssetUpdate]
    var contextUpdates: [UUID: SonderContextMetadataUpdate]
    var probeResults: [UUID: SonderMediaProbe.Result]
    var updateCount: Int
}

nonisolated enum SonderContextResolver {
    static func metadataUpdate(for target: SonderAssetRefreshTarget) -> SonderContextMetadataUpdate? {
        guard let contextURL = contextURL(near: target.url),
              let data = try? Data(contentsOf: contextURL),
              let context = try? JSONDecoder.sonder.decode(SonderContext.self, from: data) else { return nil }

        let cacheFolder = contextURL.deletingLastPathComponent()
        let posterPath = assetPath(kind: "poster", in: context, cacheFolder: cacheFolder)
        let backdropPath = assetPath(kind: "art", in: context, cacheFolder: cacheFolder) ?? assetPath(kind: "backdrop", in: context, cacheFolder: cacheFolder)
        let episode = matchingEpisode(for: target, in: context)
        let cleanEpisodeTitle = episode?.title.trimmingCharacters(in: .whitespacesAndNewlines)
        let showTitle = context.media.title.trimmingCharacters(in: .whitespacesAndNewlines)
        let tags = Array(Set(context.media.genres.map { $0.lowercased() } + ["plex"]))
            .filter { $0.isEmpty == false }
            .sorted()

        return SonderContextMetadataUpdate(
            title: cleanEpisodeTitle?.isEmpty == false ? cleanEpisodeTitle : nil,
            subtitle: target.kind == .tvShow ? tvSubtitle(showTitle: showTitle, season: target.seasonNumber ?? episode?.seasonNumber) : nil,
            showTitle: showTitle.isEmpty ? nil : showTitle,
            summary: episode?.summary ?? context.media.summary,
            studio: context.media.studio,
            year: context.media.year,
            tags: tags,
            metadataIDSource: "plex",
            metadataID: episode.map { String($0.metadataItemID) } ?? context.source.metadataItemID.map(String.init),
            posterPath: posterPath,
            backdropPath: backdropPath
        )
    }

    private static func contextURL(near mediaURL: URL) -> URL? {
        var folder = mediaURL.deletingLastPathComponent()
        for _ in 0..<6 {
            let candidate = folder.appendingPathComponent(".sonder", isDirectory: true).appendingPathComponent("sonder-context.json")
            if FileManager.default.fileExists(atPath: candidate.path) {
                return candidate
            }
            let parent = folder.deletingLastPathComponent()
            guard parent.path != folder.path else { break }
            folder = parent
        }
        return nil
    }

    private static func matchingEpisode(for target: SonderAssetRefreshTarget, in context: SonderContext) -> SonderContextEpisode? {
        let episodes = context.episodes ?? []
        if let season = target.seasonNumber, let episode = target.episodeNumber,
           let match = episodes.first(where: { $0.seasonNumber == season && $0.episodeNumber == episode }) {
            return match
        }
        return episodes.first { episode in
            episode.mediaFiles.contains { file in
                file == target.url.path || URL(fileURLWithPath: file).standardizedFileURL.path == target.url.standardizedFileURL.path
            }
        }
    }

    private static func assetPath(kind: String, in context: SonderContext, cacheFolder: URL) -> String? {
        guard let asset = context.artwork.first(where: { $0.kind.caseInsensitiveCompare(kind) == .orderedSame }) else { return nil }
        let url = cacheFolder.appendingPathComponent(asset.path)
        return FileManager.default.fileExists(atPath: url.path) ? url.path : nil
    }

    private static func tvSubtitle(showTitle: String, season: Int?) -> String? {
        guard showTitle.isEmpty == false else { return nil }
        guard let season else { return showTitle }
        return "\(showTitle) - Season \(season)"
    }
}

nonisolated struct SonderLocalAssetRefreshService: Sendable {
    var maxConcurrentRefreshes: Int
    var minProgressPublishInterval: TimeInterval

    func refresh(
        targets: [SonderAssetRefreshTarget],
        progress: @escaping @Sendable (SonderAssetRefreshProgress) async -> Void
    ) async -> SonderAssetRefreshSummary {
        let collector = SonderAssetRefreshCollector()
        let totalTargets = targets.count
        let progressCounter = SonderConcurrentCounter()

        await SonderConcurrencyLimiter.run(limit: max(maxConcurrentRefreshes, 1), over: targets) { target in
            let didAccess = target.url.startAccessingSecurityScopedResource()
            defer { if didAccess { target.url.stopAccessingSecurityScopedResource() } }

            let assets = SonderMediaParser.localAssets(near: target.url)
            let contextUpdate = SonderContextResolver.metadataUpdate(for: target)
            let probe = await SonderMediaProbe.probe(url: target.url)
            await collector.append(SonderAssetRefreshResult(
                itemID: target.id,
                assetUpdate: SonderLocalAssetUpdate(
                    posterPath: assets.poster?.path ?? contextUpdate?.posterPath,
                    backdropPath: assets.backdrop?.path ?? contextUpdate?.backdropPath,
                    subtitlePaths: assets.subtitles.map(\.path)
                ),
                contextUpdate: contextUpdate,
                probe: probe
            ))

            let progressSnapshot = await progressCounter.incrementAndShouldPublish(
                total: totalTargets,
                minInterval: minProgressPublishInterval
            )
            guard progressSnapshot.shouldPublish else { return }
            await progress(SonderAssetRefreshProgress(completed: progressSnapshot.value, total: totalTargets))
        }

        let results = await collector.all()
        return SonderAssetRefreshSummary(
            assetUpdates: Dictionary(results.map { ($0.itemID, $0.assetUpdate) }, uniquingKeysWith: { _, latest in latest }),
            contextUpdates: Dictionary(results.compactMap { result in result.contextUpdate.map { (result.itemID, $0) } }, uniquingKeysWith: { _, latest in latest }),
            probeResults: Dictionary(results.map { ($0.itemID, $0.probe) }, uniquingKeysWith: { _, latest in latest }),
            updateCount: results.count
        )
    }
}
