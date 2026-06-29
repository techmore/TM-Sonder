import Foundation

nonisolated enum SonderManagedImportFactory {
    static func makeItem(for managedURL: URL, parsed: SonderParsedMedia, currentYear: Int = Calendar.current.component(.year, from: Date())) -> SonderMediaItem {
        let format = SonderMediaFormat(url: managedURL)
        return SonderMediaItem(
            title: parsed.title,
            subtitle: parsed.subtitle,
            kind: parsed.kind,
            studio: "Local",
            year: parsed.year ?? currentYear,
            durationSeconds: parsed.kind == .ebook ? 0 : 5400,
            format: format,
            tags: ["imported", format.rawValue.lowercased()],
            summary: summary(for: parsed.kind),
            sourcePath: managedURL.path,
            showTitle: parsed.showTitle,
            seasonNumber: parsed.season,
            episodeNumber: parsed.episode
        )
    }

    private static func summary(for kind: SonderMediaKind) -> String {
        switch kind {
        case .ebook:
            return "Imported book copied into Sonder's managed library storage."
        case .audiobook:
            return "Imported audiobook copied into Sonder's managed library storage."
        case .movie, .documentary, .tvShow:
            return "Imported video copied into Sonder's managed library storage."
        }
    }
}
