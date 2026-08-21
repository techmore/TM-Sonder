import Foundation
import Testing
@testable import SonderAPI

struct SonderAPITests {
    @Test func publicLibrarySettingsNeverEncodePairingToken() throws {
        let settings = SonderPublicServerSettings(
            isEnabled: true,
            allowLAN: true,
            port: 8797,
            themePreset: "earthy",
            requiresPairing: true,
            pairingToken: "should-not-encode"
        )
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(settings)
        let json = String(decoding: data, as: UTF8.self)
        #expect(json.contains("should-not-encode") == false)
        #expect(json.contains("requiresPairing"))
    }

    @Test func mediaItemRoundTripPreservesArtworkURLs() throws {
        let item = SonderPublicMediaItem(
            title: "Beacon",
            subtitle: "Movie",
            kind: .movie,
            studio: "Local",
            year: 2024,
            durationSeconds: 100,
            format: .mp4,
            summary: "Summary",
            posterURL: "/artwork/poster/\(UUID().uuidString)",
            bookValidation: "verified: Verified PDF (12 pages)",
            coverSource: "embedded"
        )
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let data = try encoder.encode(item)
        let decoded = try decoder.decode(SonderPublicMediaItem.self, from: data)
        #expect(decoded.posterURL == item.posterURL)
        #expect(decoded.kind == .movie)
        #expect(decoded.bookValidation == item.bookValidation)
        #expect(decoded.coverSource == "embedded")
    }

    @Test func routeHelpersBuildStablePaths() {
        let id = UUID(uuidString: "11111111-1111-4111-8111-111111111111")!
        #expect(SonderAPIRoutes.stream(itemID: id) == "/stream/11111111-1111-4111-8111-111111111111")
        #expect(SonderAPIRoutes.playbackTrackRefresh(itemID: id).hasSuffix("/refresh-tracks"))
    }

    @Test func mkvFormatGuidanceMentionsMacConversion() {
        let guidance = SonderMediaFormat.mkv.clientPlaybackGuidance
        #expect(guidance.localizedCaseInsensitiveContains("mkv"))
        #expect(guidance.localizedCaseInsensitiveContains("convert"))
        #expect(SonderMediaFormat.mp4.isPlayableInAVPlayer)
        #expect(SonderMediaFormat.mkv.isPlayableInAVPlayer == false)
        #expect(SonderMediaFormat.webm.isPlayableInAVPlayer == false)
    }

    @Test func documentGuidanceReflectsPDFReaderAvailability() {
        #expect(SonderMediaFormat.pdf.clientPlaybackGuidance.localizedCaseInsensitiveContains("built-in iOS reader"))
        #expect(SonderMediaFormat.epub.clientPlaybackGuidance.localizedCaseInsensitiveContains("still in development"))
    }
}
