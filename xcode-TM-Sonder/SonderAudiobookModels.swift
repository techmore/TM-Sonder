import Foundation

nonisolated struct SonderAudiobookChapterRecord: Codable, Hashable, Sendable {
    var index: Int
    var title: String
    var startSeconds: Double
    var endSeconds: Double?
}

nonisolated struct SonderAudiobookChapterFile: Codable, Hashable, Sendable {
    var chapters: [SonderAudiobookChapterRecord]
}
