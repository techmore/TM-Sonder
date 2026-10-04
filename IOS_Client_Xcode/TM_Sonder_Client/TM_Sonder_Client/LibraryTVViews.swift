import SwiftUI
import SonderAPI

struct TVShowsTab: View {
    @ObservedObject var model: SonderClientModel

    private var shows: [TVShowGroup] {
        let episodes = model.filteredItems(for: .tvShow)

        let groups = Dictionary(grouping: episodes) { item in
            item.showGroupID ?? (item.showTitle?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 } ?? item.title
        }

        return groups
            .map { TVShowGroup(groupID: $0.key, name: $0.value.first?.showGroupTitle ?? $0.value.first?.showTitle ?? $0.key, episodes: $0.value.sorted(by: TVShowsTab.episodeSort)) }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 18) {
                    SectionHeader(title: "TV", subtitle: "Shows, seasons, and episodes", count: shows.count)
                    LibrarySearchBar(model: model)

                    if shows.isEmpty {
                        EmptyLibraryState()
                    } else {
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 150, maximum: 180), spacing: 16)], spacing: 18) {
                            ForEach(shows) { show in
                                NavigationLink(value: show) {
                                    TVShowCard(show: show, model: model)
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
                .padding(20)
            }
            .background(SonderPalette.background.ignoresSafeArea())
            .refreshable {
                await model.reload()
            }
            .navigationTitle("TV")
            .navigationDestination(for: TVShowGroup.self) { show in
                TVSeasonsView(show: show, model: model)
            }
            .navigationDestination(for: TVSeasonGroup.self) { season in
                TVEpisodesView(season: season, model: model)
            }
            .navigationDestination(for: SonderMediaItem.self) { item in
                MediaDetailView(model: model, item: item)
            }
        }
    }

    nonisolated private static func episodeSort(_ lhs: SonderMediaItem, _ rhs: SonderMediaItem) -> Bool {
        let left = (lhs.seasonNumber ?? 0, lhs.episodeNumber ?? 0, lhs.title)
        let right = (rhs.seasonNumber ?? 0, rhs.episodeNumber ?? 0, rhs.title)
        if left.0 != right.0 { return left.0 < right.0 }
        if left.1 != right.1 { return left.1 < right.1 }
        return left.2.localizedStandardCompare(right.2) == .orderedAscending
    }
}

struct TVSeasonsView: View {
    let show: TVShowGroup
    @ObservedObject var model: SonderClientModel

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                SectionHeader(title: show.name, subtitle: "\(show.episodes.count) episodes", count: show.seasons.count)

                LazyVGrid(columns: [GridItem(.adaptive(minimum: 150, maximum: 190), spacing: 16)], spacing: 18) {
                    ForEach(show.seasons) { season in
                        NavigationLink(value: season) {
                            TVSeasonCard(season: season, model: model)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .padding(20)
        }
        .background(SonderPalette.background.ignoresSafeArea())
        .refreshable {
            await model.reload()
        }
        .navigationTitle(show.name)
    }
}

struct TVEpisodesView: View {
    let season: TVSeasonGroup
    @ObservedObject var model: SonderClientModel

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                SectionHeader(title: season.label, subtitle: season.showName, count: season.episodes.count)

                LazyVStack(spacing: 12) {
                    ForEach(season.episodes) { episode in
                        NavigationLink(value: episode) {
                            TVEpisodeRow(episode: episode, progress: model.progress(for: episode), artworkURL: model.artworkURL(for: episode), accessToken: model.accessToken)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .padding(20)
        }
        .background(SonderPalette.background.ignoresSafeArea())
        .refreshable {
            await model.reload()
        }
        .navigationTitle(season.label)
    }
}

struct TVShowGroup: Identifiable, Hashable {
    var id: String { groupID }
    let groupID: String
    let name: String
    let episodes: [SonderMediaItem]

    static func == (lhs: TVShowGroup, rhs: TVShowGroup) -> Bool {
        lhs.id == rhs.id
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }

    var seasons: [TVSeasonGroup] {
        let groups = Dictionary(grouping: episodes) { $0.seasonNumber ?? 0 }
        return groups
            .map { TVSeasonGroup(showName: name, seasonNumber: $0.key, episodes: $0.value.sorted(by: TVShowGroup.episodeSort)) }
            .sorted { $0.seasonNumber < $1.seasonNumber }
    }

    nonisolated private static func episodeSort(_ lhs: SonderMediaItem, _ rhs: SonderMediaItem) -> Bool {
        let left = (lhs.seasonNumber ?? 0, lhs.episodeNumber ?? 0, lhs.title)
        let right = (rhs.seasonNumber ?? 0, rhs.episodeNumber ?? 0, rhs.title)
        if left.0 != right.0 { return left.0 < right.0 }
        if left.1 != right.1 { return left.1 < right.1 }
        return left.2.localizedStandardCompare(right.2) == .orderedAscending
    }
}

struct TVSeasonGroup: Identifiable, Hashable {
    var id: String { "\(showName)-\(seasonNumber)" }
    let showName: String
    let seasonNumber: Int
    let episodes: [SonderMediaItem]

    static func == (lhs: TVSeasonGroup, rhs: TVSeasonGroup) -> Bool {
        lhs.id == rhs.id
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }

    var label: String {
        seasonNumber > 0 ? "Season \(seasonNumber)" : "Specials"
    }
}

struct TVShowCard: View {
    let show: TVShowGroup
    @ObservedObject var model: SonderClientModel

    private var firstEpisode: SonderMediaItem? { show.episodes.first }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let firstEpisode {
                PosterPlaceholder(item: firstEpisode, artworkURL: model.artworkURL(for: firstEpisode), accessToken: model.accessToken)
                    .frame(height: 214)
            }
            Text(show.name)
                .font(.headline)
                .foregroundStyle(SonderPalette.text)
                .lineLimit(2)
            Text("\(show.seasons.count) seasons - \(show.episodes.count) episodes")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct TVSeasonCard: View {
    let season: TVSeasonGroup
    @ObservedObject var model: SonderClientModel

    private var firstEpisode: SonderMediaItem? { season.episodes.first }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let firstEpisode {
                PosterPlaceholder(item: firstEpisode, artworkURL: model.artworkURL(for: firstEpisode), accessToken: model.accessToken)
                    .frame(height: 214)
            }
            Text(season.label)
                .font(.headline)
                .foregroundStyle(SonderPalette.text)
                .lineLimit(1)
            Text("\(season.episodes.count) episodes")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct TVEpisodeRow: View {
    let episode: SonderMediaItem
    let progress: SonderProgress
    let artworkURL: URL?
    var accessToken: String = ""

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            PosterPlaceholder(item: episode, artworkURL: artworkURL, accessToken: accessToken)
                .frame(width: 96, height: 64)

            VStack(alignment: .leading, spacing: 6) {
                Text(episodeTitle)
                    .font(.headline)
                    .foregroundStyle(SonderPalette.text)
                    .lineLimit(2)
                Text(episode.subtitle)
                    .font(.caption)
                    .foregroundStyle(SonderPalette.textLight)
                    .lineLimit(2)
                if progress.percent > 0 {
                    ProgressView(value: progress.percent)
                        .tint(SonderPalette.ironGrey)
                }
            }

            Spacer()
            Image(systemName: "chevron.right")
                .font(.caption.weight(.semibold))
                .foregroundStyle(SonderPalette.textLight)
                .padding(.top, 4)
        }
        .sonderPanel()
    }

    private var episodeTitle: String {
        let number = episode.episodeNumber.map { "E\($0)" } ?? episode.episodeCode
        return "\(number) - \(episode.title)"
    }
}
