import Foundation

nonisolated struct SonderProgress: Codable, Identifiable, Hashable {
    var id = UUID()
    var itemID: UUID
    var seconds: Double
    var duration: Double
    var updatedAt = Date()
    var audioTrackID: String?
    var subtitleTrackID: String?
    var subtitlesEnabled: Bool?

    var percent: Double {
        guard duration > 0 else { return 0 }
        return min(max(seconds / duration, 0), 1)
    }
}

nonisolated struct SonderConversionJob: Codable, Identifiable, Hashable {
    var id = UUID()
    var title: String
    var detail: String
    var status: SonderConversionStatus
    var createdAt = Date()
}

nonisolated enum SonderConversionStatus: String, Codable {
    case running
    case completed
    case failed

    var label: String {
        switch self {
        case .running: "Running"
        case .completed: "Done"
        case .failed: "Failed"
        }
    }

    var icon: String {
        switch self {
        case .running: "arrow.triangle.2.circlepath"
        case .completed: "checkmark.circle"
        case .failed: "exclamationmark.triangle"
        }
    }
}
