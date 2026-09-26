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

    // MARK: - Multi-part books

    /// The real wire shape of a 3-file book, copied from /api/library.
    private func partJSON(index: Int, duration: Double, progress: Double = 0) -> String {
        """
        {
          "id": "0000000\(index)-1111-4111-8111-111111111111",
          "title": "Complications: A Surgeon's Notes", "subtitle": "", "kind": "audiobook",
          "studio": "", "year": 2003, "durationSeconds": \(duration), "format": "m4b",
          "libraryID": null, "tags": [], "summary": "", "progressSeconds": \(progress),
          "isPlaceholder": false, "coverAvailable": false, "coverEmbedded": false,
          "author": "Atul Gawande",
          "bookGroupID": "book-e215ba285969d580",
          "bookGroupTitle": "Complications (2003)",
          "bookPartIndex": \(index),
          "bookPartCount": 3
        }
        """
    }

    private func decodeParts() throws -> [SonderPublicMediaItem] {
        let decoder = JSONDecoder()
        return try (1...3).map {
            try decoder.decode(SonderPublicMediaItem.self,
                                from: partJSON(index: $0, duration: 600 * Double($0)).data(using: .utf8)!)
        }
    }

    @Test func mediaItemDecodesBookPartsAndOrderingKeys() throws {
        let json = """
        {
          "id": "00000001-1111-4111-8111-111111111111",
          "title": "2011 - The Martian", "subtitle": "", "kind": "audiobook", "studio": "",
          "year": 2011, "durationSeconds": 100, "format": "m4b", "tags": [], "summary": "",
          "progressSeconds": 0, "isPlaceholder": false,
          "bookGroupID": "book-abc", "bookGroupTitle": "The Martian",
          "bookPartIndex": 1, "bookPartCount": 12,
          "sortTitle": "Martian, The", "seriesName": "Standalone",
          "seriesPosition": "1", "seriesNumber": 1, "publicationYear": 2011
        }
        """.data(using: .utf8)!
        let item = try JSONDecoder().decode(SonderPublicMediaItem.self, from: json)
        #expect(item.bookGroupID == "book-abc")
        #expect(item.bookGroupTitle == "The Martian")
        #expect(item.bookPartIndex == 1)
        #expect(item.bookPartCount == 12)
        #expect(item.sortTitle == "Martian, The")
        #expect(item.seriesName == "Standalone")
        #expect(item.seriesPosition == "1")
        #expect(item.publicationYear == 2011)
        #expect(item.isPartOfMultiFileBook)
    }

    @Test func mediaItemToleratesOmittedZeroBookFields() throws {
        // The wire format drops zero and empty values (omitempty), so a head
        // part with bookPartIndex 0 and a single-file book both arrive with
        // keys missing. A non-optional property would fail the whole decode.
        let json = """
        {
          "id": "00000002-1111-4111-8111-111111111111",
          "title": "Dune", "subtitle": "", "kind": "audiobook", "studio": "",
          "year": 1965, "durationSeconds": 3600, "format": "m4b", "tags": [],
          "summary": "", "progressSeconds": 0, "isPlaceholder": false
        }
        """.data(using: .utf8)!
        let item = try JSONDecoder().decode(SonderPublicMediaItem.self, from: json)
        #expect(item.bookGroupID == nil)
        #expect(item.bookPartIndex == nil)
        #expect(item.bookPartCount == nil)
        #expect(item.isPartOfMultiFileBook == false)
    }

    @Test func aBookIsRepresentedByItsHeadPartWhateverTheArrivalOrder() throws {
        let parts = try decodeParts()
        // Reversed, to prove the head is chosen by index and not by position.
        let head = parts.reversed().first!.bookRepresentative(in: parts)
        #expect(head?.bookPartIndex == 1)
        #expect(head?.durationSeconds == 600)
    }

    @Test func aBooksRuntimeIsTheSumOfItsFiles() throws {
        let parts = try decodeParts()
        // Any part reports 36 minutes, not just its own fragment -- this is the
        // bug that showed a 7.8-hour book as 66 seconds.
        for part in parts {
            #expect(part.bookDuration(in: parts) == 600 + 1200 + 1800)
        }
    }

    @Test func aSingleFileBookIsItsOwnBook() throws {
        let json = partJSON(index: 1, duration: 3600)
            .replacingOccurrences(of: "\"bookGroupID\": \"book-e215ba285969d580\",", with: "\"bookGroupID\": null,")
            .replacingOccurrences(of: "\"bookPartCount\": 3", with: "\"bookPartCount\": null")
        let item = try JSONDecoder().decode(SonderPublicMediaItem.self, from: json.data(using: .utf8)!)
        #expect(item.bookDuration(in: [item]) == 3600)
        #expect(item.bookProgressFraction(in: [item]) == 0)
        #expect(item.isPartOfMultiFileBook == false)
    }

    @Test func aBookIsFinishedOnlyWhenEveryFileIs() throws {
        let decoder = JSONDecoder()
        func part(_ index: Int, progress: Double) throws -> SonderPublicMediaItem {
            try decoder.decode(SonderPublicMediaItem.self,
                                from: partJSON(index: index, duration: 600, progress: progress).data(using: .utf8)!)
        }
        let almost = try [part(1, progress: 600), part(2, progress: 600), part(3, progress: 300)]
        #expect(almost[0].isBookFinished(in: almost) == false)
        let done = try [part(1, progress: 600), part(2, progress: 600), part(3, progress: 600)]
        #expect(done[0].isBookFinished(in: done))
    }

    @Test func bookProgressIsMeasuredOverTheWholeBook() throws {
        let decoder = JSONDecoder()
        let parts = try (1...3).map {
            try decoder.decode(SonderPublicMediaItem.self,
                               from: partJSON(index: $0, duration: 1000, progress: $0 == 1 ? 1000 : 0).data(using: .utf8)!)
        }
        // One of three equal files listened to is a third of the way in. Reading
        // the head's 1000s against the full 3000s gives the same answer here, but
        // the fraction must come from the sum, not from the head's own row.
        #expect(abs(parts[0].bookProgressFraction(in: parts) - 1.0 / 3.0) < 0.0001)
    }

    @Test func twoBooksSharingATitleStayTwoBooks() throws {
        // Grouping must be by bookGroupID, never by title text: a book and its
        // abridgement are two books, and folding them drops one off the shelf.
        let decoder = JSONDecoder()
        let a = try decoder.decode(SonderPublicMediaItem.self,
                                   from: partJSON(index: 1, duration: 100).data(using: .utf8)!)
        let b = try decoder.decode(SonderPublicMediaItem.self,
                                   from: partJSON(index: 1, duration: 200)
                                    .replacingOccurrences(of: "book-e215ba285969d580", with: "book-different")
                                    .data(using: .utf8)!)
        #expect(a.bookGroupID != b.bookGroupID)
        #expect(a.bookRepresentative(in: [a, b])?.id == a.id)
        #expect(b.bookRepresentative(in: [a, b])?.id == b.id)
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
