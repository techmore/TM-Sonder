import Foundation

nonisolated struct SonderDerivedData {
    var playableCount: Int
    var averageProgress: Double
    var allTags: [String]
    var tagUsage: [(tag: String, count: Int)]
    var mediaKindCounts: [SonderMediaKind: Int]
    var inProgressItems: [SonderMediaItem]
    var tvShowGroups: [SonderTVShowGroup]

    static func make(items: [SonderMediaItem], progressRecords: [SonderProgress]) -> SonderDerivedData {
        let playableCount = items.filter(\.hasFile).count
        let averageProgress = progressRecords.isEmpty ? 0 : progressRecords.map(\.percent).reduce(0, +) / Double(progressRecords.count)
        let allTags = Array(Set(items.flatMap(\.tags))).sorted()
        let tagUsage: [(tag: String, count: Int)] = Dictionary(grouping: items.flatMap(\.tags), by: { $0 })
            .map { (tag: $0.key, count: $0.value.count) }
            .sorted { lhs, rhs in lhs.count > rhs.count }
        let mediaKindCounts = Dictionary(grouping: items, by: \.kind).mapValues(\.count)

        let itemsByID = Dictionary(items.map { ($0.id, $0) }, uniquingKeysWith: { current, _ in current })
        let inProgressItems = progressRecords
            .filter { $0.seconds > 0 && $0.percent < 0.96 }
            .sorted { $0.updatedAt > $1.updatedAt }
            .compactMap { itemsByID[$0.itemID] }

        let episodes = items
            .filter { $0.kind == .tvShow }
            .sorted { lhs, rhs in
                let lhsKey = lhs.showTitle ?? lhs.title
                let rhsKey = rhs.showTitle ?? rhs.title
                if lhsKey != rhsKey { return lhsKey < rhsKey }
                if lhs.seasonNumber != rhs.seasonNumber { return (lhs.seasonNumber ?? 0) < (rhs.seasonNumber ?? 0) }
                if lhs.episodeNumber != rhs.episodeNumber { return (lhs.episodeNumber ?? 0) < (rhs.episodeNumber ?? 0) }
                return lhs.title < rhs.title
            }
        // Broken into named steps with explicit types.
        //
        // As one nested expression -- Dictionary(grouping:).map { Dictionary(
        // grouping:).map { ... }.sorted }.sorted -- the Swift type checker gives
        // up on it: "unable to type-check this expression in reasonable time" on
        // Xcode 26.6, while a newer compiler manages. Naming each intermediate
        // means nothing has to be inferred through the nesting, so it compiles
        // the same way on every toolchain instead of depending on how capable
        // the checker happens to be that day.
        let episodesByShow: [String: [SonderMediaItem]] = Dictionary(
            grouping: episodes,
            by: { $0.showTitle ?? $0.title }
        )
        let showGroups: [SonderTVShowGroup] = episodesByShow.map { showName, showEpisodes in
            let episodesBySeason: [Int: [SonderMediaItem]] = Dictionary(
                grouping: showEpisodes,
                by: { $0.seasonNumber ?? SonderTVSeasonGroup.unknownSeasonNumber }
            )
            let seasons: [SonderTVSeasonGroup] = episodesBySeason
                .map { seasonNumber, seasonEpisodes in
                    let orderedEpisodes: [SonderMediaItem] = seasonEpisodes.sorted { lhs, rhs in
                        (lhs.episodeNumber ?? 0, lhs.title) < (rhs.episodeNumber ?? 0, rhs.title)
                    }
                    return SonderTVSeasonGroup(seasonNumber: seasonNumber, episodes: orderedEpisodes)
                }
                .sorted { $0.sortOrder < $1.sortOrder }
            return SonderTVShowGroup(name: showName, seasons: seasons)
        }
        let tvShowGroups: [SonderTVShowGroup] = showGroups
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }

        return SonderDerivedData(
            playableCount: playableCount,
            averageProgress: averageProgress,
            allTags: allTags,
            tagUsage: tagUsage,
            mediaKindCounts: mediaKindCounts,
            inProgressItems: inProgressItems,
            tvShowGroups: tvShowGroups
        )
    }
}
