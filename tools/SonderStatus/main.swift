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

private struct BrewInfo {
    let formula: String
    let installedVersion: String?
    let availableVersion: String?
    let outdated: Bool
}

private struct BrewCommandResult {
    let status: Int32
    let output: String
}

private final class CommandOutputBuffer: @unchecked Sendable {
    private let lock = NSLock()
    private var data = Data()

    func append(_ newData: Data) {
        lock.lock()
        data.append(newData)
        lock.unlock()
    }

    func value() -> String {
        lock.lock()
        defer { lock.unlock() }
        return String(data: data, encoding: .utf8) ?? ""
    }
}

@MainActor
private func menuLabel(_ text: String, size: CGFloat, weight: NSFont.Weight = .regular, color: NSColor = .labelColor) -> NSTextField {
    let label = NSTextField(labelWithString: text)
    label.font = .systemFont(ofSize: size, weight: weight)
    label.textColor = color
    label.lineBreakMode = .byTruncatingTail
    label.maximumNumberOfLines = 1
    label.translatesAutoresizingMaskIntoConstraints = false
    return label
}

private final class MenuSectionView: NSView {
    private let label: NSTextField

    init(_ title: String) {
        label = menuLabel(title, size: 10, weight: .semibold, color: .secondaryLabelColor)
        super.init(frame: .zero)
        label.stringValue = title.uppercased()
        addSubview(label)
        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 6),
            label.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor, constant: -6),
            label.topAnchor.constraint(equalTo: topAnchor, constant: 7),
            label.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -2),
            widthAnchor.constraint(equalToConstant: 320),
            heightAnchor.constraint(equalToConstant: 25),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
}

private final class StatusHeaderView: NSView {
    private let iconView = NSImageView()
    private let titleLabel = menuLabel("TM Sonder", size: 16, weight: .semibold)
    private let statusLabel = menuLabel("Checking server…", size: 12, weight: .semibold)
    private let detailLabel = menuLabel("", size: 11, color: .secondaryLabelColor)
    private let statusDot = NSView()

    override var intrinsicContentSize: NSSize { NSSize(width: 320, height: 70) }

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        wantsLayer = true
        iconView.imageScaling = .scaleProportionallyUpOrDown
        iconView.wantsLayer = true
        iconView.layer?.cornerRadius = 9
        iconView.layer?.masksToBounds = true
        iconView.translatesAutoresizingMaskIntoConstraints = false

        statusDot.wantsLayer = true
        statusDot.layer?.cornerRadius = 4
        statusDot.translatesAutoresizingMaskIntoConstraints = false

        let statusRow = NSStackView(views: [statusDot, statusLabel])
        statusRow.orientation = .horizontal
        statusRow.alignment = .centerY
        statusRow.spacing = 6
        statusRow.translatesAutoresizingMaskIntoConstraints = false

        let copy = NSStackView(views: [titleLabel, statusRow, detailLabel])
        copy.orientation = .vertical
        copy.alignment = .leading
        copy.spacing = 3
        copy.translatesAutoresizingMaskIntoConstraints = false

        addSubview(iconView)
        addSubview(copy)
        NSLayoutConstraint.activate([
            iconView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 10),
            iconView.centerYAnchor.constraint(equalTo: centerYAnchor),
            iconView.widthAnchor.constraint(equalToConstant: 40),
            iconView.heightAnchor.constraint(equalToConstant: 40),
            statusDot.widthAnchor.constraint(equalToConstant: 8),
            statusDot.heightAnchor.constraint(equalToConstant: 8),
            copy.leadingAnchor.constraint(equalTo: iconView.trailingAnchor, constant: 11),
            copy.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -12),
            copy.centerYAnchor.constraint(equalTo: centerYAnchor),
            widthAnchor.constraint(equalToConstant: 320),
            heightAnchor.constraint(equalToConstant: 70),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    func setIcon(_ image: NSImage?) { iconView.image = image }

    func update(status: String, detail: String, color: NSColor) {
        statusLabel.stringValue = status
        detailLabel.stringValue = detail
        statusDot.layer?.backgroundColor = color.cgColor
    }

    func setDetail(_ detail: String) { detailLabel.stringValue = detail }
}

private final class MenuMetricsView: NSView {
    private let version = menuLabel("—", size: 12, weight: .medium)
    private let library = menuLabel("—", size: 12, weight: .medium)
    private let uptime = menuLabel("—", size: 12, weight: .medium)
    private let network = menuLabel("—", size: 12, weight: .medium)

    override var intrinsicContentSize: NSSize { NSSize(width: 320, height: 70) }

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        let grid = NSGridView(views: [
            [cell("VERSION", version), cell("LIBRARY", library)],
            [cell("UPTIME", uptime), cell("NETWORK", network)],
        ])
        grid.rowSpacing = 9
        grid.columnSpacing = 18
        grid.translatesAutoresizingMaskIntoConstraints = false
        addSubview(grid)
        NSLayoutConstraint.activate([
            grid.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 12),
            grid.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -12),
            grid.topAnchor.constraint(equalTo: topAnchor, constant: 5),
            grid.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -5),
            widthAnchor.constraint(equalToConstant: 320),
            heightAnchor.constraint(equalToConstant: 70),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    private func cell(_ title: String, _ value: NSTextField) -> NSView {
        let heading = menuLabel(title, size: 9, weight: .semibold, color: .tertiaryLabelColor)
        let stack = NSStackView(views: [heading, value])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 2
        stack.translatesAutoresizingMaskIntoConstraints = false
        return stack
    }

    func update(version: String, library: String, uptime: String, network: String) {
        self.version.stringValue = version
        self.library.stringValue = library
        self.uptime.stringValue = uptime
        self.network.stringValue = network
    }
}

private final class MenuConnectionsView: NSView {
    private let web = menuLabel("Web: —", size: 11, color: .secondaryLabelColor)
    private let api = menuLabel("API: —", size: 11, color: .secondaryLabelColor)
    private let publicPulse = menuLabel("Public: —", size: 11, color: .secondaryLabelColor)

    override var intrinsicContentSize: NSSize { NSSize(width: 320, height: 56) }

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        let stack = NSStackView(views: [web, api, publicPulse])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 3
        stack.translatesAutoresizingMaskIntoConstraints = false
        addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 12),
            stack.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -12),
            stack.topAnchor.constraint(equalTo: topAnchor, constant: 4),
            stack.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -4),
            widthAnchor.constraint(equalToConstant: 320),
            heightAnchor.constraint(equalToConstant: 56),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    func update(web: String, api: String, publicPulse: String) {
        self.web.stringValue = "Web: \(web)"
        self.api.stringValue = "API: \(api)"
        self.publicPulse.stringValue = "Public: \(publicPulse)"
    }
}

@MainActor
final class StatusApp: NSObject, NSApplicationDelegate {
    private var item: NSStatusItem!
    private let headerView = StatusHeaderView()
    private let metricsView = MenuMetricsView()
    private let connectionsView = MenuConnectionsView()
    private let updateLine = NSMenuItem(title: "Updates: Not checked", action: nil, keyEquivalent: "")
    private let bindMenu = NSMenu()
    private var checkUpdatesItem: NSMenuItem!
    private var installUpdateItem: NSMenuItem!
    private var scanItem: NSMenuItem!
    private var timer: Timer?
    private var request: Task<Void, Never>?
    private var updateCheckTask: Task<Void, Never>?
    private var updateTask: Task<Void, Never>?
    private var baseURL = URL(string: "http://127.0.0.1:8096")!
    private var currentNetwork: NetworkEnvelope?
    private var bindingInProgress = false
    private var scanInProgress = false
    private var updateCheckInProgress = false
    private var updateInProgress = false
    private var updateInfo: BrewInfo?
    private var lastUpdateCheck: Date?
    private var manualRestartVersion: String?
    private var lastAlertedError = ""

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        let menu = NSMenu()
        let headerItem = NSMenuItem()
        headerItem.view = headerView
        menu.addItem(headerItem)
        menu.addItem(.separator())
        let metricsItem = NSMenuItem()
        metricsItem.view = metricsView
        menu.addItem(metricsItem)
        let connectionsItem = NSMenuItem()
        connectionsItem.view = connectionsView
        menu.addItem(connectionsItem)
        menu.addItem(.separator())

        menu.addItem(section("SERVER"))
        add("Open Sonder", #selector(openSonder), to: menu, symbol: "safari")
        scanItem = add("Scan Library", #selector(scanLibrary), to: menu, symbol: "arrow.triangle.2.circlepath")
        add("Refresh Status", #selector(refresh), to: menu, symbol: "arrow.clockwise")

        let networkMenu = NSMenu()
        let bindItem = NSMenuItem(title: "Bind interface", action: nil, keyEquivalent: "")
        bindItem.image = menuSymbol("network")
        bindItem.submenu = bindMenu
        networkMenu.addItem(bindItem)
        networkMenu.addItem(.separator())
        add("Change web port…", #selector(changeWebPort), to: networkMenu, symbol: "arrow.left.and.right")
        add("Change API port…", #selector(changeAPIPort), to: networkMenu, symbol: "lock.shield")
        let accessItem = NSMenuItem(title: "Network & Access", action: nil, keyEquivalent: "")
        accessItem.image = menuSymbol("network")
        accessItem.submenu = networkMenu
        menu.addItem(accessItem)

        menu.addItem(.separator())
        menu.addItem(section("UPDATES"))
        updateLine.image = menuSymbol("arrow.down.circle")
        menu.addItem(updateLine)
        checkUpdatesItem = add("Check for Updates…", #selector(checkForUpdates), to: menu)
        checkUpdatesItem.image = menuSymbol("magnifyingglass")
        installUpdateItem = add("Install Update…", #selector(installUpdate), to: menu)
        installUpdateItem.image = menuSymbol("sparkles")
        installUpdateItem.isEnabled = false

        menu.addItem(.separator())
        menu.addItem(section("TOOLS"))
        add("Open Server Logs", #selector(openLogs), to: menu, symbol: "doc.text.magnifyingglass")
        menu.addItem(.separator())
        let note = NSMenuItem(title: "Quitting this indicator leaves the server running", action: nil, keyEquivalent: "")
        note.isEnabled = false
        menu.addItem(note)
        add("Quit Status Indicator", #selector(quit), to: menu, symbol: "power")
        item.menu = menu
        headerView.setIcon(menuIcon(size: 40))
        display("Checking server…", symbol: "server.rack", detail: "")
        updateLine.title = "Updates: Checking Homebrew…"
        refreshMenuInteractivity()
        refresh()
        beginUpdateCheck(refreshHomebrew: false, showFailureAlert: false)
        timer = Timer.scheduledTimer(withTimeInterval: 30, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.refresh()
                if self.lastUpdateCheck == nil || Date().timeIntervalSince(self.lastUpdateCheck!) > 600 {
                    self.beginUpdateCheck(refreshHomebrew: false, showFailureAlert: false)
                }
            }
        }
    }

    @discardableResult
    private func add(_ title: String, _ action: Selector, to menu: NSMenu, symbol: String? = nil) -> NSMenuItem {
        let entry = NSMenuItem(title: title, action: action, keyEquivalent: "")
        entry.target = self
        if let symbol { entry.image = menuSymbol(symbol) }
        menu.addItem(entry)
        return entry
    }

    private func section(_ title: String) -> NSMenuItem {
        let item = NSMenuItem()
        item.view = MenuSectionView(title)
        return item
    }

    private func menuSymbol(_ name: String) -> NSImage? {
        let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)
        image?.isTemplate = true
        image?.size = NSSize(width: 16, height: 16)
        return image
    }

    private func configuredServer() -> ServerConnection {
        let config = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".config/sonder/server.json")
        var port = 8097
        var pairingToken: String?
        var dataDirectory = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/TM-Sonder-Server")
        if let data = try? Data(contentsOf: config),
           let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            let webPort = (object["webPort"] as? Int) ?? (object["port"] as? Int) ?? 8096
            let configuredPort = (object["apiPort"] as? Int) ?? (webPort + 1)
            if (1...65535).contains(configuredPort) {
                port = configuredPort
            }
            if let configuredToken = object["pairingToken"] as? String, !configuredToken.isEmpty {
                pairingToken = configuredToken
            }
            if let configuredDataDirectory = object["dataDir"] as? String, !configuredDataDirectory.isEmpty {
                dataDirectory = URL(fileURLWithPath: (configuredDataDirectory as NSString).expandingTildeInPath)
            }
        }
        let runtimeState = dataDirectory.appendingPathComponent("runtime-state.json")
        if let data = try? Data(contentsOf: runtimeState),
           let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let configuredPort = object["apiPort"] as? Int,
           (1...65535).contains(configuredPort) {
            port = configuredPort
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
                display("Server unreachable", symbol: "exclamationmark.triangle", detail: "No response on port \(self.baseURL.port ?? 8096)")
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

    @objc private func scanLibrary() {
        guard request == nil, !bindingInProgress, !scanInProgress, !updateCheckInProgress, !updateInProgress else { return }
        scanInProgress = true
        refreshMenuInteractivity()
        display("Starting library scan", symbol: "arrow.triangle.2.circlepath", detail: "Reconciling media files…")
        let connection = configuredServer()
        request = Task { [weak self] in
            guard let self else { return }
            defer {
                self.request = nil
                self.scanInProgress = false
                self.refreshMenuInteractivity()
            }
            do {
                var request = URLRequest(url: connection.url(path: "api/settings/rescan"), timeoutInterval: 5)
                request.httpMethod = "POST"
                if let pairingToken = connection.pairingToken {
                    request.setValue("Bearer \(pairingToken)", forHTTPHeaderField: "Authorization")
                }
                let (data, response) = try await URLSession.shared.data(for: request)
                let statusCode = (response as? HTTPURLResponse)?.statusCode ?? -1
                guard statusCode == 202 || statusCode == 409 else {
                    let message = (try? JSONDecoder().decode(APIError.self, from: data)).flatMap(\.error) ?? "Scan could not start (HTTP \(statusCode))"
                    throw NSError(domain: "SonderStatus", code: statusCode, userInfo: [NSLocalizedDescriptionKey: message])
                }
                self.display("Scanning library", symbol: "arrow.triangle.2.circlepath", detail: statusCode == 409 ? "A scan is already running" : "Reconciliation started")
                try? await Task.sleep(for: .seconds(1))
                self.request = nil
                self.scanInProgress = false
                self.refreshMenuInteractivity()
                self.refresh()
            } catch is CancellationError {
                // The menu-bar app may be terminated while a scan request is in flight.
            } catch {
                self.display("Scan failed", symbol: "exclamationmark.triangle", detail: error.localizedDescription)
                self.showAlert(title: "Sonder could not start a scan", message: error.localizedDescription)
                self.refresh()
            }
        }
    }

    private func render(server: ServerStatus, network envelope: NetworkEnvelope) {
        let status = envelope.status
        currentNetwork = envelope
        let label = server.scanning ? "Scanning library" : server.enriching ? "Updating metadata" : status.webHealthy ? "Server running" : "Server needs attention"
        let symbol = server.scanning || server.enriching ? "arrow.triangle.2.circlepath" : status.webHealthy ? "checkmark.circle" : "exclamationmark.triangle"
        if !updateInProgress {
            display(label, symbol: symbol, detail: "\(server.itemCount.formatted()) items")
        }
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
        metricsView.update(
            version: "\(envelope.version) · \(envelope.build)",
            library: "\(server.itemCount.formatted()) items",
            uptime: formatUptime(status.uptimeSeconds),
            network: status.state.selectedInterface.isEmpty ? status.state.selectedMode : status.state.selectedInterface
        )
        connectionsView.update(
            web: "\(status.state.webBindAddress):\(status.state.webPort) · \(status.webHealthy ? "up" : "down")",
            api: "\(status.state.apiBindAddress):\(status.state.apiPort) · \(status.apiHealthy ? "ready" : "down")",
            publicPulse: publicState
        )
        rebuildBindingMenu(status: status)
        refreshUpdateSummary()
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
            item.isEnabled = !bindingInProgress && !updateCheckInProgress && !updateInProgress && !status.rebinding
        }
        refreshMenuInteractivity()
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
              let mode = values["mode"],
              let state = currentNetwork?.status.state else { return }
        let interfaceID = values["interfaceID"] ?? ""
        startExposureChange(mode: mode, interfaceID: interfaceID, webPort: state.webPort, apiPort: state.apiPort,
                            message: "Rebinding web + API…",
                            detail: "Validating \(interfaceID.isEmpty ? mode : interfaceID)")
    }

    @objc private func changeWebPort() {
        promptForPort(title: "Change web port", current: currentNetwork?.status.state.webPort, changingWebPort: true)
    }

    @objc private func changeAPIPort() {
        promptForPort(title: "Change private API port", current: currentNetwork?.status.state.apiPort, changingWebPort: false)
    }

    private func promptForPort(title: String, current: Int?, changingWebPort: Bool) {
        guard let state = currentNetwork?.status.state else {
            showAlert(title: "Sonder status unavailable", message: "Refresh the status indicator before changing port exposure.")
            refresh()
            return
        }
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = "Ports apply live after the listeners restart. The API remains loopback-only."
        let field = NSTextField(string: String(current ?? (changingWebPort ? 8096 : 8097)))
        field.frame = NSRect(x: 0, y: 0, width: 180, height: 24)
        alert.accessoryView = field
        alert.addButton(withTitle: "Apply")
        alert.addButton(withTitle: "Cancel")
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        guard let port = Int(field.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)), (1...65535).contains(port) else {
            showAlert(title: "Invalid port", message: "Choose a port between 1 and 65535.")
            return
        }
        let webPort = changingWebPort ? port : state.webPort
        let apiPort = changingWebPort ? state.apiPort : port
        guard webPort != apiPort else {
            showAlert(title: "Ports must be different", message: "Choose a different web and private API port.")
            return
        }
        startExposureChange(mode: state.selectedMode, interfaceID: state.selectedInterface,
                            webPort: webPort, apiPort: apiPort,
                            message: "Applying port exposure…",
                            detail: "Validating web \(webPort) · API \(apiPort)")
    }

    private func startExposureChange(mode: String, interfaceID: String, webPort: Int, apiPort: Int, message: String, detail: String) {
        guard !bindingInProgress, !updateCheckInProgress, !updateInProgress else { return }
        bindingInProgress = true
        for item in bindMenu.items { item.isEnabled = false }
        refreshMenuInteractivity()
        display(message, symbol: "arrow.triangle.2.circlepath", detail: detail)
        let connection = configuredServer()
        request = Task { [weak self] in
            guard let self else { return }
            defer {
                self.request = nil
                self.bindingInProgress = false
                self.refreshMenuInteractivity()
            }
            do {
                var request = URLRequest(url: connection.url(path: "api/network/exposure"), timeoutInterval: 5)
                request.httpMethod = "POST"
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                if let pairingToken = connection.pairingToken {
                    request.setValue("Bearer \(pairingToken)", forHTTPHeaderField: "Authorization")
                }
                request.httpBody = try JSONSerialization.data(withJSONObject: [
                    "mode": mode,
                    "interfaceID": interfaceID,
                    "webPort": webPort,
                    "apiPort": apiPort,
                ])
                let (data, response) = try await URLSession.shared.data(for: request)
                let statusCode = (response as? HTTPURLResponse)?.statusCode ?? -1
                guard (200..<300).contains(statusCode) else {
                    let message = (try? JSONDecoder().decode(APIError.self, from: data)).flatMap(\.error) ?? "Exposure change failed (HTTP \(statusCode))"
                    throw NSError(domain: "SonderStatus", code: statusCode, userInfo: [NSLocalizedDescriptionKey: message])
                }
				try await Task.sleep(for: .seconds(1))
				self.request = nil
				self.bindingInProgress = false
				self.refresh()
			} catch {
				self.request = nil
				self.bindingInProgress = false
				self.showAlert(title: "Sonder could not update exposure", message: error.localizedDescription)
                self.refresh()
            }
        }
    }

    private func resetDetails() {
        currentNetwork = nil
        metricsView.update(version: "—", library: "—", uptime: "—", network: "—")
        connectionsView.update(web: "—", api: "—", publicPulse: "—")
        bindMenu.removeAllItems()
    }

    private func refreshMenuInteractivity() {
        let busy = bindingInProgress || scanInProgress || updateCheckInProgress || updateInProgress
        checkUpdatesItem?.isEnabled = !busy
        installUpdateItem?.isEnabled = !busy && updateInfo?.outdated == true
        scanItem?.isEnabled = !busy
    }

    @objc private func checkForUpdates() {
        guard !bindingInProgress, !updateCheckInProgress, !updateInProgress else { return }
        beginUpdateCheck(refreshHomebrew: true, showFailureAlert: true)
    }

    private func beginUpdateCheck(refreshHomebrew: Bool, showFailureAlert: Bool) {
        guard updateCheckTask == nil, !updateInProgress else { return }
        updateCheckInProgress = true
        updateInfo = nil
        updateLine.title = "Updates: Checking Homebrew…"
        refreshMenuInteractivity()
        updateCheckTask = Task { [weak self] in
            guard let self else { return }
            do {
                let brewPath = try self.homebrewPath()
                if refreshHomebrew {
                    self.updateLine.title = "Updates: Refreshing Homebrew…"
                    let result = try await self.runCommand(path: brewPath, arguments: ["update"])
                    try self.requireSuccess(result, operation: "Homebrew metadata refresh")
                }
                self.updateLine.title = "Updates: Reading Sonder release…"
                let result = try await self.runCommand(path: brewPath, arguments: ["info", "--json=v2", "tm-sonder"])
                try self.requireSuccess(result, operation: "Homebrew update check")
                guard let info = self.parseBrewInfo(result.output) else {
                    throw self.updateError("Homebrew returned an unreadable tm-sonder formula record.")
                }
                self.updateInfo = info
                self.lastUpdateCheck = Date()
                self.updateCheckInProgress = false
                self.updateCheckTask = nil
                self.refreshUpdateSummary()
                self.refreshMenuInteractivity()
            } catch is CancellationError {
                self.updateCheckInProgress = false
                self.updateCheckTask = nil
                self.refreshMenuInteractivity()
            } catch {
                self.updateCheckInProgress = false
                self.updateCheckTask = nil
                self.lastUpdateCheck = Date()
                self.updateInfo = nil
                self.updateLine.title = "Updates: Check failed"
                self.refreshMenuInteractivity()
                if showFailureAlert {
                    self.showAlert(title: "Sonder could not check for updates", message: error.localizedDescription)
                }
            }
        }
    }

    @objc private func installUpdate() {
        guard !bindingInProgress, !updateCheckInProgress, !updateInProgress,
              let info = updateInfo, info.outdated,
              let availableVersion = info.availableVersion else { return }
        let alert = NSAlert()
        alert.messageText = "Install Sonder \(availableVersion)?"
        let installedText = info.installedVersion.map { "Current Homebrew version: \($0)." } ?? ""
        alert.informativeText = "Homebrew will download and install the update. The catalog and media files are not modified. \(installedText)"
        alert.addButton(withTitle: "Install Update")
        alert.addButton(withTitle: "Cancel")
        guard alert.runModal() == .alertFirstButtonReturn else { return }

        updateInProgress = true
        updateInfo = nil
        refreshMenuInteractivity()
        updateTask = Task { [weak self] in
            guard let self else { return }
            do {
                let brewPath = try self.homebrewPath()
                self.setUpdateStage("Preparing update…", detail: "Refreshing Homebrew metadata")
                let metadata = try await self.runCommand(path: brewPath, arguments: ["update"])
                try self.requireSuccess(metadata, operation: "Homebrew metadata refresh")

                self.setUpdateStage("Downloading update…", detail: "Homebrew is fetching Sonder \(availableVersion)")
                let upgrade = try await self.runCommand(path: brewPath, arguments: ["upgrade", info.formula])
                try self.requireSuccess(upgrade, operation: "Sonder upgrade")

                self.setUpdateStage("Checking server ownership…", detail: "Confirming which instance can be restarted safely")
                let services = try await self.runCommand(path: brewPath, arguments: ["services", "list", "--json"])
                try self.requireSuccess(services, operation: "Homebrew service check")
                let serviceRunning = self.homebrewServiceIsRunning(services.output, formula: info.formula)

                if serviceRunning {
                    self.setUpdateStage("Restarting Sonder…", detail: "Restarting the Homebrew-managed server")
                    let restart = try await self.runCommand(path: brewPath, arguments: ["services", "restart", info.formula])
                    try self.requireSuccess(restart, operation: "Sonder service restart")
                    self.setUpdateStage("Verifying new version…", detail: "Waiting for the API and web server to return")
                    guard let verified = await self.waitForServerVersion(availableVersion, timeout: 30) else {
                        throw self.updateError("The Homebrew service restarted, but Sonder did not report version \(availableVersion) within 30 seconds.")
                    }
                    self.finishUpdate(success: true, message: "Sonder \(verified) is running.", updatedVersion: verified)
                } else {
                    self.setUpdateStage("Verifying installation…", detail: "Homebrew installed the update; the active server was not restarted")
                    let installed = try await self.runCommand(path: brewPath, arguments: ["info", "--json=v2", info.formula])
                    try self.requireSuccess(installed, operation: "Installed version check")
                    guard let installedInfo = self.parseBrewInfo(installed.output), installedInfo.installedVersion == availableVersion else {
                        throw self.updateError("Homebrew finished without reporting Sonder \(availableVersion) as installed.")
                    }
                    self.finishUpdate(success: true, message: "Sonder \(availableVersion) is installed. The active server is not a Homebrew service, so it was left running safely; restart that instance to load the update.", updatedVersion: availableVersion, needsManualRestart: true)
                }
            } catch is CancellationError {
                self.finishUpdate(success: false, message: "The update was cancelled.")
            } catch {
                self.finishUpdate(success: false, message: error.localizedDescription)
            }
        }
    }

    private func setUpdateStage(_ stage: String, detail: String) {
        updateLine.title = "Updates: \(stage)"
        display(stage, symbol: "arrow.down.circle", detail: detail)
    }

    private func finishUpdate(success: Bool, message: String, updatedVersion: String? = nil, needsManualRestart: Bool = false) {
        updateInProgress = false
        updateTask = nil
        refreshMenuInteractivity()
        if success {
            manualRestartVersion = needsManualRestart ? updatedVersion : nil
            updateLine.title = needsManualRestart ? "Updates: Installed · restart needed" : "Updates: Complete"
            display(needsManualRestart ? "Update installed" : "Update complete", symbol: needsManualRestart ? "exclamationmark.triangle" : "checkmark.circle", detail: message)
            showAlert(title: needsManualRestart ? "Sonder update installed" : "Sonder updated", message: message, style: needsManualRestart ? .warning : .informational)
            if !needsManualRestart {
                beginUpdateCheck(refreshHomebrew: false, showFailureAlert: false)
            }
            refresh()
        } else {
            updateLine.title = "Updates: Failed"
            display("Update failed", symbol: "exclamationmark.triangle", detail: message)
            showAlert(title: "Sonder update failed", message: message, style: .critical)
            refresh()
        }
    }

    private func refreshUpdateSummary() {
        guard !updateCheckInProgress, !updateInProgress else { return }
        if let manualRestartVersion {
            if currentNetwork?.version != manualRestartVersion {
                updateLine.title = "Updates: Installed \(manualRestartVersion) · restart needed"
                return
            }
            self.manualRestartVersion = nil
            updateLine.title = "Updates: Running \(manualRestartVersion)"
        }
        guard let info = updateInfo else { return }
        if info.outdated, let availableVersion = info.availableVersion {
            let installed = info.installedVersion.map { " from \($0)" } ?? ""
            updateLine.title = "Updates: \(availableVersion) available\(installed)"
        } else if let installedVersion = info.installedVersion {
            updateLine.title = "Updates: Up to date (\(installedVersion))"
        } else {
            updateLine.title = "Updates: Homebrew formula not installed"
        }
    }

    private func homebrewPath() throws -> String {
        let candidates = [
            "/opt/homebrew/bin/brew",
            "/usr/local/bin/brew",
            "/usr/bin/brew",
        ]
        if let path = candidates.first(where: { FileManager.default.isExecutableFile(atPath: $0) }) {
            return path
        }
        throw updateError("Homebrew was not found. Install tm-sonder with the project tap to enable menu-bar updates.")
    }

    private func runCommand(path: String, arguments: [String]) async throws -> BrewCommandResult {
        try await withCheckedThrowingContinuation { continuation in
            let process = Process()
            let pipe = Pipe()
            let output = CommandOutputBuffer()
            process.executableURL = URL(fileURLWithPath: path)
            process.arguments = arguments
            process.standardOutput = pipe
            process.standardError = pipe
            pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
                let data = handle.availableData
                guard !data.isEmpty else { return }
                output.append(data)
                guard let text = String(data: data, encoding: .utf8), !text.isEmpty else { return }
                DispatchQueue.main.async {
                    self?.showCommandProgress(text)
                }
            }
            process.terminationHandler = { process in
                pipe.fileHandleForReading.readabilityHandler = nil
                let remaining = pipe.fileHandleForReading.readDataToEndOfFile()
                output.append(remaining)
                let result = BrewCommandResult(status: process.terminationStatus, output: output.value())
                DispatchQueue.main.async {
                    continuation.resume(returning: result)
                }
            }
            do {
                try process.run()
            } catch {
                pipe.fileHandleForReading.readabilityHandler = nil
                continuation.resume(throwing: error)
            }
        }
    }

    private func showCommandProgress(_ text: String) {
        guard updateInProgress else { return }
        let line = text
            .split(whereSeparator: \.isNewline)
            .map(String.init)
            .last(where: { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }) ?? ""
        if !line.isEmpty {
            let progress = String(line.prefix(120))
            headerView.setDetail(progress)
        }
    }

    private func requireSuccess(_ result: BrewCommandResult, operation: String) throws {
        guard result.status == 0 else {
            let detail = result.output
                .split(whereSeparator: \.isNewline)
                .map(String.init)
                .last(where: { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty })
            let suffix = detail.map { " \($0)" } ?? ""
            throw updateError("\(operation) failed (exit \(result.status)).\(suffix)")
        }
    }

    private func parseBrewInfo(_ output: String) -> BrewInfo? {
        guard let start = output.firstIndex(of: "{"),
              let data = String(output[start...]).data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let formula = (object["formulae"] as? [[String: Any]])?.first else { return nil }
        let formulaName = (formula["full_name"] as? String) ?? (formula["name"] as? String) ?? "tm-sonder"
        let versions = formula["versions"] as? [String: Any]
        let availableVersion = versions?["stable"] as? String
        let installedVersion = (formula["installed"] as? [[String: Any]])?.first?["version"] as? String
        let outdated = formula["outdated"] as? Bool ?? false
        return BrewInfo(formula: formulaName, installedVersion: installedVersion, availableVersion: availableVersion, outdated: outdated)
    }

    private func homebrewServiceIsRunning(_ output: String, formula: String) -> Bool {
        guard let data = output.data(using: .utf8),
              let services = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return false }
        let shortName = formula.split(separator: "/").last.map(String.init) ?? formula
        return services.contains { service in
            let name = service["name"] as? String
            let status = service["status"] as? String
            return (name == formula || name == shortName) && (status == "started" || status == "running")
        }
    }

    private func waitForServerVersion(_ expectedVersion: String, timeout: TimeInterval) async -> String? {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            do {
                let connection = configuredServer()
                let network: NetworkEnvelope = try await fetch(connection: connection, path: "api/network/status")
                currentNetwork = network
                if network.version == expectedVersion {
                    return network.version
                }
            } catch {
                // The service may be between processes; keep waiting until the deadline.
            }
            try? await Task.sleep(for: .seconds(1))
        }
        return nil
    }

    private func updateError(_ message: String) -> NSError {
        NSError(domain: "SonderStatus.Update", code: 1, userInfo: [NSLocalizedDescriptionKey: message])
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
        let color: NSColor
        if symbol.contains("exclamation") { color = .systemOrange }
        else if symbol.contains("arrow") { color = .systemBlue }
        else { color = .systemGreen }
        headerView.update(status: text, detail: detail, color: color)
        item.button?.title = ""
        item.button?.image = menuIcon(size: 18) ?? NSImage(systemSymbolName: symbol, accessibilityDescription: text)
        item.button?.image?.isTemplate = false
        item.button?.toolTip = "Sonder: \(text). \(detail)"
        item.button?.setAccessibilityLabel("Sonder: \(text)")
    }

    private func showAlert(title: String, message: String, style: NSAlert.Style = .warning) {
        let alert = NSAlert()
        alert.alertStyle = style
        alert.messageText = title
        alert.informativeText = message
        alert.addButton(withTitle: "OK")
        alert.runModal()
    }

    private func menuIcon(size: CGFloat = 18) -> NSImage? {
        guard let path = Bundle.main.path(forResource: "TM-Sonder", ofType: "png"),
              let image = NSImage(contentsOfFile: path) else { return nil }
        image.size = NSSize(width: size, height: size)
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
        components.port = state?.webPort ?? 8096
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
