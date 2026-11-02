import XCTest
@testable import OfflineCore

final class ServerAddressTests: XCTestCase {
    func testParseAddsSchemeAndNormalizes() {
        XCTAssertEqual(SonderServerAddresses.parse("  sonder.stoverparc.org/ ", defaultScheme: "https")?.absoluteString,
                       "https://sonder.stoverparc.org")
        XCTAssertEqual(SonderServerAddresses.parse("10.20.0.117:8096", defaultScheme: "http")?.absoluteString,
                       "http://10.20.0.117:8096")
        XCTAssertEqual(SonderServerAddresses.parse("HTTPS://Sonder.Example.org/base/?x=1#y", defaultScheme: "http")?.absoluteString,
                       "https://sonder.example.org/base")
    }

    func testParseRejectsUnusableText() {
        XCTAssertNil(SonderServerAddresses.parse("", defaultScheme: "https"))
        XCTAssertNil(SonderServerAddresses.parse("   ", defaultScheme: "https"))
        XCTAssertNil(SonderServerAddresses.parse("ftp://files.example.org", defaultScheme: "https"))
        XCTAssertNil(SonderServerAddresses.parse("https://", defaultScheme: "https"))
    }

    func testIdentityPrefersPublicAddress() {
        let publicURL = URL(string: "https://sonder.example.org")!
        let localURL = URL(string: "http://10.0.0.5:8096")!
        XCTAssertEqual(SonderServerAddresses(publicURL: publicURL, localURL: localURL).identityURL, publicURL)
        XCTAssertEqual(SonderServerAddresses(publicURL: nil, localURL: localURL).identityURL, localURL)
        XCTAssertTrue(SonderServerAddresses().isEmpty)
    }
}

final class EndpointSelectorTests: XCTestCase {
    private let publicURL = URL(string: "https://sonder.example.org")!
    private let localURL = URL(string: "http://10.0.0.5:8096")!

    func testUsesLocalWhenItAnswers() async {
        let probed = ProbeLog()
        let choice = await SonderEndpointSelector.choose(.init(publicURL: publicURL, localURL: localURL)) { url in
            await probed.add(url); return true
        }
        XCTAssertEqual(choice, SonderEndpointChoice(url: localURL, route: .local))
        let urls = await probed.urls
        XCTAssertEqual(urls, [localURL])
    }

    func testFallsBackToPublicWhenLocalDoesNotAnswer() async {
        let choice = await SonderEndpointSelector.choose(.init(publicURL: publicURL, localURL: localURL)) { _ in false }
        XCTAssertEqual(choice, SonderEndpointChoice(url: publicURL, route: .public))
    }

    func testPublicOnlyIsNotProbed() async {
        let probed = ProbeLog()
        let choice = await SonderEndpointSelector.choose(.init(publicURL: publicURL, localURL: nil)) { url in
            await probed.add(url); return false
        }
        XCTAssertEqual(choice, SonderEndpointChoice(url: publicURL, route: .public))
        let urls = await probed.urls
        XCTAssertTrue(urls.isEmpty)
    }

    func testSameAddressTwiceIsTreatedAsPublic() async {
        let probed = ProbeLog()
        let choice = await SonderEndpointSelector.choose(.init(publicURL: publicURL, localURL: publicURL)) { url in
            await probed.add(url); return true
        }
        XCTAssertEqual(choice?.route, .public)
        let urls = await probed.urls
        XCTAssertTrue(urls.isEmpty)
    }

    func testLocalOnlyIsUsedEvenWhenUnreachable() async {
        let choice = await SonderEndpointSelector.choose(.init(publicURL: nil, localURL: localURL)) { _ in false }
        XCTAssertEqual(choice, SonderEndpointChoice(url: localURL, route: .local))
    }

    func testNoAddressesGivesNoChoice() async {
        let choice = await SonderEndpointSelector.choose(.init()) { _ in true }
        XCTAssertNil(choice)
    }
}

final class ServerProbeTests: XCTestCase {
    override func tearDown() { StubURLProtocol.handler = nil }

    private var probe: SonderServerProbe { SonderServerProbe(session: StubURLProtocol.session(), timeout: 1) }
    private let base = URL(string: "http://10.0.0.5:8096")!

    func testSonderIdentityHeaderCountsAsReachable() async {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/auth/session")
            XCTAssertNil(request.value(forHTTPHeaderField: "Cookie"))
            return (401, ["X-Sonder-Server": "tm-sonder"], Data())
        }
        let reachable = await probe.isSonderServer(base)
        XCTAssertTrue(reachable)
    }

    func testOtherWebServerIsNotReachable() async {
        StubURLProtocol.handler = { _ in (200, ["Content-Type": "text/html"], Data("<html>portal</html>".utf8)) }
        let reachable = await probe.isSonderServer(base)
        XCTAssertFalse(reachable)
    }

    func testNetworkFailureIsNotReachable() async {
        StubURLProtocol.handler = { _ in throw URLError(.cannotConnectToHost) }
        let reachable = await probe.isSonderServer(base)
        XCTAssertFalse(reachable)
    }
}

final class AccountSignInTests: XCTestCase {
    override func tearDown() { StubURLProtocol.handler = nil }

    private var client: SonderAccountClient { SonderAccountClient(session: StubURLProtocol.session()) }
    private let base = URL(string: "https://sonder.example.org")!

    func testSignInReturnsSessionCookieAsToken() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/api/auth/login")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Content-Type"), "application/json")
            let body = try JSONSerialization.jsonObject(with: StubURLProtocol.body(of: request)) as? [String: String]
            XCTAssertEqual(body?["username"], "sean")
            XCTAssertEqual(body?["password"], "correct horse")
            return (200, ["Set-Cookie": "sonder_session=abc123; Path=/; Expires=Wed, 02 Dec 2026 18:00:00 GMT; HttpOnly; Secure; SameSite=Lax",
                          "Content-Type": "application/json"],
                    Data(#"{"authenticated":true,"username":"sean","expiresAt":"2026-12-02T18:00:00.123456789Z"}"#.utf8))
        }
        let result = try await client.signIn(baseURL: base, username: " sean ", password: "correct horse")
        XCTAssertEqual(result.token, "abc123")
        XCTAssertEqual(result.username, "sean")
        XCTAssertEqual(result.expiresAt?.timeIntervalSince1970 ?? 0, 1_796_234_400.123, accuracy: 0.01)
    }

    func testWrongPasswordFromSonderIsInvalidCredentials() async {
        StubURLProtocol.handler = { _ in (401, ["X-Sonder-Server": "tm-sonder"], Data(#"{"error":"Invalid credentials"}"#.utf8)) }
        await assertSignIn(throws: .invalidCredentials)
    }

    func testUnauthorizedWithoutSonderHeaderIsNotSonder() async {
        StubURLProtocol.handler = { _ in (401, [:], Data()) }
        await assertSignIn(throws: .notSonderServer)
    }

    func testThrottleCarriesRetryAfter() async {
        StubURLProtocol.handler = { _ in (429, ["Retry-After": "900", "X-Sonder-Server": "tm-sonder"], Data()) }
        await assertSignIn(throws: .throttled(retryAfterSeconds: 900))
    }

    func testNoAccountYetIsSetupRequired() async {
        StubURLProtocol.handler = { _ in (409, ["X-Sonder-Server": "tm-sonder"], Data()) }
        await assertSignIn(throws: .setupRequired)
    }

    func testSuccessWithoutCookieIsReported() async {
        StubURLProtocol.handler = { _ in (200, ["X-Sonder-Server": "tm-sonder"], Data(#"{"authenticated":true}"#.utf8)) }
        await assertSignIn(throws: .noSessionIssued)
    }

    func testSessionStatusSendsBearerToken() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/auth/session")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer tok")
            return (200, ["X-Sonder-Server": "tm-sonder"],
                    Data(#"{"authenticated":true,"googleLinked":false,"setupRequired":false,"username":"sean"}"#.utf8))
        }
        let status = try await client.sessionStatus(baseURL: base, token: "tok")
        XCTAssertEqual(status, SonderAuthSessionStatus(setupRequired: false, authenticated: true, username: "sean"))
    }

    func testServerDateParsingAcceptsGoNanoseconds() {
        XCTAssertNotNil(SonderAccountClient.parseServerDate("2026-12-02T18:00:00.123456789Z"))
        XCTAssertNotNil(SonderAccountClient.parseServerDate("2026-12-02T18:00:00-05:00"))
        XCTAssertNil(SonderAccountClient.parseServerDate("not a date"))
        XCTAssertNil(SonderAccountClient.parseServerDate(nil))
    }

    private func assertSignIn(throws expected: SonderSignInError, file: StaticString = #filePath, line: UInt = #line) async {
        do {
            _ = try await client.signIn(baseURL: base, username: "sean", password: "pw")
            XCTFail("Expected \(expected)", file: file, line: line)
        } catch {
            XCTAssertEqual(error as? SonderSignInError, expected, file: file, line: line)
        }
    }
}

private actor ProbeLog {
    var urls: [URL] = []
    func add(_ url: URL) { urls.append(url) }
}

/// Serves canned responses so the networking code runs without a server.
final class StubURLProtocol: URLProtocol, @unchecked Sendable {
    typealias Handler = @Sendable (URLRequest) throws -> (Int, [String: String], Data)
    nonisolated(unsafe) static var handler: Handler?

    static func session() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubURLProtocol.self]
        return URLSession(configuration: configuration)
    }

    static func body(of request: URLRequest) -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count <= 0 { break }
            data.append(buffer, count: count)
        }
        return data
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = Self.handler else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        do {
            let (status, headers, data) = try handler(request)
            let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
