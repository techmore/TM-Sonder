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
        NSWorkspace.shared.open(URL(string: "http://127.0.0.1:8797")!)
    }

    @objc private func quitSonder() {
        NSApp.terminate(nil)
    }

    private func applyServerSettings() {
        if library.serverSettings.isEnabled {
            httpServer?.start(port: UInt16(library.serverSettings.port), allowLAN: library.serverSettings.allowLAN)
        } else {
            httpServer?.stop()
        }
    }
}

final class SonderHTTPServer {
    private let serverName = "TM Sonder"
    private let serviceType = "_tmsonder._tcp"
    private let serviceDomain = "local."
    private let queue = DispatchQueue(label: "tm.sonder.http-server")
    private let library: SonderLibrary
    private var listener: NWListener?
    private var activePort: UInt16?
    private var allowLAN = false

    init(library: SonderLibrary) {
        self.library = library
    }

    func start(port: UInt16 = 8797, allowLAN: Bool = false) {
        if listener != nil, activePort == port, self.allowLAN == allowLAN {
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
                        "library": "/api/library"
                    ])
                )
            }
            listener.newConnectionHandler = { [weak self] connection in
                self?.handle(connection)
            }
            listener.start(queue: queue)
            self.listener = listener
            self.activePort = port
            self.allowLAN = allowLAN
        } catch {
            NSLog("TM Sonder server failed to start on port \(port): \(error.localizedDescription)")
        }
    }

    func stop() {
        listener?.cancel()
        listener = nil
        activePort = nil
    }

    private func handle(_ connection: NWConnection) {
        connection.start(queue: queue)
        connection.receive(minimumIncompleteLength: 1, maximumLength: 16_384) { [weak self] data, _, _, _ in
            guard let self else {
                connection.cancel()
                return
            }
            let request = HTTPRequest(data: data ?? Data())
            Task { @MainActor in
                let response = self.response(for: request)
                connection.send(content: response, completion: .contentProcessed { _ in
                    connection.cancel()
                })
            }
        }
    }

    @MainActor
    private func response(for request: HTTPRequest) -> Data {
        guard allowLAN || request.isLocalhostRequest else {
            return jsonResponse(["error": "LAN access is disabled in Sonder settings."], status: "403 Forbidden")
        }

        switch request.path {
        case _ where request.method == "OPTIONS":
            return httpResponse(contentType: "text/plain", body: Data())
        case "/", "/index.html":
            return httpResponse(contentType: "text/html; charset=utf-8", body: Data(webInterface.utf8))
        case "/health", "/api/health":
            return jsonResponse([
                "status": "ok",
                "name": serverName,
                "app": "TM Sonder",
                "id": "tm-sonder",
                "service": serviceType,
                "library": "/api/library"
            ])
        case "/api/library", "/library.json":
            return encodedResponse(SonderLibraryResponse(items: library.items, progress: library.progressRecords))
        case let path where path.hasPrefix("/api/progress/"):
            return updateProgress(path: path, body: request.body)
        case let path where path.hasPrefix("/stream/"):
            return streamResponse(path: path, rangeHeader: request.headers["range"])
        default:
            return jsonResponse(["error": "Not found"], status: "404 Not Found")
        }
    }

    @MainActor
    private func updateProgress(path: String, body: Data) -> Data {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID),
              let payload = try? JSONDecoder.sonder.decode(SonderProgressUpdate.self, from: body) else {
            return jsonResponse(["error": "Invalid progress payload"], status: "400 Bad Request")
        }
        library.updateProgress(itemID: id, seconds: payload.seconds, duration: payload.duration)
        return jsonResponse(["status": "ok"])
    }

    @MainActor
    private func streamResponse(path: String, rangeHeader: String?) -> Data {
        let rawID = URL(fileURLWithPath: path).lastPathComponent
        guard let id = UUID(uuidString: rawID),
              let item = library.item(id: id),
              let url = item.playableURL else {
            return jsonResponse(["error": "Media not found"], status: "404 Not Found")
        }
        let didAccess = url.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                url.stopAccessingSecurityScopedResource()
            }
        }
        guard let attributes = try? FileManager.default.attributesOfItem(atPath: url.path),
              let fileSize = attributes[.size] as? NSNumber,
              let handle = try? FileHandle(forReadingFrom: url) else {
            return jsonResponse(["error": "Media could not be opened"], status: "404 Not Found")
        }
        defer { try? handle.close() }

        let totalLength = fileSize.uint64Value
        let range = HTTPByteRange(header: rangeHeader, fileLength: totalLength)
        do {
            try handle.seek(toOffset: range.start)
            let body = try handle.read(upToCount: Int(range.length)) ?? Data()
            var headers = [
                "Accept-Ranges": "bytes",
                "Content-Type": item.contentType,
                "Content-Length": "\(body.count)"
            ]
            if range.isPartial {
                headers["Content-Range"] = "bytes \(range.start)-\(range.end)/\(totalLength)"
            }
            return httpResponse(status: range.isPartial ? "206 Partial Content" : "200 OK", headers: headers, body: body)
        } catch {
            return jsonResponse(["error": "Media could not be opened"], status: "404 Not Found")
        }
    }

    private func encodedResponse<T: Encodable>(_ value: T) -> Data {
        do {
            return httpResponse(contentType: "application/json", body: try JSONEncoder.sonder.encode(value))
        } catch {
            return jsonResponse(["error": "Could not encode response"], status: "500 Internal Server Error")
        }
    }

    private func jsonResponse(_ payload: [String: String], status: String = "200 OK") -> Data {
        let body = (try? JSONSerialization.data(withJSONObject: payload, options: [.prettyPrinted, .sortedKeys])) ?? Data("{}".utf8)
        return httpResponse(status: status, contentType: "application/json", body: body)
    }

    private func httpResponse(status: String = "200 OK", contentType: String, body: Data) -> Data {
        httpResponse(status: status, headers: ["Content-Type": contentType, "Content-Length": "\(body.count)"], body: body)
    }

    private func httpResponse(status: String = "200 OK", headers: [String: String], body: Data) -> Data {
        var response = Data()
        response.append(Data("HTTP/1.1 \(status)\r\n".utf8))
        for (key, value) in headers.sorted(by: { $0.key < $1.key }) {
            response.append(Data("\(key): \(value)\r\n".utf8))
        }
        response.append(Data("Connection: close\r\n".utf8))
        response.append(Data("Access-Control-Allow-Origin: http://127.0.0.1:\(activePort ?? 8797)\r\n".utf8))
        response.append(Data("Access-Control-Allow-Methods: GET, POST, OPTIONS\r\n".utf8))
        response.append(Data("Access-Control-Allow-Headers: Content-Type\r\n".utf8))
        response.append(Data("\r\n".utf8))
        response.append(body)
        return response
    }

    private var webInterface: String {
        """
        <!doctype html>
        <html>
        <head>
          <meta charset="utf-8">
          <meta name="viewport" content="width=device-width, initial-scale=1">
          <title>TM Sonder</title>
          <style>
            :root { color-scheme: dark; --bg:#10130f; --panel:#171d15; --panel2:#202819; --line:#344128; --text:#eff4e8; --muted:#a8b39c; --accent:#b6d56d; }
            body { margin:0; font:14px -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:var(--bg); color:var(--text); }
            header { position:sticky; top:0; z-index:1; display:flex; align-items:center; gap:16px; padding:18px 22px; background:rgba(16,19,15,.94); border-bottom:1px solid var(--line); }
            h1 { margin:0; font-size:26px; }
            input, select { background:var(--panel); border:1px solid var(--line); color:var(--text); border-radius:8px; padding:9px 11px; }
            main { display:grid; grid-template-columns:repeat(auto-fill,minmax(220px,1fr)); gap:16px; padding:22px; }
            article { background:var(--panel); border:1px solid var(--line); border-radius:8px; overflow:hidden; }
            .poster { aspect-ratio:2/3; display:grid; place-items:center; background:linear-gradient(145deg,#334124,#11160f); color:var(--accent); font-size:42px; }
            .body { padding:13px; }
            h2 { margin:0 0 4px; font-size:17px; }
            p { margin:0 0 10px; color:var(--muted); line-height:1.35; }
            progress { width:100%; accent-color:var(--accent); }
            video { width:100%; margin-top:10px; background:#050604; border-radius:6px; }
            .pill { display:inline-block; color:#11160f; background:var(--accent); border-radius:4px; padding:3px 6px; font-size:11px; font-weight:700; margin-bottom:8px; }
          </style>
        </head>
        <body>
          <header>
            <h1>TM Sonder</h1>
            <input id="q" placeholder="Search movies, shows, documentaries">
            <select id="kind"><option>All</option><option>Movie</option><option>TV Show</option><option>Documentary</option></select>
          </header>
          <main id="grid"></main>
          <script>
            let items = [];
            const grid = document.querySelector("#grid");
            const q = document.querySelector("#q");
            const kind = document.querySelector("#kind");
            function render() {
              const term = q.value.toLowerCase();
              const selected = kind.value;
              grid.innerHTML = items.filter(i => (selected === "All" || i.kind === selected) && JSON.stringify(i).toLowerCase().includes(term)).map(i => `
                <article>
                  <div class="poster">▶</div>
                  <div class="body">
                    <span class="pill">${escapeHTML(i.kind)}</span>
                    <h2>${escapeHTML(i.title)}</h2>
                    <p>${escapeHTML(i.subtitle)}</p>
                    <progress max="${Math.max(i.durationSeconds, 1)}" value="${progressFor(i.id).seconds}"></progress>
                    ${i.hasFile ? `<video controls src="/stream/${i.id}" data-id="${i.id}" onloadedmetadata="resumeProgress(this)" ontimeupdate="saveProgress('${i.id}', this.currentTime, this.duration)"></video>` : `<p>Import a playable file in the Mac app to stream here.</p>`}
                  </div>
                </article>`).join("");
            }
            function escapeHTML(value) {
              return String(value ?? "").replace(/[&<>"']/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" }[c]));
            }
            function progressFor(id) {
              return progressByID.get(id) ?? { seconds:0, duration:1 };
            }
            function resumeProgress(video) {
              const progress = progressFor(video.dataset.id);
              if (progress.seconds > 5 && progress.seconds < Math.max(video.duration - 8, 0)) {
                video.currentTime = progress.seconds;
              }
            }
            async function saveProgress(id, seconds, duration) {
              if (!duration || Math.floor(seconds) % 15 !== 0) return;
              await fetch(`/api/progress/${id}`, { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({seconds, duration}) });
            }
            const progressByID = new Map();
            fetch("/api/library").then(r => r.json()).then(data => {
              items = data.items;
              (data.progress ?? []).forEach(p => progressByID.set(p.itemID, p));
              render();
            });
            q.oninput = render; kind.onchange = render;
          </script>
        </body>
        </html>
        """
    }
}

private struct HTTPRequest {
    var method = "GET"
    var path = "/"
    var headers: [String: String] = [:]
    var body = Data()

    init(data: Data) {
        guard let raw = String(data: data, encoding: .utf8) else { return }
        let parts = raw.components(separatedBy: "\r\n\r\n")
        if let firstLine = parts.first?.split(separator: "\r\n", maxSplits: 1).first {
            let tokens = firstLine.split(separator: " ")
            if tokens.count >= 2 {
                method = String(tokens[0])
                path = String(tokens[1]).removingPercentEncoding ?? String(tokens[1])
            }
        }
        for line in (parts.first ?? "").components(separatedBy: "\r\n").dropFirst() {
            guard let separator = line.firstIndex(of: ":") else { continue }
            let key = line[..<separator].trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let value = line[line.index(after: separator)...].trimmingCharacters(in: .whitespacesAndNewlines)
            headers[key] = value
        }
        if parts.count > 1 {
            body = Data(parts.dropFirst().joined(separator: "\r\n\r\n").utf8)
        }
    }

    var isLocalhostRequest: Bool {
        guard let host = headers["host"]?.lowercased() else { return true }
        return host.hasPrefix("127.0.0.1") || host.hasPrefix("localhost") || host.hasPrefix("[::1]")
    }
}

private struct HTTPByteRange {
    var start: UInt64
    var end: UInt64
    var fileLength: UInt64
    var isPartial: Bool

    var length: UInt64 {
        guard end >= start else { return 0 }
        return end - start + 1
    }

    init(header: String?, fileLength: UInt64) {
        self.fileLength = fileLength
        guard fileLength > 0 else {
            start = 0
            end = 0
            isPartial = false
            return
        }

        let fullEnd = fileLength - 1
        guard let header,
              header.lowercased().hasPrefix("bytes=") else {
            start = 0
            end = fullEnd
            isPartial = false
            return
        }

        let rawRange = header.dropFirst("bytes=".count).split(separator: ",").first.map(String.init) ?? ""
        let bounds = rawRange.split(separator: "-", omittingEmptySubsequences: false)
        if bounds.count == 2, let requestedStart = UInt64(bounds[0]) {
            start = min(requestedStart, fullEnd)
            end = bounds[1].isEmpty ? fullEnd : min(UInt64(bounds[1]) ?? fullEnd, fullEnd)
            if end < start { end = start }
            isPartial = true
        } else {
            start = 0
            end = fullEnd
            isPartial = false
        }
    }
}

private struct SonderLibraryResponse: Codable {
    var items: [SonderMediaItem]
    var progress: [SonderProgress]
}

private struct SonderProgressUpdate: Codable {
    var seconds: Double
    var duration: Double
}
