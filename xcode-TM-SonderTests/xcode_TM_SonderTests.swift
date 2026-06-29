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

    @Test func derivedDataBuildsTagsProgressAndTVGroups() async throws {
        let episodeAID = UUID()
        let episodeBID = UUID()
        var episodeA = makeItem(id: episodeAID, title: "Pilot")
        episodeA.kind = .tvShow
        episodeA.showTitle = "North Pier"
        episodeA.seasonNumber = 1
        episodeA.episodeNumber = 1
        episodeA.tags = ["drama", "coastal"]

        var episodeB = makeItem(id: episodeBID, title: "Harbor Lights")
        episodeB.kind = .tvShow
        episodeB.showTitle = "North Pier"
        episodeB.seasonNumber = 1
        episodeB.episodeNumber = 2
        episodeB.tags = ["drama"]

        let progress = SonderProgress(itemID: episodeBID, seconds: 50, duration: 100)
        let derivedData = SonderDerivedData.make(items: [episodeB, episodeA], progressRecords: [progress])

        #expect(derivedData.allTags == ["coastal", "drama"])
        #expect(derivedData.tagUsage.first?.tag == "drama")
        #expect(derivedData.inProgressItems.map(\.id) == [episodeBID])
        #expect(derivedData.tvShowGroups.first?.name == "North Pier")
        #expect(derivedData.tvShowGroups.first?.seasons.first?.episodes.map(\.title) == ["Pilot", "Harbor Lights"])
    }

    @Test func audiobookPlaybackResolvesChapterEndsAndCurrentChapter() async throws {
        let itemID = UUID()
        let chapters = [
            SonderAudiobookChapterRecord(index: 1, title: "Opening", startSeconds: 0, endSeconds: nil),
            SonderAudiobookChapterRecord(index: 2, title: "Crossing", startSeconds: 120, endSeconds: nil),
            SonderAudiobookChapterRecord(index: 3, title: "Arrival", startSeconds: 360, endSeconds: nil)
        ]

        let resolved = SonderAudiobookPlaybackModel.resolvedChapters(chapters, duration: 600)
        let playback = SonderAudiobookPlaybackModel.playback(
            progress: SonderProgress(itemID: itemID, seconds: 150, duration: 600),
            itemDuration: 600,
            chapters: chapters
        )

        #expect(resolved.map(\.endSeconds) == [120, 360, 600])
        #expect(playback.currentChapterIndex == 2)
        #expect(playback.currentChapterTitle == "Crossing")
        #expect(playback.nextChapterStartSeconds == 360)
        #expect(playback.percent == 0.25)
    }

    @Test func audiobookMetadataDerivesAuthorAndSeriesFromFolderLayout() async throws {
        let item = makeItem(id: UUID(), title: "Beacon")
        var audiobook = item
        audiobook.kind = .audiobook
        audiobook.sourcePath = "/Library/Authors/Robin Sloan/Novel Series/Beacon.m4b"

        let metadata = SonderAudiobookImporter.metadata(
            for: audiobook,
            mediaURL: URL(fileURLWithPath: audiobook.sourcePath!)
        )

        #expect(metadata.author == "Robin Sloan")
        #expect(metadata.series == "Novel Series")
        #expect(metadata.narrator == nil)
    }

    @Test func audiobookImporterFallsBackToSingleChapterWhenNoSidecarExists() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)

        let mediaURL = rootURL.appendingPathComponent("Beacon.m4b")
        try Data([1]).write(to: mediaURL)

        var audiobook = makeItem(id: UUID(), title: "Beacon")
        audiobook.kind = .audiobook
        audiobook.sourcePath = mediaURL.path
        audiobook.durationSeconds = 1800

        let importer = SonderAudiobookImporter(store: SonderStore(rootURL: rootURL))
        let summary = await importer.refreshIndex(items: [audiobook], mediaDirectories: [])

        #expect(summary.importedCount == 1)
        let indexURL = rootURL.appendingPathComponent("AudiobookImport/audiobook-index.json")
        let data = try Data(contentsOf: indexURL)
        let index = try JSONDecoder.sonder.decode(SonderAudiobookIndex.self, from: data)
        #expect(index.items.first?.chapters.count == 1)
        #expect(index.items.first?.chapters.first?.title == "Beacon")
        #expect(index.items.first?.author != nil)
    }

    @Test func mediaLibraryDetectorRecognizesCommonFolderNames() async throws {
        #expect(SonderMediaLibraryDetector.kind(forFolderName: "TV Shows") == .tvShows)
        #expect(SonderMediaLibraryDetector.kind(forFolderName: "Movies") == .movies)
        #expect(SonderMediaLibraryDetector.kind(forFolderName: "Audio Books") == .audiobooks)
        #expect(SonderMediaLibraryDetector.kind(forFolderName: "Books") == .ebooks)
        #expect(SonderMediaLibraryDetector.kind(forFolderName: "Downloads") == nil)
    }

    @Test func mediaLibraryDirectoryScannerFindsRecognizedFoldersInPriorityOrder() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)

        for folder in ["Books", "TV Shows", "Downloads", "Movies", "Audio Books"] {
            try FileManager.default.createDirectory(at: rootURL.appendingPathComponent(folder, isDirectory: true), withIntermediateDirectories: true)
        }
        try Data().write(to: rootURL.appendingPathComponent("Movie.txt"))

        let detected = SonderMediaLibraryDirectoryScanner.detect(in: rootURL)

        #expect(detected.map(\.kind) == [.tvShows, .movies, .ebooks, .audiobooks])
        #expect(detected.map { $0.url.lastPathComponent } == ["TV Shows", "Movies", "Books", "Audio Books"])
    }

    @Test func scanProgressFactoryBuildsStableProgressSnapshots() async throws {
        let directory = SonderMediaDirectory(
            name: "TV Shows",
            path: "/Media/TV Shows",
            bookmark: Data(),
            kind: .tvShows,
            libraryID: SonderLibraryImportKind.tvShows.defaultLibraryID
        )

        let preparing = SonderScanProgressFactory.preparing(directoriesTotal: 4)
        let indexing = SonderScanProgressFactory.indexing(
            directory: directory,
            detail: "/Media/TV Shows/North Pier",
            filesSeen: 20,
            mediaFound: 6,
            indexedCount: 3,
            directoriesDone: 1,
            directoriesTotal: 4
        )
        let discovered = SonderScanProgressFactory.discoveredRootTitles(
            count: 2,
            directory: directory,
            filesSeen: 0,
            mediaFound: 0,
            indexedCount: 2,
            directoriesDone: 0,
            directoriesTotal: 4
        )
        let localAssets = SonderScanProgressFactory.localAssets(directoriesTotal: 0)

        #expect(preparing.phase == .fastTitles)
        #expect(preparing.title == "Preparing scan")
        #expect(indexing.title == "Indexing TV Shows")
        #expect(indexing.detail == "/Media/TV Shows/North Pier")
        #expect(indexing.filesSeen == 20)
        #expect(indexing.mediaFound == 6)
        #expect(indexing.indexedCount == 3)
        #expect(discovered.detail == "Discovered 2 root title(s). Deep scan continues.")
        #expect(localAssets.phase == .localAssets)
        #expect(localAssets.directoriesTotal == 1)
    }

    @Test func managedImportFactoryBuildsItemDefaultsFromParsedMedia() async throws {
        let url = URL(fileURLWithPath: "/Managed/Books/Beacon.m4b")
        let parsed = SonderParsedMedia(
            title: "Beacon",
            subtitle: "Audiobook - 2022",
            kind: .audiobook,
            year: 2022,
            showTitle: nil,
            season: nil,
            episode: nil,
            metadataIDSource: nil,
            metadataID: nil,
            edition: nil,
            splitPart: nil,
            localPosterPath: nil,
            localBackdropPath: nil
        )

        let item = SonderManagedImportFactory.makeItem(for: url, parsed: parsed, currentYear: 2026)

        #expect(item.title == "Beacon")
        #expect(item.kind == .audiobook)
        #expect(item.year == 2022)
        #expect(item.durationSeconds == 5400)
        #expect(item.format == .m4b)
        #expect(item.tags == ["imported", "m4b"])
        #expect(item.summary == "Imported audiobook copied into Sonder's managed library storage.")
        #expect(item.sourcePath == url.path)
    }

    @Test func libraryQueriesSearchByQueryKindAndTag() async throws {
        var movie = makeItem(id: UUID(), title: "Night Harbor")
        movie.kind = .movie
        movie.tags = ["coastal", "drama"]
        movie.summary = "A quiet mystery by the water."
        var audiobook = makeItem(id: UUID(), title: "Signal Path")
        audiobook.kind = .audiobook
        audiobook.tags = ["audio", "science"]

        #expect(SonderLibraryQueries.search(items: [movie, audiobook], query: "harbor", kind: nil, tag: nil).map(\.id) == [movie.id])
        #expect(SonderLibraryQueries.search(items: [movie, audiobook], query: "", kind: .audiobook, tag: nil).map(\.id) == [audiobook.id])
        #expect(SonderLibraryQueries.search(items: [movie, audiobook], query: "", kind: nil, tag: "coastal").map(\.id) == [movie.id])
        #expect(SonderLibraryQueries.audiobookItems(in: [movie, audiobook]).map(\.id) == [audiobook.id])
        #expect(SonderLibraryQueries.audiobookItem(id: audiobook.id, in: [movie, audiobook])?.id == audiobook.id)
        #expect(SonderLibraryQueries.audiobookItem(id: movie.id, in: [movie, audiobook]) == nil)
    }

    @Test func libraryQueriesPrioritizeAssetTargetsForShowAndSingleTitle() async throws {
        var episodeA = makeItem(id: UUID(), title: "Pilot")
        episodeA.kind = .tvShow
        episodeA.showTitle = "North Pier"
        episodeA.sourcePath = "/Media/North Pier/S01E01.mkv"
        var episodeB = makeItem(id: UUID(), title: "Harbor Lights")
        episodeB.kind = .tvShow
        episodeB.showTitle = "North Pier"
        episodeB.sourcePath = "/Media/North Pier/S01E02.mkv"
        episodeB.localPosterPath = "/posters/north-pier.jpg"
        episodeB.localBackdropPath = "/backdrops/north-pier.jpg"
        episodeB.probedWidth = 1920
        var otherShow = makeItem(id: UUID(), title: "Pilot")
        otherShow.kind = .tvShow
        otherShow.showTitle = "South Pier"
        otherShow.sourcePath = "/Media/South Pier/S01E01.mkv"
        var movie = makeItem(id: UUID(), title: "Night Harbor")
        movie.sourcePath = "/Media/Movies/Night Harbor.mkv"

        #expect(SonderLibraryQueries.priorityAssetTargetIDs(for: episodeA.id, in: [episodeA, episodeB, otherShow], maxItems: 10) == [episodeA.id])
        #expect(SonderLibraryQueries.priorityAssetTargetIDs(for: movie.id, in: [movie], maxItems: 10) == [movie.id])
        movie.localPosterPath = "/posters/night-harbor.jpg"
        movie.localBackdropPath = "/backdrops/night-harbor.jpg"
        movie.probedWidth = 1920
        #expect(SonderLibraryQueries.priorityAssetTargetIDs(for: movie.id, in: [movie], maxItems: 10).isEmpty)
    }

    @Test func conversionPlannerSelectsPlayableNonMP4ItemsAndBuildsJob() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let mkvURL = rootURL.appendingPathComponent("Beacon.mkv")
        let mp4URL = rootURL.appendingPathComponent("Finished.mp4")
        try Data([1]).write(to: mkvURL)
        try Data([1]).write(to: mp4URL)

        var convertible = makeItem(id: UUID(), title: "Beacon")
        convertible.format = .mkv
        convertible.sourcePath = mkvURL.path
        var alreadyMP4 = makeItem(id: UUID(), title: "Finished")
        alreadyMP4.format = .mp4
        alreadyMP4.sourcePath = mp4URL.path
        var missingFile = makeItem(id: UUID(), title: "Missing")
        missingFile.format = .mkv
        missingFile.sourcePath = rootURL.appendingPathComponent("Missing.mkv").path

        let candidates = SonderConversionPlanner.candidates(from: [convertible, alreadyMP4, missingFile])
        let outputURL = rootURL.appendingPathComponent("Beacon.mp4")
        let plan = SonderConversionPlanner.plan(for: convertible, outputURL: outputURL)

        #expect(candidates.map(\.id) == [convertible.id])
        #expect(plan?.itemID == convertible.id)
        #expect(plan?.sourceURL.path == mkvURL.path)
        #expect(plan?.outputURL.path == outputURL.path)
        #expect(plan?.job.title == "Beacon")
        #expect(plan?.job.detail == "Beacon.mkv -> Beacon.mp4")
        #expect(SonderConversionPlanner.plan(for: alreadyMP4, outputURL: outputURL) == nil)
    }

    @Test func progressRecordsClampAndUpdateExistingRecords() async throws {
        let itemID = UUID()
        let otherID = UUID()
        let originalDate = Date(timeIntervalSince1970: 100)
        let updateDate = Date(timeIntervalSince1970: 200)
        let existing = [
            SonderProgress(itemID: itemID, seconds: 10, duration: 100, updatedAt: originalDate),
            SonderProgress(itemID: otherID, seconds: 5, duration: 50, updatedAt: originalDate)
        ]

        let updated = SonderProgressRecords.upserting(
            itemID: itemID,
            seconds: 250,
            duration: 120,
            in: existing,
            now: updateDate
        )
        let inserted = SonderProgressRecords.upserting(
            itemID: UUID(),
            seconds: -10,
            duration: 0,
            in: [],
            now: updateDate
        )

        #expect(updated.records.count == 2)
        #expect(updated.seconds == 120)
        #expect(updated.duration == 120)
        #expect(updated.records.first?.seconds == 120)
        #expect(updated.records.first?.duration == 120)
        #expect(updated.records.first?.updatedAt == updateDate)
        #expect(inserted.records.first?.seconds == 0)
        #expect(inserted.records.first?.duration == 1)
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
