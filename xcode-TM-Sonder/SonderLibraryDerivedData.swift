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
        let tvShowGroups = Dictionary(grouping: episodes, by: { $0.showTitle ?? $0.title })
            .map { showName, showEpisodes in
                let seasons = Dictionary(grouping: showEpisodes, by: { $0.seasonNumber ?? SonderTVSeasonGroup.unknownSeasonNumber })
                    .map { seasonNumber, seasonEpisodes in
                        SonderTVSeasonGroup(
                            seasonNumber: seasonNumber,
                            episodes: seasonEpisodes.sorted { ($0.episodeNumber ?? 0, $0.title) < ($1.episodeNumber ?? 0, $1.title) }
                        )
                    }
                    .sorted { $0.sortOrder < $1.sortOrder }
                return SonderTVShowGroup(name: showName, seasons: seasons)
            }
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
