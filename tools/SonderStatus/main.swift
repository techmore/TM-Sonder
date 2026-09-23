import AppKit

private struct ServerStatus: Decodable {
    let scanning: Bool
    let enriching: Bool
    let itemCount: Int
}

private struct RuntimeState: Decodable {
    let selectedMode: String
    let selectedInterface: String
    let selectedIPv4: String
    let webBindAddress: String
    let apiBindAddress: String
    let webPort: Int
    let apiPort: Int
    let publicDomain: String
    let caddyEnabled: Bool
    let caddyUpstream: String
    let processStartTime: Date?
    let lastError: String?
}

private struct NetworkInterface: Decodable {
    let id: String
    let name: String
    let type: String
    let ipv4: String
    let active: Bool
    let bindingModes: [String]
}

private struct CaddyStatus: Decodable {
    let enabled: Bool
    let configured: Bool
    let domain: String
    let upstream: String
    let listening: Bool
    let publicHealthy: Bool
    let error: String?
}

private struct NetworkStatus: Decodable {
    let state: RuntimeState
    let interfaces: [NetworkInterface]
    let webHealthy: Bool
    let apiHealthy: Bool
    let rebinding: Bool
    let caddy: CaddyStatus
    let databasePublic: Bool
    let publicPrerequisites: [String]
    let uptimeSeconds: Int64
    let lastError: String?
}

private struct NetworkEnvelope: Decodable {
    let version: String
    let build: String
    let status: NetworkStatus
}

private struct APIError: Decodable {
    let error: String?
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
    private let versionLine = NSMenuItem(title: "Version: —", action: nil, keyEquivalent: "")
    private let networkLine = NSMenuItem(title: "Network: —", action: nil, keyEquivalent: "")
    private let webLine = NSMenuItem(title: "Web: —", action: nil, keyEquivalent: "")
    private let apiLine = NSMenuItem(title: "API: —", action: nil, keyEquivalent: "")
    private let uptimeLine = NSMenuItem(title: "Uptime: —", action: nil, keyEquivalent: "")
    private let publicLine = NSMenuItem(title: "Public pulse: —", action: nil, keyEquivalent: "")
    private let bindMenu = NSMenu()
    private var timer: Timer?
    private var request: Task<Void, Never>?
    private var baseURL = URL(string: "http://127.0.0.1:8797")!
    private var currentNetwork: NetworkEnvelope?
    private var bindingInProgress = false
    private var lastAlertedError = ""

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        let menu = NSMenu()
        menu.addItem(statusLine)
        menu.addItem(detailLine)
        menu.addItem(.separator())
        menu.addItem(versionLine)
        menu.addItem(networkLine)
        menu.addItem(webLine)
        menu.addItem(apiLine)
        menu.addItem(uptimeLine)
        menu.addItem(publicLine)
        let bindItem = NSMenuItem(title: "Bind interface", action: nil, keyEquivalent: "")
        bindItem.submenu = bindMenu
        menu.addItem(bindItem)
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
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
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
        var port = 8798
        var pairingToken: String?
        if let data = try? Data(contentsOf: config),
           let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            let webPort = (object["webPort"] as? Int) ?? (object["port"] as? Int) ?? 8797
            let configuredPort = (object["apiPort"] as? Int) ?? (webPort + 1)
            if (1...65535).contains(configuredPort) {
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
                async let server: ServerStatus = fetch(connection: connection, path: "api/status")
                async let network: NetworkEnvelope = fetch(connection: connection, path: "api/network/status")
                let (serverStatus, networkStatus) = try await (server, network)
                render(server: serverStatus, network: networkStatus)
            } catch {
                display("Server unreachable", symbol: "exclamationmark.triangle", detail: "No response on port \(self.baseURL.port ?? 8797)")
                resetDetails()
            }
        }
    }

    private func fetch<T: Decodable>(connection: ServerConnection, path: String) async throws -> T {
        var query = URLRequest(
            url: connection.url(path: path),
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
            if statusCode == 401 {
                throw NSError(domain: "SonderStatus", code: statusCode, userInfo: [NSLocalizedDescriptionKey: "Pairing token unavailable"])
            }
            throw NSError(domain: "SonderStatus", code: statusCode, userInfo: [NSLocalizedDescriptionKey: "Unexpected response (\(statusCode))"])
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(T.self, from: data)
    }

    private func render(server: ServerStatus, network envelope: NetworkEnvelope) {
        let status = envelope.status
        currentNetwork = envelope
        let label = server.scanning ? "Scanning library" : server.enriching ? "Updating metadata" : status.webHealthy ? "Server running" : "Server needs attention"
        let symbol = server.scanning || server.enriching ? "arrow.triangle.2.circlepath" : status.webHealthy ? "checkmark.circle" : "exclamationmark.triangle"
        display(label, symbol: symbol, detail: "\(server.itemCount.formatted()) items")
        versionLine.title = "Version: \(envelope.version) (\(envelope.build))"
        networkLine.title = "Network: \(status.state.selectedMode) · \(status.state.selectedInterface) · \(status.state.selectedIPv4)"
        webLine.title = "Web: \(status.state.webBindAddress):\(status.state.webPort) · \(status.webHealthy ? "up" : "down")"
        apiLine.title = "API: \(status.state.apiBindAddress):\(status.state.apiPort) · \(status.apiHealthy ? "ready" : "down")"
        uptimeLine.title = "Uptime: \(formatUptime(status.uptimeSeconds))"
        let publicState: String
        if !status.caddy.enabled {
            publicState = "Caddy disabled"
        } else if status.caddy.publicHealthy {
            publicState = "healthy · \(status.caddy.domain)"
        } else if status.caddy.listening {
            publicState = "listening · public check unavailable"
        } else {
            publicState = "down"
        }
        publicLine.title = "Public pulse: \(publicState)"
        rebuildBindingMenu(status: status)
        if let error = status.lastError, !error.isEmpty, error != lastAlertedError {
            lastAlertedError = error
            showAlert(title: "Sonder network operation failed", message: error)
        } else if let error = status.caddy.error, !error.isEmpty, error != lastAlertedError {
            lastAlertedError = error
            showAlert(title: "Sonder Caddy error", message: error)
        }
    }

    private func rebuildBindingMenu(status: NetworkStatus) {
        bindMenu.removeAllItems()
        let modes: [(String, String, String)] = [
            ("Loopback", "loopback", ""),
            ("Wi‑Fi / LAN", "wifi/lan", ""),
            ("Ethernet", "ethernet", ""),
            ("VPN", "vpn", ""),
            ("Public", "public", "")
        ]
        for (title, mode, interfaceID) in modes {
            addBindingItem(title: title, mode: mode, interfaceID: interfaceID, selected: status.state.selectedMode == mode && interfaceID.isEmpty)
        }
        bindMenu.addItem(.separator())
        for interface in status.interfaces where interface.active {
            let title = "\(interface.id) · \(interface.ipv4)"
            addBindingItem(title: title, mode: "interface", interfaceID: interface.id, selected: status.state.selectedInterface == interface.id)
        }
        for item in bindMenu.items {
            item.isEnabled = !bindingInProgress && !status.rebinding
        }
    }

    private func addBindingItem(title: String, mode: String, interfaceID: String, selected: Bool) {
        let entry = NSMenuItem(title: selected ? "✓ \(title)" : title, action: #selector(selectBinding(_:)), keyEquivalent: "")
        entry.target = self
        entry.representedObject = ["mode": mode, "interfaceID": interfaceID]
        bindMenu.addItem(entry)
    }

    @objc private func selectBinding(_ sender: NSMenuItem) {
        guard !bindingInProgress,
              let values = sender.representedObject as? [String: String],
              let mode = values["mode"] else { return }
        let interfaceID = values["interfaceID"] ?? ""
        bindingInProgress = true
        for item in bindMenu.items { item.isEnabled = false }
        display("Rebinding web + Caddy…", symbol: "arrow.triangle.2.circlepath", detail: "Validating \(interfaceID.isEmpty ? mode : interfaceID)")
        let connection = configuredServer()
        request = Task { [weak self] in
            guard let self else { return }
            defer {
                self.request = nil
                self.bindingInProgress = false
            }
            do {
                var request = URLRequest(url: connection.url(path: "api/network/rebind"), timeoutInterval: 5)
                request.httpMethod = "POST"
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                if let pairingToken = connection.pairingToken {
                    request.setValue("Bearer \(pairingToken)", forHTTPHeaderField: "Authorization")
                }
                request.httpBody = try JSONSerialization.data(withJSONObject: ["mode": mode, "interfaceID": interfaceID])
                let (data, response) = try await URLSession.shared.data(for: request)
                let statusCode = (response as? HTTPURLResponse)?.statusCode ?? -1
                guard (200..<300).contains(statusCode) else {
                    let message = (try? JSONDecoder().decode(APIError.self, from: data)).flatMap(\.error) ?? "Rebind failed (HTTP \(statusCode))"
                    throw NSError(domain: "SonderStatus", code: statusCode, userInfo: [NSLocalizedDescriptionKey: message])
                }
                try await Task.sleep(for: .seconds(1))
                self.refresh()
            } catch {
                self.showAlert(title: "Sonder could not rebind", message: error.localizedDescription)
                self.refresh()
            }
        }
    }

    private func resetDetails() {
        currentNetwork = nil
        versionLine.title = "Version: —"
        networkLine.title = "Network: —"
        webLine.title = "Web: —"
        apiLine.title = "API: —"
        uptimeLine.title = "Uptime: —"
        publicLine.title = "Public pulse: —"
        bindMenu.removeAllItems()
    }

    private func formatUptime(_ seconds: Int64) -> String {
        guard seconds > 0 else { return "—" }
        let days = seconds / 86_400
        let hours = (seconds % 86_400) / 3_600
        let minutes = (seconds % 3_600) / 60
        if days > 0 { return "\(days)d \(hours)h \(minutes)m" }
        if hours > 0 { return "\(hours)h \(minutes)m" }
        return "\(minutes)m \(seconds % 60)s"
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

    private func showAlert(title: String, message: String) {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = title
        alert.informativeText = message
        alert.addButton(withTitle: "OK")
        alert.runModal()
    }

    private func menuIcon() -> NSImage? {
        guard let path = Bundle.main.path(forResource: "TM-Sonder", ofType: "png"),
              let image = NSImage(contentsOfFile: path) else { return nil }
        image.size = NSSize(width: 18, height: 18)
        return image
    }

    @objc private func openSonder() {
        let connection = configuredServer()
        let state = currentNetwork?.status.state
        var host = state?.webBindAddress ?? "127.0.0.1"
        if host == "0.0.0.0" || host == "::" || host.isEmpty {
            host = "127.0.0.1"
        }
        if host != "127.0.0.1", let selected = state?.selectedIPv4, !selected.isEmpty {
            host = selected
        }
        var components = URLComponents()
        components.scheme = "http"
        components.host = host
        components.port = state?.webPort ?? 8797
        components.path = "/"
        if let pairingToken = connection.pairingToken, !pairingToken.isEmpty {
            components.queryItems = [URLQueryItem(name: "token", value: pairingToken)]
        }
        if let url = components.url {
            NSWorkspace.shared.open(url)
        }
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
