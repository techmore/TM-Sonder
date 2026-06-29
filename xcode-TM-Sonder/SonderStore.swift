import AVFoundation
import Foundation

final class SonderStore: @unchecked Sendable {
    let rootURL: URL

    init(rootURL: URL? = nil) {
        if let rootURL {
            self.rootURL = rootURL
        } else {
            let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory
            self.rootURL = support.appendingPathComponent("TM Sonder", isDirectory: true)
        }
    }

    nonisolated var databaseURL: URL {
        rootURL.appendingPathComponent("library.json")
    }

    nonisolated var uploadsURL: URL {
        rootURL.appendingPathComponent("Media", isDirectory: true)
    }

    nonisolated func load() -> SonderSnapshot {
        do {
            let data = try Data(contentsOf: databaseURL)
            return Self.sanitize(try JSONDecoder.sonder.decode(SonderSnapshot.self, from: data))
        } catch {
            return .seeded
        }
    }

    func save(_ snapshot: SonderSnapshot) {
        do {
            try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
            let data = try JSONEncoder.sonder.encode(snapshot)
            try data.write(to: databaseURL, options: .atomic)
        } catch {
            assertionFailure("Could not save Sonder library: \(error)")
        }
    }

    func storageURL(from bookmark: Data?) -> URL? {
        guard let bookmark else { return nil }
        var isStale = false
        return try? URL(resolvingBookmarkData: bookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale)
    }

    func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    func copyIntoManagedStorage(_ sourceURL: URL, storagePath: String, storageBookmark: Data?) throws -> URL {
        let mediaURL = storageURL(from: storageBookmark) ?? URL(fileURLWithPath: storagePath, isDirectory: true)
        let didAccessMedia = mediaURL.startAccessingSecurityScopedResource()
        defer {
            if didAccessMedia {
                mediaURL.stopAccessingSecurityScopedResource()
            }
        }
        try FileManager.default.createDirectory(at: mediaURL, withIntermediateDirectories: true)
        let destination = uniqueDestination(for: sourceURL.lastPathComponent, in: mediaURL)
        let didAccessSource = sourceURL.startAccessingSecurityScopedResource()
        if didAccessSource {
            defer { sourceURL.stopAccessingSecurityScopedResource() }
            try FileManager.default.copyItem(at: sourceURL, to: destination)
        } else {
            try FileManager.default.copyItem(at: sourceURL, to: destination)
        }
        return destination
    }

    nonisolated func rootMaterials(in directoryPath: String, bookmark: Data?, kind: SonderLibraryImportKind) -> [SonderRootMaterial] {
        let scopedURL = Self.storageURL(from: bookmark) ?? URL(fileURLWithPath: directoryPath, isDirectory: true)
        let didAccess = scopedURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                scopedURL.stopAccessingSecurityScopedResource()
            }
        }

        guard let urls = try? FileManager.default.contentsOfDirectory(
            at: scopedURL,
            includingPropertiesForKeys: [.isDirectoryKey, .isRegularFileKey],
            options: [.skipsHiddenFiles]
        ) else { return [] }

        return urls.compactMap { url in
            let values = try? url.resourceValues(forKeys: [.isDirectoryKey, .isRegularFileKey])
            if values?.isDirectory == true {
                return SonderRootMaterial(url: url, name: url.lastPathComponent.cleanedMediaTitle, kind: kind)
            }
            guard values?.isRegularFile == true, SonderMediaFormat.isSupported(url: url) else { return nil }
            guard kind != .tvShows else { return nil }
            return SonderRootMaterial(url: url, name: url.deletingPathExtension().lastPathComponent.cleanedMediaTitle, kind: kind)
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    nonisolated func mediaFiles(
        in directoryPath: String,
        bookmark: Data?,
        progressStride: Int,
        discoveryBatchSize: Int,
        progress: @escaping @Sendable (_ filesSeenDelta: Int, _ mediaFoundDelta: Int, _ currentPath: String) async -> Void,
        discovered: @escaping @Sendable (_ files: [SonderScannedMediaFile]) async -> Void
    ) async throws -> SonderMediaScanResult {
        let scopedURL = Self.storageURL(from: bookmark) ?? URL(fileURLWithPath: directoryPath, isDirectory: true)
        let didAccess = scopedURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                scopedURL.stopAccessingSecurityScopedResource()
            }
        }

        guard let enumerator = FileManager.default.enumerator(
            at: scopedURL,
            includingPropertiesForKeys: [.isRegularFileKey],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]
        ) else {
            return SonderMediaScanResult(filesSeen: 0, mediaFound: 0, unsupportedMediaCount: 0)
        }

        var discoveryBatch: [SonderScannedMediaFile] = []
        var filesSeen = 0
        var mediaFound = 0
        var unsupportedMediaCount = 0
        var lastProgressUpdate = Date()
        while let url = enumerator.nextObject() as? URL {
            filesSeen += 1
            guard SonderMediaFormat.isSupported(url: url) else {
                if Self.looksLikeUnsupportedVideo(url) {
                    unsupportedMediaCount += 1
                }
                if filesSeen.isMultiple(of: max(progressStride, 1)) || Date().timeIntervalSince(lastProgressUpdate) >= 1 {
                    lastProgressUpdate = Date()
                    await progress(filesSeen, mediaFound, url.path)
                }
                continue
            }
            let values = try? url.resourceValues(forKeys: [.isRegularFileKey])
            if values?.isRegularFile == true {
                mediaFound += 1
                discoveryBatch.append(SonderScannedMediaFile(url: url, bookmark: nil))
                if discoveryBatch.count >= max(discoveryBatchSize, 1) {
                    await discovered(discoveryBatch)
                    discoveryBatch.removeAll(keepingCapacity: true)
                }
            }
            if filesSeen.isMultiple(of: max(progressStride, 1)) || Date().timeIntervalSince(lastProgressUpdate) >= 1 {
                lastProgressUpdate = Date()
                await progress(filesSeen, mediaFound, url.path)
            }
        }
        if discoveryBatch.isEmpty == false {
            await discovered(discoveryBatch)
        }
        if filesSeen > 0 || mediaFound > 0 {
            await progress(filesSeen, mediaFound, scopedURL.path)
        }
        return SonderMediaScanResult(
            filesSeen: filesSeen,
            mediaFound: mediaFound,
            unsupportedMediaCount: unsupportedMediaCount
        )
    }

    nonisolated static func storageURL(from bookmark: Data?) -> URL? {
        guard let bookmark else { return nil }
        var isStale = false
        return try? URL(resolvingBookmarkData: bookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale)
    }

    nonisolated static func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    nonisolated private static func looksLikeUnsupportedVideo(_ url: URL) -> Bool {
        ["flv", "wmv", "divx", "vob", "ogm", "rm", "rmvb"].contains(url.pathExtension.lowercased())
    }

    nonisolated static func sanitize(_ snapshot: SonderSnapshot) -> SonderSnapshot {
        var sanitized = snapshot
        var seenItemIDs = Set<UUID>()
        sanitized.items = sanitized.items.filter { item in
            seenItemIDs.insert(item.id).inserted
        }
        let itemIDs = Set(sanitized.items.map(\.id))
        sanitized.progress.removeAll { itemIDs.contains($0.itemID) == false }
        sanitized.collections = sanitized.collections.map { collection in
            var copy = collection
            var seenCollectionItemIDs = Set<UUID>()
            copy.itemIDs = copy.itemIDs.filter { itemID in
                itemIDs.contains(itemID) && seenCollectionItemIDs.insert(itemID).inserted
            }
            return copy
        }
        return sanitized
    }

    func moveAvoidingCollision(from sourceURL: URL, to destinationURL: URL) throws -> URL {
        let destination = uniqueDestination(for: destinationURL.lastPathComponent, in: destinationURL.deletingLastPathComponent())
        if sourceURL.path == destination.path {
            return sourceURL
        }
        try FileManager.default.moveItem(at: sourceURL, to: destination)
        return destination
    }

    func convertToMP4(sourceURL: URL, outputURL: URL) async -> Bool {
        let didAccess = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                sourceURL.stopAccessingSecurityScopedResource()
            }
        }
        let asset = AVURLAsset(url: sourceURL)
        guard let session = AVAssetExportSession(asset: asset, presetName: AVAssetExportPresetHighestQuality),
              session.supportedFileTypes.contains(.mp4) else {
            return false
        }
        do {
            session.outputURL = outputURL
            session.outputFileType = .mp4
            try await session.export(to: outputURL, as: .mp4)
            return FileManager.default.fileExists(atPath: outputURL.path)
        } catch {
            return false
        }
    }

    func availableConversionURL(for sourceURL: URL) -> URL {
        uniqueDestination(
            for: sourceURL.deletingPathExtension().appendingPathExtension("mp4").lastPathComponent,
            in: sourceURL.deletingLastPathComponent()
        )
    }

    private func uniqueDestination(for fileName: String, in folderURL: URL) -> URL {
        let base = URL(fileURLWithPath: fileName).deletingPathExtension().lastPathComponent
        let ext = URL(fileURLWithPath: fileName).pathExtension
        var candidate = folderURL.appendingPathComponent(fileName)
        var counter = 2
        while FileManager.default.fileExists(atPath: candidate.path) {
            let nextName = ext.isEmpty ? "\(base)-\(counter)" : "\(base)-\(counter).\(ext)"
            candidate = folderURL.appendingPathComponent(nextName)
            counter += 1
        }
        return candidate
    }
}
