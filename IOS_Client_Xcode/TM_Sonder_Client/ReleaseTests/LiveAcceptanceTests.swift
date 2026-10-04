import XCTest
import SonderAPI
@testable import OfflineCore

final class LiveAcceptanceTests: XCTestCase {
    func testLiveCatalogDecodes() async throws {
        guard let base = ProcessInfo.processInfo.environment["SONDER_TEST_URL"], let url = URL(string: base + "/api/library") else {
            throw XCTSkip("Set SONDER_TEST_URL to exercise a running server")
        }
        let (data, response) = try await URLSession.shared.data(from: url)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        let catalog = try JSONDecoder.sonder.decode(SonderLibraryResponse.self, from: data)
        XCTAssertFalse(catalog.items.isEmpty)
        XCTAssertFalse(catalog.mediaDirectories.isEmpty)
        print("Decoded live catalog: \(catalog.items.count) items")
    }

    func testRealEPUB() throws {
        guard let path = ProcessInfo.processInfo.environment["SONDER_TEST_EPUB"] else {
            throw XCTSkip("Set SONDER_TEST_EPUB to exercise a real EPUB")
        }
        let book = try SonderEPUB.open(URL(fileURLWithPath: path))
        defer { try? FileManager.default.removeItem(at: book.root) }
        XCTAssertFalse(book.chapters.isEmpty)
        for chapter in book.chapters { XCTAssertGreaterThan(try Data(contentsOf: chapter).count, 0) }
        print("Opened real EPUB: \(book.chapters.count) sections")
    }
}
