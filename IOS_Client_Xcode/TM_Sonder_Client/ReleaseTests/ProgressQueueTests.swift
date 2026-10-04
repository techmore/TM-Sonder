import XCTest
import SonderAPI
@testable import OfflineCore

final class ProgressQueueTests: XCTestCase {
    @MainActor func testDurableServerScopesAndStaleAcknowledgement() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let file = root.appendingPathComponent("progress.json")
        let a = URL(string: "https://a.example")!, b = URL(string: "https://b.example")!
        let id = UUID(), date = Date(timeIntervalSince1970: 100)
        let old = SonderQueuedProgressUpdate(itemID: id, seconds: 10, duration: 100, updatedAt: date, serverURL: a)
        let newer = SonderQueuedProgressUpdate(itemID: id, seconds: 20, duration: 100, updatedAt: date.addingTimeInterval(1), serverURL: a)
        let other = SonderQueuedProgressUpdate(itemID: id, seconds: 30, duration: 100, updatedAt: date, serverURL: b)
        let queue = SonderProgressQueue(fileURL: file)
        queue.enqueue(old); queue.enqueue(newer); queue.enqueue(old); queue.enqueue(other)
        queue.acknowledge(old)
        let reopened = SonderProgressQueue(fileURL: file)
        XCTAssertEqual(reopened.pending(for: a), [newer])
        XCTAssertEqual(reopened.pending(for: b), [other])
        reopened.acknowledge(newer)
        XCTAssertTrue(reopened.pending(for: a).isEmpty)
        XCTAssertEqual(reopened.pending(for: b), [other])
    }

    @MainActor func testLegacyRequiresExplicitAdoption() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let queue = SonderProgressQueue(fileURL: root.appendingPathComponent("progress.json"))
        let server = URL(string: "https://previous.example")!
        queue.enqueue(SonderQueuedProgressUpdate(itemID: UUID(), seconds: 5, duration: 10))
        XCTAssertTrue(queue.pending(for: server).isEmpty)
        queue.adoptLegacy(for: server)
        XCTAssertEqual(queue.pending(for: server).count, 1)
    }

    func testPlaybackTimestampEncodesForServerConflictResolution() throws {
        let date = Date(timeIntervalSince1970: 100)
        let update = SonderPlaybackStateUpdate(seconds: 10, duration: 100, updatedAt: date)
        let encoded = try JSONEncoder.sonder.encode(update)
        let decoded = try JSONDecoder.sonder.decode(SonderPlaybackStateUpdate.self, from: encoded)
        XCTAssertEqual(decoded.updatedAt, date)
    }
}
