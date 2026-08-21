import AVFoundation
import AppKit
import PDFKit

/// Validates book/audiobook containers and extracts artwork that travels with
/// the file.  This is deliberately local-first: no metadata service is needed
/// to decide whether a file is genuine or to use its embedded cover.
nonisolated enum SonderBookInspector {
    struct Result: Sendable {
        var isValid: Bool
        var detail: String
        var title: String?
        var author: String?
        var coverData: Data?
    }

    static func inspect(url: URL, format: SonderMediaFormat) async -> Result? {
        switch format {
        case .pdf:
            return await inspectPDF(url: url)
        case .epub:
            return inspectEPUB(url: url)
        case .m4b, .mp3, .m4a:
            return await inspectAudio(url: url)
        default:
            return nil
        }
    }

    static func cachedCoverPath(for itemID: UUID, data: Data) -> String? {
        let root = (FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory)
            .appendingPathComponent("TM Sonder", isDirectory: true)
            .appendingPathComponent("EmbeddedArtwork", isDirectory: true)
        let url = root.appendingPathComponent("\(itemID.uuidString).jpg")
        do {
            try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
            try data.write(to: url, options: .atomic)
            return url.path
        } catch { return nil }
    }

    @MainActor
    private static func inspectPDF(url: URL) -> Result {
        guard let document = PDFDocument(url: url), document.pageCount > 0 else {
            return Result(isValid: false, detail: "Unreadable PDF", title: nil, author: nil, coverData: nil)
        }
        let attributes = document.documentAttributes
        let title = attributes?[PDFDocumentAttribute.titleAttribute] as? String
        let author = attributes?[PDFDocumentAttribute.authorAttribute] as? String
        let cover = document.page(at: 0)?.thumbnail(of: NSSize(width: 600, height: 900), for: .mediaBox).jpegData(compressionQuality: 0.88)
        return Result(isValid: true, detail: "Verified PDF (\(document.pageCount) pages)", title: title, author: author, coverData: cover)
    }

    private static func inspectEPUB(url: URL) -> Result {
        guard let handle = try? FileHandle(forReadingFrom: url) else {
            return Result(isValid: false, detail: "Unreadable EPUB", title: nil, author: nil, coverData: nil)
        }
        defer { try? handle.close() }
        let prefix = (try? handle.read(upToCount: 4)) ?? Data()
        guard prefix == Data([0x50, 0x4b, 0x03, 0x04]) else {
            return Result(isValid: false, detail: "EPUB is not a ZIP container", title: nil, author: nil, coverData: nil)
        }
        return Result(isValid: true, detail: "Verified EPUB container", title: nil, author: nil, coverData: nil)
    }

    private static func inspectAudio(url: URL) async -> Result {
        let asset = AVURLAsset(url: url)
        do {
            let playable = try await asset.load(.isPlayable)
            guard playable else { return Result(isValid: false, detail: "Audio container is not playable", title: nil, author: nil, coverData: nil) }
            let metadata = try await asset.load(.commonMetadata)
            let title = try await metadata.first(where: { $0.commonKey == .commonKeyTitle })?.load(.stringValue)
            let author = try await metadata.first(where: { $0.commonKey == .commonKeyArtist })?.load(.stringValue)
            let artwork = try await metadata.first(where: { $0.commonKey == .commonKeyArtwork })?.load(.dataValue)
            return Result(isValid: true, detail: "Verified audio container", title: title, author: author, coverData: artwork)
        } catch {
            return Result(isValid: false, detail: "Unreadable audio container", title: nil, author: nil, coverData: nil)
        }
    }
}

private extension NSImage {
    func jpegData(compressionQuality: CGFloat) -> Data? {
        guard let tiffRepresentation,
              let bitmap = NSBitmapImageRep(data: tiffRepresentation) else { return nil }
        return bitmap.representation(using: .jpeg, properties: [.compressionFactor: compressionQuality])
    }
}
