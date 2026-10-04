import Foundation
import SonderAPI

/// Durable queue of playback progress updates that failed to reach the server.
/// Last write wins per item so offline scrubbing does not grow unbounded.
struct SonderQueuedProgressUpdate: Codable, Identifiable, Hashable, Sendable {
    var id: UUID { itemID }
    var itemID: UUID
    var seconds: Double
    var duration: Double
    var audioTrackID: String?
    var subtitleTrackID: String?
    var subtitlesEnabled: Bool?
    var updatedAt: Date
    var serverURL: URL? = nil

    init(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil,
        updatedAt: Date = Date(),
        serverURL: URL? = nil
    ) {
        self.itemID = itemID
        self.seconds = seconds
        self.duration = duration
        self.audioTrackID = audioTrackID
        self.subtitleTrackID = subtitleTrackID
        self.subtitlesEnabled = subtitlesEnabled
        self.updatedAt = updatedAt
        self.serverURL = serverURL
    }
}

@MainActor
final class SonderProgressQueue {
    private(set) var pending: [SonderQueuedProgressUpdate] = []
    private let fileURL: URL
    private let maxEntries = 200

    init(fileURL: URL? = nil) {
        if let fileURL {
            self.fileURL = fileURL
        } else {
            let root = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
                ?? FileManager.default.temporaryDirectory
            let dir = root.appendingPathComponent("TM-Sonder", isDirectory: true)
            self.fileURL = dir.appendingPathComponent("PendingProgress.json")
        }
        load()
    }

    var count: Int { pending.count }

    func enqueue(_ update: SonderQueuedProgressUpdate) {
        if let index = pending.firstIndex(where: { $0.itemID == update.itemID && $0.serverURL == update.serverURL }) {
            guard pending[index].updatedAt <= update.updatedAt else { return }
            pending[index] = update
        } else {
            pending.append(update)
        }
        pending.sort { $0.updatedAt < $1.updatedAt }
        if pending.count > maxEntries {
            pending.removeFirst(pending.count - maxEntries)
        }
        persist()
    }

    func pending(for serverURL: URL) -> [SonderQueuedProgressUpdate] {
        pending.filter { $0.serverURL == serverURL }
    }

    /// Adopt pre-scoping records only for the previously configured server.
    /// Unknown legacy records stay preserved, never sent to a newly chosen host.
    func adoptLegacy(for serverURL: URL) {
        for index in pending.indices where pending[index].serverURL == nil { pending[index].serverURL = serverURL }
        persist()
    }

    func acknowledge(_ update: SonderQueuedProgressUpdate) {
        pending.removeAll { $0.itemID == update.itemID && $0.serverURL == update.serverURL && $0.updatedAt <= update.updatedAt }
        persist()
    }

    func clear() {
        pending = []
        persist()
    }

    private func load() {
        guard let data = try? Data(contentsOf: fileURL) else {
            pending = []
            return
        }
        pending = (try? JSONDecoder.sonder.decode([SonderQueuedProgressUpdate].self, from: data)) ?? []
    }

    private func persist() {
        do {
            try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            let data = try JSONEncoder.sonder.encode(pending)
            try data.write(to: fileURL, options: [.atomic])
        } catch {
            // Best-effort; in-memory queue still works for the session.
        }
    }
}
