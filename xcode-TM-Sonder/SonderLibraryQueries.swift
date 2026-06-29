import Foundation

nonisolated enum SonderLibraryQueries {
    static func search(items: [SonderMediaItem], query: String, kind: SonderMediaKind?, tag: String?) -> [SonderMediaItem] {
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        return items.filter { item in
            let kindMatch = kind == nil || item.kind == kind
            let tagMatch = tag == nil || item.tags.contains(tag ?? "")
            let queryMatch = trimmed.isEmpty || item.searchText.localizedCaseInsensitiveContains(trimmed)
            return kindMatch && tagMatch && queryMatch
        }
    }

    static func item(id: UUID, in items: [SonderMediaItem]) -> SonderMediaItem? {
        items.first { $0.id == id }
    }

    static func audiobookItems(in items: [SonderMediaItem]) -> [SonderMediaItem] {
        items.filter { $0.kind == .audiobook }
    }

    static func audiobookItem(id: UUID, in items: [SonderMediaItem]) -> SonderMediaItem? {
        items.first { $0.id == id && $0.kind == .audiobook }
    }

    static func priorityAssetTargetIDs(
        for itemID: UUID,
        in items: [SonderMediaItem],
        maxItems: Int
    ) -> [UUID] {
        guard let item = item(id: itemID, in: items) else { return [] }
        if item.kind == .tvShow {
            let showTitle = (item.showTitle ?? item.title).cleanedMediaTitle.lowercased()
            return items
                .filter { candidate in
                    candidate.kind == .tvShow &&
                    candidate.sourcePath != nil &&
                    ((candidate.showTitle ?? candidate.title).cleanedMediaTitle.lowercased() == showTitle) &&
                    needsAssetRefresh(candidate)
                }
                .prefix(maxItems)
                .map(\.id)
        }
        guard item.sourcePath != nil else { return [] }
        guard needsAssetRefresh(item) else { return [] }
        return [item.id]
    }

    private static func needsAssetRefresh(_ item: SonderMediaItem) -> Bool {
        item.localPosterPath == nil || item.localBackdropPath == nil || item.probedWidth == nil
    }
}
