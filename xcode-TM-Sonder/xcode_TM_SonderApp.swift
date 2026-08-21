import AppKit
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
        NSApp.appearance = NSAppearance(named: .aqua)
        installMenuBarIcon()
        httpServer = SonderHTTPServer(library: library)
        // Install this before the initial start. Persisted settings load
        // asynchronously and may finish during application launch; registering
        // afterward can miss the change and leave the live server local-only even
        // though the dashboard shows LAN sharing enabled.
        serverSettingsObserver = NotificationCenter.default.addObserver(
            forName: .sonderServerSettingsDidChange,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            self?.applyServerSettings()
        }
        applyServerSettings()
        library.importPlexContextIfAvailable()
        library.importAudiobookContextIfAvailable()
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
        item.button?.toolTip = "TM Sonder menu bar controls. Click to open the app menu."

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
        let port = httpServer?.port ?? safeServerPort
        SonderSystemServices.shared.openLocalWebInterface(port: port)
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

    private var safeServerPort: UInt16 {
        UInt16(clamping: min(max(library.serverSettings.port, 1024), 65535))
    }

    private func applyServerSettings() {
        if library.serverSettings.isEnabled {
            httpServer?.start(port: safeServerPort, allowLAN: library.serverSettings.allowLAN, pairingToken: library.serverSettings.pairingToken)
        } else {
            httpServer?.stop()
        }
    }
}
