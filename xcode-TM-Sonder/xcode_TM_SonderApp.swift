import AppKit
import Network
import SwiftUI

@main
struct xcode_TM_SonderApp: App {
    @NSApplicationDelegateAdaptor(SonderAppDelegate.self) private var appDelegate

    var body: some Scene {
        WindowGroup(id: "main") {
            ContentView()
                .environmentObject(appDelegate.library)
        }
        .defaultSize(width: 1480, height: 920)
        .windowResizability(.contentMinSize)
    }
}

final class SonderAppDelegate: NSObject, NSApplicationDelegate {
    let library = SonderLibrary()
    private var statusItem: NSStatusItem?
    private var httpServer: SonderHTTPServer?
    private var serverSettingsObserver: NSObjectProtocol?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        installMenuBarIcon()
        httpServer = SonderHTTPServer(library: library)
        applyServerSettings()
        library.importPlexContextIfAvailable()
        library.importAudiobookContextIfAvailable()
        serverSettingsObserver = NotificationCenter.default.addObserver(
            forName: .sonderServerSettingsDidChange,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.applyServerSettings()
        }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if flag == false {
            openSonder()
        }
        return true
    }

    private func installMenuBarIcon() {
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        item.button?.image = statusIcon()
        item.button?.imagePosition = .imageOnly
        item.button?.toolTip = "TM Sonder"

        let menu = NSMenu()
        let openItem = NSMenuItem(title: "Open Sonder", action: #selector(openSonder), keyEquivalent: "")
        openItem.target = self
        menu.addItem(openItem)

        let webItem = NSMenuItem(title: "Open Web Interface", action: #selector(openWebInterface), keyEquivalent: "")
        webItem.target = self
        menu.addItem(webItem)

        let plexImportItem = NSMenuItem(title: "Import Plex Context", action: #selector(importPlexContext), keyEquivalent: "")
        plexImportItem.target = self
        menu.addItem(plexImportItem)

        let audiobookImportItem = NSMenuItem(title: "Refresh Audiobook Index", action: #selector(importAudiobookContext), keyEquivalent: "")
        audiobookImportItem.target = self
        menu.addItem(audiobookImportItem)

        menu.addItem(.separator())

        let quitItem = NSMenuItem(title: "Quit Sonder", action: #selector(quitSonder), keyEquivalent: "q")
        quitItem.target = self
        menu.addItem(quitItem)
        item.menu = menu

        statusItem = item
    }

    private func statusIcon() -> NSImage {
        let image = NSImage(systemSymbolName: "play.tv", accessibilityDescription: "TM Sonder") ?? NSImage(size: NSSize(width: 18, height: 18))
        image.isTemplate = true
        return image
    }

    @objc private func openSonder() {
        NSApp.activate(ignoringOtherApps: true)
        for window in NSApp.windows where window.canBecomeMain {
            window.makeKeyAndOrderFront(nil)
            window.deminiaturize(nil)
        }
    }

    @objc private func openWebInterface() {
        let port = httpServer?.port ?? UInt16(library.serverSettings.port)
        NSWorkspace.shared.open(URL(string: "http://127.0.0.1:\(port)")!)
    }

    @objc private func importPlexContext() {
        library.importPlexContextIfAvailable(force: true)
    }

    @objc private func importAudiobookContext() {
        library.importAudiobookContextIfAvailable(force: true)
    }

    @objc private func quitSonder() {
        NSApp.terminate(nil)
    }

    private func applyServerSettings() {
        if library.serverSettings.isEnabled {
            httpServer?.start(port: UInt16(library.serverSettings.port), allowLAN: library.serverSettings.allowLAN, pairingToken: library.serverSettings.pairingToken)
        } else {
            httpServer?.stop()
        }
    }
}

/// Native macOS HTTP media server.
///
/// All request handling runs on a dedicated background GCD queue and never touches the
/// main actor for I/O. Streaming responses are emitted in fixed-size chunks via
/// completion callbacks (tail-call recursion), so the app never holds an entire media
/// file (or an arbitrarily large requested range) in memory, and playback cannot freeze
/// the UI. Library data is read through small `@MainActor` snapshots that cross actor
/// boundaries as `Sendable` value types.
///
/// The class is marked `nonisolated` to opt out of the project-wide `@MainActor` default
/// actor isolation, since all its work runs on a dedicated dispatch queue.
nonisolated final class SonderHTTPServer: @unchecked Sendable {
    private let serverName = "TM Sonder"
    private let serviceType = "_tmsonder._tcp"
    private let serviceDomain = "local."
    /// Utility QoS so I/O never competes with UI work on the main actor or the
    /// SwiftUI rendering thread.
    private let queue = DispatchQueue(label: "tm.sonder.http-server", qos: .utility)
    private let library: SonderLibrary
    private var listener: NWListener?
    private(set) var port: UInt16 = 8797
    private var allowLAN = false
    private var pairingToken = ""
    private let hostName = ProcessInfo.processInfo.hostName.split(separator: ".").first.map(String.init) ?? "localhost"

    /// Largest request (headers + body) we will buffer before rejecting. Progress POSTs
    /// are tiny; this only guards against pathological or malicious inputs.
    private let maxRequestBytes = 10 * 1024 * 1024
    /// Bytes pulled from disk and handed to the socket per write during streaming.
    private let streamChunkSize: UInt64 = 512 * 1024

    // ── Connection metering ──────────────────────────────────────────────
    /// Maximum concurrent in-flight connections.  Exceeding this causes new
    /// connections to be rejected immediately so a misbehaving client (or
    /// aggressive browser polling) cannot exhaust the process.
    private static let maxConnections = 12

    /// The number of connections currently being processed.  Synchronised via
    /// `connectionLock` so the queue hop is minimal.
    private var activeConnections = 0
    private let connectionLock = NSLock()

    /// Attempts to claim a connection slot.  Returns `false` when at capacity.
    private func acquireSlot() -> Bool {
        connectionLock.withLock {
            guard activeConnections < Self.maxConnections else { return false }
            activeConnections += 1
            return true
        }
    }

    private func releaseSlot() {
        connectionLock.withLock {
            activeConnections -= 1
        }
    }
    // ─────────────────────────────────────────────────────────────────────

    init(library: SonderLibrary) {
        self.library = library
    }

    func start(port: UInt16 = 8797, allowLAN: Bool = true, pairingToken: String = "") {
        if listener != nil, self.port == port, self.allowLAN == allowLAN, self.pairingToken == pairingToken {
            return
        }
        stop()

        do {
            let listener = try NWListener(using: .tcp, on: NWEndpoint.Port(rawValue: port)!)
            if allowLAN {
                listener.service = NWListener.Service(
                    name: serverName,
                    type: serviceType,
                    domain: serviceDomain,
                    txtRecord: NWTXTRecord([
                        "app": "TM Sonder",
                        "id": "tm-sonder",
                        "api": "1",
                        "health": "/api/health",
                        "library": "/api/library",
                        "discovery": "/api/discovery",
                        "books": "1",
                        "theme": "earthy"
                    ])
                )
            }
            listener.newConnectionHandler = { [weak self] connection in
                self?.handle(connection)
            }
            listener.start(queue: queue)
            self.listener = listener
            self.port = port
            self.allowLAN = allowLAN
            self.pairingToken = pairingToken
        } catch {
            NSLog("TM Sonder server failed to start on port \(port): \(error.localizedDescription)")
        }
    }

    func stop() {
        listener?.cancel()
        listener = nil
    }

    // MARK: - Connection handling

    private func handle(_ connection: NWConnection) {
        guard acquireSlot() else {
            // At capacity — reject immediately instead of queueing.
            connection.cancel()
            return
        }
        connection.start(queue: queue)
        readRequest(connection, buffer: Data()) { [weak self] request in
            guard let self else {
                connection.cancel()
                return
            }
            self.route(connection: connection, request: request)
        }
    }

    /// Accumulates incoming bytes until the full request head (and any body declared via
    /// `Content-Length`) has arrived, then hands the complete `HTTPRequest` off. This
    /// replaces the previous single-`receive` reader that truncated anything beyond the
    /// first 16 KB packet.
    private func readRequest(_ connection: NWConnection, buffer: Data, completion: @escaping (HTTPRequest) -> Void) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, error in
            guard let self else {
                completion(HTTPRequest(data: buffer))
                return
            }
            if let error {
                NSLog("TM Sonder read error: \(error.localizedDescription)")
                completion(HTTPRequest(data: buffer))
                return
            }

            var accumulated = buffer
            if let data {
                accumulated.append(data)
            }

            // Reject requests that exceed the buffer cap so memory cannot grow unbounded.
            if accumulated.count > self.maxRequestBytes {
                completion(HTTPRequest(data: accumulated))
                return
            }

            if let headerEnd = HTTPRequest.headerTerminatorRange(in: accumulated) {
                let request = HTTPRequest(data: accumulated, headerEnd: headerEnd)
                let bodyReceived = accumulated.count - (headerEnd + 4)
                if bodyReceived >= request.contentLength || isComplete {
                    completion(request)
                } else {
                    self.readRequest(connection, buffer: accumulated, completion: completion)
                }
            } else if isComplete {
                completion(HTTPRequest(data: accumulated))
            } else {
                self.readRequest(connection, buffer: accumulated, completion: completion)
            }
        }
    }

    // MARK: - Routing (off the main actor)

    private func route(connection: NWConnection, request: HTTPRequest) {
        if request.method == "OPTIONS" {
            send(connection, response: makeResponse(status: "204 No Content", contentType: "text/plain", body: Data()))
            return
        }

        let auth = SonderAuthSnapshot(allowLAN: allowLAN, pairingToken: pairingToken)
        guard auth.isAuthorized(localhost: request.isLocalhostRequest, bearer: request.bearerToken, queryToken: request.queryToken) else {
            send(connection, response: makeJSONResponse(["error": "LAN access is disabled or pairing is required in Sonder settings."], status: "403 Forbidden"))
            return
        }

        switch request.path {
        case "/", "/index.html":
            send(connection, response: makeResponse(status: "200 OK", contentType: "text/html; charset=utf-8", body: Data(webInterface.utf8)))
        case "/health", "/api/health":
            send(connection, response: makeJSONResponse([
                "status": "ok",
                "name": serverName,
                "app": "TM Sonder",
                "id": "tm-sonder",
                "service": serviceType,
                "library": "/api/library",
                "allowLAN": allowLAN ? "true" : "false"
            ]))
        case "/api/library", "/library.json":
            sendLibrary(connection)
        case "/api/audiobooks":
            sendAudiobooks(connection)
        case let path where path.hasPrefix("/api/audiobooks/"):
            sendAudiobookDetail(connection, path: path)
        case "/api/discovery":
            sendDiscovery(connection)
        case let path where path.hasPrefix("/api/progress/"):
            applyProgress(connection: connection, path: path, body: request.body)
        case let path where path.hasPrefix("/stream/"):
            streamMedia(connection: connection, path: path, rangeHeader: request.headers["range"])
        case let path where path.hasPrefix("/artwork/poster/"):
            sendArtwork(connection: connection, path: path, kind: .poster)
        case let path where path.hasPrefix("/artwork/backdrop/"):
            sendArtwork(connection: connection, path: path, kind: .backdrop)
        default:
            send(connection, response: makeJSONResponse(["error": "Not found"], status: "404 Not Found"))
        }
    }

    private func send(_ connection: NWConnection, response: Data) {
        connection.send(content: response, completion: .contentProcessed { _ in
            connection.cancel()
            self.releaseSlot()
        })
    }

    // MARK: - Library endpoint

    private func sendLibrary(_ connection: NWConnection) {
        Task { @MainActor in
            let cache = self.library.httpCache
            let (cached, gen) = cache.cachedLibrary()
            if let encoded = cached {
                self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
                return
            }
            let theme = SonderThemeSnapshot(
                preset: self.library.serverSettings.themePreset,
                background: SonderTheme.background.hexString,
                sidebar: SonderTheme.sidebar.hexString,
                surface: SonderTheme.surface.hexString,
                border: SonderTheme.border.hexString,
                accent: SonderTheme.accent.hexString,
                text: SonderTheme.text.hexString
            )
            let snapshot = SonderLibraryResponse(
                items: self.library.items,
                progress: self.library.progressRecords,
                serverSettings: self.library.serverSettings,
                theme: theme
            )
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            let encoded = (try? encoder.encode(snapshot)) ?? Data("{}".utf8)
            cache.setLibrary(encoded, generation: gen)
            self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
        }
    }

    private func sendAudiobooks(_ connection: NWConnection) {
        Task { @MainActor in
            let audiobooks = self.library.audiobookItems().map { item in
                let metadata = self.library.audiobookMetadata(for: item.id)
                return SonderAudiobookItem(
                    item,
                    chapterCount: self.library.audiobookChapters(for: item.id).count,
                    author: metadata?.author,
                    series: metadata?.series,
                    narrator: metadata?.narrator
                )
            }
            let response = SonderAudiobookResponse(items: audiobooks, count: audiobooks.count, theme: SonderThemeSnapshot(
                preset: self.library.serverSettings.themePreset,
                background: SonderTheme.background.hexString,
                sidebar: SonderTheme.sidebar.hexString,
                surface: SonderTheme.surface.hexString,
                border: SonderTheme.border.hexString,
                accent: SonderTheme.accent.hexString,
                text: SonderTheme.text.hexString
            ), generatedAt: Date())
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            let encoded = (try? encoder.encode(response)) ?? Data("{}".utf8)
            self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
        }
    }

    private func sendAudiobookDetail(_ connection: NWConnection, path: String) {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID) else {
            send(connection, response: makeJSONResponse(["error": "Audiobook not found"], status: "404 Not Found"))
            return
        }
        Task { @MainActor in
            guard let item = self.library.audiobookItem(id: id) else {
                self.send(connection, response: self.makeJSONResponse(["error": "Audiobook not found"], status: "404 Not Found"))
                return
            }
            let chapters = self.library.audiobookChapters(for: id).map {
                SonderAudiobookChapter(index: $0.index, title: $0.title, startSeconds: $0.startSeconds, endSeconds: $0.endSeconds)
            }
            let metadata = self.library.audiobookMetadata(for: id)
            let response = SonderAudiobookDetail(item: SonderAudiobookItem(
                item,
                chapterCount: chapters.count,
                author: metadata?.author,
                series: metadata?.series,
                narrator: metadata?.narrator
            ), chapters: chapters)
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            let encoded = (try? encoder.encode(response)) ?? Data("{}".utf8)
            self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
        }
    }

    private func sendDiscovery(_ connection: NWConnection) {
        Task { @MainActor in
            let cache = self.library.httpCache
            let (cached, gen) = cache.cachedDiscovery()
            if let encoded = cached {
                self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
                return
            }
            let discovery = SonderDiscoveryResponse(
                app: "TM Sonder",
                name: serverName,
                version: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0.0",
                build: Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "0",
                isEnabled: library.serverSettings.isEnabled,
                allowLAN: library.serverSettings.allowLAN,
                port: port,
                localURL: "http://127.0.0.1:\(port)",
                lanURL: library.serverSettings.allowLAN ? "http://\(hostName):\(port)" : nil,
                discoveryMethods: library.serverSettings.allowLAN ? ["bonjour", "local-http", "lan-http"] : ["local-http"],
                tailscaleHint: "Use the same port over your tailnet URL or MagicDNS name, then pair with the server token.",
                capabilities: SonderDiscoveryCapabilities(
                    books: true,
                    audiobooks: true,
                    themes: true,
                    progressSync: true,
                    mediaStreaming: true,
                    remoteCatalog: true
                ),
                endpoints: SonderDiscoveryEndpoints(
                    health: "/api/health",
                    library: "/api/library",
                    discovery: "/api/discovery",
                    progress: "/api/progress/{id}",
                    stream: "/stream/{id}"
                ),
                theme: SonderThemeSnapshot(
                    preset: library.serverSettings.themePreset,
                    background: SonderTheme.background.hexString,
                    sidebar: SonderTheme.sidebar.hexString,
                    surface: SonderTheme.surface.hexString,
                    border: SonderTheme.border.hexString,
                    accent: SonderTheme.accent.hexString,
                    text: SonderTheme.text.hexString
                )
            )
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            let encoded = (try? encoder.encode(discovery)) ?? Data("{}".utf8)
            cache.setDiscovery(encoded, generation: gen)
            self.send(connection, response: self.makeResponse(status: "200 OK", contentType: "application/json", body: encoded))
        }
    }

    // MARK: - Artwork endpoint

    private enum ArtworkKind {
        case poster
        case backdrop
    }

    private func sendArtwork(connection: NWConnection, path: String, kind: ArtworkKind) {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID) else {
            send(connection, response: makeJSONResponse(["error": "Artwork not found"], status: "404 Not Found"))
            return
        }

        Task { @MainActor in
            guard let item = self.library.item(id: id) else {
                connection.send(content: self.makeJSONResponse(["error": "Artwork not found"], status: "404 Not Found"), completion: .contentProcessed { _ in connection.cancel(); self.releaseSlot() })
                return
            }
            let artworkPath: String?
            switch kind {
            case .poster:
                artworkPath = item.localPosterPath
            case .backdrop:
                artworkPath = item.localBackdropPath
            }
            guard let artworkPath, FileManager.default.fileExists(atPath: artworkPath) else {
                self.library.prioritizeAssets(for: id)
                connection.send(content: self.makeJSONResponse(["status": "pending", "detail": "Artwork refresh queued"], status: "202 Accepted"), completion: .contentProcessed { _ in connection.cancel(); self.releaseSlot() })
                return
            }

            self.queue.async {
                let url = URL(fileURLWithPath: artworkPath)
                guard let data = try? Data(contentsOf: url) else {
                    connection.send(content: self.makeJSONResponse(["error": "Artwork not found"], status: "404 Not Found"), completion: .contentProcessed { _ in connection.cancel(); self.releaseSlot() })
                    return
                }
                let ext = url.pathExtension.lowercased()
                let contentType: String
                switch ext {
                case "jpg", "jpeg": contentType = "image/jpeg"
                case "png": contentType = "image/png"
                case "webp": contentType = "image/webp"
                case "gif": contentType = "image/gif"
                default: contentType = "application/octet-stream"
                }
                self.send(connection, response: self.makeResponse(status: "200 OK", contentType: contentType, body: data))
            }
        }
    }

    // MARK: - Progress endpoint

    private func applyProgress(connection: NWConnection, path: String, body: Data) {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID) else {
            send(connection, response: makeJSONResponse(["error": "Invalid item id"], status: "400 Bad Request"))
            return
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        guard let payload = try? decoder.decode(SonderProgressUpdate.self, from: body) else {
            send(connection, response: makeJSONResponse(["error": "Invalid progress payload"], status: "400 Bad Request"))
            return
        }
        Task { @MainActor in
            self.library.updateProgress(itemID: id, seconds: payload.seconds, duration: payload.duration)
            self.send(connection, response: self.makeJSONResponse(["status": "ok"]))
        }
    }

    // MARK: - Streaming endpoint

    private func streamMedia(connection: NWConnection, path: String, rangeHeader: String?) {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID) else {
            send(connection, response: makeJSONResponse(["error": "Media not found"], status: "404 Not Found"))
            return
        }

        // Pull a tiny Sendable snapshot of what we need to stream, then do all file I/O
        // and socket writes on the background queue — never on the main actor.
        Task { @MainActor in
            guard let target = self.library.streamTarget(id: id) else {
                self.send(connection, response: self.makeJSONResponse(["error": "Media not found"], status: "404 Not Found"))
                return
            }
            self.queue.async {
                self.performStream(connection: connection, target: target, rangeHeader: rangeHeader)
            }
        }
    }

    private func performStream(connection: NWConnection, target: SonderStreamTarget, rangeHeader: String?) {
        guard let url = target.resolvedURL else {
            send(connection, response: makeJSONResponse(["error": "Media could not be opened"], status: "404 Not Found"))
            return
        }

        let didAccess = url.startAccessingSecurityScopedResource()

        func cleanup() {
            if didAccess {
                url.stopAccessingSecurityScopedResource()
            }
            connection.cancel()
            releaseSlot()
        }

        guard let attributes = try? FileManager.default.attributesOfItem(atPath: url.path),
              let fileSize = attributes[.size] as? NSNumber,
              let handle = try? FileHandle(forReadingFrom: url) else {
            cleanup()
            return
        }

        let totalLength = fileSize.uint64Value
        let range = HTTPByteRange(header: rangeHeader, fileLength: totalLength)

        var headers: [String: String] = [
            "Accept-Ranges": "bytes",
            "Content-Type": target.contentType,
            "Content-Length": "\(range.length)"
        ]
        if range.isPartial {
            headers["Content-Range"] = "bytes \(range.start)-\(range.end)/\(totalLength)"
        }

        let status = range.isPartial ? "206 Partial Content" : "200 OK"
        let headerData = makeResponse(status: status, headers: headers, body: Data())

        connection.send(content: headerData, completion: .contentProcessed { error in
            if error != nil {
                try? handle.close()
                cleanup()
                return
            }
            self.streamChunks(connection: connection, handle: handle, offset: range.start, end: range.end, didAccess: didAccess, url: url)
        })
    }

    /// Streams the byte range `[offset...end]` in `streamChunkSize` increments. Each chunk
    /// is read from disk and sent before the next is read, so at most one chunk is in
    /// memory at a time. Recursion is via the send completion (tail call), so it does not
    /// grow the stack.
    private func streamChunks(connection: NWConnection, handle: FileHandle, offset: UInt64, end: UInt64, didAccess: Bool, url: URL) {
        if offset > end {
            finishStream(handle: handle, didAccess: didAccess, url: url, connection: connection)
            return
        }

        let length = min(streamChunkSize, end - offset + 1)
        do {
            try handle.seek(toOffset: offset)
        } catch {
            finishStream(handle: handle, didAccess: didAccess, url: url, connection: connection)
            return
        }

        guard let chunk = try? handle.read(upToCount: Int(length)), chunk.isEmpty == false else {
            finishStream(handle: handle, didAccess: didAccess, url: url, connection: connection)
            return
        }

        connection.send(content: chunk, completion: .contentProcessed { [weak self] _ in
            self?.streamChunks(connection: connection, handle: handle, offset: offset + UInt64(chunk.count), end: end, didAccess: didAccess, url: url)
        })
    }

    private func finishStream(handle: FileHandle, didAccess: Bool, url: URL, connection: NWConnection) {
        try? handle.close()
        if didAccess {
            url.stopAccessingSecurityScopedResource()
        }
        connection.cancel()
        releaseSlot()
    }

    // MARK: - Response builders

    private func makeJSONResponse(_ payload: [String: String], status: String = "200 OK") -> Data {
        let body = (try? JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys])) ?? Data("{}".utf8)
        return makeResponse(status: status, contentType: "application/json", body: body)
    }

    private func makeResponse(status: String = "200 OK", contentType: String, body: Data) -> Data {
        makeResponse(status: status, headers: ["Content-Type": contentType, "Content-Length": "\(body.count)"], body: body)
    }

    private func makeResponse(status: String = "200 OK", headers: [String: String], body: Data) -> Data {
        var response = Data()
        response.append(Data("HTTP/1.1 \(status)\r\n".utf8))
        var combined = headers
        combined["Connection"] = "close"
        combined["Access-Control-Allow-Origin"] = "*"
        combined["Access-Control-Allow-Methods"] = "GET, POST, OPTIONS"
        combined["Access-Control-Allow-Headers"] = "Content-Type, Range"
        for (key, value) in combined.sorted(by: { $0.key < $1.key }) {
            response.append(Data("\(key): \(value)\r\n".utf8))
        }
        response.append(Data("\r\n".utf8))
        response.append(body)
        return response
    }

    private var webInterface: String { SonderWebInterface.html }

}

struct SonderLibraryResponse: Codable, Sendable {
    var items: [SonderMediaItem]
    var progress: [SonderProgress]
    var serverSettings: SonderServerSettings?
    var theme: SonderThemeSnapshot?
}

struct SonderThemeSnapshot: Codable, Sendable {
    var preset: String
    var background: String
    var sidebar: String
    var surface: String
    var border: String
    var accent: String
    var text: String
}

struct SonderDiscoveryResponse: Codable, Sendable {
    var app: String
    var name: String
    var version: String
    var build: String
    var isEnabled: Bool
    var allowLAN: Bool
    var port: UInt16
    var localURL: String
    var lanURL: String?
    var discoveryMethods: [String]
    var tailscaleHint: String
    var capabilities: SonderDiscoveryCapabilities
    var endpoints: SonderDiscoveryEndpoints
    var theme: SonderThemeSnapshot
}

struct SonderDiscoveryCapabilities: Codable, Sendable {
    var books: Bool
    var audiobooks: Bool
    var themes: Bool
    var progressSync: Bool
    var mediaStreaming: Bool
    var remoteCatalog: Bool
}

struct SonderDiscoveryEndpoints: Codable, Sendable {
    var health: String
    var library: String
    var discovery: String
    var progress: String
    var stream: String
}

struct SonderAudiobookResponse: Codable, Sendable {
    var items: [SonderAudiobookItem]
    var count: Int
    var theme: SonderThemeSnapshot
    var generatedAt: Date
}

struct SonderAudiobookDetail: Codable, Sendable {
    var item: SonderAudiobookItem
    var chapters: [SonderAudiobookChapter]
}

struct SonderAudiobookChapter: Codable, Sendable {
    var index: Int
    var title: String
    var startSeconds: Double
    var endSeconds: Double?
}

struct SonderAudiobookItem: Codable, Sendable, Identifiable {
    var id: UUID
    var title: String
    var subtitle: String
    var author: String?
    var series: String?
    var narrator: String?
    var summary: String
    var studio: String
    var year: Int
    var durationSeconds: Double
    var chapterCount: Int
    var posterURL: String?
    var backdropURL: String?
    var sourcePath: String?
    var tags: [String]

    init(_ item: SonderMediaItem) {
        id = item.id
        title = item.title
        subtitle = item.subtitle
        author = nil
        series = item.showTitle
        narrator = nil
        summary = item.summary
        studio = item.studio
        year = item.year
        durationSeconds = item.durationSeconds
        chapterCount = 0
        posterURL = item.posterURL
        backdropURL = item.backdropURL
        sourcePath = item.sourcePath
        tags = item.tags
    }

    init(_ item: SonderMediaItem, chapterCount: Int, author: String?, series: String?, narrator: String?) {
        self.init(item)
        self.chapterCount = chapterCount
        self.author = author
        self.series = series
        self.narrator = narrator
    }
}

nonisolated struct SonderProgressUpdate: Codable, Sendable {
    var seconds: Double
    var duration: Double
}
