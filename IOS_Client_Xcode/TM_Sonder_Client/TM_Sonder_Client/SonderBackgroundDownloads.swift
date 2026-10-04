import Foundation
import SonderAPI

nonisolated private struct OfflineTransferIdentity: Codable {
    let item: SonderMediaItem
    let serverURL: URL
    var isResumed: Bool? = nil
}

nonisolated private struct OfflineTransferKey: Hashable {
    let itemID: UUID
    let serverURL: URL
}

struct SonderOfflineDownloadProgress: Equatable {
    var bytesWritten: Int64
    var bytesExpected: Int64
    var bytesPerSecond: Double
    var isPaused: Bool

    var fractionCompleted: Double? {
        guard bytesExpected > 0 else { return nil }
        return min(max(Double(bytesWritten) / Double(bytesExpected), 0), 1)
    }
}

/// Uses an OS-owned background session so a download continues when Sonder is no
/// longer foregrounded. The server still authorizes every request with the same
/// bearer token used for playback.
final class SonderBackgroundDownloadCoordinator: NSObject, URLSessionDownloadDelegate, URLSessionTaskDelegate, @unchecked Sendable {
    static let shared = SonderBackgroundDownloadCoordinator()

    private let sessionIdentifier = "com.techmore.tmsonder.offline-downloads"
    private lazy var session: URLSession = {
        let configuration = URLSessionConfiguration.background(withIdentifier: sessionIdentifier)
        configuration.allowsCellularAccess = true
        configuration.isDiscretionary = false
        configuration.sessionSendsLaunchEvents = true
        configuration.waitsForConnectivity = true
        return URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
    }()
    private let lock = NSLock()
    private var progressHandlers: [OfflineTransferKey: (SonderOfflineDownloadProgress) -> Void] = [:]
    private var completionHandlers: [OfflineTransferKey: (Result<SonderOfflineDownload, Error>) -> Void] = [:]
    private var resumedTasks = Set<Int>()
    private var originalRequests: [Int: URLRequest] = [:]
    private var samples: [Int: (bytes: Int64, date: Date)] = [:]
    private var backgroundEventsCompletionHandler: (() -> Void)?

    private override init() { super.init() }

    func start(
        item: SonderMediaItem,
        serverURL: URL,
        request: URLRequest,
        progress: @escaping (SonderOfflineDownloadProgress) -> Void,
        completion: @escaping (Result<SonderOfflineDownload, Error>) -> Void
    ) {
        let key = OfflineTransferKey(itemID: item.id, serverURL: serverURL)
        lock.lock()
        progressHandlers[key] = progress
        completionHandlers[key] = completion
        lock.unlock()

        let task: URLSessionDownloadTask
        let resumeData = try? Data(contentsOf: resumeDataURL(for: key))
        if let resumeData {
            task = session.downloadTask(withResumeData: resumeData)
            lock.lock()
            resumedTasks.insert(task.taskIdentifier)
            lock.unlock()
            try? FileManager.default.removeItem(at: resumeDataURL(for: key))
        } else {
            task = session.downloadTask(with: request)
        }
        task.taskDescription = (try? JSONEncoder.sonder.encode(OfflineTransferIdentity(item: item, serverURL: serverURL, isResumed: resumeData != nil)))
            .flatMap { String(data: $0, encoding: .utf8) }
        lock.lock()
        originalRequests[task.taskIdentifier] = request
        lock.unlock()
        task.resume()
    }

    func pause(itemID: UUID, serverURL: URL, completion: @escaping () -> Void) {
        let key = OfflineTransferKey(itemID: itemID, serverURL: serverURL)
        session.getAllTasks { [weak self] tasks in
            guard let self,
                  let task = tasks.first(where: { self.key(for: $0) == key }) as? URLSessionDownloadTask else {
                completion()
                return
            }
            task.cancel { resumeData in
                if let resumeData {
                    try? FileManager.default.createDirectory(at: self.resumeDataURL(for: key).deletingLastPathComponent(), withIntermediateDirectories: true)
                    try? resumeData.write(to: self.resumeDataURL(for: key), options: .atomic)
                }
                self.emitProgress(key, SonderOfflineDownloadProgress(bytesWritten: task.countOfBytesReceived, bytesExpected: task.countOfBytesExpectedToReceive, bytesPerSecond: 0, isPaused: true))
                completion()
            }
        }
    }

    func hasPausedDownload(for itemID: UUID, serverURL: URL) -> Bool {
        FileManager.default.fileExists(atPath: resumeDataURL(for: OfflineTransferKey(itemID: itemID, serverURL: serverURL)).path)
    }

    func activeTransfers(serverURL: URL, completion: @escaping ([UUID: SonderOfflineDownloadProgress]) -> Void) {
        session.getAllTasks { [weak self] tasks in
            guard let self else { return }
            var values: [UUID: SonderOfflineDownloadProgress] = [:]
            for task in tasks where task.state == .running || task.state == .suspended {
                guard let key = self.key(for: task), key.serverURL == serverURL else { continue }
                let id = key.itemID
                values[id] = SonderOfflineDownloadProgress(bytesWritten: task.countOfBytesReceived, bytesExpected: task.countOfBytesExpectedToReceive, bytesPerSecond: 0, isPaused: false)
            }
            let snapshot = values
            Task { @MainActor in completion(snapshot) }
        }
    }

    func setBackgroundEventsCompletionHandler(_ handler: @escaping () -> Void) {
        lock.lock()
        backgroundEventsCompletionHandler = handler
        lock.unlock()
        _ = session // Reconnect to the OS-owned session after a background launch.
    }

    private func resumeDataURL(for key: OfflineTransferKey) -> URL {
        SonderOfflineDownloadStore().resumeDataURL(itemID: key.itemID, serverURL: key.serverURL)
    }

    private func key(for task: URLSessionTask) -> OfflineTransferKey? {
        guard let identity = identity(for: task) else { return nil }
        return OfflineTransferKey(itemID: identity.item.id, serverURL: identity.serverURL)
    }

    private func identity(for task: URLSessionTask) -> OfflineTransferIdentity? {
        guard let data = task.taskDescription?.data(using: .utf8) else { return nil }
        return try? JSONDecoder.sonder.decode(OfflineTransferIdentity.self, from: data)
    }

    private func emitProgress(_ key: OfflineTransferKey, _ progress: SonderOfflineDownloadProgress) {
        lock.lock()
        let handler = progressHandlers[key]
        lock.unlock()
        Task { @MainActor in handler?(progress) }
    }

    private func complete(_ key: OfflineTransferKey, _ result: Result<SonderOfflineDownload, Error>) {
        lock.lock()
        let handler = completionHandlers.removeValue(forKey: key)
        progressHandlers[key] = nil
        lock.unlock()
        Task { @MainActor in handler?(result) }
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64, totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        guard let key = key(for: downloadTask) else { return }
        let now = Date()
        lock.lock()
        let prior = samples[downloadTask.taskIdentifier]
        samples[downloadTask.taskIdentifier] = (totalBytesWritten, now)
        lock.unlock()
        let elapsed = prior.map { now.timeIntervalSince($0.date) } ?? 0
        let speed = elapsed > 0 ? Double(totalBytesWritten - (prior?.bytes ?? totalBytesWritten)) / elapsed : 0
        emitProgress(key, SonderOfflineDownloadProgress(bytesWritten: totalBytesWritten, bytesExpected: totalBytesExpectedToWrite, bytesPerSecond: max(speed, 0), isPaused: false))
    }

    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        guard let key = key(for: downloadTask) else { return }
        // URLSession owns `location` only for this delegate callback. Move it
        // synchronously before handing off to the main actor.
        do {
            guard let identity = identity(for: downloadTask),
                  let http = downloadTask.response as? HTTPURLResponse,
                  http.statusCode == 200 || http.statusCode == 206 else {
                throw URLError(.badServerResponse)
            }
            let store = SonderOfflineDownloadStore()
            let saved = try store.save(temporaryURL: location, item: identity.item, serverURL: identity.serverURL)
            var manifest = store.downloads(for: identity.serverURL)
            manifest[saved.itemID] = saved
            try store.saveManifest(manifest, for: identity.serverURL)
            complete(key, .success(saved))
        } catch {
            complete(key, .failure(error))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        guard let key = key(for: task) else { return }
        lock.lock()
        samples[task.taskIdentifier] = nil
        let resumed = resumedTasks.remove(task.taskIdentifier) != nil || identity(for: task)?.isResumed == true
        let original = originalRequests.removeValue(forKey: task.taskIdentifier) ?? task.originalRequest
        lock.unlock()
        guard let error else { return }
        let nsError = error as NSError
        guard nsError.domain != NSURLErrorDomain || nsError.code != NSURLErrorCancelled else { return }
        // Stale OS resume data can be rejected. Retry once from the current,
        // authorized original request; the fresh task is never marked resumed.
        if resumed, let original, let url = original.url,
           url.scheme == key.serverURL.scheme, url.host == key.serverURL.host,
           url.port == key.serverURL.port {
            let fresh = session.downloadTask(with: original)
            if var identity = identity(for: task) {
                identity.isResumed = false
                fresh.taskDescription = (try? JSONEncoder.sonder.encode(identity)).flatMap { String(data: $0, encoding: .utf8) }
            }
            lock.lock()
            originalRequests[fresh.taskIdentifier] = original
            lock.unlock()
            fresh.resume()
            return
        }
        complete(key, .failure(error))
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        lock.lock()
        let handler = backgroundEventsCompletionHandler
        backgroundEventsCompletionHandler = nil
        lock.unlock()
        DispatchQueue.main.async { handler?() }
    }
}
