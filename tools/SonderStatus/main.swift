import AppKit

struct Status: Decodable {
    let scanning: Bool
    let enriching: Bool
    let itemCount: Int
}

private struct ServerConnection {
    let baseURL: URL
    let pairingToken: String?

    func url(path: String, includingPairingToken: Bool = false) -> URL {
        var components = URLComponents(
            url: baseURL.appendingPathComponent(path),
            resolvingAgainstBaseURL: false
        )!
        if includingPairingToken, let pairingToken, !pairingToken.isEmpty {
            components.queryItems = [URLQueryItem(name: "token", value: pairingToken)]
        }
        return components.url!
    }
}

@MainActor
final class StatusApp: NSObject, NSApplicationDelegate {
    private var item: NSStatusItem!
    private let statusLine = NSMenuItem(title: "Checking server…", action: nil, keyEquivalent: "")
    private let detailLine = NSMenuItem(title: "", action: nil, keyEquivalent: "")
    private var timer: Timer?
    private var request: Task<Void, Never>?
    private var baseURL = URL(string: "http://127.0.0.1:8797")!

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        let menu = NSMenu()
        menu.addItem(statusLine)
        menu.addItem(detailLine)
        menu.addItem(.separator())
        add("Open Sonder", #selector(openSonder), to: menu)
        add("Refresh Status", #selector(refresh), to: menu)
        add("Open Server Logs", #selector(openLogs), to: menu)
        menu.addItem(.separator())
        let note = NSMenuItem(title: "Quitting this indicator leaves the server running", action: nil, keyEquivalent: "")
        menu.addItem(note)
        add("Quit Status Indicator", #selector(quit), to: menu)
        item.menu = menu
        display("Checking server…", symbol: "server.rack", detail: "")
        refresh()
        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
    }

    private func add(_ title: String, _ action: Selector, to menu: NSMenu) {
        let entry = NSMenuItem(title: title, action: action, keyEquivalent: "")
        entry.target = self
        menu.addItem(entry)
    }

    private func configuredServer() -> ServerConnection {
        let config = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".config/sonder/server.json")
        var port = 8797
        var pairingToken: String?
        if let data = try? Data(contentsOf: config),
           let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if let configuredPort = object["port"] as? Int, (1...65535).contains(configuredPort) {
                port = configuredPort
            }
            if let configuredToken = object["pairingToken"] as? String, !configuredToken.isEmpty {
                pairingToken = configuredToken
            }
        }
        return ServerConnection(
            baseURL: URL(string: "http://127.0.0.1:\(port)")!,
            pairingToken: pairingToken
        )
    }

    @objc private func refresh() {
        guard request == nil else { return }
        let connection = configuredServer()
        baseURL = connection.baseURL
        request = Task { [weak self] in
            guard let self else { return }
            defer { self.request = nil }
            do {
                var query = URLRequest(
                    url: connection.url(path: "api/status"),
                    cachePolicy: .reloadIgnoringLocalCacheData,
                    timeoutInterval: 4
                )
                query.httpMethod = "GET"
                if let pairingToken = connection.pairingToken {
                    query.setValue("Bearer \(pairingToken)", forHTTPHeaderField: "Authorization")
                }
                let (data, response) = try await URLSession.shared.data(for: query)
                let statusCode = (response as? HTTPURLResponse)?.statusCode ?? -1
                guard statusCode == 200 else {
                    let detail = statusCode == 401
                        ? "Pairing token unavailable · \(self.baseURL.host ?? "localhost")"
                        : "Unexpected response (\(statusCode)) · \(self.baseURL.host ?? "localhost")"
                    self.display("Server needs attention", symbol: "exclamationmark.triangle", detail: detail)
                    return
                }
                let status = try JSONDecoder().decode(Status.self, from: data)
                let label = status.scanning ? "Scanning library" : status.enriching ? "Updating metadata" : "Server running"
                self.display(label, symbol: status.scanning || status.enriching ? "arrow.triangle.2.circlepath" : "checkmark.circle", detail: "\(status.itemCount.formatted()) items · port \(self.baseURL.port ?? 8797)")
            } catch {
                self.display("Server unreachable", symbol: "exclamationmark.triangle", detail: "No response on port \(self.baseURL.port ?? 8797)")
            }
        }
    }

    private func display(_ text: String, symbol: String, detail: String) {
        statusLine.title = "Sonder · \(text)"
        detailLine.title = detail
        item.button?.title = ""
        item.button?.image = menuIcon() ?? NSImage(systemSymbolName: symbol, accessibilityDescription: text)
        item.button?.image?.isTemplate = false
        item.button?.toolTip = "Sonder: \(text). \(detail)"
        item.button?.setAccessibilityLabel("Sonder: \(text)")
    }

    private func menuIcon() -> NSImage? {
        guard let path = Bundle.main.path(forResource: "TM-Sonder", ofType: "png"),
              let image = NSImage(contentsOfFile: path) else { return nil }
        image.size = NSSize(width: 18, height: 18)
        return image
    }

    @objc private func openSonder() {
        let connection = configuredServer()
        NSWorkspace.shared.open(connection.url(path: "", includingPairingToken: true))
    }
    @objc private func openLogs() {
        let manager = FileManager.default
        let folder = manager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/TM-Sonder-Server/logs")
        let fallback = manager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/sonder.log")
        NSWorkspace.shared.open(manager.fileExists(atPath: folder.path) ? folder : fallback)
    }
    @objc private func quit() { NSApp.terminate(nil) }
}

let app = NSApplication.shared
let delegate = StatusApp()
app.delegate = delegate
app.run()
