import Foundation

/// The addresses a person gives for one Sonder server: a public address that
/// works anywhere, and an optional local address used when it is reachable.
struct SonderServerAddresses: Codable, Equatable, Sendable {
    var publicURL: URL?
    var localURL: URL?

    /// Stable identity for everything stored per server (sign-in token, offline
    /// copies, queued progress, caches). It does not change when the app
    /// switches between the local and public route to the same server.
    var identityURL: URL? { publicURL ?? localURL }

    var isEmpty: Bool { publicURL == nil && localURL == nil }

    /// Parses what someone typed into an address field. Text without a scheme
    /// gets `defaultScheme`, so `sonder.example.org` works as well as a full URL.
    static func parse(_ text: String, defaultScheme: String) -> URL? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        let candidate = trimmed.contains("://") ? trimmed : "\(defaultScheme)://\(trimmed)"
        guard let url = URL(string: candidate) else { return nil }
        return normalize(url)
    }

    /// Accepts only complete http(s) URLs and drops trailing slashes, query and fragment.
    static func normalize(_ url: URL) -> URL? {
        guard let scheme = url.scheme?.lowercased(), ["http", "https"].contains(scheme),
              let host = url.host, host.isEmpty == false,
              var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return nil }
        components.scheme = scheme
        components.host = host.lowercased()
        components.path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if components.path.isEmpty == false { components.path = "/" + components.path }
        components.query = nil
        components.fragment = nil
        components.user = nil
        components.password = nil
        return components.url
    }
}

enum SonderServerRoute: String, Codable, Sendable {
    case local
    case `public`

    var label: String {
        switch self {
        case .local: "Local network"
        case .public: "Public internet"
        }
    }
}

struct SonderEndpointChoice: Equatable, Sendable {
    var url: URL
    var route: SonderServerRoute
}

enum SonderEndpointSelector {
    /// Prefers the local address when it answers as a Sonder server, otherwise
    /// uses the public address. Only the local address is probed: the public
    /// address is the fallback, and its errors surface through normal requests.
    static func choose(
        _ addresses: SonderServerAddresses,
        isReachable: @Sendable (URL) async -> Bool
    ) async -> SonderEndpointChoice? {
        if let local = addresses.localURL, local != addresses.publicURL, await isReachable(local) {
            return SonderEndpointChoice(url: local, route: .local)
        }
        if let publicURL = addresses.publicURL {
            return SonderEndpointChoice(url: publicURL, route: .public)
        }
        if let local = addresses.localURL {
            // Local is the only address. Use it so requests report the real error.
            return SonderEndpointChoice(url: local, route: .local)
        }
        return nil
    }
}

/// A quick check that a base URL is a reachable Sonder server. It asks the
/// unauthenticated `/api/auth/session` route and requires Sonder's identity
/// header, so a captive portal or another web server does not count.
struct SonderServerProbe: Sendable {
    static let identityHeader = "X-Sonder-Server"
    static let identityValue = "tm-sonder"

    var session: URLSession
    var timeout: TimeInterval

    init(session: URLSession? = nil, timeout: TimeInterval = 1.5) {
        self.timeout = timeout
        if let session {
            self.session = session
        } else {
            let configuration = URLSessionConfiguration.ephemeral
            configuration.timeoutIntervalForRequest = timeout
            configuration.timeoutIntervalForResource = timeout + 0.5
            configuration.waitsForConnectivity = false
            configuration.httpShouldSetCookies = false
            configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
            self.session = URLSession(configuration: configuration)
        }
    }

    func isSonderServer(_ baseURL: URL) async -> Bool {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/auth/session"),
                                 cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        request.httpShouldHandleCookies = false
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        guard let (_, response) = try? await session.data(for: request),
              let http = response as? HTTPURLResponse else { return false }
        return http.value(forHTTPHeaderField: Self.identityHeader)?.lowercased() == Self.identityValue
    }
}

/// `GET /api/auth/session`: whether the server needs first-time setup and
/// whether the credential sent with the request is accepted.
struct SonderAuthSessionStatus: Decodable, Equatable, Sendable {
    var setupRequired: Bool
    var authenticated: Bool
    var username: String?
}

enum SonderSignInError: LocalizedError, Equatable {
    case invalidCredentials
    case throttled(retryAfterSeconds: Int?)
    case setupRequired
    case noSessionIssued
    case notSonderServer
    case server(statusCode: Int)

    var errorDescription: String? {
        switch self {
        case .invalidCredentials:
            "Incorrect username or password."
        case let .throttled(seconds):
            if let seconds, seconds > 0 {
                "Too many sign-in attempts. Try again in \(max(1, seconds / 60)) minute\(seconds >= 120 ? "s" : "")."
            } else {
                "Too many sign-in attempts. Try again later."
            }
        case .setupRequired:
            "This server has no account yet. Create the owner account in a browser first."
        case .noSessionIssued:
            "The server accepted the sign-in but did not return a session."
        case .notSonderServer:
            "That address did not answer as a TM Sonder server."
        case let .server(statusCode):
            "Sign-in failed: the server returned HTTP \(statusCode)."
        }
    }
}

struct SonderSignInResult: Equatable, Sendable {
    var token: String
    var username: String
    var expiresAt: Date?
}

/// Account sign-in for the native client. `POST /api/auth/login` answers with
/// an HTTP-only `sonder_session` cookie; the same value is accepted as a
/// Bearer token, which works on both the local and public address.
struct SonderAccountClient: Sendable {
    static let sessionCookieName = "sonder_session"

    var session: URLSession = .shared

    func signIn(baseURL: URL, username: String, password: String) async throws -> SonderSignInResult {
        let trimmedUser = username.trimmingCharacters(in: .whitespacesAndNewlines)
        let url = baseURL.appendingPathComponent("api/auth/login")
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 20)
        request.httpMethod = "POST"
        // Keep the cookie out of the shared cookie store: the server reads a
        // cookie before the Authorization header, so a stale cookie for one
        // address would hide a valid Bearer token.
        request.httpShouldHandleCookies = false
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.httpBody = try JSONEncoder().encode(["username": trimmedUser, "password": password])

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw SonderSignInError.server(statusCode: -1) }
        switch http.statusCode {
        case 200..<300:
            break
        case 401:
            throw http.value(forHTTPHeaderField: SonderServerProbe.identityHeader) == nil
                ? SonderSignInError.notSonderServer : SonderSignInError.invalidCredentials
        case 404:
            throw SonderSignInError.notSonderServer
        case 409:
            throw SonderSignInError.setupRequired
        case 429:
            throw SonderSignInError.throttled(retryAfterSeconds: http.value(forHTTPHeaderField: "Retry-After").flatMap(Int.init))
        default:
            throw SonderSignInError.server(statusCode: http.statusCode)
        }
        guard let token = Self.sessionToken(from: http, url: url) else { throw SonderSignInError.noSessionIssued }
        let body = try? JSONDecoder().decode(LoginResponse.self, from: data)
        let username = body?.username.flatMap { $0.isEmpty ? nil : $0 } ?? trimmedUser
        return SonderSignInResult(token: token, username: username, expiresAt: Self.parseServerDate(body?.expiresAt))
    }

    func sessionStatus(baseURL: URL, token: String) async throws -> SonderAuthSessionStatus {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/auth/session"),
                                 cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 8)
        request.httpShouldHandleCookies = false
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let trimmed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty == false { request.setValue("Bearer \(trimmed)", forHTTPHeaderField: "Authorization") }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw SonderSignInError.server(statusCode: (response as? HTTPURLResponse)?.statusCode ?? -1)
        }
        return try JSONDecoder().decode(SonderAuthSessionStatus.self, from: data)
    }

    /// Best effort: revoke the session on the server.
    func signOut(baseURL: URL, token: String) async {
        var request = URLRequest(url: baseURL.appendingPathComponent("api/auth/logout"), timeoutInterval: 8)
        request.httpMethod = "POST"
        request.httpShouldHandleCookies = false
        request.setValue("\(Self.sessionCookieName)=\(token)", forHTTPHeaderField: "Cookie")
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        _ = try? await session.data(for: request)
    }

    static func sessionToken(from response: HTTPURLResponse, url: URL) -> String? {
        var headers: [String: String] = [:]
        for (key, value) in response.allHeaderFields {
            if let key = key as? String, let value = value as? String { headers[key] = value }
        }
        return HTTPCookie.cookies(withResponseHeaderFields: headers, for: url)
            .first { $0.name == sessionCookieName && $0.value.isEmpty == false }?.value
    }

    private struct LoginResponse: Decodable {
        var username: String?
        var expiresAt: String?
    }
}

extension SonderAccountClient {
    /// Go encodes times as RFC 3339 with up to nine fractional digits, which
    /// ISO8601DateFormatter rejects; trim to milliseconds first.
    static func parseServerDate(_ raw: String?) -> Date? {
        guard var raw, raw.isEmpty == false else { return nil }
        if let dot = raw.firstIndex(of: "."),
           let zone = raw[dot...].firstIndex(where: { $0 == "Z" || $0 == "+" || $0 == "-" }) {
            let digits = raw[raw.index(after: dot)..<zone].prefix(3)
            raw = String(raw[..<dot]) + "." + digits + String(raw[zone...])
        }
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return fractional.date(from: raw) ?? ISO8601DateFormatter().date(from: raw)
    }
}
