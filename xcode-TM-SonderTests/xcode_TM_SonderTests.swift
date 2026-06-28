import Foundation
import Testing
@testable import xcode_TM_Sonder

struct xcode_TM_SonderTests {
    @Test func parserRecognizesEpisodeFilename() async throws {
        let url = URL(fileURLWithPath: "/Media/Shows/Example Show/Season 02/Example Show - S02E05 - Signal Loss.mkv")
        let parsed = SonderMediaParser.parseTitle(url: url, libraryKind: .tvShows)

        #expect(parsed.kind == .tvShow)
        #expect(parsed.showTitle == "Example Show")
        #expect(parsed.season == 2)
        #expect(parsed.episode == 5)
        #expect(parsed.title == "Signal Loss")
    }

    @Test func parserRecognizesMovieYearAndTags() async throws {
        let url = URL(fileURLWithPath: "/Media/Movies/Night Harbor (2020) {imdb-tt1234567} {edition-directors cut}.mkv")
        let parsed = SonderMediaParser.parseTitle(url: url, libraryKind: .movies)

        #expect(parsed.kind == .movie)
        #expect(parsed.title == "Night Harbor")
        #expect(parsed.year == 2020)
        #expect(parsed.metadataIDSource == "imdb")
        #expect(parsed.metadataID == "tt1234567")
        #expect(parsed.edition == "directors cut")
    }

    @Test func storeSanitizeRemovesDuplicateAndDanglingReferences() async throws {
        let keptID = UUID()
        let duplicate = makeItem(id: keptID, title: "Kept")
        let danglingID = UUID()
        let snapshot = SonderSnapshot(
            items: [duplicate, duplicate, makeItem(id: UUID(), title: "Other")],
            progress: [
                SonderProgress(itemID: keptID, seconds: 10, duration: 100),
                SonderProgress(itemID: danglingID, seconds: 10, duration: 100)
            ],
            collections: [SonderCollection(name: "Queue", itemIDs: [keptID, keptID, danglingID])],
            activity: [],
            storagePath: nil,
            storageBookmark: nil,
            conversionJobs: [],
            libraryDefinitions: nil,
            mediaDirectories: [],
            serverSettings: nil
        )

        let sanitized = SonderStore.sanitize(snapshot)

        #expect(sanitized.items.count == 2)
        #expect(sanitized.progress.map(\.itemID) == [keptID])
        #expect(sanitized.collections.first?.itemIDs == [keptID])
    }

    @Test func byteRangeParsesStandardAndSuffixRanges() async throws {
        let partial = HTTPByteRange(header: "bytes=10-19", fileLength: 100)
        #expect(partial.start == 10)
        #expect(partial.end == 19)
        #expect(partial.length == 10)
        #expect(partial.isPartial)

        let suffix = HTTPByteRange(header: "bytes=-25", fileLength: 100)
        #expect(suffix.start == 75)
        #expect(suffix.end == 99)
        #expect(suffix.length == 25)
        #expect(suffix.isPartial)
    }

    @Test func serverSettingsAuthSnapshotRequiresTokenWhenConfigured() async throws {
        let snapshot = SonderAuthSnapshot(allowLAN: true, pairingToken: "secret")

        #expect(snapshot.isAuthorized(localhost: true, bearer: nil, queryToken: nil))
        #expect(snapshot.isAuthorized(localhost: false, bearer: "secret", queryToken: nil))
        #expect(snapshot.isAuthorized(localhost: false, bearer: nil, queryToken: "secret"))
        #expect(snapshot.isAuthorized(localhost: false, bearer: nil, queryToken: nil) == false)
    }

    private func makeItem(id: UUID, title: String) -> SonderMediaItem {
        SonderMediaItem(
            id: id,
            title: title,
            subtitle: "Movie",
            kind: .movie,
            studio: "Local",
            year: 2024,
            durationSeconds: 100,
            format: .mp4,
            tags: [],
            summary: "Summary"
        )
    }
}
