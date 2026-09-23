import Foundation

/// One genre filter chip: a normalized selection key, a display label, and how
/// many items (within the other active filters) carry it.
nonisolated struct SonderGenreFacet: Identifiable, Hashable, Sendable {
    let key: String
    let label: String
    let count: Int

    var id: String { key }
}

nonisolated enum SonderLibraryQueries {
    /// Base filter: query, kind, and tag. Genres are applied separately so the
    /// genre facet list can be scoped to the other active filters.
    static func search(items: [SonderMediaItem], query: String, kind: SonderMediaKind?, tag: String?) -> [SonderMediaItem] {
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        return items.filter { item in
            let kindMatch = kind == nil || item.kind == kind
            let tagMatch = tag == nil || item.tags.contains(tag ?? "")
            let queryMatch = trimmed.isEmpty || item.searchText.localizedCaseInsensitiveContains(trimmed)
            return kindMatch && tagMatch && queryMatch
        }
    }

    static func search(items: [SonderMediaItem], query: String, kind: SonderMediaKind?, tag: String?, genres: Set<String>) -> [SonderMediaItem] {
        applyGenres(search(items: items, query: query, kind: kind, tag: tag), genres: genres)
    }

    /// Applies the genre selection (any-of) to an already base-filtered list.
    static func applyGenres(_ items: [SonderMediaItem], genres: Set<String>) -> [SonderMediaItem] {
        guard !genres.isEmpty else { return items }
        return items.filter { item in
            !genres.isDisjoint(with: Set(item.genres.map(normalizeGenre)))
        }
    }

    /// Facets for `items`: grouped case-insensitively, counted once per item,
    /// and ordered most-used-first so the useful chips come first.
    static func genreFacets(in items: [SonderMediaItem]) -> [SonderGenreFacet] {
        var labels: [String: String] = [:]
        var counts: [String: Int] = [:]
        for item in items {
            var seenThisItem = Set<String>()
            for raw in item.genres {
                let label = raw.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !label.isEmpty else { continue }
                let key = normalizeGenre(label)
                guard seenThisItem.insert(key).inserted else { continue }
                if labels[key] == nil { labels[key] = label }
                counts[key, default: 0] += 1
            }
        }
        return counts.keys
            .map { SonderGenreFacet(key: $0, label: labels[$0] ?? $0, count: counts[$0] ?? 0) }
            .sorted { lhs, rhs in
                lhs.count == rhs.count
                    ? lhs.label.localizedCaseInsensitiveCompare(rhs.label) == .orderedAscending
                    : lhs.count > rhs.count
            }
    }

    static func normalizeGenre(_ value: String) -> String {
        value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
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
