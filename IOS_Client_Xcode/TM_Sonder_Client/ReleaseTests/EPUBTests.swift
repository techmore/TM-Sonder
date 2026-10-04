import XCTest
import ZIPFoundation
@testable import OfflineCore

final class EPUBTests: XCTestCase {
    func testSpineOrderAndLocalResources() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let archive = try Archive(url: root.appendingPathComponent("book.epub"), accessMode: .create)
        let files = [
            "META-INF/container.xml": "<container><rootfiles><rootfile full-path='OEBPS/book.opf'/></rootfiles></container>",
            "OEBPS/book.opf": "<package><manifest><item id='a' href='a.xhtml' media-type='application/xhtml+xml'/><item id='b' href='b.xhtml' media-type='application/xhtml+xml'/></manifest><spine><itemref idref='b'/><itemref idref='a'/></spine></package>",
            "OEBPS/a.xhtml": "<html><body>First file</body></html>",
            "OEBPS/b.xhtml": "<html><body>First in reading order</body></html>"
        ]
        for (path, value) in files {
            let data = Data(value.utf8)
            try archive.addEntry(with: path, type: .file, uncompressedSize: Int64(data.count)) { position, size in
                data.subdata(in: Int(position)..<Int(position)+size)
            }
        }
        let book = try SonderEPUB.open(root.appendingPathComponent("book.epub"))
        defer { try? FileManager.default.removeItem(at: book.root) }
        XCTAssertEqual(book.chapters.map(\.lastPathComponent), ["b.xhtml", "a.xhtml"])
        XCTAssertTrue(try String(contentsOf: book.chapters[0], encoding: .utf8).contains("First in reading order"))
    }

    func testRejectsEscapingAndRemoteResourcePaths() throws {
        let root = URL(fileURLWithPath: "/tmp/book")
        for path in ["../outside", "/etc/passwd", "%2e%2e/outside", "https://example.com/book", "a\\b"] {
            XCTAssertThrowsError(try SonderEPUB.containedURL(path, in: root), path)
        }
        XCTAssertEqual(try SonderEPUB.containedURL("Text/chapter.xhtml", in: root).path, "/tmp/book/Text/chapter.xhtml")
    }
}
