import ACTestSupport
import Foundation
import Testing

@testable import ACKit

/// The parts that meet the platform: the Keychain and URLSession.
@Suite(.serialized) struct PlatformTests {
    private let session = StoredSession(
        accessToken: "a",
        refreshToken: "r",
        idToken: nil,
        expiresAt: Date(timeIntervalSince1970: 1_800_000_000),
        claims: SessionClaims(tenantId: "t1", membershipId: "m1", role: "client")
    )

    @Test func keychainKeepsOneSessionPerService() throws {
        let store = KeychainSessionStore(service: "ackit.tests.\(UUID().uuidString)")
        defer { store.clear() }
        #expect(store.load() == nil)
        try store.save(session)
        #expect(store.load() == session)
        // saving again replaces the item instead of failing on a duplicate
        let renewed = StoredSession(
            accessToken: "b",
            refreshToken: nil,
            idToken: "i",
            expiresAt: session.expiresAt,
            claims: session.claims
        )
        try store.save(renewed)
        #expect(store.load() == renewed)
        store.clear()
        #expect(store.load() == nil)
    }

    @Test func keychainItemThatIsNoSessionIsNoSignIn() throws {
        let service = "ackit.tests.\(UUID().uuidString)"
        let store = KeychainSessionStore(service: service)
        defer { store.clear() }
        let item: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "session",
            kSecValueData as String: Data("not json".utf8),
        ]
        #expect(SecItemAdd(item as CFDictionary, nil) == errSecSuccess)
        #expect(store.load() == nil)
    }

    @Test func urlSessionTransportReturnsTheHTTPAnswer() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [EchoProtocol.self]
        let transport = URLSessionTransport(session: URLSession(configuration: configuration))
        let (data, response) = try await transport.send(URLRequest(url: try #require(URL(string: "http://echo.test/v1/x"))))
        #expect(response.statusCode == 202)
        #expect(try #require(String(bytes: data, encoding: .utf8)) == "/v1/x")
    }

    @Test func authenticatorDefaultsToTheClockAndNoCallback() async throws {
        let configuration = OIDCConfiguration(
            issuer: try #require(URL(string: "http://kc.test")),
            clientId: "c",
            redirectURI: try #require(URL(string: "x://y"))
        )
        let stored = StoredSession(
            accessToken: "a",
            refreshToken: nil,
            idToken: nil,
            expiresAt: Date().addingTimeInterval(600),
            claims: session.claims
        )
        let auth = Authenticator(configuration: configuration, role: "client", store: MemoryStore(stored))
        #expect(try await auth.credentials().accessToken == "a")
        await auth.refused()
        #expect(await !auth.isSignedIn)
    }

    @Test func nullConstantRefusesAValue() {
        let body = Data(#"{"projection":"offer","status":"offered","severity":"high"}"#.utf8)
        #expect(throws: DecodingError.self) { try JSONDecoder().decode(JobOfferSummary.self, from: body) }
    }
}

/// Answers every request with 202 and its own path.
final class EchoProtocol: URLProtocol {
    override static func canInit(with request: URLRequest) -> Bool { true }
    override static func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let url = request.url ?? URL(filePath: "/")
        if let response = HTTPURLResponse(url: url, statusCode: 202, httpVersion: nil, headerFields: nil) {
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        }
        client?.urlProtocol(self, didLoad: Data(url.path().utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {
        // the answer was sent whole in startLoading
    }
}
