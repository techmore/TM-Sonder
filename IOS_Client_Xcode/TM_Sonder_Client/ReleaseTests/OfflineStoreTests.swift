import XCTest
import SonderAPI
@testable import OfflineCore

final class OfflineStoreTests: XCTestCase {
    func testOfflineFormatsSurviveStoreRecreationAndStayServerScoped() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let server = URL(string: "http://server-a:8797")!
        let other = URL(string: "http://server-b:8797")!
        let store = SonderOfflineDownloadStore(rootDirectory: root)
        for format in [SonderMediaFormat.pdf, .epub, .m4b] {
            let item = SonderPublicMediaItem(title: "Offline fixture", subtitle: "", kind: format == .m4b ? .audiobook : .ebook, studio: "", year: 2026, durationSeconds: 100, format: format, summary: "")
            let temporary = root.appendingPathComponent(UUID().uuidString)
            let payload = Data("fixture bytes".utf8)
            try payload.write(to: temporary)
            let saved = try store.save(temporaryURL: temporary, item: item, serverURL: server)
            var manifest = store.downloads(for: server)
            manifest[item.id] = saved
            try store.saveManifest(manifest, for: server)
            let reopened = SonderOfflineDownloadStore(rootDirectory: root)
            let record = try XCTUnwrap(reopened.downloads(for: server)[item.id])
            XCTAssertEqual(record.mediaItem?.id, item.id)
            XCTAssertEqual(record.format, format)
            XCTAssertEqual(try Data(contentsOf: reopened.mediaURL(for: record, serverURL: server)), payload)
            XCTAssertFalse(FileManager.default.fileExists(atPath: temporary.path))
            XCTAssertTrue(reopened.downloads(for: other).isEmpty)
            try reopened.remove(record, serverURL: server)
            XCTAssertNil(reopened.downloads(for: server)[item.id])
            XCTAssertThrowsError(try reopened.remove(record, serverURL: server))
        }
    }
    func testFailedReplacementPreservesSavedAudioAndResumeIsServerScoped() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let server = URL(string: "https://sonder-a.example")!
        let other = URL(string: "https://sonder-b.example")!
        let store = SonderOfflineDownloadStore(rootDirectory: root)
        let item = SonderPublicMediaItem(title: "Offline book", subtitle: "", kind: .audiobook, studio: "", year: 2026, durationSeconds: 100, format: .m4b, summary: "")
        let incoming = root.appendingPathComponent("incoming")
        try Data("good audio".utf8).write(to: incoming)
        let saved = try store.save(temporaryURL: incoming, item: item, serverURL: server)
        XCTAssertThrowsError(try store.save(temporaryURL: incoming, item: item, serverURL: server))
        XCTAssertEqual(try Data(contentsOf: store.mediaURL(for: saved, serverURL: server)), Data("good audio".utf8))
        try Data("replacement audio".utf8).write(to: incoming)
        let replacement = try store.save(temporaryURL: incoming, item: item, serverURL: server)
        XCTAssertEqual(try Data(contentsOf: store.mediaURL(for: replacement, serverURL: server)), Data("replacement audio".utf8))
        XCTAssertNotEqual(store.resumeDataURL(itemID: item.id, serverURL: server), store.resumeDataURL(itemID: item.id, serverURL: other))
        XCTAssertEqual(store.resumeDataURL(itemID: item.id, serverURL: server), SonderOfflineDownloadStore(rootDirectory: root).resumeDataURL(itemID: item.id, serverURL: server))
    }

}
