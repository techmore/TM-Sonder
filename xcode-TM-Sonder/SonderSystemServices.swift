import AppKit
import Foundation

@MainActor
protocol SonderSystemServicing: AnyObject {
    func open(_ url: URL)
    func openSecurityScoped(_ url: URL)
    func openLocalWebInterface(port: UInt16, path: String)
    func chooseMediaLibraryRoot() -> URL?
    func chooseMediaDirectories(kind: SonderLibraryImportKind) -> [URL]
    func chooseCustomMediaDirectory(name: String, kind: SonderLibraryImportKind) -> URL?
}

@MainActor
final class SonderSystemServices: SonderSystemServicing {
    static let shared = SonderSystemServices()

    func open(_ url: URL) {
        NSWorkspace.shared.open(url)
    }

    func openSecurityScoped(_ url: URL) {
        let didAccess = url.startAccessingSecurityScopedResource()
        defer {
            if didAccess {
                url.stopAccessingSecurityScopedResource()
            }
        }
        NSWorkspace.shared.open(url)
    }

    func openLocalWebInterface(port: UInt16, path: String = "") {
        var components = URLComponents()
        components.scheme = "http"
        components.host = "127.0.0.1"
        components.port = Int(port)
        if path.isEmpty == false {
            components.path = path.hasPrefix("/") ? path : "/\(path)"
        }
        guard let url = components.url else { return }
        open(url)
    }

    func chooseMediaLibraryRoot() -> URL? {
        let panel = NSOpenPanel()
        panel.title = "Add Media Library"
        panel.prompt = "Add Library"
        panel.message = "Choose a folder that contains Movies, TV Shows, Books, or Audiobooks subfolders. Other folders will be ignored."
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        panel.resolvesAliases = false
        panel.directoryURL = FileManager.default.homeDirectoryForCurrentUser

        guard panel.runModal() == .OK else { return nil }
        return panel.urls.first
    }

    func chooseMediaDirectories(kind: SonderLibraryImportKind) -> [URL] {
        let panel = NSOpenPanel()
        panel.title = "Add \(kind.label) Directories"
        panel.prompt = "Add"
        panel.message = "Choose mounted SMB, NFS, NAS, or external \(kind.label.lowercased()) folders. Sonder will scan subfolders and remember the index locally."
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = true
        panel.canCreateDirectories = false
        panel.resolvesAliases = false
        panel.directoryURL = FileManager.default.homeDirectoryForCurrentUser

        guard panel.runModal() == .OK else { return [] }
        return panel.urls
    }

    func chooseCustomMediaDirectory(name: String, kind: SonderLibraryImportKind) -> URL? {
        let panel = NSOpenPanel()
        panel.title = "Add \(name)"
        panel.prompt = "Add"
        panel.message = "Choose one folder for this custom \(kind.label.lowercased())-style library. Sonder will show it as its own category."
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = false
        panel.resolvesAliases = false
        panel.directoryURL = FileManager.default.homeDirectoryForCurrentUser

        guard panel.runModal() == .OK else { return nil }
        return panel.urls.first
    }
}
