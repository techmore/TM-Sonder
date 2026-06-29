import CryptoKit
import Foundation

nonisolated struct SonderContextAsset: Codable, Hashable, Sendable {
    var path: String
    var sourceURL: String?
    var kind: String
}

nonisolated struct SonderContextSource: Codable, Hashable, Sendable {
    var provider: String
    var metadataItemID: Int?
    var guid: String?
    var librarySectionID: Int?
    var sourceDBPath: String?
    var sourceBundlePath: String?
}

nonisolated struct SonderContextMedia: Codable, Hashable, Sendable {
    var title: String
    var summary: String?
    var tagline: String?
    var studio: String?
    var year: Int?
    var rating: Double?
    var genres: [String]
}

nonisolated struct SonderContextFileMapping: Codable, Hashable, Sendable {
    var rootFolder: String
    var matchedFiles: [String]
}

nonisolated struct SonderContextTimestamps: Codable, Hashable, Sendable {
    var importedAt: Date
    var updatedAt: Date
    var sourceRefreshedAt: Date?
}

nonisolated struct SonderContextHashes: Codable, Hashable, Sendable {
    var sourceFingerprint: String?
    var contentFingerprint: String?
}

nonisolated struct SonderContext: Codable, Hashable, Sendable {
    var schemaVersion: Int
    var source: SonderContextSource
    var media: SonderContextMedia
    var artwork: [SonderContextAsset]
    var fileMapping: SonderContextFileMapping
    var timestamps: SonderContextTimestamps
    var hashes: SonderContextHashes
}

nonisolated struct PlexMetadataRecord: Hashable, Sendable {
    var metadataItemID: Int
    var librarySectionID: Int?
    var title: String
    var summary: String?
    var tagline: String?
    var studio: String?
    var year: Int?
    var rating: Double?
    var genres: [String]
    var guid: String?
    var thumbURL: String?
    var artURL: String?
    var clearLogoURL: String?
    var squareArtURL: String?
    var mediaFiles: [String]
    var directoryPath: String?
    var refreshedAt: Date?
}

nonisolated struct PlexImportStatus: Sendable, Hashable {
    var lastRunAt: Date?
    var importedCount: Int = 0
    var unchangedCount: Int = 0
    var skippedCount: Int = 0
    var lastMessage: String = "Not run yet."
    var isRunning: Bool = false
    var processedCount: Int = 0
    var totalCount: Int = 0
    var currentTitle: String?

    var progressFraction: Double {
        guard totalCount > 0 else { return isRunning ? 0 : 1 }
        return min(1, Double(processedCount) / Double(totalCount))
    }
}

nonisolated struct PlexImportProgress: Sendable, Hashable {
    var processedCount: Int
    var totalCount: Int
    var currentTitle: String?
    var importedCount: Int
    var unchangedCount: Int
    var skippedCount: Int
}

actor PlexDatabaseReader {
    private let databaseURL: URL
    private let sqliteURL = URL(fileURLWithPath: "/usr/bin/sqlite3")

    init(databaseURL: URL) {
        self.databaseURL = databaseURL
    }

    func discoverShowRecords(limit: Int = 1000) async -> [PlexMetadataRecord] {
        let sql = """
        select
          m.id,
          m.library_section_id,
          coalesce(m.title, ''),
          m.summary,
          m.tagline,
          m.studio,
          m.year,
          m.rating,
          m.tags_genre,
          m.guid,
          m.user_thumb_url,
          m.user_art_url,
          m.user_clear_logo_url,
          m.user_square_art_url,
          coalesce(group_concat(distinct p.file), ''),
          max(d.path),
          m.refreshed_at
        from metadata_items m
        left join media_items mi on mi.metadata_item_id = m.id
        left join media_parts p on p.media_item_id = mi.id and p.deleted_at is null
        left join directories d on d.id = p.directory_id
        where m.metadata_type = 2 and m.deleted_at is null
        group by m.id
        order by m.refreshed_at desc
        limit \(limit);
        """
        guard let output = await runSQL(sql) else { return [] }
        return output.compactMap(Self.parseRecord)
    }

    private func runSQL(_ sql: String) async -> [String]? {
        let process = Process()
        process.executableURL = sqliteURL
        process.arguments = [databaseURL.path, "-separator", "\u{1f}", "-list", sql]
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = Pipe()
        do {
            try process.run()
            process.waitUntilExit()
            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            guard process.terminationStatus == 0 else { return nil }
            let text = String(data: data, encoding: .utf8) ?? ""
            return text.split(separator: "\n", omittingEmptySubsequences: true).map(String.init)
        } catch {
            return nil
        }
    }

    private static func parseRecord(_ line: String) -> PlexMetadataRecord? {
        let parts = line.split(separator: "\u{1f}", omittingEmptySubsequences: false).map(String.init)
        guard parts.count >= 17, let id = Int(parts[0]) else { return nil }
        let genres = parts[8].split(separator: "|").map(String.init).filter { $0.isEmpty == false }
        let files = parts[14].split(separator: ",").map(String.init).filter { $0.isEmpty == false }
        return PlexMetadataRecord(
            metadataItemID: id,
            librarySectionID: Int(parts[1]),
            title: parts[2],
            summary: parts[3].isEmpty ? nil : parts[3],
            tagline: parts[4].isEmpty ? nil : parts[4],
            studio: parts[5].isEmpty ? nil : parts[5],
            year: Int(parts[6]),
            rating: Double(parts[7]),
            genres: genres,
            guid: parts[9].isEmpty ? nil : parts[9],
            thumbURL: parts[10].isEmpty ? nil : parts[10],
            artURL: parts[11].isEmpty ? nil : parts[11],
            clearLogoURL: parts[12].isEmpty ? nil : parts[12],
            squareArtURL: parts[13].isEmpty ? nil : parts[13],
            mediaFiles: files,
            directoryPath: parts[15].isEmpty ? nil : parts[15],
            refreshedAt: Date(timeIntervalSince1970: Double(parts[16]) ?? 0)
        )
    }
}

actor SonderContextWriter {
    private let fileManager = FileManager.default

    func write(_ context: SonderContext, to rootURL: URL, assets: [SonderContextAsset]) async throws -> Bool {
        let cacheURL = rootURL.appendingPathComponent(".sonder", isDirectory: true)
        try fileManager.createDirectory(at: cacheURL, withIntermediateDirectories: true)
        let contextURL = cacheURL.appendingPathComponent("sonder-context.json")
        if let existingData = try? Data(contentsOf: contextURL),
           let existing = try? JSONDecoder.sonder.decode(SonderContext.self, from: existingData),
           existing.hashes == context.hashes {
            return false
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        let data = try encoder.encode(context)
        try data.write(to: contextURL, options: .atomic)
        for asset in assets {
            let source = URL(fileURLWithPath: asset.sourceURL ?? asset.path)
            let destination = cacheURL.appendingPathComponent(asset.path)
            if fileManager.fileExists(atPath: source.path) {
                try? fileManager.copyItem(at: source, to: destination)
            }
        }
        return true
    }
}

nonisolated struct PlexImportSummary: Sendable, Hashable {
    var importedCount: Int = 0
    var unchangedCount: Int = 0
    var skippedCount: Int = 0

    var isEmpty: Bool {
        importedCount == 0 && unchangedCount == 0 && skippedCount == 0
    }
}

actor PlexImporter {
    private let dbURL: URL
    private let bundleRootURL: URL

    init(dbURL: URL, bundleRootURL: URL) {
        self.dbURL = dbURL
        self.bundleRootURL = bundleRootURL
    }

    func importShowContexts(progressHandler: (@Sendable (PlexImportProgress) async -> Void)? = nil) async -> PlexImportSummary {
        let reader = PlexDatabaseReader(databaseURL: dbURL)
        let records = await reader.discoverShowRecords(limit: 500)
        var summary = PlexImportSummary()
        await progressHandler?(PlexImportProgress(
            processedCount: 0,
            totalCount: records.count,
            currentTitle: records.first?.title,
            importedCount: 0,
            unchangedCount: 0,
            skippedCount: 0
        ))
        for (index, record) in records.enumerated() {
            let rootURL: URL
            if let directoryPath = record.directoryPath {
                rootURL = URL(fileURLWithPath: directoryPath, isDirectory: true)
            } else if let firstFile = record.mediaFiles.first {
                rootURL = URL(fileURLWithPath: firstFile).deletingLastPathComponent()
            } else {
                summary.skippedCount += 1
                await progressHandler?(Self.progress(for: summary, record: record, processedCount: index + 1, totalCount: records.count))
                continue
            }
            let assets = localBundleAssets(for: record)
            let context = buildContext(record: record, rootURL: rootURL, assets: assets)
            let didWrite = (try? await SonderContextWriter().write(context, to: rootURL, assets: assets)) ?? false
            if didWrite {
                summary.importedCount += 1
            } else {
                summary.unchangedCount += 1
            }
            await progressHandler?(Self.progress(for: summary, record: record, processedCount: index + 1, totalCount: records.count))
        }
        return summary
    }

    private static func progress(for summary: PlexImportSummary, record: PlexMetadataRecord, processedCount: Int, totalCount: Int) -> PlexImportProgress {
        PlexImportProgress(
            processedCount: processedCount,
            totalCount: totalCount,
            currentTitle: record.title,
            importedCount: summary.importedCount,
            unchangedCount: summary.unchangedCount,
            skippedCount: summary.skippedCount
        )
    }

    private func localBundleAssets(for record: PlexMetadataRecord) -> [SonderContextAsset] {
        var assets: [SonderContextAsset] = []
        if let thumbURL = record.thumbURL, let path = plexMetadataPath(from: thumbURL, preferredFolder: "posters") {
            assets.append(SonderContextAsset(path: "poster\(path.pathExtension.isEmpty ? "" : ".\(path.pathExtension)")", sourceURL: path.path, kind: "poster"))
        }
        if let artURL = record.artURL, let path = plexMetadataPath(from: artURL, preferredFolder: "art") {
            assets.append(SonderContextAsset(path: "background\(path.pathExtension.isEmpty ? "" : ".\(path.pathExtension)")", sourceURL: path.path, kind: "art"))
        }
        if let clearLogoURL = record.clearLogoURL, let path = plexMetadataPath(from: clearLogoURL, preferredFolder: "clearLogos") {
            assets.append(SonderContextAsset(path: "clearlogo\(path.pathExtension.isEmpty ? "" : ".\(path.pathExtension)")", sourceURL: path.path, kind: "clearLogo"))
        }
        if let squareArtURL = record.squareArtURL, let path = plexMetadataPath(from: squareArtURL, preferredFolder: "squareArt") {
            assets.append(SonderContextAsset(path: "squareart\(path.pathExtension.isEmpty ? "" : ".\(path.pathExtension)")", sourceURL: path.path, kind: "squareArt"))
        }
        return assets
    }

    private func plexMetadataPath(from urlString: String, preferredFolder: String) -> URL? {
        let name = URL(string: urlString)?.lastPathComponent ?? URL(fileURLWithPath: urlString).lastPathComponent
        let combinedRoot = bundleRootURL.appendingPathComponent("Contents/_combined", isDirectory: true)
        let preferred = combinedRoot.appendingPathComponent(preferredFolder, isDirectory: true)
        if let match = firstFile(named: name, in: preferred) {
            return match
        }
        if let match = firstFile(named: name, in: combinedRoot) {
            return match
        }
        return nil
    }

    private func firstFile(named name: String, in folder: URL) -> URL? {
        guard let enumerator = FileManager.default.enumerator(at: folder, includingPropertiesForKeys: [.isRegularFileKey], options: [.skipsHiddenFiles]) else {
            return nil
        }
        while let url = enumerator.nextObject() as? URL {
            let values = try? url.resourceValues(forKeys: [.isRegularFileKey])
            if values?.isRegularFile == true && url.lastPathComponent.contains(name) {
                return url
            }
        }
        return nil
    }

    private func buildContext(record: PlexMetadataRecord, rootURL: URL, assets: [SonderContextAsset]) -> SonderContext {
        let assetFingerprint = assets.map { "\($0.kind):\($0.path)" }.sorted().joined(separator: "|")
        let fingerprintData = "\(record.metadataItemID)|\(record.guid ?? "")|\(record.refreshedAt?.timeIntervalSince1970 ?? 0)|\(assetFingerprint)".data(using: .utf8) ?? Data()
        let fingerprint = SHA256.hash(data: fingerprintData).map { String(format: "%02x", $0) }.joined()
        return SonderContext(
            schemaVersion: 1,
            source: SonderContextSource(
                provider: "plex",
                metadataItemID: record.metadataItemID,
                guid: record.guid,
                librarySectionID: record.librarySectionID,
                sourceDBPath: dbURL.path,
                sourceBundlePath: bundleRootURL.path
            ),
            media: SonderContextMedia(
                title: record.title,
                summary: record.summary,
                tagline: record.tagline,
                studio: record.studio,
                year: record.year,
                rating: record.rating,
                genres: record.genres
            ),
            artwork: assets,
            fileMapping: SonderContextFileMapping(rootFolder: rootURL.path, matchedFiles: record.mediaFiles),
            timestamps: SonderContextTimestamps(importedAt: Date(), updatedAt: Date(), sourceRefreshedAt: record.refreshedAt),
            hashes: SonderContextHashes(sourceFingerprint: fingerprint, contentFingerprint: fingerprint)
        )
    }
}
