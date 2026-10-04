import Foundation
import SonderAPI

/// Persistent, user-managed media downloads. These intentionally live in Application
/// Support rather than URLCache/Caches so iOS does not treat explicitly saved offline
/// titles as disposable network data.
nonisolated struct SonderOfflineDownload: Codable, Hashable, Identifiable, Sendable {
    var itemID: UUID
    var title: String
    var fileName: String
    var format: SonderMediaFormat
    var downloadedAt: Date
    var byteCount: Int64
    var mediaItem: SonderMediaItem? = nil

    var id: UUID { itemID }
}

nonisolated final class SonderOfflineDownloadStore: @unchecked Sendable {
    private let fileManager: FileManager
    private let rootDirectory: URL?

    init(fileManager: FileManager = .default, rootDirectory: URL? = nil) {
        self.fileManager = fileManager
        self.rootDirectory = rootDirectory
    }

    func downloads(for serverURL: URL) -> [UUID: SonderOfflineDownload] {
        let manifestURL = manifestURL(for: serverURL)
        guard let data = try? Data(contentsOf: manifestURL),
              let manifest = try? JSONDecoder.sonder.decode([SonderOfflineDownload].self, from: data) else {
            return [:]
        }
        return Dictionary(
            manifest.compactMap { download in
                fileManager.fileExists(atPath: mediaURL(for: download, serverURL: serverURL).path) ? (download.itemID, download) : nil
            },
            uniquingKeysWith: { _, latest in latest }
        )
    }

    func mediaURL(for download: SonderOfflineDownload, serverURL: URL) -> URL {
        serverDirectory(for: serverURL).appendingPathComponent(download.fileName)
    }

    func save(
        temporaryURL: URL,
        item: SonderMediaItem,
        serverURL: URL
    ) throws -> SonderOfflineDownload {
        let extensionName = item.format.rawValue.isEmpty ? "media" : item.format.rawValue.lowercased()
        let fileName = "\(item.id.uuidString).\(extensionName)"
        let destination = serverDirectory(for: serverURL).appendingPathComponent(fileName)
        try fileManager.createDirectory(at: destination.deletingLastPathComponent(), withIntermediateDirectories: true)
        // Stage on the destination filesystem before replacing a working copy.
        // A missing/failed incoming transfer must never erase saved media.
        let staged = destination.deletingLastPathComponent().appendingPathComponent(".\(UUID().uuidString).partial")
        try fileManager.moveItem(at: temporaryURL, to: staged)
        defer { try? fileManager.removeItem(at: staged) }
        if fileManager.fileExists(atPath: destination.path) {
            _ = try fileManager.replaceItemAt(destination, withItemAt: staged)
        } else {
            try fileManager.moveItem(at: staged, to: destination)
        }
        let byteCount = (try? destination.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
        return SonderOfflineDownload(
            itemID: item.id,
            title: item.title,
            fileName: fileName,
            format: item.format,
            downloadedAt: Date(),
            byteCount: byteCount,
            mediaItem: item
        )
    }

    func remove(_ download: SonderOfflineDownload, serverURL: URL) throws {
        try fileManager.removeItem(at: mediaURL(for: download, serverURL: serverURL))
    }

    func chapterURL(itemID: UUID, serverURL: URL) -> URL {
        serverDirectory(for: serverURL).appendingPathComponent("\(itemID.uuidString)-chapters.json")
    }

    func posterURL(itemID: UUID, serverURL: URL) -> URL {
        serverDirectory(for: serverURL).appendingPathComponent("\(itemID.uuidString)-poster")
    }

    func saveManifest(_ downloads: [UUID: SonderOfflineDownload], for serverURL: URL) throws {
        let url = manifestURL(for: serverURL)
        try fileManager.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        let ordered = downloads.values.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
        try JSONEncoder.sonder.encode(ordered).write(to: url, options: .atomic)
    }

    func resumeDataURL(itemID: UUID, serverURL: URL) -> URL {
        serverDirectory(for: serverURL).appendingPathComponent("Resume", isDirectory: true)
            .appendingPathComponent("\(itemID.uuidString).resume")
    }

    private func serverDirectory(for serverURL: URL) -> URL {
        let root = rootDirectory ?? fileManager.urls(for: .applicationSupportDirectory, in: .userDomainMask).first ?? fileManager.temporaryDirectory
        let identity = Data(serverURL.absoluteString.utf8).base64EncodedString()
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "=", with: "")
        return root
            .appendingPathComponent("TM-Sonder", isDirectory: true)
            .appendingPathComponent("OfflineMedia", isDirectory: true)
            .appendingPathComponent(identity, isDirectory: true)
    }

    private func manifestURL(for serverURL: URL) -> URL {
        serverDirectory(for: serverURL).appendingPathComponent("manifest.json")
    }
}
