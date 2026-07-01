import Foundation

nonisolated struct SonderMetadataRefreshService: Sendable {
    var cacheRoot: URL

    func refresh(items: [SonderMediaItem]) async -> [UUID: SonderMetadataEnrichment] {
        let enricher = SonderMetadataEnricher(cacheRoot: cacheRoot)
        var enrichments: [UUID: SonderMetadataEnrichment] = [:]
        for item in items {
            if let enrichment = await enricher.enrich(item: item) {
                enrichments[item.id] = enrichment
            }
        }
        return enrichments
    }
}
