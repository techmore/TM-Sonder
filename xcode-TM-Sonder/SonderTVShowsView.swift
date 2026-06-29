import SwiftUI

struct TVShowsView: View {
    @ObservedObject var library: SonderLibrary
    @Binding var selectedItemID: UUID?
    @State private var displayMode: LibraryDisplayMode = .grid
    @State private var searchText = ""
    @State private var selectedShowName: String?

    private var selectedShow: SonderTVShowGroup? {
        guard let selectedShowName else { return nil }
        return library.tvShowGroups.first { $0.name == selectedShowName }
    }

    private var filteredShows: [SonderTVShowGroup] {
        let trimmed = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return library.tvShowGroups }
        return library.tvShowGroups.compactMap { show in
            if show.name.localizedCaseInsensitiveContains(trimmed) {
                return show
            }
            let seasons = show.seasons.compactMap { season -> SonderTVSeasonGroup? in
                let matchingEpisodes = season.episodes.filter { $0.searchText.localizedCaseInsensitiveContains(trimmed) }
                guard matchingEpisodes.isEmpty == false else { return nil }
                return SonderTVSeasonGroup(seasonNumber: season.seasonNumber, episodes: matchingEpisodes)
            }
            guard seasons.isEmpty == false else { return nil }
            return SonderTVShowGroup(name: show.name, seasons: seasons)
        }
    }

    var body: some View {
        Group {
            if let selectedShow {
                TVShowDetailPage(show: selectedShow, library: library) { item in
                    selectedItemID = item.id
                } back: {
                    selectedShowName = nil
                }
            } else {
                VStack(spacing: 0) {
                    libraryModeBar(title: "TV Shows", count: filteredShows.count, countLabel: "shows", mode: $displayMode, searchText: $searchText, searchPrompt: "Search shows, seasons, or episodes")
                    switch displayMode {
                    case .grid:
                        ScrollView {
                            if filteredShows.isEmpty {
                                EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add a TV Shows library folder to group episodes by show and season." : "No shows match this search.")
                                    .padding(20)
                            } else {
                                LazyVGrid(columns: [GridItem(.adaptive(minimum: 240, maximum: 300), spacing: 16)], spacing: 16) {
                                    ForEach(filteredShows) { show in
                                        TVShowCard(show: show, library: library) {
                                            selectedShowName = show.name
                                        } play: { item in
                                            library.play(item)
                                        }
                                    }
                                }
                                .padding(20)
                            }
                        }
                    case .spreadsheet:
                        List {
                            ForEach(filteredShows) { show in
                                DisclosureGroup {
                                    ForEach(show.seasons) { season in
                                        DisclosureGroup {
                                            ForEach(season.episodes) { item in
                                                MediaListRow(item: item, detail: library.progressLabel(for: item)) {
                                                    selectedItemID = item.id
                                                }
                                            }
                                        } label: {
                                            HStack {
                                                Label(season.label, systemImage: "rectangle.stack")
                                                Spacer()
                                                Text("\(season.episodes.count) episodes")
                                                    .font(.caption.monospacedDigit())
                                                    .foregroundStyle(SonderTheme.textLight)
                                            }
                                        }
                                    }
                                } label: {
                                    HStack {
                                        Label(show.name, systemImage: "tv")
                                        Button {
                                            selectedShowName = show.name
                                        } label: {
                                            Label("Open", systemImage: "arrow.right")
                                        }
                                        .buttonStyle(.borderless)
                                        Spacer()
                                        Text("\(show.seasons.count) seasons")
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(SonderTheme.textLight)
                                        Text("\(show.displayCount) episodes")
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(SonderTheme.textLight)
                                    }
                                }
                            }
                            if filteredShows.isEmpty {
                                EmptyLibraryMessage(text: searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Add a TV Shows library folder to group episodes by show and season." : "No shows match this search.")
                            }
                        }
                        .scrollContentBackground(.hidden)
                    }
                }
            }
        }
        .navigationTitle(selectedShow?.name ?? "TV Shows")
        .background(SonderTheme.background)
    }
}

struct TVShowDetailPage: View {
    let show: SonderTVShowGroup
    @ObservedObject var library: SonderLibrary
    let selectEpisode: (SonderMediaItem) -> Void
    let back: () -> Void

    private var episodes: [SonderMediaItem] {
        show.seasons.flatMap(\.episodes)
    }

    private var realEpisodes: [SonderMediaItem] {
        episodes.filter { $0.isPlaceholder == false }
    }

    private var representativeEpisode: SonderMediaItem? {
        episodes.first { $0.localPosterPath != nil } ?? realEpisodes.first ?? episodes.first
    }

    private var playableCount: Int {
        realEpisodes.filter(\.hasFile).count
    }

    private var totalRuntime: Double {
        realEpisodes.map(\.durationSeconds).filter { $0 > 0 }.reduce(0, +)
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                Button(action: back) {
                    Label("TV Shows", systemImage: "chevron.left")
                }
                .buttonStyle(.borderless)

                HStack(alignment: .top, spacing: 22) {
                    if let representativeEpisode {
                        PosterView(item: representativeEpisode)
                            .frame(width: 180, height: 270)
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .onAppear {
                                library.prioritizeAssets(for: representativeEpisode.id)
                            }
                    } else {
                        RoundedRectangle(cornerRadius: 8)
                            .fill(SonderTheme.surfaceDeep)
                            .frame(width: 180, height: 270)
                            .overlay {
                                Image(systemName: "tv")
                                    .font(.system(size: 46, weight: .semibold))
                                    .foregroundStyle(SonderTheme.accent)
                            }
                    }

                    VStack(alignment: .leading, spacing: 14) {
                        Text(show.name)
                            .font(.largeTitle.weight(.bold))
                            .foregroundStyle(SonderTheme.text)
                            .lineLimit(2)
                        Text(show.displayCount == 0 ? "Scanning show details" : "\(show.seasons.count) season(s), \(show.displayCount) episode(s)")
                            .font(.title3)
                            .foregroundStyle(SonderTheme.textLight)

                        HStack(spacing: 10) {
                            TVShowStatTile(label: "Seasons", value: "\(show.seasons.count)", icon: "rectangle.stack")
                            TVShowStatTile(label: "Episodes", value: "\(show.displayCount)", icon: "play.rectangle")
                            TVShowStatTile(label: "Playable", value: "\(playableCount)", icon: "play.circle")
                            TVShowStatTile(label: "Runtime", value: totalRuntime > 0 ? SonderTime.format(totalRuntime) : "Pending", icon: "clock")
                        }
                    }
                }

                DashboardPanel(title: "Seasons") {
                    VStack(alignment: .leading, spacing: 14) {
                        ForEach(show.seasons) { season in
                            TVSeasonDetailSection(season: season, library: library, selectEpisode: selectEpisode)
                        }
                    }
                }
            }
            .padding(24)
        }
        .background(SonderTheme.background)
    }
}

struct TVShowStatTile: View {
    let label: String
    let value: String
    let icon: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(label, systemImage: icon)
                .font(.caption)
                .foregroundStyle(SonderTheme.textLight)
            Text(value)
                .font(.headline.monospacedDigit())
                .foregroundStyle(SonderTheme.text)
        }
        .frame(minWidth: 116, alignment: .leading)
        .padding(12)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}

struct TVSeasonDetailSection: View {
    let season: SonderTVSeasonGroup
    @ObservedObject var library: SonderLibrary
    let selectEpisode: (SonderMediaItem) -> Void

    private var visibleEpisodes: [SonderMediaItem] {
        let realEpisodes = season.episodes.filter { $0.isPlaceholder == false }
        return realEpisodes.isEmpty ? season.episodes : realEpisodes
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label(season.label, systemImage: "rectangle.stack")
                    .font(.headline)
                Spacer()
                Text("\(visibleEpisodes.count) episode(s)")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(SonderTheme.textLight)
            }

            ForEach(visibleEpisodes) { episode in
                HStack(spacing: 12) {
                    Button {
                        selectEpisode(episode)
                        library.prioritizeAssets(for: episode.id)
                    } label: {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(episode.episodeCode == "Not episodic" ? episode.title : "\(episode.episodeCode) - \(episode.title)")
                                .font(.body.weight(.medium))
                                .lineLimit(1)
                            Text([episode.format.rawValue.uppercased(), library.progressLabel(for: episode)].joined(separator: " - "))
                                .font(.caption)
                                .foregroundStyle(SonderTheme.textLight)
                                .lineLimit(1)
                        }
                    }
                    .buttonStyle(.plain)

                    Spacer()

                    Button {
                        library.play(episode)
                    } label: {
                        Image(systemName: "play.fill")
                    }
                    .buttonStyle(.borderless)
                    .disabled(episode.hasFile == false)
                }
                .padding(.vertical, 7)
                .padding(.horizontal, 10)
                .background(SonderTheme.surfaceDeep, in: RoundedRectangle(cornerRadius: 6))
            }
        }
    }
}

struct TVShowCard: View {
    let show: SonderTVShowGroup
    @ObservedObject var library: SonderLibrary
    let open: () -> Void
    let play: (SonderMediaItem) -> Void

    private var episodes: [SonderMediaItem] {
        show.seasons.flatMap(\.episodes)
    }

    private var representativeEpisode: SonderMediaItem? {
        episodes.first { $0.localPosterPath != nil } ?? episodes.first
    }

    private var seasonCount: Int {
        show.seasons.count
    }

    private var progressLabel: String {
        guard let episode = representativeEpisode else { return "No episodes" }
        return library.progressLabel(for: episode)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button(action: open) {
                if let representativeEpisode {
                    PosterView(item: representativeEpisode)
                        .frame(maxWidth: .infinity)
                        .frame(height: 240)
                } else {
                    Rectangle()
                        .fill(SonderTheme.surfaceDeep)
                        .overlay {
                            Image(systemName: "tv")
                                .font(.system(size: 42, weight: .semibold))
                                .foregroundStyle(SonderTheme.accent)
                        }
                        .frame(maxWidth: .infinity)
                        .frame(height: 240)
                }
            }
            .buttonStyle(.plain)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .contentShape(RoundedRectangle(cornerRadius: 8))

            VStack(alignment: .leading, spacing: 5) {
                Text(show.name)
                    .font(.headline)
                    .lineLimit(2)
                HStack(spacing: 8) {
                    Label("\(seasonCount) seasons", systemImage: "rectangle.stack")
                    Label("\(show.episodeCount) episodes", systemImage: "play.rectangle")
                }
                .font(.caption.monospacedDigit())
                .foregroundStyle(SonderTheme.textLight)
                Text(progressLabel)
                    .font(.caption)
                    .foregroundStyle(SonderTheme.textLight)
                    .lineLimit(1)
            }
            .frame(minHeight: 62, alignment: .top)

            HStack {
                Button(action: open) {
                    Label("Open", systemImage: "rectangle.stack")
                }
                Spacer()
                Button {
                    if let representativeEpisode {
                        play(representativeEpisode)
                    }
                } label: {
                    Image(systemName: "play.fill")
                }
                .buttonStyle(.borderless)
                .disabled(representativeEpisode?.hasFile != true)
            }
        }
        .padding(12)
        .background(SonderTheme.surface, in: RoundedRectangle(cornerRadius: 8))
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(SonderTheme.border))
    }
}
