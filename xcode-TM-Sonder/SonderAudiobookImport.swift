import CryptoKit
import Foundation

nonisolated struct SonderAudiobookImportStatus: Sendable, Hashable {
    var lastRunAt: Date?
    var importedCount: Int = 0
    var updatedCount: Int = 0
    var unchangedCount: Int = 0
    var skippedCount: Int = 0
    var lastMessage: String = "Not run yet."
    var isRunning: Bool = false
}

nonisolated struct SonderAudiobookImportSummary: Sendable, Hashable {
    var importedCount: Int = 0
    var updatedCount: Int = 0
    var unchangedCount: Int = 0
    var skippedCount: Int = 0
    var message: String = "Audiobook index is current."
}

nonisolated struct SonderAudiobookImportResult: Sendable, Hashable {
    var status: SonderAudiobookImportStatus
    var activity: SonderLibraryActivityDraft
}

nonisolated struct SonderAudiobookImportService: Sendable {
    var store: SonderStore

    func refreshIndex(items: [SonderMediaItem], mediaDirectories: [SonderMediaDirectory]) async -> SonderAudiobookImportResult {
        let importer = SonderAudiobookImporter(store: store)
        let summary = await importer.refreshIndex(items: items, mediaDirectories: mediaDirectories)
        return SonderAudiobookImportResult(
            status: SonderAudiobookImportStatus(
                lastRunAt: Date(),
                importedCount: summary.importedCount,
                updatedCount: summary.updatedCount,
                unchangedCount: summary.unchangedCount,
                skippedCount: summary.skippedCount,
                lastMessage: summary.message,
                isRunning: false
            ),
            activity: SonderLibraryActivityDraft(
                title: "Refreshed audiobook index",
                detail: summary.message,
                icon: "headphones"
            )
        )
    }
}

nonisolated struct SonderAudiobookIndex: Codable, Hashable, Sendable {
    var schemaVersion: Int
    var generatedAt: Date
    var items: [SonderAudiobookIndexEntry]
}

nonisolated struct SonderAudiobookIndexEntry: Codable, Hashable, Sendable, Identifiable {
    var id: UUID
    var itemID: UUID
    var title: String
    var subtitle: String
    var author: String?
    var series: String?
    var narrator: String?
    var studio: String?
    var year: Int?
    var sourcePath: String
    var sourceFingerprint: String
    var artworkPath: String?
    var chapters: [SonderAudiobookChapterRecord]
    var importedAt: Date
    var updatedAt: Date
}

actor SonderAudiobookImporter {
    private let store: SonderStore
    private let fileManager = FileManager.default

    init(store: SonderStore) {
        self.store = store
    }

    func refreshIndex(items: [SonderMediaItem], mediaDirectories: [SonderMediaDirectory]) async -> SonderAudiobookImportSummary {
        let audiobookItems = items.filter { $0.kind == .audiobook && $0.sourcePath != nil }
        let cacheRoot = store.rootURL.appendingPathComponent("AudiobookImport", isDirectory: true)
        try? fileManager.createDirectory(at: cacheRoot, withIntermediateDirectories: true)
        let indexURL = cacheRoot.appendingPathComponent("audiobook-index.json")
        let existingIndex = (try? Data(contentsOf: indexURL)).flatMap { try? JSONDecoder.sonder.decode(SonderAudiobookIndex.self, from: $0) }
        let existingEntries = Dictionary(uniqueKeysWithValues: (existingIndex?.items ?? []).map { ($0.itemID, $0) })

        var entries: [SonderAudiobookIndexEntry] = []
        var summary = SonderAudiobookImportSummary()

        for item in audiobookItems {
            guard let sourcePath = item.sourcePath else {
                summary.skippedCount += 1
                continue
            }
            let sourceURL = URL(fileURLWithPath: sourcePath)
            let fingerprint = fingerprint(for: item, mediaURL: sourceURL)
            let chapters = chapters(for: item, mediaURL: sourceURL)
            let metadata = Self.metadata(for: item, mediaURL: sourceURL)
            let artworkPath = item.localPosterPath ?? item.localBackdropPath
            let entry = SonderAudiobookIndexEntry(
                id: item.id,
                itemID: item.id,
                title: item.title,
                subtitle: item.subtitle,
                author: metadata.author,
                series: metadata.series,
                narrator: metadata.narrator,
                studio: item.studio,
                year: item.year,
                sourcePath: sourcePath,
                sourceFingerprint: fingerprint,
                artworkPath: artworkPath,
                chapters: chapters,
                importedAt: existingEntries[item.id]?.importedAt ?? Date(),
                updatedAt: Date()
            )
            entries.append(entry)
            if let existing = existingEntries[item.id] {
                if existing.sourceFingerprint == fingerprint && existing.chapters == chapters && existing.artworkPath == artworkPath {
                    summary.unchangedCount += 1
                } else {
                    summary.updatedCount += 1
                }
            } else {
                summary.importedCount += 1
            }
        }

        let index = SonderAudiobookIndex(schemaVersion: 1, generatedAt: Date(), items: entries.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending })
        if let data = try? JSONEncoder.sonder.encode(index) {
            try? data.write(to: indexURL, options: .atomic)
        }
        if summary.importedCount == 0, summary.updatedCount == 0, summary.unchangedCount == 0 {
            summary.message = "No audiobook files were found."
        } else if summary.importedCount > 0 {
            summary.message = "Imported \(summary.importedCount) audiobook(s) into Sonder's native index."
        } else if summary.updatedCount > 0 {
            summary.message = "Updated \(summary.updatedCount) audiobook record(s) in Sonder's native index."
        } else {
            summary.message = "Audiobook index already up to date."
        }
        _ = mediaDirectories
        return summary
    }

    private func fingerprint(for item: SonderMediaItem, mediaURL: URL) -> String {
        let sidecar = chapterSidecarFingerprint(for: mediaURL)
        let payload = "\(item.id.uuidString)|\(item.title)|\(item.year)|\(item.durationSeconds)|\(item.sourcePath ?? "")|\(sidecar)"
        return SHA256.hash(data: Data(payload.utf8)).map { String(format: "%02x", $0) }.joined()
    }

    nonisolated static func metadata(for item: SonderMediaItem, mediaURL: URL) -> (author: String?, series: String?, narrator: String?) {
        let folder = mediaURL.deletingLastPathComponent()
        let parent = folder.deletingLastPathComponent().lastPathComponent.cleanedMediaTitle
        let series = folder.lastPathComponent.cleanedMediaTitle
        let author = parent.isEmpty ? nil : parent
        let normalizedSeries = series.isEmpty || series == item.title ? nil : series
        return (author, normalizedSeries, nil)
    }

    private func chapterSidecarFingerprint(for mediaURL: URL) -> String {
        let candidates = chapterSidecarCandidates(for: mediaURL)
        for candidate in candidates {
            if let data = try? Data(contentsOf: candidate) {
                return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
            }
        }
        return ""
    }

    private func chapters(for item: SonderMediaItem, mediaURL: URL) -> [SonderAudiobookChapterRecord] {
        let candidates = chapterSidecarCandidates(for: mediaURL)
        for candidate in candidates {
            guard let data = try? Data(contentsOf: candidate) else { continue }
            if let decoded = try? JSONDecoder.sonder.decode(SonderAudiobookChapterFile.self, from: data) {
                return decoded.chapters
            }
            if let decoded = decodePlainChapterList(data: data) {
                return decoded
            }
        }
        let duration = max(item.durationSeconds, 1)
        return [
            SonderAudiobookChapterRecord(index: 1, title: item.title, startSeconds: 0, endSeconds: duration)
        ]
    }

    private func decodePlainChapterList(data: Data) -> [SonderAudiobookChapterRecord]? {
        guard let text = String(data: data, encoding: .utf8) else { return nil }
        let lines = text.split(whereSeparator: \.isNewline).map(String.init).filter { $0.isEmpty == false }
        guard lines.isEmpty == false else { return nil }
        var chapters: [SonderAudiobookChapterRecord] = []
        for (offset, line) in lines.enumerated() {
            let parts = line.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
            guard parts.count >= 2 else { continue }
            let start = Double(parts[0]) ?? Double(offset * 600)
            let title = parts.dropFirst().joined(separator: "|")
            chapters.append(SonderAudiobookChapterRecord(index: offset + 1, title: title, startSeconds: start, endSeconds: nil))
        }
        return chapters.isEmpty ? nil : chapters
    }

    private func chapterSidecarCandidates(for mediaURL: URL) -> [URL] {
        let folder = mediaURL.deletingLastPathComponent()
        let parent = folder.deletingLastPathComponent()
        let base = mediaURL.deletingPathExtension().lastPathComponent
        return [
            folder.appendingPathComponent("\(base).chapters.json"),
            folder.appendingPathComponent("chapters.json"),
            parent.appendingPathComponent("\(base).chapters.json"),
            parent.appendingPathComponent("chapters.json")
        ]
    }
}
