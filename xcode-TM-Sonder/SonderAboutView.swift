import SwiftUI

struct AboutSonderView: View {
    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                PageHeader(
                    title: "About TM Sonder",
                    subtitle: "A private, self-hosted media library for local video collections."
                )

                AboutPanel(
                    title: "Native Host",
                    icon: "macwindow",
                    bodyText: "Sonder runs as a macOS app, manages imported media in Application Support, keeps a menu-bar presence, and exposes a browser interface on the local network service."
                )

                AboutPanel(
                    title: "Plex-Like Basics",
                    icon: "play.rectangle.on.rectangle",
                    bodyText: "The app catalogs movies, television, documentaries, books, and audiobooks, tracks watched position, groups titles into collections, and serves playable files through HTTP routes."
                )

                AboutPanel(
                    title: "TV Show Workflow",
                    icon: "tv",
                    bodyText: "TV libraries are documented as Show -> Season -> Episode. Opening a show should expose show metrics, series information, season artwork, season drill-down, and episode playback. Unknown seasons are kept in an Unsorted Season bucket until filenames or folders provide a season number."
                )

                AboutPanel(
                    title: "Server and iOS Contract",
                    icon: "iphone.and.arrow.forward",
                    bodyText: "Client workflows should use the server library JSON, group TV episodes by showTitle and seasonNumber, open /stream/{id} for playback, and POST progress to /api/progress/{id}. Browser-playable files stream directly; catalog-only formats should open in a native player or use a future transcoding route."
                )

                AboutPanel(
                    title: "Transcoding and Cache Policy",
                    icon: "externaldrive.badge.icloud",
                    bodyText: "Direct play is preferred. On-demand conversion, local cache, and pre-cache controls should be explicit user choices with visible storage impact, rather than automatic background work during browsing."
                )

                AboutPanel(
                    title: "Alexandria-Inspired Interface",
                    icon: "rectangle.3.group",
                    bodyText: "The layout mirrors TM Alexandria's three-column macOS structure, dense poster grid, management dashboard, muted olive theme, and focused title detail panes."
                )
            }
            .padding(20)
        }
        .background(SonderTheme.background)
        .navigationTitle("About")
    }
}
