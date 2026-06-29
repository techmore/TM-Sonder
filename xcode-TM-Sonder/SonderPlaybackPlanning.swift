import Foundation

nonisolated struct SonderConversionPlan: Sendable, Hashable {
    var itemID: UUID
    var sourceURL: URL
    var outputURL: URL
    var job: SonderConversionJob
}

nonisolated enum SonderConversionPlanner {
    static func candidates(from items: [SonderMediaItem]) -> [SonderMediaItem] {
        items.filter { $0.hasFile && $0.format != .mp4 }
    }

    static func plan(for item: SonderMediaItem, outputURL: URL) -> SonderConversionPlan? {
        guard let sourceURL = item.playableURL, item.format != .mp4 else { return nil }
        let job = SonderConversionJob(
            title: item.title,
            detail: "\(sourceURL.lastPathComponent) -> \(outputURL.lastPathComponent)",
            status: .running
        )
        return SonderConversionPlan(itemID: item.id, sourceURL: sourceURL, outputURL: outputURL, job: job)
    }
}
