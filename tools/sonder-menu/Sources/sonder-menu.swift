import Cocoa

// TM Sonder menu bar companion.
// Shows a live status icon (green = running, red = down) and quick actions.

final class AppDelegate: NSObject, NSApplicationDelegate {
    let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    var timer: Timer?
    var statusLine: NSMenuItem?
    var isRunning = false

    let base = "http://127.0.0.1:8797"

    func applicationDidFinishLaunching(_ notification: Notification) {
        let menu = NSMenu()

        statusLine = NSMenuItem(title: "Checking…", action: nil, keyEquivalent: "")
        statusLine?.isEnabled = false
        menu.addItem(statusLine!)
        menu.addItem(.separator())

        let dash = NSMenuItem(title: "Open Dashboard", action: #selector(openDashboard), keyEquivalent: "d")
        dash.target = self
        menu.addItem(dash)

        let library = NSMenuItem(title: "Open Library", action: #selector(openLibrary), keyEquivalent: "l")
        library.target = self
        menu.addItem(library)

        menu.addItem(.separator())

        let rescan = NSMenuItem(title: "Trigger Rescan", action: #selector(triggerRescan), keyEquivalent: "r")
        rescan.target = self
        menu.addItem(rescan)

        let refresh = NSMenuItem(title: "Refresh Status", action: #selector(refreshNow), keyEquivalent: "u")
        refresh.target = self
        menu.addItem(refresh)

        menu.addItem(.separator())
        let quit = NSMenuItem(title: "Quit Sonder Menu", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)

        menu.autoenablesItems = false
        statusItem.menu = menu
        setStatus(running: false, checking: true)

        timer = Timer.scheduledTimer(withTimeInterval: 10.0, repeats: true) { _ in
            self.check()
        }
        RunLoop.main.add(timer!, forMode: .common)
        check()
    }

    func setStatus(running: Bool, checking: Bool = false) {
        isRunning = running
        if checking {
            statusItem.button?.title = "◐"
            statusItem.button?.appearsDisabled = true
        } else {
            statusItem.button?.appearsDisabled = false
            statusItem.button?.title = running ? "●" : "○"
            statusItem.button?.contentTintColor = running ?
                NSColor.systemGreen : NSColor.systemRed
        }
        statusLine?.title = running
            ? "TM Sonder — running (port 8797)"
            : "TM Sonder — not running"
    }

    func check() {
        setStatus(running: false, checking: true)
        var req = URLRequest(url: URL(string: "\(base)/api/health")!)
        req.timeoutInterval = 4
        URLSession.shared.dataTask(with: req) { data, resp, _ in
            let ok = (resp as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async { self.setStatus(running: ok) }
        }.resume()
    }

    @objc func openDashboard() {
        NSWorkspace.shared.open(URL(string: base)!)
    }

    @objc func openLibrary() {
        NSWorkspace.shared.open(URL(string: "\(base)/api/library")!)
    }

    @objc func refreshNow() { check() }

    @objc func triggerRescan() {
        var req = URLRequest(url: URL(string: "\(base)/api/settings/rescan")!)
        req.httpMethod = "POST"
        req.timeoutInterval = 10
        URLSession.shared.dataTask(with: req) { _, _, _ in
            DispatchQueue.main.asyncAfter(deadline: .now() + 2) { self.check() }
        }.resume()
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)   // menu bar only, no dock icon
app.run()
