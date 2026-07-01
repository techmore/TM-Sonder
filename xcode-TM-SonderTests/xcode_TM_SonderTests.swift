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

    @Test func parserRecognizesAudnexusCompatibleAudiobookTags() async throws {
        let url = URL(fileURLWithPath: "/Media/Audiobooks/Andy Weir/Project Hail Mary {audible-B08G9PRS1K}.m4b")
        let parsed = SonderMediaParser.parseTitle(url: url, libraryKind: .audiobooks)

        #expect(parsed.kind == .audiobook)
        #expect(parsed.title == "Project Hail Mary")
        #expect(parsed.metadataIDSource == "audible")
        #expect(parsed.metadataID == "B08G9PRS1K")
    }

    @Test func parserRecognizesAnimeStyleAbsoluteEpisodeNumbers() async throws {
        let releaseGroupURL = URL(fileURLWithPath: "/Media/TV Shows/Escaflowne/[Fansub] Escaflowne - 01 [BD 1080p AAC].mkv")
        let numberedURL = URL(fileURLWithPath: "/Media/TV Shows/Escaflowne/02 - The Girl From Mystic Moon.mkv")
        let seasonFolderURL = URL(fileURLWithPath: "/Media/TV Shows/Escaflowne/Season 01/03 - The Gallant Swordsman.mkv")

        let releaseGroup = SonderMediaParser.parseTitle(url: releaseGroupURL, libraryKind: .tvShows)
        let numbered = SonderMediaParser.parseTitle(url: numberedURL, libraryKind: .tvShows)
        let seasonFolder = SonderMediaParser.parseTitle(url: seasonFolderURL, libraryKind: .tvShows)

        #expect(releaseGroup.kind == .tvShow)
        #expect(releaseGroup.showTitle == "Escaflowne")
        #expect(releaseGroup.season == 1)
        #expect(releaseGroup.episode == 1)
        #expect(releaseGroup.title == "Episode 1")
        #expect(numbered.showTitle == "Escaflowne")
        #expect(numbered.season == 1)
        #expect(numbered.episode == 2)
        #expect(numbered.title == "The Girl From Mystic Moon")
        #expect(seasonFolder.showTitle == "Escaflowne")
        #expect(seasonFolder.season == 1)
        #expect(seasonFolder.episode == 3)
    }

    @Test func sonderContextResolverAppliesPlexShowAndEpisodeContext() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        let showURL = rootURL.appendingPathComponent("Escaflowne", isDirectory: true)
        let seasonURL = showURL.appendingPathComponent("Season 01", isDirectory: true)
        let cacheURL = showURL.appendingPathComponent(".sonder", isDirectory: true)
        try FileManager.default.createDirectory(at: seasonURL, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: cacheURL, withIntermediateDirectories: true)
        let mediaURL = seasonURL.appendingPathComponent("Escaflowne - S01E01.mkv")
        let posterURL = cacheURL.appendingPathComponent("poster.jpg")
        let backdropURL = cacheURL.appendingPathComponent("background.jpg")
        try Data([1]).write(to: mediaURL)
        try Data([2]).write(to: posterURL)
        try Data([3]).write(to: backdropURL)

        let context = SonderContext(
            schemaVersion: 2,
            source: SonderContextSource(provider: "plex", metadataItemID: 10, guid: "plex://show/10", librarySectionID: 1, sourceDBPath: nil, sourceBundlePath: nil),
            media: SonderContextMedia(title: "Escaflowne", summary: "A transported student meets a young king.", tagline: nil, studio: "Sunrise", year: 1996, rating: 8.2, genres: ["Anime", "Adventure"]),
            artwork: [
                SonderContextAsset(path: "poster.jpg", sourceURL: nil, kind: "poster"),
                SonderContextAsset(path: "background.jpg", sourceURL: nil, kind: "art")
            ],
            episodes: [
                SonderContextEpisode(metadataItemID: 101, title: "Fateful Confession", summary: "Hitomi sees a vision of battle.", seasonNumber: 1, episodeNumber: 1, guid: "plex://episode/101", rating: 7.8, mediaFiles: [mediaURL.path])
            ],
            fileMapping: SonderContextFileMapping(rootFolder: showURL.path, matchedFiles: [mediaURL.path]),
            timestamps: SonderContextTimestamps(importedAt: Date(), updatedAt: Date(), sourceRefreshedAt: Date()),
            hashes: SonderContextHashes(sourceFingerprint: "abc", contentFingerprint: "abc")
        )
        try JSONEncoder.sonder.encode(context).write(to: cacheURL.appendingPathComponent("sonder-context.json"), options: .atomic)

        let target = SonderAssetRefreshTarget(
            id: UUID(),
            url: mediaURL,
            kind: .tvShow,
            title: "Episode 1",
            showTitle: "Escaflowne",
            seasonNumber: 1,
            episodeNumber: 1
        )
        let update = SonderContextResolver.metadataUpdate(for: target)

        #expect(update?.title == "Fateful Confession")
        #expect(update?.showTitle == "Escaflowne")
        #expect(update?.summary == "Hitomi sees a vision of battle.")
        #expect(update?.studio == "Sunrise")
        #expect(update?.year == 1996)
        #expect(update?.posterPath == posterURL.path)
        #expect(update?.backdropPath == backdropURL.path)
        #expect(update?.metadataID == "101")
        #expect(update?.tags.contains("anime") == true)
        #expect(update?.tags.contains("plex") == true)
    }

    @Test func snapshotFactoryCapturesPersistedLibraryState() async throws {
        let item = makeItem(id: UUID(), title: "Beacon")
        let progress = SonderProgress(itemID: item.id, seconds: 10, duration: 100)
        let collection = SonderCollection(name: "Queue", itemIDs: [item.id])
        let activity = SonderActivityEvent(title: "Updated", detail: "Detail", icon: "checkmark")
        let job = SonderConversionJob(title: "Beacon", detail: "Convert", status: .running)
        let directory = SonderMediaDirectory(
            name: "Movies",
            path: "/Media/Movies",
            bookmark: Data([1]),
            kind: .movies,
            libraryID: SonderLibraryImportKind.movies.defaultLibraryID
        )
        let snapshot = SonderSnapshot.libraryState(
            items: [item],
            progressRecords: [progress],
            collections: [collection],
            activity: [activity],
            storagePath: "/Media",
            storageBookmark: Data([2]),
            conversionJobs: [job],
            libraryDefinitions: SonderLibraryDefinition.defaults,
            mediaDirectories: [directory],
            serverSettings: .default
        )

        #expect(snapshot.items.map(\.id) == [item.id])
        #expect(snapshot.progress.map(\.itemID) == [item.id])
        #expect(snapshot.collections.first?.itemIDs == [item.id])
        #expect(snapshot.activity.first?.title == "Updated")
        #expect(snapshot.storagePath == "/Media")
        #expect(snapshot.storageBookmark == Data([2]))
        #expect(snapshot.conversionJobs?.first?.title == "Beacon")
        #expect(snapshot.mediaDirectories?.first?.path == "/Media/Movies")
        #expect(snapshot.serverSettings == .default)
    }

    @Test func storeSanitizeRemovesDuplicateAndDanglingReferences() async throws {
        let keptID = UUID()
        let duplicate = makeItem(id: keptID, title: "Kept")
        let placeholderID = UUID()
        var placeholder = makeItem(id: placeholderID, title: "Placeholder")
        placeholder.isPlaceholder = true
        let danglingID = UUID()
        let snapshot = SonderSnapshot(
            items: [duplicate, duplicate, placeholder, makeItem(id: UUID(), title: "Other")],
            progress: [
                SonderProgress(itemID: keptID, seconds: 10, duration: 100),
                SonderProgress(itemID: placeholderID, seconds: 10, duration: 100),
                SonderProgress(itemID: danglingID, seconds: 10, duration: 100)
            ],
            collections: [SonderCollection(name: "Queue", itemIDs: [keptID, keptID, placeholderID, danglingID])],
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

    @Test func httpRouteIDRejectsMalformedIDs() async throws {
        let id = UUID()

        #expect(SonderHTTPRouteID.uuid(from: "/api/progress/\(id.uuidString)") == id)
        #expect(SonderHTTPRouteID.uuid(from: "/api/progress/not-a-uuid") == nil)
        #expect(SonderHTTPRouteID.uuid(from: "/stream/") == nil)
    }

    @Test func serverSettingsAuthSnapshotRequiresTokenWhenConfigured() async throws {
        let snapshot = SonderAuthSnapshot(allowLAN: true, pairingToken: "secret")

        #expect(snapshot.isAuthorized(localhost: true, bearer: nil, queryToken: nil))
        #expect(snapshot.isAuthorized(localhost: false, bearer: "secret", queryToken: nil))
        #expect(snapshot.isAuthorized(localhost: false, bearer: nil, queryToken: "secret"))
        #expect(snapshot.isAuthorized(localhost: false, bearer: nil, queryToken: nil) == false)
    }

    @Test func serverSettingsPlannerClampsPortAndAutoGeneratesLANToken() async throws {
        let settings = SonderServerSettings(isEnabled: false, allowLAN: false, port: 8797, themePreset: SonderThemePreset.earthy.rawValue, pairingToken: "")

        let mutation = SonderServerSettingsPlanner.updating(
            settings,
            isEnabled: true,
            allowLAN: true,
            port: 99,
            tokenGenerator: { "paired" }
        )

        #expect(mutation.settings.isEnabled)
        #expect(mutation.settings.allowLAN)
        #expect(mutation.settings.port == 1024)
        #expect(mutation.settings.pairingToken == "paired")
        #expect(mutation.activity.title == "Updated server settings")
        #expect(mutation.postsServerNotification)
    }

    @Test func serverSettingsPlannerAppliesThemeAndRotatesToken() async throws {
        let settings = SonderServerSettings.default

        let theme = SonderServerSettingsPlanner.applyingTheme(.earthy, to: settings)
        let rotated = SonderServerSettingsPlanner.rotatingPairingToken(in: settings) { "new-token" }

        #expect(theme.settings.themePreset == SonderThemePreset.earthy.rawValue)
        #expect(theme.themePreset == .earthy)
        #expect(theme.postsServerNotification == false)
        #expect(rotated.settings.pairingToken == "new-token")
        #expect(rotated.activity.title == "Rotated LAN pairing token")
        #expect(rotated.postsServerNotification)
    }

    @Test func derivedDataDoesNotCountTVPlaceholdersAsEpisodes() async throws {
        var placeholder = makeItem(id: UUID(), title: "Escaflowne")
        placeholder.kind = .tvShow
        placeholder.showTitle = "Escaflowne"
        placeholder.isPlaceholder = true
        placeholder.sourcePath = nil

        let derivedData = SonderDerivedData.make(items: [placeholder], progressRecords: [])

        #expect(derivedData.tvShowGroups.first?.episodeCount == 0)
        #expect(derivedData.tvShowGroups.first?.displayCount == 0)
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

    @Test func audiobookChapterServicePrefersCachedIndexAndReadsSidecars() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let mediaFolder = rootURL.appendingPathComponent("Books", isDirectory: true)
        try FileManager.default.createDirectory(at: mediaFolder, withIntermediateDirectories: true)
        let mediaURL = mediaFolder.appendingPathComponent("Beacon.m4b")
        try Data([1]).write(to: mediaURL)

        let itemID = UUID()
        var audiobook = makeItem(id: itemID, title: "Beacon")
        audiobook.kind = .audiobook
        audiobook.sourcePath = mediaURL.path
        let sidecarChapter = SonderAudiobookChapterRecord(index: 1, title: "Sidecar", startSeconds: 0, endSeconds: 60)
        let cachedChapter = SonderAudiobookChapterRecord(index: 1, title: "Cached", startSeconds: 0, endSeconds: 120)
        let sidecar = SonderAudiobookChapterFile(chapters: [sidecarChapter])
        try JSONEncoder.sonder.encode(sidecar).write(to: mediaFolder.appendingPathComponent("Beacon.chapters.json"), options: .atomic)

        let service = SonderAudiobookChapterService(storeRootURL: rootURL)
        let sidecarResult = service.chapters(for: audiobook)
        let cacheRoot = rootURL.appendingPathComponent("AudiobookImport", isDirectory: true)
        try FileManager.default.createDirectory(at: cacheRoot, withIntermediateDirectories: true)
        let index = SonderAudiobookIndex(
            schemaVersion: 1,
            generatedAt: Date(),
            items: [SonderAudiobookIndexEntry(
                id: itemID,
                itemID: itemID,
                title: "Beacon",
                subtitle: "Audiobook",
                author: nil,
                series: nil,
                narrator: nil,
                studio: nil,
                year: nil,
                sourcePath: mediaURL.path,
                sourceFingerprint: "fingerprint",
                artworkPath: nil,
                chapters: [cachedChapter],
                importedAt: Date(),
                updatedAt: Date()
            )]
        )
        try JSONEncoder.sonder.encode(index).write(to: cacheRoot.appendingPathComponent("audiobook-index.json"), options: .atomic)

        let cachedResult = service.chapters(for: audiobook)

        #expect(sidecarResult == [sidecarChapter])
        #expect(cachedResult == [cachedChapter])
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

        let store = await MainActor.run { SonderStore(rootURL: rootURL) }
        let importer = SonderAudiobookImporter(store: store)
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

    @Test func customLibraryPlannerBuildsDefinitionsForSupportedKinds() async throws {
        let id = UUID()
        let plan = SonderCustomLibraryPlanner.plan(named: "  Family Movies  ", kind: .movies) { id }
        let unsupported = SonderCustomLibraryPlanner.plan(named: "Audio", kind: .audiobooks) { UUID() }
        let blank = SonderCustomLibraryPlanner.plan(named: "   ", kind: .tvShows) { UUID() }

        #expect(plan?.definition.id == id)
        #expect(plan?.definition.name == "Family Movies")
        #expect(plan?.definition.kind == .movies)
        #expect(plan?.activity.title == "Added custom library")
        #expect(plan?.activity.detail == "Family Movies scans as movies.")
        #expect(unsupported == nil)
        #expect(blank == nil)
    }

    @Test func mediaDirectoryPlannerUpsertsExistingDirectory() async throws {
        let libraryID = UUID()
        let url = URL(fileURLWithPath: "/Media/Movies", isDirectory: true)
        let existing = SonderMediaDirectory(
            name: "Old Movies",
            path: url.path,
            bookmark: Data([1]),
            kind: .movies,
            libraryID: libraryID
        )

        let mutation = SonderMediaDirectoryPlanner.upserting(
            urls: [url],
            kind: .movies,
            libraryID: libraryID,
            in: [existing]
        ) { _ in Data([2]) }

        #expect(mutation.directories.count == 1)
        #expect(mutation.addedDirectories.count == 1)
        #expect(mutation.failures.isEmpty)
        #expect(mutation.directories.first?.name == "Movies")
        #expect(mutation.directories.first?.bookmark == Data([2]))
    }

    @Test func mediaDirectoryPlannerMigratesUnknownLibraryIDs() async throws {
        let unknownID = UUID()
        let directory = SonderMediaDirectory(
            name: "TV Shows",
            path: "/Media/TV Shows",
            bookmark: Data(),
            kind: .tvShows,
            libraryID: unknownID
        )

        let migrated = SonderMediaDirectoryPlanner.migrated([directory], libraries: SonderLibraryDefinition.defaults)

        #expect(migrated.first?.libraryID == SonderLibraryImportKind.tvShows.defaultLibraryID)
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
        let localAssets = SonderScanProgressFactory.localAssets(directoriesTotal: 0)

        #expect(preparing.phase == .fastTitles)
        #expect(preparing.title == "Preparing scan")
        #expect(indexing.title == "Indexing TV Shows")
        #expect(indexing.detail == "/Media/TV Shows/North Pier")
        #expect(indexing.filesSeen == 20)
        #expect(indexing.mediaFound == 6)
        #expect(indexing.indexedCount == 3)
        #expect(localAssets.phase == .localAssets)
        #expect(localAssets.directoriesTotal == 1)
    }

    @Test func managedImportServiceCopiesAndBuildsItems() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let sourceURL = rootURL.appendingPathComponent("Beacon.m4b")
        try Data([1]).write(to: sourceURL)
        let storageURL = rootURL.appendingPathComponent("Managed", isDirectory: true)
        let service = SonderManagedImportService(store: SonderStore(rootURL: rootURL))

        let imported = service.importFiles(.success([sourceURL]), storagePath: storageURL.path, storageBookmark: nil)
        let cancelled = service.importFiles(.success([]), storagePath: storageURL.path, storageBookmark: nil)

        #expect(imported.importedCount == 1)
        #expect(imported.importedItems.first?.item.title == "Beacon")
        #expect(imported.importedItems.first?.managedURL.deletingLastPathComponent().path == storageURL.path)
        #expect(imported.activities.last?.title == "Imported media")
        #expect(cancelled.importedItems.isEmpty)
        #expect(cancelled.activities.isEmpty)
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

    @Test func collectionPlannerCreatesAndAddsWithoutDuplicates() async throws {
        let item = makeItem(id: UUID(), title: "Beacon")
        let created = SonderCollectionPlanner.creating(
            named: "  Weekend Queue  ",
            kind: .playlist,
            in: []
        )
        guard let collection = created.collections.first else {
            Issue.record("Expected created collection")
            return
        }

        let added = SonderCollectionPlanner.adding(item: item, to: collection, in: created.collections)
        let duplicate = SonderCollectionPlanner.adding(item: item, to: collection, in: added.collections)
        let emptyName = SonderCollectionPlanner.creating(named: "   ", kind: .collection, in: [])

        #expect(created.didMutate)
        #expect(created.collections.first?.name == "Weekend Queue")
        #expect(created.collections.first?.kind == .playlist)
        #expect(created.activity?.title == "Created playlist")
        #expect(added.didMutate)
        #expect(added.collections.first?.itemIDs == [item.id])
        #expect(duplicate.didMutate == false)
        #expect(duplicate.collections.first?.itemIDs == [item.id])
        #expect(emptyName.didMutate == false)
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

    @Test func storageCommandServiceBuildsSuccessAndFailureResults() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let storageURL = rootURL.appendingPathComponent("Media", isDirectory: true)
        try FileManager.default.createDirectory(at: storageURL, withIntermediateDirectories: true)
        let service = SonderStorageCommandService(store: SonderStore(rootURL: rootURL))

        let success = service.storageUpdate(from: .success([storageURL]))
        let cancelled = service.storageUpdate(from: .success([]))

        #expect(success.didUpdate)
        #expect(success.storagePath == storageURL.path)
        #expect(success.storageBookmark != nil)
        #expect(success.activity.title == "Updated storage")
        #expect(cancelled.didUpdate == false)
        #expect(cancelled.storagePath == nil)
        #expect(cancelled.activity.title == "Storage unchanged")
    }

    @Test func playbackCommandsClampProgressAndFormatLabels() async throws {
        let itemID = UUID()
        let item = {
            var item = makeItem(id: itemID, title: "Beacon")
            item.progressSeconds = 12
            item.durationSeconds = 120
            return item
        }()
        let existing = [SonderProgress(itemID: itemID, seconds: 150, duration: 100)]

        let update = SonderPlaybackCommands.progressUpdate(
            itemID: itemID,
            seconds: 250,
            duration: 120,
            records: existing
        )
        let label = await MainActor.run {
            SonderPlaybackCommands.progressLabel(for: item, record: update.records.first)
        }

        #expect(update.seconds == 120)
        #expect(update.duration == 120)
        #expect(SonderPlaybackCommands.progress(for: item, record: update.records.first) == 1)
        #expect(label == "2m of 2m")
    }

    @Test func fileAndConversionCommandsBuildPlans() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let mkvURL = rootURL.appendingPathComponent("Signal Path.mkv")
        try Data([1]).write(to: mkvURL)

        var item = makeItem(id: UUID(), title: "Signal Path")
        item.format = .mkv
        item.sourcePath = mkvURL.path
        item.year = 2026

        let renamePlan = SonderFileCommandPlanner.renameForPlexPlan(for: item)
        let outputURL = rootURL.appendingPathComponent("Signal Path.mp4")
        let conversionPlan = SonderConversionCommands.plan(for: item, outputURL: outputURL)

        #expect(renamePlan?.sourceURL.path == mkvURL.path)
        #expect(renamePlan?.destinationURL.lastPathComponent == item.plexFileName)
        #expect(SonderConversionCommands.candidates(from: [item]).map(\.id) == [item.id])
        #expect(conversionPlan?.itemID == item.id)
        #expect(conversionPlan?.outputURL.path == outputURL.path)
    }

    @Test func fileCommandServiceRenamesForPlexAndReturnsItemPatch() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let sourceURL = rootURL.appendingPathComponent("Signal Path.mkv")
        try Data([1]).write(to: sourceURL)

        var item = makeItem(id: UUID(), title: "Signal Path")
        item.format = .mkv
        item.sourcePath = sourceURL.path
        item.year = 2026
        let service = SonderFileCommandService(store: SonderStore(rootURL: rootURL))

        let result = service.renameForPlex(item)

        #expect(result.didRename)
        #expect(result.itemID == item.id)
        #expect(result.sourcePath?.hasSuffix("Signal Path (2026).mkv") == true)
        #expect(result.format == .mkv)
        #expect(FileManager.default.fileExists(atPath: result.sourcePath ?? ""))
        #expect(FileManager.default.fileExists(atPath: sourceURL.path) == false)
    }

    @Test func scanIndexStateDeduplicatesDiscoveredFiles() async throws {
        let rootURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: rootURL) }
        try FileManager.default.createDirectory(at: rootURL, withIntermediateDirectories: true)
        let mediaURL = rootURL.appendingPathComponent("North Pier S01E01.mkv")
        try Data([1]).write(to: mediaURL)
        let directory = SonderMediaDirectory(
            name: "TV Shows",
            path: rootURL.path,
            bookmark: Data(),
            kind: .tvShows,
            libraryID: SonderLibraryImportKind.tvShows.defaultLibraryID
        )
        let state = SonderScanIndexState(existingPaths: [])

        let first = await state.index([SonderScannedMediaFile(url: mediaURL, bookmark: nil)], directory: directory)
        let second = await state.index([SonderScannedMediaFile(url: mediaURL, bookmark: nil)], directory: directory)

        #expect(first.count == 1)
        #expect(second.isEmpty)
        #expect(await state.currentDirectoryIndexedCount() == 1)
        #expect(await state.currentDirectorySkippedDuplicateCount() == 1)
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
