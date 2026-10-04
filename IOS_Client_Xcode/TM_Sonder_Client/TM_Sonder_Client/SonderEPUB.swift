import Foundation
import ZIPFoundation

nonisolated struct SonderEPUB: Sendable {
    let root: URL
    let chapters: [URL]

    static func open(_ source: URL) throws -> SonderEPUB {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("epub-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        do {
            let archive = try Archive(url: source, accessMode: .read)
            var total: UInt64 = 0
            var count = 0
            for entry in archive {
                count += 1
                total += UInt64(entry.uncompressedSize)
                guard count <= 10_000, total <= 512 * 1024 * 1024, entry.type != .symlink else { throw EPUBError.unsafeArchive }
                let destination = try containedURL(entry.path, in: root)
                _ = try archive.extract(entry, to: destination)
            }
            guard !FileManager.default.fileExists(atPath: root.appendingPathComponent("META-INF/encryption.xml").path) else { throw EPUBError.encrypted }
            let container = try parseXML(root.appendingPathComponent("META-INF/container.xml"))
            guard let packagePath = container.packagePath else { throw EPUBError.invalidBook }
            let packageURL = try containedURL(packagePath, in: root)
            let package = try parseXML(packageURL)
            let chapters = try package.spine.compactMap { id -> URL? in
                guard let resource = package.manifest[id], ["application/xhtml+xml", "text/html"].contains(resource.type) else { return nil }
                let url = try containedURL(resource.href, in: packageURL.deletingLastPathComponent(), boundary: root)
                guard FileManager.default.fileExists(atPath: url.path) else { throw EPUBError.invalidBook }
                return url
            }
            guard !chapters.isEmpty else { throw EPUBError.invalidBook }
            return SonderEPUB(root: root, chapters: chapters)
        } catch {
            try? FileManager.default.removeItem(at: root)
            throw error
        }
    }

    static func containedURL(_ path: String, in directory: URL, boundary: URL? = nil) throws -> URL {
        guard !path.hasPrefix("/"), !path.contains("\\"), !path.contains(":"), !path.contains("\0") else { throw EPUBError.unsafeArchive }
        let decoded = path.removingPercentEncoding ?? path
        let url = directory.appendingPathComponent(decoded).standardizedFileURL
        let root = (boundary ?? directory).standardizedFileURL.path + "/"
        guard url.path.hasPrefix(root) else { throw EPUBError.unsafeArchive }
        return url
    }

    private static func parseXML(_ url: URL) throws -> EPUBXML {
        let data = try Data(contentsOf: url)
        guard data.count <= 8 * 1024 * 1024 else { throw EPUBError.invalidBook }
        let delegate = EPUBXML()
        let parser = XMLParser(data: data)
        parser.shouldResolveExternalEntities = false
        parser.delegate = delegate
        guard parser.parse() else { throw EPUBError.invalidBook }
        return delegate
    }
}

nonisolated enum EPUBError: LocalizedError {
    case invalidBook, unsafeArchive, encrypted
    var errorDescription: String? {
        switch self {
        case .invalidBook: "This EPUB is incomplete or has no readable sections."
        case .unsafeArchive: "This EPUB contains unsafe paths or exceeds the supported size."
        case .encrypted: "This EPUB contains encrypted resources and cannot be opened by Sonder."
        }
    }
}

nonisolated private final class EPUBXML: NSObject, XMLParserDelegate {
    var packagePath: String?
    var manifest: [String: (href: String, type: String)] = [:]
    var spine: [String] = []
    func parser(_ parser: XMLParser, didStartElement elementName: String, namespaceURI: String?, qualifiedName qName: String?, attributes: [String: String]) {
        let name = elementName.split(separator: ":").last.map(String.init) ?? elementName
        if name == "rootfile", packagePath == nil { packagePath = attributes["full-path"] }
        if name == "item", let id = attributes["id"], let href = attributes["href"], let type = attributes["media-type"] { manifest[id] = (href, type) }
        if name == "itemref", attributes["linear"] != "no", let id = attributes["idref"] { spine.append(id) }
    }
}
