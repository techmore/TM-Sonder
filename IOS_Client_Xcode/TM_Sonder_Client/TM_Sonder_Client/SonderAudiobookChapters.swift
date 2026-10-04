import Foundation

nonisolated struct SonderAudiobookDetailResponse: Decodable, Sendable {
    let chapters: [SonderAudiobookChapter]
}

nonisolated struct SonderAudiobookChapter: Codable, Identifiable, Hashable, Sendable {
    let index: Int
    let title: String
    let startSeconds: Double
    let endSeconds: Double?
    var partID: UUID? = nil
    var partIndex: Int? = nil

    var id: Int { index }

    func end(in bookDuration: Double) -> Double {
        max(endSeconds ?? bookDuration, startSeconds)
    }
}
