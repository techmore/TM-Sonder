import Foundation

/// Everything the HTTP server needs to stream a title, captured as a `Sendable` snapshot
/// on the main actor and handed off to the background queue. The server resolves the
/// security-scoped bookmark and brackets file access itself.
nonisolated struct SonderStreamTarget: Sendable {
    let contentType: String
    let sourceBookmark: Data?
    let sourcePath: String?

    var resolvedURL: URL? {
        if let sourceBookmark {
            var stale = false
            if let url = try? URL(resolvingBookmarkData: sourceBookmark, options: [.withSecurityScope], relativeTo: nil, bookmarkDataIsStale: &stale),
               FileManager.default.fileExists(atPath: url.path) {
                return url
            }
        }
        guard let sourcePath, FileManager.default.fileExists(atPath: sourcePath) else { return nil }
        return URL(fileURLWithPath: sourcePath)
    }
}

/// Auth snapshot the background HTTP queue reads to decide whether to allow a request.
/// `Sendable` so it can be captured from the `@MainActor` library into the detached
/// routing path without crossing actor isolation unsafely.
nonisolated struct SonderAuthSnapshot: Sendable {
    let allowLAN: Bool
    let pairingToken: String

    /// Whether a request is authorized. Localhost is always allowed. For non-local
    /// requests, LAN must be enabled and (if a token is set) the request must carry it.
    func isAuthorized(localhost: Bool, bearer: String?, queryToken: String?) -> Bool {
        if localhost { return true }
        guard allowLAN else { return false }
        // No token configured => unauthenticated LAN (insecure, but user's choice).
        guard pairingToken.isEmpty == false else { return true }
        let presented = bearer ?? queryToken ?? ""
        return presented == pairingToken
    }
}

nonisolated struct HTTPRequest {
    var method = "GET"
    var path = "/"
    var queryItems: [String: String] = [:]
    var headers: [String: String] = [:]
    var body = Data()

    /// Convenience initializer used when the full request has not necessarily been parsed
    /// (e.g. an early read error); parses whatever head is available.
    init(data: Data) {
        self.init(data: data, headerEnd: HTTPRequest.headerTerminatorRange(in: data))
    }

    /// Designated initializer. `headerEnd` is the byte index where `\r\n\r\n` begins, so
    /// the body starts at `headerEnd + 4`.
    init(data: Data, headerEnd: Int?) {
        guard let raw = String(data: data, encoding: .utf8) else { return }
        let headerString: String
        let bodyStart: Int
        if let headerEnd {
            headerString = String(decoding: data.prefix(headerEnd), as: UTF8.self)
            bodyStart = min(headerEnd + 4, data.count)
        } else {
            headerString = raw
            bodyStart = data.count
        }

        let lines = headerString.components(separatedBy: "\r\n")
        if let firstLine = lines.first {
            let tokens = firstLine.split(separator: " ")
            if tokens.count >= 2 {
                method = String(tokens[0])
                let target = String(tokens[1]).removingPercentEncoding ?? String(tokens[1])
                if let components = URLComponents(string: target) {
                    path = components.path.isEmpty ? "/" : components.path
                    queryItems = Dictionary(uniqueKeysWithValues: (components.queryItems ?? []).compactMap { item in
                        item.value.map { (item.name.lowercased(), $0) }
                    })
                } else {
                    path = target.components(separatedBy: "?").first ?? target
                }
            }
        }
        for line in lines.dropFirst() {
            guard let separator = line.firstIndex(of: ":") else { continue }
            let key = String(line[..<separator]).trimmingCharacters(in: .whitespaces).lowercased()
            let value = String(line[line.index(after: separator)...]).trimmingCharacters(in: .whitespaces)
            headers[key] = value
        }

        if bodyStart < data.count {
            body = data.subdata(in: bodyStart..<data.count)
        }
    }

    var contentLength: Int {
        Int(headers["content-length"] ?? "") ?? 0
    }

    var bearerToken: String? {
        guard let authorization = headers["authorization"] else { return nil }
        let prefix = "Bearer "
        guard authorization.localizedCaseInsensitiveContains(prefix) else { return nil }
        return String(authorization.dropFirst(prefix.count)).trimmingCharacters(in: .whitespacesAndNewlines)
    }

    var queryToken: String? {
        queryItems["token"] ?? queryItems["pairingtoken"]
    }

    var isLocalhostRequest: Bool {
        guard let host = headers["host"]?.lowercased() else { return true }
        return host.hasPrefix("127.0.0.1") || host.hasPrefix("localhost") || host.hasPrefix("[::1]")
    }

    /// Returns the byte index of the `\r\n\r\n` request head terminator, or nil if the
    /// head has not fully arrived yet.
    static func headerTerminatorRange(in data: Data) -> Int? {
        guard data.count >= 4 else { return nil }
        let terminator = Data([0x0D, 0x0A, 0x0D, 0x0A])
        return data.range(of: terminator)?.lowerBound
    }
}

nonisolated struct HTTPByteRange {
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
        } else if bounds.count == 2, bounds[0].isEmpty, let suffixLength = UInt64(bounds[1]) {
            // Suffix range: "bytes=-500" -> last 500 bytes
            start = suffixLength >= fileLength ? 0 : fileLength - suffixLength
            end = fullEnd
            isPartial = true
        } else {
            start = 0
            end = fullEnd
            isPartial = false
        }
    }
}
