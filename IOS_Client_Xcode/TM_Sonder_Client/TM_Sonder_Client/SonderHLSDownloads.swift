import AVFoundation
import Foundation
import SonderAPI

nonisolated private struct HLSDownloadIdentity: Codable {
    let item: SonderMediaItem
    let serverURL: URL
}

/// Apple's background HLS downloader owns the package and resumes through app
/// suspension. Only complete VOD playlists are accepted by the caller.
final class SonderHLSDownloadCoordinator: NSObject, AVAssetDownloadDelegate, @unchecked Sendable {
    static let shared = SonderHLSDownloadCoordinator()
    static let identifier = "com.techmore.tmsonder.hls-downloads"
    private lazy var session: AVAssetDownloadURLSession = {
        let config = URLSessionConfiguration.background(withIdentifier: Self.identifier)
        config.isDiscretionary = false
        config.sessionSendsLaunchEvents = true
        config.allowsCellularAccess = true
        return AVAssetDownloadURLSession(configuration: config, assetDownloadDelegate: self, delegateQueue: .main)
    }()
    private let lock = NSLock()
    private var progressHandlers: [String: (SonderOfflineDownloadProgress) -> Void] = [:]
    private var completionHandlers: [String: (Result<SonderOfflineDownload, Error>) -> Void] = [:]
    private var locations: [Int: URL] = [:]
    private var backgroundCompletion: (() -> Void)?

    private func key(_ id: UUID, _ server: URL) -> String { server.absoluteString + "|" + id.uuidString }
    private func identity(_ task: URLSessionTask) -> HLSDownloadIdentity? {
        guard let data = task.taskDescription?.data(using: .utf8) else { return nil }
        return try? JSONDecoder.sonder.decode(HLSDownloadIdentity.self, from: data)
    }

    func start(item: SonderMediaItem, serverURL: URL, asset: AVURLAsset,
               progress: @escaping (SonderOfflineDownloadProgress) -> Void,
               completion: @escaping (Result<SonderOfflineDownload, Error>) -> Void) {
        let transferKey = key(item.id, serverURL)
        lock.lock()
        progressHandlers[transferKey] = progress
        completionHandlers[transferKey] = completion
        lock.unlock()
        session.getAllTasks { [weak self] tasks in
            guard let self else { return }
            if let existing = tasks.first(where: { task in
                guard let value = self.identity(task) else { return false }
                return value.item.id == item.id && value.serverURL == serverURL
            }) {
                existing.resume()
                return
            }
            let configuration = AVAssetDownloadConfiguration(asset: asset, title: item.title)
            let task = self.session.makeAssetDownloadTask(downloadConfiguration: configuration)
            task.taskDescription = (try? JSONEncoder.sonder.encode(HLSDownloadIdentity(item: item, serverURL: serverURL)))
                .flatMap { String(data: $0, encoding: .utf8) }
            task.resume()
        }
    }

    func control(itemID: UUID, serverURL: URL, cancel: Bool, completion: @escaping () -> Void) {
        session.getAllTasks { [weak self] tasks in
            guard let self else { return }
            for task in tasks {
                guard let value = self.identity(task), value.item.id == itemID, value.serverURL == serverURL else { continue }
                if cancel { task.cancel() } else { task.suspend() }
            }
            DispatchQueue.main.async { completion() }
        }
    }

    func activeTransfers(serverURL: URL, completion: @escaping ([UUID: SonderOfflineDownloadProgress]) -> Void) {
        session.getAllTasks { [weak self] tasks in
            guard let self else { return }
            var values: [UUID: SonderOfflineDownloadProgress] = [:]
            for task in tasks {
                guard let value = self.identity(task), value.serverURL == serverURL,
                      task.state == .running || task.state == .suspended else { continue }
                values[value.item.id] = SonderOfflineDownloadProgress(bytesWritten: 0, bytesExpected: 0, bytesPerSecond: 0, isPaused: task.state == .suspended)
            }
            let snapshot = values
            DispatchQueue.main.async { completion(snapshot) }
        }
    }

    func setBackgroundEventsCompletionHandler(_ handler: @escaping () -> Void) {
        lock.lock(); backgroundCompletion = handler; lock.unlock()
        _ = session
    }

    private func complete(_ key: String, _ result: Result<SonderOfflineDownload, Error>) {
        lock.lock()
        let handler = completionHandlers.removeValue(forKey: key)
        progressHandlers[key] = nil
        lock.unlock()
        DispatchQueue.main.async { handler?(result) }
    }

    func urlSession(_ session: URLSession, assetDownloadTask: AVAssetDownloadTask,
                    didLoad timeRange: CMTimeRange, totalTimeRangesLoaded loadedTimeRanges: [NSValue],
                    timeRangeExpectedToLoad: CMTimeRange) {
        guard let value = identity(assetDownloadTask) else { return }
        let loaded = loadedTimeRanges.reduce(0.0) { $0 + CMTimeGetSeconds($1.timeRangeValue.duration) }
        let expected = CMTimeGetSeconds(timeRangeExpectedToLoad.duration)
        guard loaded.isFinite, expected.isFinite else { return }
        // HLS reports playable duration, not byte totals; scale to preserve the
        // shared progress bar while the UI labels this as media progress.
        let status = SonderOfflineDownloadProgress(bytesWritten: Int64(loaded * 1000), bytesExpected: Int64(expected * 1000), bytesPerSecond: 0, isPaused: false, isMediaDuration: true)
        lock.lock(); let handler = progressHandlers[key(value.item.id, value.serverURL)]; lock.unlock()
        DispatchQueue.main.async { handler?(status) }
    }

    func urlSession(_ session: URLSession, assetDownloadTask: AVAssetDownloadTask, didFinishDownloadingTo location: URL) {
        lock.lock(); locations[assetDownloadTask.taskIdentifier] = location; lock.unlock()
        // Preserve the package location across a process exit between the finish
        // and completion callbacks; the background session can reconnect later.
        if let value = identity(assetDownloadTask) {
            let home = URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true).resolvingSymlinksInPath().path + "/"
            let path = location.resolvingSymlinksInPath().path
            if path.hasPrefix(home) {
                UserDefaults.standard.set(String(path.dropFirst(home.count)), forKey: "sonder.hlsCompletion." + key(value.item.id, value.serverURL))
            }
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        guard let value = identity(task) else { return }
        lock.lock(); let currentLocation = locations.removeValue(forKey: task.taskIdentifier); lock.unlock()
        let transferKey = key(value.item.id, value.serverURL)
        let completionKey = "sonder.hlsCompletion." + transferKey
        let persisted = UserDefaults.standard.string(forKey: completionKey).map {
            URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true).appendingPathComponent($0)
        }
        let location = currentLocation ?? persisted
        defer { UserDefaults.standard.removeObject(forKey: completionKey) }
        if let error {
            if let location { try? FileManager.default.removeItem(at: location) }
            complete(transferKey, .failure(error))
            return
        }
        do {
            guard let location, AVURLAsset(url: location).assetCache?.isPlayableOffline == true else {
                if let location { try? FileManager.default.removeItem(at: location) }
                throw URLError(.cannotDecodeContentData)
            }
            let policy = AVMutableAssetDownloadStorageManagementPolicy()
            policy.expirationDate = .distantFuture
            policy.priority = .important
            AVAssetDownloadStorageManager.shared().setStorageManagementPolicy(policy, for: location)
            let manager = FileManager.default
            var bytes: Int64 = 0
            if let enumerator = manager.enumerator(at: location, includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey]) {
                for case let file as URL in enumerator {
                    if let values = try? file.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey]), values.isRegularFile == true {
                        bytes += Int64(values.fileSize ?? 0)
                    }
                }
            }
            let home = URL(fileURLWithPath: NSHomeDirectory(), isDirectory: true).resolvingSymlinksInPath().path + "/"
            let packagePath = location.resolvingSymlinksInPath().path
            guard packagePath.hasPrefix(home) else { throw URLError(.cannotCreateFile) }
            let relativePath = String(packagePath.dropFirst(home.count))
            let saved = SonderOfflineDownload(itemID: value.item.id, title: value.item.title, fileName: location.lastPathComponent,
                format: value.item.format, downloadedAt: Date(), byteCount: bytes, mediaItem: value.item, hlsRelativePath: relativePath)
            let store = SonderOfflineDownloadStore()
            let previous: SonderOfflineDownload?
            do { previous = try store.record(saved, serverURL: value.serverURL) }
            catch { try? manager.removeItem(at: location); throw error }
            if let previous, store.mediaURL(for: previous, serverURL: value.serverURL) != location {
                try? store.remove(previous, serverURL: value.serverURL)
            }
            complete(transferKey, .success(saved))
        } catch { complete(transferKey, .failure(error)) }
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        lock.lock(); let handler = backgroundCompletion; backgroundCompletion = nil; lock.unlock()
        DispatchQueue.main.async { handler?() }
    }
}
