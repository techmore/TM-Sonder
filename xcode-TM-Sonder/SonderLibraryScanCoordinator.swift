import Foundation

nonisolated struct SonderLibraryScanCoordinatorResult {
    var directoryUpdates: [UUID: SonderScanDiagnostics]
    var discoveredItemIDs: [UUID]
}

nonisolated struct SonderLibraryScanCoordinator {
    let store: SonderStore
    let progressStride: Int
    let discoveryBatchSize: Int

    func scan(
        directories: [SonderMediaDirectory],
        existingPaths: Set<String>,
        onDirectoryStart: @escaping (SonderMediaDirectory, Int, Int, Int, Int) async -> Void,
        onProgress: @escaping (SonderScanProgress) async -> Void,
        onDiscoveredChunk: @escaping ([SonderMediaItem], SonderMediaDirectory, String, Int, Int, Int, Int, Int) async -> Void,
        onDirectoryFailure: @escaping (SonderMediaDirectory, Error) async -> Void
    ) async -> SonderLibraryScanCoordinatorResult {
        var directoryUpdates: [UUID: SonderScanDiagnostics] = [:]
        var totalFilesSeen = 0
        var totalMediaFound = 0
        let scanIndexState = SonderScanIndexState(existingPaths: existingPaths)

        for (offset, directory) in directories.enumerated() {
            await scanIndexState.beginDirectory()
            let startFilesSeen = totalFilesSeen
            let startMediaFound = totalMediaFound
            let currentIndexedCount = 0
            await onDirectoryStart(directory, offset, startFilesSeen, startMediaFound, currentIndexedCount)

            do {
                let scanResult = try await store.mediaFiles(
                    in: directory.path,
                    bookmark: directory.bookmark,
                    progressStride: progressStride,
                    discoveryBatchSize: discoveryBatchSize
                ) { filesSeen, mediaFound, currentPath in
                    let progressFilesSeen = startFilesSeen + filesSeen
                    let progressMediaFound = startMediaFound + mediaFound
                    let indexedSoFar = await scanIndexState.currentDirectoryIndexedCount()
                    let progress = SonderScanProgressFactory.indexing(
                        directory: directory,
                        detail: currentPath,
                        filesSeen: progressFilesSeen,
                        mediaFound: progressMediaFound,
                        indexedCount: currentIndexedCount + indexedSoFar,
                        directoriesDone: offset + 1,
                        directoriesTotal: directories.count
                    )
                    await onProgress(progress)
                } discovered: { scannedFiles, filesSeen, mediaFound in
                    let lastPath = scannedFiles.last?.url.path ?? directory.path
                    let batchStartedAt = Date()
                    await onProgress(SonderScanProgressFactory.indexing(
                        directory: directory,
                        detail: "Indexing batch ending at: \(lastPath)",
                        filesSeen: startFilesSeen + filesSeen,
                        mediaFound: startMediaFound + mediaFound,
                        indexedCount: await scanIndexState.currentDirectoryIndexedCount(),
                        directoriesDone: offset + 1,
                        directoriesTotal: directories.count
                    ))
                    let chunk = await scanIndexState.index(scannedFiles, directory: directory)
                    let batchDuration = Date().timeIntervalSince(batchStartedAt)
                    if batchDuration >= 2 {
                        NSLog("TM Sonder scan slow batch indexing: %.2fs for %d files ending at %@", batchDuration, scannedFiles.count, lastPath)
                    }
                    guard chunk.isEmpty == false else { return }
                    let indexedSoFar = await scanIndexState.currentDirectoryIndexedCount()
                    await onDiscoveredChunk(
                        chunk,
                        directory,
                        lastPath,
                        startFilesSeen,
                        startMediaFound,
                        indexedSoFar,
                        offset,
                        directories.count
                    )
                }

                totalFilesSeen += scanResult.filesSeen
                totalMediaFound += scanResult.mediaFound
                directoryUpdates[directory.id] = SonderScanDiagnostics(
                    mediaCount: scanResult.mediaFound,
                    fileCount: scanResult.filesSeen,
                    unsupportedCount: scanResult.unsupportedMediaCount,
                    skippedDuplicateCount: await scanIndexState.currentDirectorySkippedDuplicateCount(),
                    parseFailureCount: 0,
                    scannedAt: Date()
                )
            } catch {
                await onDirectoryFailure(directory, error)
            }
        }

        return SonderLibraryScanCoordinatorResult(
            directoryUpdates: directoryUpdates,
            discoveredItemIDs: await scanIndexState.discoveredItemIDs()
        )
    }
}
