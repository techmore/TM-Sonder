import AVFoundation
import Foundation

final class SonderStore: @unchecked Sendable {
    let rootURL: URL

    nonisolated init(rootURL: URL? = nil) {
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
            NSLog("TM Sonder could not load persisted library: \(error.localizedDescription)")
            return .startupPlaceholder
        }
    }

    nonisolated func save(_ snapshot: SonderSnapshot) {
        do {
            try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
            let data = try JSONEncoder.sonder.encode(snapshot)
            try data.write(to: databaseURL, options: .atomic)
        } catch {
            assertionFailure("Could not save Sonder library: \(error)")
        }
    }

    nonisolated func storageURL(from bookmark: Data?) -> URL? {
        guard let bookmark else { return nil }
        var isStale = false
        return try? URL(resolvingBookmarkData: bookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &isStale)
    }

    nonisolated func bookmark(for url: URL) throws -> Data {
        try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
    }

    nonisolated func copyIntoManagedStorage(_ sourceURL: URL, storagePath: String, storageBookmark: Data?) throws -> URL {
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
                if kind == .movies && Self.isGenericMovieGroupingFolder(url.lastPathComponent) {
                    return nil
                }
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
        discovered: @escaping @Sendable (_ files: [SonderScannedMediaFile], _ filesSeen: Int, _ mediaFound: Int) async -> Void
    ) async throws -> SonderMediaScanResult {
        let scopedURL = Self.storageURL(from: bookmark) ?? URL(fileURLWithPath: directoryPath, isDirectory: true)
        let didAccess = scopedURL.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                scopedURL.stopAccessingSecurityScopedResource()
            }
        }

        var pendingFolders = [scopedURL]
        var discoveryBatch: [SonderScannedMediaFile] = []
        var filesSeen = 0
        var mediaFound = 0
        var unsupportedMediaCount = 0
        var lastProgressUpdate = Date()
        let batchSize = max(discoveryBatchSize, 1)
        let progressEvery = max(progressStride, 1)

        while pendingFolders.isEmpty == false {
            let folder = pendingFolders.removeLast()
            await progress(filesSeen, mediaFound, "Listing folder: \(folder.path)")
            let listStartedAt = Date()
            let children = (try? FileManager.default.contentsOfDirectory(
                at: folder,
                includingPropertiesForKeys: [.isDirectoryKey, .isRegularFileKey, .isPackageKey],
                options: [.skipsHiddenFiles]
            )) ?? []
            let listDuration = Date().timeIntervalSince(listStartedAt)
            if listDuration >= 2 {
                NSLog("TM Sonder scan slow folder listing: %.2fs for %d entries at %@", listDuration, children.count, folder.path)
            }
            await progress(filesSeen, mediaFound, "Scanning folder: \(folder.path) (\(children.count) entries)")

            for url in children.sorted(by: { $0.lastPathComponent.localizedStandardCompare($1.lastPathComponent) == .orderedDescending }) {
                filesSeen += 1
                let values = try? url.resourceValues(forKeys: [.isDirectoryKey, .isRegularFileKey, .isPackageKey])
                if values?.isDirectory == true {
                    if values?.isPackage != true {
                        pendingFolders.append(url)
                    }
                } else if values?.isRegularFile == true {
                    if SonderMediaFormat.isSupported(url: url) {
                        mediaFound += 1
                        discoveryBatch.append(SonderScannedMediaFile(url: url, bookmark: try? Self.bookmark(for: url)))
                        if discoveryBatch.count >= batchSize {
                            await discovered(discoveryBatch, filesSeen, mediaFound)
                            discoveryBatch.removeAll(keepingCapacity: true)
                        }
                    } else if Self.looksLikeUnsupportedVideo(url) {
                        unsupportedMediaCount += 1
                    }
                }

                if filesSeen.isMultiple(of: progressEvery) || Date().timeIntervalSince(lastProgressUpdate) >= 1 {
                    lastProgressUpdate = Date()
                    await progress(filesSeen, mediaFound, url.path)
                }
            }
        }

        if discoveryBatch.isEmpty == false {
            await discovered(discoveryBatch, filesSeen, mediaFound)
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

    nonisolated private static func isGenericMovieGroupingFolder(_ name: String) -> Bool {
        let normalized = name.cleanedMediaTitle.lowercased()
        return ["documentaries", "documentary", "docs", "shorts", "extras", "featurettes"].contains(normalized)
    }

    nonisolated static func sanitize(_ snapshot: SonderSnapshot) -> SonderSnapshot {
        var sanitized = snapshot
        sanitized.items.removeAll { item in
            if item.isPlaceholder {
                return true
            }
            let title = item.title.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return Self.demoItemTitles.contains(title)
        }
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
        sanitized.collections.removeAll { collection in
            let title = collection.name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return Self.demoCollectionTitles.contains(title)
        }
        sanitized.conversionJobs?.removeAll { job in
            let title = job.title.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return Self.demoJobTitles.contains(title)
        }
        return sanitized
    }

    private nonisolated static let demoItemTitles: Set<String> = [
        "northstar",
        "the last signal",
        "workshop sessions",
        "archive road",
        "deep workbench",
        "night harbor",
        "the long walk home",
        "foundations of calm"
    ]

    private nonisolated static let demoCollectionTitles: Set<String> = [
        "saturday feature queue",
        "documentary shelf",
        "shows in rotation"
    ]

    private nonisolated static let demoJobTitles: Set<String> = [
        "seeded demo library"
    ]

    nonisolated func moveAvoidingCollision(from sourceURL: URL, to destinationURL: URL) throws -> URL {
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
            NSLog("TM Sonder conversion failed for \(sourceURL.lastPathComponent): \(error.localizedDescription)")
            return false
        }
    }

    nonisolated func availableConversionURL(for sourceURL: URL) -> URL {
        uniqueDestination(
            for: sourceURL.deletingPathExtension().appendingPathExtension("mp4").lastPathComponent,
            in: sourceURL.deletingLastPathComponent()
        )
    }

    private nonisolated func uniqueDestination(for fileName: String, in folderURL: URL) -> URL {
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
