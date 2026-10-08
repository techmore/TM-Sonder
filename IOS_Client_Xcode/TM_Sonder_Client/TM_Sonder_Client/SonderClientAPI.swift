import AVFoundation
import Combine
import Foundation
import SonderAPI

@MainActor
final class SonderClientModel: ObservableObject {
    @Published var serverBaseURL: URL
    @Published var accessToken: String
    @Published var hasConfiguredServer: Bool
    @Published var items: [SonderMediaItem] = []
    @Published var itemsByKind: [SonderMediaKind: [SonderMediaItem]] = [:]
    @Published private(set) var filteredItemsByKind: [SonderMediaKind: [SonderMediaItem]] = [:]
    @Published var inProgressItems: [SonderMediaItem] = []
    @Published var progressByItemID: [UUID: SonderProgress] = [:]
    @Published var health: SonderHealthResponse?
    @Published var discovery: SonderDiscoveryResponse?
    @Published var serverSettings: SonderServerSettings?
    @Published var theme: SonderThemeSnapshot?
    @Published var connectionMode: SonderConnectionMode = .offline
    @Published var isShowingCachedData = false
    @Published var lastSyncedAt: Date?
    @Published var isLoading = false
    @Published var isSavingProgress = false
    @Published var pendingProgressCount = 0
    @Published private(set) var offlineDownloads: [UUID: SonderOfflineDownload] = [:]
    @Published private(set) var downloadingItemIDs = Set<UUID>()
    @Published private(set) var preparingOfflineItemIDs = Set<UUID>()
    @Published private(set) var offlineDownloadProgress: [UUID: SonderOfflineDownloadProgress] = [:]
    @Published var errorMessage: String?
    @Published var events: [SonderClientEvent] = []
    @Published var searchText = "" {
        didSet { rebuildFilteredItems() }
    }
    @Published var selectedKind: SonderMediaKind = .all

    let audiobookPlayer = SonderAudiobookPlayer()

    private let defaultsURLKey = "sonder.serverBaseURL"
    /// Legacy UserDefaults key; migrated into Keychain on first launch then cleared.
    private let legacyDefaultsTokenKey = "sonder.serverAccessToken"
    private let defaultsLibraryETagsKey = "sonder.libraryETagsByServer"
    private var didLoadOnce = false
    private var libraryETag: String?
    private let progressQueue = SonderProgressQueue()
    private let offlineDownloadStore = SonderOfflineDownloadStore()
    private let backgroundDownloads = SonderBackgroundDownloadCoordinator.shared
    private let hlsDownloads = SonderHLSDownloadCoordinator.shared
    private var preparationTasks: [UUID: Task<Void, Never>] = [:]
    private var isFlushingProgress = false

    init() {
        let stored = UserDefaults.standard.string(forKey: defaultsURLKey)
        self.serverBaseURL = URL(string: stored ?? "http://127.0.0.1:8797")!
        self.accessToken = Self.loadStoredAccessToken()
        self.hasConfiguredServer = stored != nil
        if stored != nil { progressQueue.adoptLegacy(for: serverBaseURL) }
        self.pendingProgressCount = progressQueue.pending(for: serverBaseURL).count
        self.offlineDownloads = offlineDownloadStore.downloads(for: serverBaseURL)
        refreshOfflineDownloads()
        Task { await loadCachedLibrary() }
    }

    private static func loadStoredAccessToken() -> String {
        if let keychainToken = SonderKeychain.loadAccessToken(), keychainToken.isEmpty == false {
            return keychainToken
        }
        // One-time migration from older builds that stored the token in UserDefaults.
        if let legacy = UserDefaults.standard.string(forKey: "sonder.serverAccessToken"),
           legacy.isEmpty == false {
            SonderKeychain.saveAccessToken(legacy)
            UserDefaults.standard.removeObject(forKey: "sonder.serverAccessToken")
            return legacy
        }
        return ""
    }

    func loadIfNeeded() async {
        guard didLoadOnce == false else { return }
        didLoadOnce = true
        guard hasConfiguredServer || items.isEmpty == false else { return }
        await reload()
    }

    func reload() async {
        guard hasConfiguredServer || items.isEmpty == false else { return }
        guard isLoading == false else { return }
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        async let discoveryResponse = fetchDiscovery()
        async let healthResponse = fetchHealth()
        async let libraryResponse = fetchLibrary()

        do {
            if let response = try await libraryResponse {
                apply(response, cached: false, date: Date())
                saveCachedLibrary(response)
            } else {
                // 304 Not Modified — keep in-memory catalog, refresh connection metadata.
                isShowingCachedData = false
                lastSyncedAt = Date()
            }
            discovery = try? await discoveryResponse
            health = try? await healthResponse
            connectionMode = resolvedConnectionMode()
            await flushPendingProgress()
        } catch {
            if isUnauthorized(error) {
                connectionMode = .unauthorized
            } else {
                connectionMode = items.isEmpty ? .offline : .stale
            }
            isShowingCachedData = items.isEmpty == false
            errorMessage = error.localizedDescription
            logEvent("Sync failed", detail: error.localizedDescription)
        }
    }

    var filteredItems: [SonderMediaItem] {
        selectedKind == .all ? filteredItemsByKind[.all] ?? [] : filteredItemsByKind[selectedKind] ?? []
    }

    func items(for kind: SonderMediaKind) -> [SonderMediaItem] {
        itemsByKind[kind] ?? []
    }

    func filteredItems(for kind: SonderMediaKind) -> [SonderMediaItem] {
        filteredItemsByKind[kind] ?? []
    }

    func search(items source: [SonderMediaItem]) -> [SonderMediaItem] {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard query.isEmpty == false else { return source }
        return source.filter { $0.searchableText.localizedCaseInsensitiveContains(query) }
    }

    func streamURL(for item: SonderMediaItem) -> URL {
        serverBaseURL.appendingPathComponent("stream").appendingPathComponent(item.id.uuidString.lowercased())
    }

    func audiobookChapters(for item: SonderMediaItem) async throws -> [SonderAudiobookChapter] {
        guard item.kind == .audiobook else { return [] }
        let cache = offlineDownloadStore.chapterURL(itemID: item.id, serverURL: serverBaseURL)
        if localMediaURL(for: item) != nil,
           let data = try? Data(contentsOf: cache),
           let chapters = try? JSONDecoder.sonder.decode([SonderAudiobookChapter].self, from: data) {
            return chapters
        }
        let url = serverBaseURL
            .appendingPathComponent("api/audiobooks")
            .appendingPathComponent(item.id.uuidString.lowercased())
            .appendingPathComponent("chapters")
        let (data, response) = try await URLSession.shared.data(for: authorizedRequest(for: url, timeout: 15))
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        let chapters = try await decode(SonderAudiobookDetailResponse.self, from: data).chapters
        try FileManager.default.createDirectory(at: cache.deletingLastPathComponent(), withIntermediateDirectories: true)
        try JSONEncoder.sonder.encode(chapters).write(to: cache, options: .atomic)
        return chapters
    }

    func localMediaURL(for item: SonderMediaItem) -> URL? {
        guard let download = offlineDownloads[item.id] else { return nil }
        let url = offlineDownloadStore.mediaURL(for: download, serverURL: serverBaseURL)
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        if download.hlsRelativePath != nil, AVURLAsset(url: url).assetCache?.isPlayableOffline != true { return nil }
        return url
    }

    func offlineParts(for item: SonderMediaItem) -> [SonderMediaItem] {
        guard item.kind == .audiobook, let group = item.bookGroupID else { return [item] }
        let parts = items.filter { $0.kind == .audiobook && $0.bookGroupID == group }
            .sorted { ($0.bookPartIndex ?? 0) < ($1.bookPartIndex ?? 0) }
        return parts.isEmpty ? [item] : parts
    }

    func isFullySavedOffline(_ item: SonderMediaItem) -> Bool {
        let parts = offlineParts(for: item)
        if let expected = item.bookPartCount, expected > parts.count { return false }
        return parts.allSatisfy { localMediaURL(for: $0) != nil }
    }

    func supportsOfflineDownload(for item: SonderMediaItem) -> Bool {
        supportsPreparedVideo(item) || item.format.isPlayableInAVPlayer || item.format == .pdf || item.format == .epub
    }

    func isDownloadingOffline(_ item: SonderMediaItem) -> Bool {
        downloadingItemIDs.contains(item.id)
    }

    func downloadProgress(for item: SonderMediaItem) -> SonderOfflineDownloadProgress? {
        offlineDownloadProgress[item.id]
    }

    func isOfflineDownloadPaused(_ item: SonderMediaItem) -> Bool {
        offlineDownloadProgress[item.id]?.isPaused == true || backgroundDownloads.hasPausedDownload(for: item.id, serverURL: serverBaseURL)
    }

    var offlineDownloadBytes: Int64 {
        offlineDownloads.values.reduce(0) { $0 + $1.byteCount }
    }

    var offlineDownloadedItems: [SonderMediaItem] {
        let catalog = Dictionary(items.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        return offlineDownloads.values.compactMap { catalog[$0.itemID] ?? $0.mediaItem }
            .sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    var offlineTransferItems: [SonderMediaItem] {
        items.filter { downloadingItemIDs.contains($0.id) || isOfflineDownloadPaused($0) }
            .sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
    }

    func downloadForOffline(_ item: SonderMediaItem) async {
        for part in offlineParts(for: item) { await downloadSingleForOffline(part) }
    }

    private func downloadSingleForOffline(_ item: SonderMediaItem) async {
        guard supportsOfflineDownload(for: item), localMediaURL(for: item) == nil, downloadingItemIDs.contains(item.id) == false else { return }
        downloadingItemIDs.insert(item.id)
        let downloadServerURL = serverBaseURL
        offlineDownloadProgress[item.id] = SonderOfflineDownloadProgress(bytesWritten: 0, bytesExpected: 0, bytesPerSecond: 0, isPaused: false)
        if supportsPreparedVideo(item) {
            persistVideoPreparation(item, serverURL: downloadServerURL, remove: false)
            preparingOfflineItemIDs.insert(item.id)
            preparationTasks[item.id] = Task { [weak self] in
                guard let self else { return }
                do {
                    let url = try await self.prepareVideoForOffline(item, serverURL: downloadServerURL)
                    try Task.checkCancellation()
                    guard self.serverBaseURL == downloadServerURL else { throw CancellationError() }
                    self.preparingOfflineItemIDs.remove(item.id)
                    self.hlsDownloads.start(item: item, serverURL: downloadServerURL, asset: self.streamAsset(for: url), progress: { [weak self] progress in
                        guard self?.serverBaseURL == downloadServerURL else { return }
                        self?.offlineDownloadProgress[item.id] = progress
                    }, completion: { [weak self] result in
                        self?.finishOfflineTransfer(item, serverURL: downloadServerURL, result: result)
                    })
                } catch {
                    self.finishOfflineTransfer(item, serverURL: downloadServerURL, result: .failure(error))
                }
                self.preparationTasks[item.id] = nil
            }
            return
        }
        let request = authorizedRequest(for: streamURL(for: item), timeout: 60)
        if item.kind == .audiobook { _ = try? await audiobookChapters(for: item) }
        if let coverURL = artworkURL(for: item), !coverURL.isFileURL,
           coverURL.host == downloadServerURL.host, coverURL.port == downloadServerURL.port {
            let poster = offlineDownloadStore.posterURL(itemID: item.id, serverURL: downloadServerURL)
            if let (data, response) = try? await URLSession.shared.data(for: authorizedRequest(for: coverURL)),
               let http = response as? HTTPURLResponse, http.statusCode == 200, data.count <= 20 * 1024 * 1024 {
                try? FileManager.default.createDirectory(at: poster.deletingLastPathComponent(), withIntermediateDirectories: true)
                try? data.write(to: poster, options: .atomic)
            }
        }
        backgroundDownloads.start(item: item, serverURL: downloadServerURL, request: request, progress: { [weak self] progress in
            guard self?.serverBaseURL == downloadServerURL else { return }
            self?.offlineDownloadProgress[item.id] = progress
        }, completion: { [weak self] result in
            self?.finishOfflineTransfer(item, serverURL: downloadServerURL, result: result)
        })
    }

    private func finishOfflineTransfer(_ item: SonderMediaItem, serverURL: URL, result: Result<SonderOfflineDownload, Error>) {
        persistVideoPreparation(item, serverURL: serverURL, remove: true)
        guard serverBaseURL == serverURL else { return }
        preparingOfflineItemIDs.remove(item.id)
        downloadingItemIDs.remove(item.id)
        offlineDownloadProgress[item.id] = nil
        switch result {
        case .success:
            offlineDownloads = offlineDownloadStore.downloads(for: serverURL)
            logEvent("Saved for offline", detail: item.title)
        case let .failure(error):
            let nsError = error as NSError
            if error is CancellationError || (nsError.domain == NSURLErrorDomain && nsError.code == NSURLErrorCancelled) { return }
            errorMessage = "Couldn’t save \(item.title) for offline use: \(error.localizedDescription)"
            logEvent("Offline download failed", detail: "\(item.title): \(error.localizedDescription)")
        }
    }

    func isVideo(_ item: SonderMediaItem) -> Bool {
        item.kind == .movie || item.kind == .tvShow || item.kind == .documentary
    }

    func supportsPreparedVideo(_ item: SonderMediaItem) -> Bool { item.kind == .movie }

    private func videoPreparationKey(_ serverURL: URL) -> String {
        "sonder.pendingVideoPreparations." + Data(serverURL.absoluteString.utf8).base64EncodedString()
    }

    private func pendingVideoPreparations(_ serverURL: URL) -> [SonderMediaItem] {
        guard let data = UserDefaults.standard.data(forKey: videoPreparationKey(serverURL)) else { return [] }
        return (try? JSONDecoder.sonder.decode([SonderMediaItem].self, from: data)) ?? []
    }

    private func persistVideoPreparation(_ item: SonderMediaItem, serverURL: URL, remove: Bool) {
        var pending = pendingVideoPreparations(serverURL).filter { $0.id != item.id }
        if !remove { pending.append(item) }
        if let data = try? JSONEncoder.sonder.encode(pending) {
            UserDefaults.standard.set(data, forKey: videoPreparationKey(serverURL))
        }
    }

    private struct VideoPreparation: Decodable {
        let status: String
        let error: String?
        let playlistURL: String?
    }

    private func videoPreparation(_ item: SonderMediaItem, serverURL: URL) async throws -> VideoPreparation {
        let url = serverURL.appendingPathComponent("api/movies").appendingPathComponent(item.id.uuidString.lowercased()).appendingPathComponent("preparation")
        let (data, response) = try await URLSession.shared.data(for: authorizedRequest(for: url, timeout: 20))
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        return try JSONDecoder().decode(VideoPreparation.self, from: data)
    }

    private func preparedPlaylistURL(_ status: VideoPreparation, serverURL: URL) -> URL? {
        guard status.status == "ready", let raw = status.playlistURL,
              let url = URL(string: raw, relativeTo: serverURL)?.absoluteURL,
              url.scheme == serverURL.scheme, url.host == serverURL.host, url.port == serverURL.port else { return nil }
        return url
    }

    private func prepareVideoForOffline(_ item: SonderMediaItem, serverURL: URL) async throws -> URL {
        let url = serverURL.appendingPathComponent("api/movies").appendingPathComponent(item.id.uuidString.lowercased()).appendingPathComponent("prepare")
        var request = authorizedRequest(for: url, timeout: 30)
        request.httpMethod = "POST"
        let (_, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        while true {
            try Task.checkCancellation()
            let status = try await videoPreparation(item, serverURL: serverURL)
            if let url = preparedPlaylistURL(status, serverURL: serverURL) { return url }
            if status.status == "failed" {
                throw NSError(domain: "SonderPreparation", code: 1, userInfo: [NSLocalizedDescriptionKey: status.error ?? "The server could not prepare this video."])
            }
            try await Task.sleep(for: .seconds(5))
        }
    }

    func preferredPlaybackURL(for item: SonderMediaItem, fallback: URL,
                              onPreparing: @MainActor () -> Void = {}) async throws -> URL {
        if let local = localMediaURL(for: item) { return local }
        if supportsPreparedVideo(item) {
            if let status = try? await videoPreparation(item, serverURL: serverBaseURL),
               let url = preparedPlaylistURL(status, serverURL: serverBaseURL) { return url }
            // A raw MKV/WebM URL is not an AVPlayer fallback. Wait for a complete
            // VOD timeline so native saved-position resume and seeking work.
            if !item.format.isPlayableInAVPlayer || URLComponents(url: fallback, resolvingAgainstBaseURL: true)?.queryItems?.contains(where: { $0.name == "transcode" && $0.value == "1" }) == true {
                onPreparing()
                return try await prepareVideoForOffline(item, serverURL: serverBaseURL)
            }
        }
        if isVideo(item), !item.format.isPlayableInAVPlayer {
            throw NSError(domain: "SonderPreparation", code: 2, userInfo: [NSLocalizedDescriptionKey: "This video needs conversion before this device can play it. Server preparation currently supports movies only."])
        }
        return fallback
    }

    func cancelOfflineDownload(for item: SonderMediaItem) {
        persistVideoPreparation(item, serverURL: serverBaseURL, remove: true)
        preparationTasks.removeValue(forKey: item.id)?.cancel()
        preparingOfflineItemIDs.remove(item.id)
        if supportsPreparedVideo(item) {
            let transferServerURL = serverBaseURL
            hlsDownloads.control(itemID: item.id, serverURL: transferServerURL, cancel: true) { [weak self] in
                Task { @MainActor in
                    guard self?.serverBaseURL == transferServerURL else { return }
                    self?.downloadingItemIDs.remove(item.id)
                    self?.offlineDownloadProgress[item.id] = nil
                }
            }
        }
    }

    func pauseOfflineDownload(for item: SonderMediaItem) {
        guard downloadingItemIDs.contains(item.id) else { return }
        let transferServerURL = serverBaseURL
        if supportsPreparedVideo(item) {
            persistVideoPreparation(item, serverURL: transferServerURL, remove: true)
            if preparingOfflineItemIDs.contains(item.id) { cancelOfflineDownload(for: item); return }
            hlsDownloads.control(itemID: item.id, serverURL: transferServerURL, cancel: false) { [weak self] in
                Task { @MainActor in
                    guard self?.serverBaseURL == transferServerURL else { return }
                    self?.downloadingItemIDs.remove(item.id)
                    if var progress = self?.offlineDownloadProgress[item.id] {
                        progress.isPaused = true
                        self?.offlineDownloadProgress[item.id] = progress
                    }
                }
            }
            return
        }
        backgroundDownloads.pause(itemID: item.id, serverURL: transferServerURL) { [weak self] in
            Task { @MainActor in
                guard self?.serverBaseURL == transferServerURL else { return }
                self?.downloadingItemIDs.remove(item.id)
                if let progress = self?.offlineDownloadProgress[item.id] {
                    self?.offlineDownloadProgress[item.id] = SonderOfflineDownloadProgress(bytesWritten: progress.bytesWritten, bytesExpected: progress.bytesExpected, bytesPerSecond: 0, isPaused: true)
                }
            }
        }
    }

    func refreshOfflineDownloads() {
        for item in pendingVideoPreparations(serverBaseURL) where !downloadingItemIDs.contains(item.id) {
            Task { await downloadSingleForOffline(item) }
        }
        offlineDownloads = offlineDownloadStore.downloads(for: serverBaseURL)
        for id in offlineDownloads.keys {
            downloadingItemIDs.remove(id)
            offlineDownloadProgress[id] = nil
        }
        let transferServerURL = serverBaseURL
        hlsDownloads.activeTransfers(serverURL: transferServerURL) { [weak self] transfers in
            guard let self, self.serverBaseURL == transferServerURL else { return }
            for (id, progress) in transfers {
                if progress.isPaused { self.downloadingItemIDs.remove(id) }
                else { self.downloadingItemIDs.insert(id) }
                self.offlineDownloadProgress[id] = progress
            }
        }
        backgroundDownloads.activeTransfers(serverURL: transferServerURL) { [weak self] transfers in
            guard let self, self.serverBaseURL == transferServerURL else { return }
            for (id, progress) in transfers {
                self.downloadingItemIDs.insert(id)
                self.offlineDownloadProgress[id] = progress
            }
        }
    }

    func removeOfflineDownload(for item: SonderMediaItem) {
        guard let download = offlineDownloads[item.id] else { return }
        do {
            try offlineDownloadStore.removeSaved(download, serverURL: serverBaseURL)
            offlineDownloads = offlineDownloadStore.downloads(for: serverBaseURL)
            logEvent("Removed offline copy", detail: item.title)
        } catch {
            errorMessage = "Couldn’t remove the offline copy of \(item.title): \(error.localizedDescription)"
        }
    }

    func streamAsset(for item: SonderMediaItem) -> AVURLAsset {
        streamAsset(for: streamURL(for: item))
    }

    func streamAsset(for url: URL) -> AVURLAsset {
        guard url.scheme == serverBaseURL.scheme, url.host == serverBaseURL.host, url.port == serverBaseURL.port else {
            return AVURLAsset(url: url)
        }
        let token = accessToken.trimmingCharacters(in: .whitespacesAndNewlines)
        var components = URLComponents(url: url, resolvingAgainstBaseURL: true)
        if !token.isEmpty {
            var query = components?.queryItems ?? []
            query.removeAll { $0.name == "token" }
            query.append(URLQueryItem(name: "token", value: token))
            components?.queryItems = query
        }
        let cookies = HTTPCookieStorage.shared.cookies(for: url) ?? []
        return AVURLAsset(url: components?.url ?? url, options: [AVURLAssetHTTPCookiesKey: cookies])
    }

    func resolvedServerURL(from rawValue: String) -> URL? {
        if let absoluteURL = URL(string: rawValue), absoluteURL.scheme != nil {
            return absoluteURL
        }
        return URL(string: rawValue, relativeTo: serverBaseURL)?.absoluteURL
    }

    func artworkURL(for item: SonderMediaItem) -> URL? {
        let local = offlineDownloadStore.posterURL(itemID: item.id, serverURL: serverBaseURL)
        if FileManager.default.fileExists(atPath: local.path) { return local }
        // Landscape backdrops are not a substitute for portrait video covers.
        let isVideo = item.kind == .movie || item.kind == .tvShow || item.kind == .documentary
        guard let rawURL = item.posterURL ?? (isVideo ? nil : item.backdropURL) else { return nil }
        if let absoluteURL = URL(string: rawURL), absoluteURL.scheme != nil {
            return absoluteURL
        }
        return serverBaseURL.appendingPathComponent(rawURL.trimmingCharacters(in: CharacterSet(charactersIn: "/")))
    }

    func progress(for item: SonderMediaItem) -> SonderProgress {
        progressByItemID[item.id] ?? SonderProgress(itemID: item.id, seconds: item.progressSeconds, duration: item.durationSeconds)
    }

    /// Resolve a saved package and local progress before any server request.
    func localPlaybackResponse(for item: SonderMediaItem, duration: Double) -> SonderPlaybackResponse? {
        guard let localURL = localMediaURL(for: item) else { return nil }
        let storedProgress = progress(for: item)
        let resolvedDuration = duration > 0 ? duration : item.durationSeconds
        return SonderPlaybackResponse(itemID: item.id, streamURL: localURL.absoluteString,
            seconds: storedProgress.seconds, duration: resolvedDuration,
            percent: resolvedDuration > 0 ? storedProgress.seconds / resolvedDuration : 0)
    }

    func startPlayback(for item: SonderMediaItem, seconds: Double, duration: Double) async throws -> SonderPlaybackResponse {
        if let response = localPlaybackResponse(for: item, duration: duration) { return response }
        var response = try await postPlayback(
            itemID: item.id,
            seconds: seconds,
            duration: duration,
            audioTrackID: nil,
            subtitleTrackID: nil,
            subtitlesEnabled: nil
        )
        if item.kind != .audiobook && (response.audioTracks.isEmpty || response.subtitleTracks.isEmpty) {
            response = try await refreshPlaybackTracks(
                for: item,
                seconds: response.seconds,
                duration: response.duration > 0 ? response.duration : duration
            )
        }
        applyPlaybackResponse(response, for: item)
        return response
    }

    func refreshPlaybackTracks(for item: SonderMediaItem, seconds: Double, duration: Double) async throws -> SonderPlaybackResponse {
        let response = try await postPlaybackRefreshTracks(
            itemID: item.id,
            seconds: seconds,
            duration: duration,
            audioTrackID: nil,
            subtitleTrackID: nil,
            subtitlesEnabled: nil
        )
        applyPlaybackResponse(response, for: item)
        logEvent("Track refresh complete", detail: "\(item.title): \(response.audioTracks.count) audio, \(response.subtitleTracks.count) subtitle")
        return response
    }

    @discardableResult
    func stagePlaybackCheckpoint(for item: SonderMediaItem, seconds: Double, duration: Double,
        audioTrackID: String? = nil, subtitleTrackID: String? = nil, subtitlesEnabled: Bool? = nil) -> SonderQueuedProgressUpdate {
        let sanitizedDuration = max(duration, 1)
        let sanitizedSeconds = min(max(seconds, 0), sanitizedDuration)
        let checkpoint = SonderQueuedProgressUpdate(itemID: item.id, seconds: sanitizedSeconds, duration: sanitizedDuration,
            audioTrackID: audioTrackID, subtitleTrackID: subtitleTrackID, subtitlesEnabled: subtitlesEnabled, serverURL: serverBaseURL)
        // Persist before awaiting the network, so background termination cannot
        // lose a checkpoint merely because a request has not yet failed.
        progressQueue.enqueue(checkpoint)
        pendingProgressCount = progressQueue.pending(for: serverBaseURL).count
        progressByItemID[item.id] = SonderProgress(itemID: item.id, seconds: sanitizedSeconds, duration: sanitizedDuration,
            updatedAt: checkpoint.updatedAt, audioTrackID: audioTrackID, subtitleTrackID: subtitleTrackID, subtitlesEnabled: subtitlesEnabled)
        return checkpoint
    }

    func updatePlaybackState(
        for item: SonderMediaItem,
        seconds: Double,
        duration: Double,
        audioTrackID: String? = nil,
        subtitleTrackID: String? = nil,
        subtitlesEnabled: Bool? = nil
    ) async {
        let checkpoint = stagePlaybackCheckpoint(for: item, seconds: seconds, duration: duration,
            audioTrackID: audioTrackID, subtitleTrackID: subtitleTrackID, subtitlesEnabled: subtitlesEnabled)
        let sanitizedSeconds = checkpoint.seconds, sanitizedDuration = checkpoint.duration
        do {
            let response = try await postPlayback(itemID: item.id, seconds: sanitizedSeconds, duration: sanitizedDuration,
                audioTrackID: audioTrackID, subtitleTrackID: subtitleTrackID, subtitlesEnabled: subtitlesEnabled, updatedAt: checkpoint.updatedAt)
            progressQueue.acknowledge(checkpoint)
            guard serverBaseURL == checkpoint.serverURL else { return }
            if (progressByItemID[item.id]?.updatedAt ?? .distantPast) <= checkpoint.updatedAt { applyPlaybackResponse(response, for: item) }
        } catch {
            if serverBaseURL == checkpoint.serverURL { logEvent("Progress saved offline", detail: error.localizedDescription) }
        }
        pendingProgressCount = progressQueue.pending(for: serverBaseURL).count
    }

    func updateProgress(for item: SonderMediaItem, seconds: Double, duration: Double) async {
        isSavingProgress = true
        errorMessage = nil
        defer { isSavingProgress = false }
        await updatePlaybackState(for: item, seconds: seconds, duration: duration)
        if pendingProgressCount > 0 { errorMessage = "Progress saved offline. It will sync when the server is reachable." }
    }

    /// Replay only this server's durable checkpoints. A response cannot discard
    /// a newer checkpoint created while the earlier request was in flight.
    func flushPendingProgress() async {
        guard !isFlushingProgress else { return }
        let server = serverBaseURL
        let snapshot = progressQueue.pending(for: server)
        guard !snapshot.isEmpty else { pendingProgressCount = 0; return }
        isFlushingProgress = true
        defer { isFlushingProgress = false; pendingProgressCount = progressQueue.pending(for: serverBaseURL).count }
        for update in snapshot {
            guard serverBaseURL == server else { break }
            // Skip a snapshot superseded before its request starts.
            guard progressQueue.pending(for: server).contains(update) else { continue }
            do {
                let response = try await postPlayback(itemID: update.itemID, seconds: update.seconds, duration: update.duration,
                    audioTrackID: update.audioTrackID, subtitleTrackID: update.subtitleTrackID, subtitlesEnabled: update.subtitlesEnabled, updatedAt: update.updatedAt)
                progressQueue.acknowledge(update)
                guard serverBaseURL == server else { break }
                guard (progressByItemID[update.itemID]?.updatedAt ?? .distantPast) <= update.updatedAt else { continue }
                if let item = items.first(where: { $0.id == update.itemID }) { applyPlaybackResponse(response, for: item) }
            } catch { logEvent("Progress flush paused", detail: error.localizedDescription); break }
        }
    }

    func connect(to endpoint: SonderServerEndpoint) async {
        saveServer(baseURL: endpoint.url, accessToken: accessToken)
        discovery = endpoint.discovery
        connectionMode = endpoint.mode
        await reload()
    }

    /// A loopback URL saved while using the simulator points back to the iPhone on a
    /// physical device. Once Bonjour finds the real Mac endpoint, replace only that
    /// unambiguously invalid configuration; never override a deliberate LAN/Tailscale
    /// server choice.
    func shouldAutomaticallyUseDiscoveredEndpoint(_ endpoint: SonderServerEndpoint) -> Bool {
        guard hasConfiguredServer, endpoint.url != serverBaseURL else { return false }
        guard let host = serverBaseURL.host?.lowercased() else { return false }
        return host == "localhost" || host == "127.0.0.1" || host == "::1"
    }

    @discardableResult
    func saveServer(baseURL: URL, accessToken: String) -> Bool {
        guard let normalizedURL = Self.normalizedServerURL(baseURL) else {
            errorMessage = "Enter a complete http:// or https:// server URL."
            return false
        }
        let serverChanged = normalizedURL != serverBaseURL
        if serverChanged { audiobookPlayer.stop() }
        serverBaseURL = normalizedURL
        self.accessToken = accessToken.trimmingCharacters(in: .whitespacesAndNewlines)
        hasConfiguredServer = true
        UserDefaults.standard.set(normalizedURL.absoluteString, forKey: defaultsURLKey)
        UserDefaults.standard.removeObject(forKey: legacyDefaultsTokenKey)
        SonderKeychain.saveAccessToken(self.accessToken)
        offlineDownloads = offlineDownloadStore.downloads(for: normalizedURL)
        if serverChanged {
            // Validators are meaningful only for the server that issued them.
            libraryETag = nil
            preparationTasks.values.forEach { $0.cancel() }
            preparationTasks.removeAll()
            preparingOfflineItemIDs.removeAll()
            downloadingItemIDs.removeAll()
            offlineDownloadProgress.removeAll()
            progressByItemID.removeAll()
            pendingProgressCount = progressQueue.pending(for: normalizedURL).count
            refreshOfflineDownloads()
        }
        didLoadOnce = false
        return true
    }

    /// Builds an authorized request for media assets (artwork, subtitles) that
    /// cannot use AVURLAsset header options.
    func authorizedMediaRequest(for url: URL, timeout: TimeInterval = 8) -> URLRequest {
        authorizedRequest(for: url, timeout: timeout)
    }

    func saveServerBaseURL(_ url: URL) {
        saveServer(baseURL: url, accessToken: accessToken)
    }

    private func apply(_ response: SonderLibraryResponse, cached: Bool, date: Date) {
        let sortedItems = response.items.sorted { $0.title.localizedStandardCompare($1.title) == .orderedAscending }
        var progress = Dictionary(uniqueKeysWithValues: response.progress.map { ($0.itemID, $0) })
        for update in progressQueue.pending(for: serverBaseURL) {
            guard (progress[update.itemID]?.updatedAt ?? .distantPast) <= update.updatedAt else { continue }
            progress[update.itemID] = SonderProgress(itemID: update.itemID, seconds: update.seconds, duration: update.duration,
                updatedAt: update.updatedAt, audioTrackID: update.audioTrackID, subtitleTrackID: update.subtitleTrackID, subtitlesEnabled: update.subtitlesEnabled)
        }

        items = sortedItems
        itemsByKind = Dictionary(grouping: sortedItems, by: \.kind)
        rebuildFilteredItems()
        progressByItemID = progress
        inProgressItems = sortedItems
            .filter { item in
                let record = progress[item.id] ?? SonderProgress(itemID: item.id, seconds: item.progressSeconds, duration: item.durationSeconds)
                return record.percent > 0 && record.percent < 0.98
            }
            .sorted {
                let left = progress[$0.id]?.updatedAt ?? .distantPast
                let right = progress[$1.id]?.updatedAt ?? .distantPast
                return left > right
            }
        serverSettings = response.serverSettings
        theme = response.theme ?? discovery?.theme
        lastSyncedAt = date
        isShowingCachedData = cached
    }

    private func rebuildFilteredItems() {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        let visibleItems: [SonderMediaItem]
        if query.isEmpty {
            visibleItems = items
        } else {
            visibleItems = items.filter { $0.searchableText.localizedCaseInsensitiveContains(query) }
        }
        var grouped = Dictionary(grouping: visibleItems, by: \.kind)
        grouped[.all] = visibleItems
        filteredItemsByKind = grouped
    }

    private func resolvedConnectionMode() -> SonderConnectionMode {
        if serverBaseURL.host == "127.0.0.1" || serverBaseURL.host == "localhost" {
            return .local
        }
        if serverBaseURL.host?.localizedCaseInsensitiveContains("ts.net") == true {
            return .tailscale
        }
        if health?.allowLAN == true || serverSettings?.allowLAN == true || discovery?.allowLAN == true {
            return .lan
        }
        return .manual
    }

    private func fetchDiscovery() async throws -> SonderDiscoveryResponse {
        let request = authorizedRequest(for: serverBaseURL.appendingPathComponent("api/discovery"))
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw URLError(.badServerResponse)
        }
        return try await decode(SonderDiscoveryResponse.self, from: data)
    }

    private func fetchHealth() async throws -> SonderHealthResponse {
        let request = authorizedRequest(for: serverBaseURL.appendingPathComponent("api/health"))
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw URLError(.badServerResponse)
        }
        return try await decode(SonderHealthResponse.self, from: data)
    }

    /// Returns a fresh library payload, or `nil` when the server responds `304 Not Modified`.
    private func fetchLibrary() async throws -> SonderLibraryResponse? {
        var request = authorizedRequest(for: serverBaseURL.appendingPathComponent("api/library"), timeout: 20)
        if let libraryETag, libraryETag.isEmpty == false {
            request.setValue(libraryETag, forHTTPHeaderField: "If-None-Match")
        }
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw HTTPStatusError(statusCode: -1)
        }
        if http.statusCode == 304 {
            return nil
        }
        guard 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: http.statusCode)
        }
        if let etag = http.value(forHTTPHeaderField: "ETag") {
            libraryETag = etag
            saveETag(etag, for: serverBaseURL)
        }
        return try await decode(SonderLibraryResponse.self, from: data)
    }

    private func postProgress(itemID: UUID, seconds: Double, duration: Double) async throws {
        let url = serverBaseURL
            .appendingPathComponent("api/progress")
            .appendingPathComponent(itemID.uuidString.lowercased())
        var request = authorizedRequest(for: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder.sonder.encode(SonderProgressUpdate(seconds: seconds, duration: duration, audioTrackID: nil, subtitleTrackID: nil, subtitlesEnabled: nil))

        let (_, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
    }

    private func postPlayback(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String?,
        subtitleTrackID: String?,
        subtitlesEnabled: Bool?,
        updatedAt: Date? = nil
    ) async throws -> SonderPlaybackResponse {
        let url = serverBaseURL
            .appendingPathComponent("api/playback")
            .appendingPathComponent(itemID.uuidString.lowercased())
        var request = authorizedRequest(for: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder.sonder.encode(SonderPlaybackStateUpdate(
            seconds: seconds,
            duration: duration,
            audioTrackID: audioTrackID,
            subtitleTrackID: subtitleTrackID,
            subtitlesEnabled: subtitlesEnabled,
            updatedAt: updatedAt
        ))

        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        return try await decode(SonderPlaybackResponse.self, from: data)
    }

    private func postPlaybackRefreshTracks(
        itemID: UUID,
        seconds: Double,
        duration: Double,
        audioTrackID: String?,
        subtitleTrackID: String?,
        subtitlesEnabled: Bool?
    ) async throws -> SonderPlaybackResponse {
        let url = serverBaseURL
            .appendingPathComponent("api/playback")
            .appendingPathComponent(itemID.uuidString.lowercased())
            .appendingPathComponent("refresh-tracks")
        var request = authorizedRequest(for: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder.sonder.encode(SonderPlaybackStateUpdate(
            seconds: seconds,
            duration: duration,
            audioTrackID: audioTrackID,
            subtitleTrackID: subtitleTrackID,
            subtitlesEnabled: subtitlesEnabled
        ))

        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw HTTPStatusError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        return try await decode(SonderPlaybackResponse.self, from: data)
    }

    private func applyPlaybackResponse(_ response: SonderPlaybackResponse, for item: SonderMediaItem) {
        let resolvedDuration = response.duration > 0 ? response.duration : item.durationSeconds
        progressByItemID[item.id] = SonderProgress(itemID: item.id, seconds: response.seconds, duration: resolvedDuration, updatedAt: Date())
    }

    private func authorizedRequest(for url: URL, timeout: TimeInterval = 5) -> URLRequest {
        var request = URLRequest(url: url, timeoutInterval: timeout)
        request.cachePolicy = .reloadIgnoringLocalCacheData
        let token = accessToken.trimmingCharacters(in: .whitespacesAndNewlines)
        if token.isEmpty == false {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    private func loadCachedLibrary() async {
        let url = cacheURL
        guard let cache = await Task.detached(priority: .utility, operation: {
            guard let data = try? Data(contentsOf: url) else { return nil as SonderLibraryCache? }
            return try? JSONDecoder.sonder.decode(SonderLibraryCache.self, from: data)
        }).value else {
            return
        }
        apply(cache.response, cached: true, date: cache.cachedAt)
        if let etag = cache.etag {
            libraryETag = etag
        }
        if cache.serverURL == serverBaseURL {
            libraryETag = cache.etag ?? storedETag(for: serverBaseURL)
            connectionMode = .stale
            hasConfiguredServer = true
        }
    }

    private func saveCachedLibrary(_ response: SonderLibraryResponse) {
        let cacheURL = cacheURL
        let cache = SonderLibraryCache(response: response, cachedAt: Date(), serverURL: serverBaseURL, etag: libraryETag)
        Task.detached(priority: .utility) {
            do {
                try FileManager.default.createDirectory(at: cacheURL.deletingLastPathComponent(), withIntermediateDirectories: true)
                let data = try JSONEncoder.sonder.encode(cache)
                try data.write(to: cacheURL, options: [.atomic])
            } catch {
                // Cache writes are best-effort and should not block browsing or playback.
            }
        }
    }

    private func decode<T: Decodable & Sendable>(_ type: T.Type, from data: Data) async throws -> T {
        try await Task.detached(priority: .utility) {
            try JSONDecoder.sonder.decode(T.self, from: data)
        }.value
    }

    func logEvent(_ title: String, detail: String) {
        events.insert(SonderClientEvent(title: title, detail: detail), at: 0)
        if events.count > 60 {
            events.removeLast(events.count - 60)
        }
    }

    private var cacheURL: URL {
        let root = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first ?? FileManager.default.temporaryDirectory
        let identity = serverBaseURL.absoluteString.data(using: .utf8)?.base64EncodedString() ?? "default"
        let safeIdentity = identity.replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "=", with: "")
        return root.appendingPathComponent("TM-Sonder", isDirectory: true).appendingPathComponent("LibraryCache-\(safeIdentity).json")
    }

    private func storedETag(for url: URL) -> String? {
        let values = UserDefaults.standard.dictionary(forKey: defaultsLibraryETagsKey) as? [String: String]
        return values?[url.absoluteString]
    }

    private func saveETag(_ etag: String, for url: URL) {
        var values = UserDefaults.standard.dictionary(forKey: defaultsLibraryETagsKey) as? [String: String] ?? [:]
        values[url.absoluteString] = etag
        UserDefaults.standard.set(values, forKey: defaultsLibraryETagsKey)
    }

    private static func normalizedServerURL(_ url: URL) -> URL? {
        guard let scheme = url.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              url.host?.isEmpty == false else { return nil }
        var components = URLComponents(url: url, resolvingAgainstBaseURL: false)
        let path = components?.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")) ?? ""
        components?.path = path
        components?.query = nil
        components?.fragment = nil
        return components?.url
    }

    private func isUnauthorized(_ error: Error) -> Bool {
        (error as? HTTPStatusError)?.statusCode == 401 || (error as? HTTPStatusError)?.statusCode == 403
    }
}

final class SonderServerBrowser: NSObject, ObservableObject, NetServiceBrowserDelegate, NetServiceDelegate {
    @Published private(set) var endpoints: [SonderServerEndpoint] = []
    @Published private(set) var isSearching = false

    private let browser = NetServiceBrowser()
    private var services: [NetService] = []

    override init() {
        super.init()
        browser.delegate = self
    }

    func start() {
        guard isSearching == false else { return }
        isSearching = true
        browser.searchForServices(ofType: "_tmsonder._tcp.", inDomain: "local.")
    }

    func stop() {
        browser.stop()
        isSearching = false
    }

    func netServiceBrowser(_ browser: NetServiceBrowser, didFind service: NetService, moreComing: Bool) {
        services.append(service)
        service.delegate = self
        service.resolve(withTimeout: 5)
    }

    func netServiceBrowser(_ browser: NetServiceBrowser, didNotSearch errorDict: [String: NSNumber]) {
        isSearching = false
    }

    func netServiceBrowserDidStopSearch(_ browser: NetServiceBrowser) {
        isSearching = false
    }

    func netServiceBrowser(_ browser: NetServiceBrowser, didRemove service: NetService, moreComing: Bool) {
        services.removeAll { $0 === service }
        let removedName = service.name
        endpoints.removeAll { $0.name == removedName }
    }

    func netServiceDidResolveAddress(_ sender: NetService) {
        guard sender.port > 0 else { return }
        let host = sender.hostName?.trimmingCharacters(in: CharacterSet(charactersIn: ".")) ?? "\(sender.name).local"
        guard let url = URL(string: "http://\(host):\(sender.port)") else { return }
        let endpoint = SonderServerEndpoint(name: sender.name, url: url, mode: .lan, discovery: nil)
        if endpoints.contains(where: { $0.url == endpoint.url }) == false {
            endpoints.append(endpoint)
        }
    }
}

struct SonderServerEndpoint: Identifiable, Hashable {
    var id: String { url.absoluteString }
    var name: String
    var url: URL
    var mode: SonderConnectionMode
    var discovery: SonderDiscoveryResponse?
}

struct HTTPStatusError: LocalizedError {
    var statusCode: Int

    var errorDescription: String? {
        switch statusCode {
        case 401, 403:
            return "This server is restricted. Add a token in Server settings or disable restrictions on the host."
        default:
            return "Server returned HTTP \(statusCode)."
        }
    }
}
