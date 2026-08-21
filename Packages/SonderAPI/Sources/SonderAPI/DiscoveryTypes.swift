import Foundation

public struct SonderHealthResponse: Codable, Sendable, Hashable {
    public var status: String
    public var name: String
    public var app: String
    public var id: String
    public var service: String
    public var library: String
    public var allowLAN: Bool
    public var requiresPairing: Bool

    public init(
        status: String = "ok",
        name: String,
        app: String,
        id: String = "tm-sonder",
        service: String = "_tmsonder._tcp",
        library: String = "/api/library",
        allowLAN: Bool,
        requiresPairing: Bool
    ) {
        self.status = status
        self.name = name
        self.app = app
        self.id = id
        self.service = service
        self.library = library
        self.allowLAN = allowLAN
        self.requiresPairing = requiresPairing
    }

    enum CodingKeys: String, CodingKey {
        case status, name, app, id, service, library, allowLAN, requiresPairing
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        status = try container.decodeIfPresent(String.self, forKey: .status) ?? "unknown"
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? "Sonder"
        app = try container.decodeIfPresent(String.self, forKey: .app) ?? name
        id = try container.decodeIfPresent(String.self, forKey: .id) ?? "tm-sonder"
        service = try container.decodeIfPresent(String.self, forKey: .service) ?? "_tmsonder._tcp"
        library = try container.decodeIfPresent(String.self, forKey: .library) ?? "/api/library"
        allowLAN = Self.decodeFlexibleBool(from: container, forKey: .allowLAN) ?? false
        requiresPairing = Self.decodeFlexibleBool(from: container, forKey: .requiresPairing) ?? false
    }

    private static func decodeFlexibleBool(
        from container: KeyedDecodingContainer<CodingKeys>,
        forKey key: CodingKeys
    ) -> Bool? {
        if let boolValue = try? container.decode(Bool.self, forKey: key) {
            return boolValue
        }
        if let stringValue = try? container.decode(String.self, forKey: key) {
            return stringValue.caseInsensitiveCompare("true") == .orderedSame
        }
        return nil
    }
}

public struct SonderDiscoveryCapabilities: Codable, Sendable, Hashable {
    public var books: Bool
    public var ebooks: Bool
    public var audiobooks: Bool
    public var themes: Bool
    public var themeSync: Bool
    public var progressSync: Bool
    public var mediaStreaming: Bool
    public var videoStreaming: Bool
    public var artwork: Bool
    public var librarySync: Bool
    public var remoteCatalog: Bool

    public init(
        books: Bool = true,
        ebooks: Bool = true,
        audiobooks: Bool = true,
        themes: Bool = true,
        themeSync: Bool = true,
        progressSync: Bool = true,
        mediaStreaming: Bool = true,
        videoStreaming: Bool = true,
        artwork: Bool = true,
        librarySync: Bool = true,
        remoteCatalog: Bool = true
    ) {
        self.books = books
        self.ebooks = ebooks
        self.audiobooks = audiobooks
        self.themes = themes
        self.themeSync = themeSync
        self.progressSync = progressSync
        self.mediaStreaming = mediaStreaming
        self.videoStreaming = videoStreaming
        self.artwork = artwork
        self.librarySync = librarySync
        self.remoteCatalog = remoteCatalog
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        books = try container.decodeIfPresent(Bool.self, forKey: .books) ?? false
        ebooks = try container.decodeIfPresent(Bool.self, forKey: .ebooks) ?? books
        audiobooks = try container.decodeIfPresent(Bool.self, forKey: .audiobooks) ?? false
        themes = try container.decodeIfPresent(Bool.self, forKey: .themes) ?? false
        themeSync = try container.decodeIfPresent(Bool.self, forKey: .themeSync) ?? themes
        progressSync = try container.decodeIfPresent(Bool.self, forKey: .progressSync) ?? false
        mediaStreaming = try container.decodeIfPresent(Bool.self, forKey: .mediaStreaming) ?? false
        videoStreaming = try container.decodeIfPresent(Bool.self, forKey: .videoStreaming) ?? mediaStreaming
        artwork = try container.decodeIfPresent(Bool.self, forKey: .artwork) ?? false
        librarySync = try container.decodeIfPresent(Bool.self, forKey: .librarySync) ?? false
        remoteCatalog = try container.decodeIfPresent(Bool.self, forKey: .remoteCatalog) ?? librarySync
    }
}

public struct SonderDiscoveryEndpoints: Codable, Sendable, Hashable {
    public var health: String
    public var library: String
    public var audiobooks: String?
    public var audiobookBrowser: String?
    public var discovery: String
    public var progress: String
    public var playback: String
    public var playbackTrackRefresh: String?
    public var refreshTracks: String?
    public var stream: String
    public var subtitles: String?
    public var poster: String?
    public var backdrop: String?

    public init(
        health: String = SonderAPIRoutes.health,
        library: String = SonderAPIRoutes.library,
        audiobooks: String? = SonderAPIRoutes.audiobooks,
        audiobookBrowser: String? = SonderAPIRoutes.audiobookBrowser,
        discovery: String = SonderAPIRoutes.discovery,
        progress: String = SonderAPIRoutes.progress,
        playback: String = SonderAPIRoutes.playback,
        playbackTrackRefresh: String? = SonderAPIRoutes.playbackTrackRefresh,
        refreshTracks: String? = SonderAPIRoutes.playbackTrackRefresh,
        stream: String = SonderAPIRoutes.stream,
        subtitles: String? = SonderAPIRoutes.subtitles,
        poster: String? = SonderAPIRoutes.poster,
        backdrop: String? = SonderAPIRoutes.backdrop
    ) {
        self.health = health
        self.library = library
        self.audiobooks = audiobooks
        self.audiobookBrowser = audiobookBrowser
        self.discovery = discovery
        self.progress = progress
        self.playback = playback
        self.playbackTrackRefresh = playbackTrackRefresh
        self.refreshTracks = refreshTracks
        self.stream = stream
        self.subtitles = subtitles
        self.poster = poster
        self.backdrop = backdrop
    }

    public var trackRefreshPath: String? {
        refreshTracks ?? playbackTrackRefresh
    }
}

public struct SonderDiscoveryResponse: Codable, Sendable, Hashable {
    public var app: String
    public var name: String
    public var serverID: String
    public var version: String
    public var build: String
    public var isEnabled: Bool
    public var allowLAN: Bool
    public var requiresPairing: Bool
    public var port: UInt16
    public var localURL: String
    public var lanURL: String?
    public var discoveryMethods: [String]
    public var tailscaleHint: String
    public var capabilities: SonderDiscoveryCapabilities
    public var endpoints: SonderDiscoveryEndpoints
    public var theme: SonderThemeSnapshot

    public init(
        app: String,
        name: String,
        serverID: String = "tm-sonder",
        version: String,
        build: String,
        isEnabled: Bool,
        allowLAN: Bool,
        requiresPairing: Bool,
        port: UInt16,
        localURL: String,
        lanURL: String?,
        discoveryMethods: [String],
        tailscaleHint: String,
        capabilities: SonderDiscoveryCapabilities,
        endpoints: SonderDiscoveryEndpoints,
        theme: SonderThemeSnapshot
    ) {
        self.app = app
        self.name = name
        self.serverID = serverID
        self.version = version
        self.build = build
        self.isEnabled = isEnabled
        self.allowLAN = allowLAN
        self.requiresPairing = requiresPairing
        self.port = port
        self.localURL = localURL
        self.lanURL = lanURL
        self.discoveryMethods = discoveryMethods
        self.tailscaleHint = tailscaleHint
        self.capabilities = capabilities
        self.endpoints = endpoints
        self.theme = theme
    }

    public var preferredURLString: String? {
        lanURL ?? localURL
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        app = try container.decodeIfPresent(String.self, forKey: .app) ?? "TM Sonder"
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? app
        serverID = try container.decodeIfPresent(String.self, forKey: .serverID) ?? "tm-sonder"
        if let stringVersion = try? container.decode(String.self, forKey: .version) {
            version = stringVersion
        } else if let intVersion = try? container.decode(Int.self, forKey: .version) {
            version = String(intVersion)
        } else {
            version = "0"
        }
        build = try container.decodeIfPresent(String.self, forKey: .build) ?? "0"
        isEnabled = try container.decodeIfPresent(Bool.self, forKey: .isEnabled) ?? true
        allowLAN = try container.decodeIfPresent(Bool.self, forKey: .allowLAN) ?? false
        requiresPairing = try container.decodeIfPresent(Bool.self, forKey: .requiresPairing) ?? false
        if let uPort = try? container.decode(UInt16.self, forKey: .port) {
            port = uPort
        } else if let intPort = try? container.decode(Int.self, forKey: .port) {
            port = UInt16(clamping: intPort)
        } else {
            port = 8797
        }
        localURL = try container.decodeIfPresent(String.self, forKey: .localURL) ?? ""
        lanURL = try container.decodeIfPresent(String.self, forKey: .lanURL)
        discoveryMethods = try container.decodeIfPresent([String].self, forKey: .discoveryMethods) ?? []
        tailscaleHint = try container.decodeIfPresent(String.self, forKey: .tailscaleHint) ?? ""
        capabilities = try container.decodeIfPresent(SonderDiscoveryCapabilities.self, forKey: .capabilities) ?? SonderDiscoveryCapabilities()
        endpoints = try container.decodeIfPresent(SonderDiscoveryEndpoints.self, forKey: .endpoints) ?? SonderDiscoveryEndpoints()
        theme = try container.decodeIfPresent(SonderThemeSnapshot.self, forKey: .theme)
            ?? SonderThemeSnapshot(preset: "earthy", background: "", sidebar: "", surface: "", border: "", accent: "", text: "")
    }
}
