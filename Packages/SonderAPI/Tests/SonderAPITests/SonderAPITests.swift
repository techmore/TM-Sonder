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

    @Test func mediaItemDecodesGenresAndPeopleAndToleratesOldServers() throws {
        // Current server shape: genres are a distinct field from tags.
        let json = """
        {
          "id": "4D4C8B3A-1C2E-4A6F-9B1D-2E3F4A5B6C7D",
          "title": "The Fifth Season", "subtitle": "", "kind": "audiobook",
          "studio": "", "year": 2015, "durationSeconds": 40000, "format": "m4b",
          "tags": ["audnexus"], "genres": ["Fantasy"], "summary": "",
          "progressSeconds": 0, "isPlaceholder": false,
          "author": "N. K. Jemisin", "narrator": "Robin Miles"
        }
        """.data(using: .utf8)!
        let decoded = try JSONDecoder().decode(SonderPublicMediaItem.self, from: json)
        #expect(decoded.genres == ["Fantasy"])
        #expect(decoded.author == "N. K. Jemisin")
        #expect(decoded.narrator == "Robin Miles")

        // Older servers omit the new keys entirely; decoding must still succeed.
        let legacy = """
        {
          "id": "4D4C8B3A-1C2E-4A6F-9B1D-2E3F4A5B6C7E",
          "title": "Old", "subtitle": "", "kind": "movie", "studio": "",
          "year": 1999, "durationSeconds": 0, "format": "mkv", "tags": ["wikipedia"],
          "summary": "", "progressSeconds": 0, "isPlaceholder": false
        }
        """.data(using: .utf8)!
        let old = try JSONDecoder().decode(SonderPublicMediaItem.self, from: legacy)
        #expect(old.genres.isEmpty)
        #expect(old.author == nil)
        #expect(old.narrator == nil)
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
