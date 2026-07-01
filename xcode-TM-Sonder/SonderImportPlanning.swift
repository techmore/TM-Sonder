import Foundation

nonisolated struct SonderManagedImportItem: Hashable, Sendable {
    var item: SonderMediaItem
    var managedURL: URL
}

nonisolated struct SonderManagedImportResult: Hashable, Sendable {
    var importedItems: [SonderManagedImportItem]
    var activities: [SonderLibraryActivityDraft]

    var importedIDs: [UUID] {
        importedItems.map { $0.item.id }
    }

    var importedCount: Int {
        importedItems.count
    }
}

nonisolated struct SonderManagedImportService: Sendable {
    var store: SonderStore

    func importFiles(
        _ result: Result<[URL], Error>,
        storagePath: String,
        storageBookmark: Data?
    ) -> SonderManagedImportResult {
        guard case .success(let urls) = result else {
            return SonderManagedImportResult(
                importedItems: [],
                activities: [SonderLibraryActivityDraft(
                    title: "Import failed",
                    detail: "The selected files could not be read.",
                    icon: "exclamationmark.triangle"
                )]
            )
        }

        var importedItems: [SonderManagedImportItem] = []
        var activities: [SonderLibraryActivityDraft] = []
        for url in urls {
            do {
                let managedURL = try store.copyIntoManagedStorage(url, storagePath: storagePath, storageBookmark: storageBookmark)
                let parsed = SonderMediaParser.parse(url: managedURL)
                let item = SonderManagedImportFactory.makeItem(for: managedURL, parsed: parsed)
                importedItems.append(SonderManagedImportItem(item: item, managedURL: managedURL))
            } catch {
                activities.append(SonderLibraryActivityDraft(
                    title: "Import failed",
                    detail: "\(url.lastPathComponent): \(error.localizedDescription)",
                    icon: "exclamationmark.triangle"
                ))
            }
        }

        if importedItems.isEmpty == false {
            activities.append(SonderLibraryActivityDraft(
                title: "Imported media",
                detail: "\(importedItems.count) item(s) copied into managed storage.",
                icon: "square.and.arrow.down"
            ))
        }

        return SonderManagedImportResult(importedItems: importedItems, activities: activities)
    }
}

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
            libraryID: parsed.kind.importKind?.defaultLibraryID,
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
        case .all, .movie, .documentary, .tvShow:
            return "Imported video copied into Sonder's managed library storage."
        }
    }
}
