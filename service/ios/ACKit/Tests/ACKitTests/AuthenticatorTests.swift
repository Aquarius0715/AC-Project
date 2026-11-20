import ACTestSupport
import CryptoKit
import Foundation
import Testing

@testable import ACKit

@Suite struct AuthenticatorTests {
    private let issuer = URL(string: "http://kc.test/realms/ac") ?? URL(filePath: "/")
    private let redirect = URL(string: "ac-technician://oauth/callback") ?? URL(filePath: "/")
    private let discovery = #"""
        {"authorization_endpoint":"http://kc.test/realms/ac/auth","token_endpoint":"http://kc.test/realms/ac/token",
         "end_session_endpoint":"http://kc.test/realms/ac/logout"}
        """#
    private let start = Date(timeIntervalSince1970: 1_800_000_000)
    private var technician: String { jwt(["tenant_id": "t1", "membership_id": "m1", "role": "technician"]) }

    private func tokens(_ access: String, refresh: String? = "r1", expiresIn: Int = 300) -> String {
        let refreshPart = refresh.map { #","refresh_token":"\#($0)""# } ?? ""
        return #"{"access_token":"\#(access)","expires_in":\#(expiresIn),"id_token":"id1"\#(refreshPart)}"#
    }

    private func authenticator(
        _ transport: StubTransport,
        store: MemoryStore = MemoryStore(),
        now: Date? = nil,
        signedOut: (@Sendable () async -> Void)? = nil
    ) -> Authenticator {
        let configuration = OIDCConfiguration(issuer: issuer, clientId: "ac-technician-ios", redirectURI: redirect)
        let at = now ?? start
        return Authenticator(
            configuration: configuration,
            role: "technician",
            transport: transport,
            store: store,
            now: { at },
            signedOut: signedOut
        )
    }

    private func signedIn(expiresAt: Date, refresh: String? = "r1") -> StoredSession {
        StoredSession(
            accessToken: "old",
            refreshToken: refresh,
            idToken: nil,
            expiresAt: expiresAt,
            claims: SessionClaims(tenantId: "t1", membershipId: "m1", role: "technician")
        )
    }

    private func items(_ url: URL) -> [String: String] {
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        return Dictionary(items.map { ($0.name, $0.value ?? "") }) { first, _ in first }
    }

    private func form(_ request: URLRequest?) -> [String: String] {
        let body = request?.httpBody.flatMap { String(bytes: $0, encoding: .utf8) } ?? ""
        return items(URL(string: "x:?\(body)") ?? URL(filePath: "/"))
    }

    @Test func beginAsksForACodeWithPKCE() async throws {
        let transport = StubTransport([.http(200, discovery)])
        let pending = try await authenticator(transport).begin(locale: "ja")
        let query = items(pending.url)
        #expect(pending.url.absoluteString.hasPrefix("http://kc.test/realms/ac/auth?"))
        #expect(query["client_id"] == "ac-technician-ios")
        #expect(query["response_type"] == "code")
        #expect(query["redirect_uri"] == "ac-technician://oauth/callback")
        #expect(query["code_challenge_method"] == "S256")
        #expect(query["ui_locales"] == "ja")
        #expect(query["state"] == pending.state)
        let challenge = Data(SHA256.hash(data: Data(pending.verifier.utf8))).base64URLEncoded()
        #expect(query["code_challenge"] == challenge)
        #expect(pending.verifier.count >= 43)  // RFC 7636: 43–128 characters
        let asked = try #require(await transport.requests.first?.url)
        #expect(asked.absoluteString == "http://kc.test/realms/ac/.well-known/openid-configuration")
    }

    @Test func beginWithoutTheProviderIsUnavailable() async throws {
        for answer in [StubTransport.Answer.http(503, "down"), .http(200, "{}"), .failure(URLError(.timedOut))] {
            await #expect(throws: SignInError.unavailable) {
                try await authenticator(StubTransport([answer])).begin()
            }
        }
    }

    @Test func completeExchangesTheCodeAndKeepsTheSession() async throws {
        let store = MemoryStore()
        let transport = StubTransport([.http(200, discovery), .http(200, tokens(technician))])
        let auth = authenticator(transport, store: store)
        let pending = try await auth.begin()
        let callback = try #require(URL(string: "ac-technician://oauth/callback?code=c1&state=\(pending.state)"))
        try await auth.complete(pending, callback: callback)
        #expect(await auth.isSignedIn)
        #expect(await auth.claims == SessionClaims(tenantId: "t1", membershipId: "m1", role: "technician"))
        let exchange = await transport.requests.last
        #expect(exchange?.url?.absoluteString == "http://kc.test/realms/ac/token")
        #expect(exchange?.httpMethod == "POST")
        #expect(
            form(exchange) == [
                "grant_type": "authorization_code", "client_id": "ac-technician-ios", "code": "c1",
                "redirect_uri": "ac-technician://oauth/callback", "code_verifier": pending.verifier,
            ]
        )
        let stored = try #require(store.load())
        #expect(stored.accessToken == technician)
        #expect(stored.refreshToken == "r1")
        #expect(stored.idToken == "id1")
        #expect(stored.expiresAt == start.addingTimeInterval(300))
        #expect(try await auth.credentials() == Credentials(accessToken: technician, tenantId: "t1", membershipId: "m1"))
    }

    @Test func redirectOfAnotherSignInIsRefused() async throws {
        let transport = StubTransport([.http(200, discovery)])
        let auth = authenticator(transport)
        let pending = try await auth.begin()
        for callback in [
            "ac-technician://oauth/callback", "ac-technician://oauth/callback?code=c1&state=other",
            "ac-technician://oauth/callback?state=\(pending.state)&error=access_denied",
        ] {
            await #expect(throws: SignInError.refused) {
                try await auth.complete(pending, callback: try #require(URL(string: callback)))
            }
        }
        #expect(await transport.requests.count == 1)
        #expect(await !auth.isSignedIn)
    }

    @Test func refusedCodeOrATokenWithoutClaimsIsRefused() async throws {
        for answer in [StubTransport.Answer.http(400, #"{"error":"invalid_grant"}"#), .http(200, tokens("not-a-jwt")), .http(200, "{}")] {
            let auth = authenticator(StubTransport([.http(200, discovery), answer]))
            let pending = try await auth.begin()
            await #expect(throws: SignInError.refused) {
                try await auth.complete(
                    pending,
                    callback: try #require(URL(string: "ac-technician://oauth/callback?code=c1&state=\(pending.state)"))
                )
            }
        }
    }

    @Test func tokenEndpointThatDoesNotAnswerIsUnavailable() async throws {
        let auth = authenticator(StubTransport([.http(200, discovery), .failure(URLError(.notConnectedToInternet))]))
        let pending = try await auth.begin()
        await #expect(throws: SignInError.unavailable) {
            try await auth.complete(
                pending,
                callback: try #require(URL(string: "ac-technician://oauth/callback?code=c1&state=\(pending.state)"))
            )
        }
    }

    @Test func accountOfAnotherRoleIsRefusedAndItsProviderSessionEnds() async throws {
        let customer = jwt(["tenant_id": "t1", "membership_id": "m9", "role": "client"])
        let store = MemoryStore()
        let transport = StubTransport([.http(200, discovery), .http(200, tokens(customer, refresh: "r9")), .http(204, "")])
        let auth = authenticator(transport, store: store)
        let pending = try await auth.begin()
        await #expect(throws: SignInError.wrongRole("client")) {
            try await auth.complete(
                pending,
                callback: try #require(URL(string: "ac-technician://oauth/callback?code=c1&state=\(pending.state)"))
            )
        }
        #expect(store.load() == nil)
        let logout = await transport.requests.last
        #expect(logout?.url?.absoluteString == "http://kc.test/realms/ac/logout")
        #expect(form(logout) == ["client_id": "ac-technician-ios", "refresh_token": "r9"])
    }

    @Test func storedSessionIsUsedUntilShortlyBeforeItExpires() async throws {
        let transport = StubTransport([])
        let auth = authenticator(transport, store: MemoryStore(signedIn(expiresAt: start.addingTimeInterval(31))))
        #expect(try await auth.credentials().accessToken == "old")
        #expect(await transport.requests.isEmpty)
    }

    @Test func tokenAboutToExpireIsRefreshedOnceForCallsThatOverlap() async throws {
        let renewed = jwt(["tenant_id": "t1", "membership_id": "m1", "role": "technician", "v": "2"])
        let store = MemoryStore(signedIn(expiresAt: start.addingTimeInterval(10)))
        let transport = StubTransport([.http(200, discovery), .http(200, tokens(renewed, refresh: nil))], delay: .milliseconds(20))
        let auth = authenticator(transport, store: store)
        async let a = auth.credentials()
        async let b = auth.credentials()
        let (first, second) = try await (a, b)
        #expect(first.accessToken == renewed)
        #expect(second.accessToken == renewed)
        #expect(await transport.requests.count == 2)  // the endpoints and one refresh
        #expect(
            form(await transport.requests.last) == ["grant_type": "refresh_token", "client_id": "ac-technician-ios", "refresh_token": "r1"]
        )
        // a refresh answer without a new refresh token keeps the one it used
        #expect(store.load()?.refreshToken == "r1")
    }

    @Test(arguments: ["no refresh token", "refused", "another membership"])
    func failedRefreshEndsTheSession(case name: String) async throws {
        let other = jwt(["tenant_id": "t1", "membership_id": "m2", "role": "technician"])
        let answers: [StubTransport.Answer] =
            switch name {
            case "refused": [.http(200, discovery), .http(400, #"{"error":"invalid_grant"}"#)]
            case "another membership": [.http(200, discovery), .http(200, tokens(other))]
            default: []
            }
        let store = MemoryStore(signedIn(expiresAt: start, refresh: name == "no refresh token" ? nil : "r1"))
        let ended = Counter()
        let auth = authenticator(StubTransport(answers), store: store) { await ended.add() }
        do {
            _ = try await auth.credentials()
            Issue.record("a failed refresh throws")
        } catch {
            #expect(error.error.code == .unauthenticated)
        }
        #expect(await !auth.isSignedIn)
        #expect(store.load() == nil)
        #expect(await ended.value == 1)
    }

    @Test(arguments: [StubTransport.Answer.failure(URLError(.notConnectedToInternet)), .http(503, "down")])
    func providerOutOfReachKeepsTheSession(answer: StubTransport.Answer) async throws {
        let store = MemoryStore(signedIn(expiresAt: start))
        let ended = Counter()
        let auth = authenticator(StubTransport([answer]), store: store) { await ended.add() }
        do {
            _ = try await auth.credentials()
            Issue.record("a refresh the provider did not answer throws")
        } catch {
            #expect(error.error.code == .unavailable)
        }
        #expect(await auth.isSignedIn)
        #expect(store.load() != nil)
        #expect(await ended.value == 0)
    }

    @Test func tokenEndpointOutOfReachKeepsTheSession() async throws {
        let store = MemoryStore(signedIn(expiresAt: start))
        let auth = authenticator(StubTransport([.http(200, discovery), .failure(URLError(.timedOut))]), store: store)
        await #expect(throws: CoreError.self) { try await auth.credentials() }
        #expect(store.load() != nil)
    }

    @Test func noSessionIsUnauthenticated() async throws {
        do {
            _ = try await authenticator(StubTransport([])).credentials()
            Issue.record("no session throws")
        } catch {
            #expect(error.status == 401)
        }
    }

    @Test func refusalDropsTheSessionOnce() async throws {
        let ended = Counter()
        let store = MemoryStore(signedIn(expiresAt: start.addingTimeInterval(300)))
        let auth = authenticator(StubTransport([]), store: store) { await ended.add() }
        await auth.refused()
        await auth.refused()
        #expect(store.load() == nil)
        #expect(await ended.value == 1)
    }

    @Test func signOutDropsTheSessionAndEndsTheProviderSession() async throws {
        let store = MemoryStore(signedIn(expiresAt: start.addingTimeInterval(300)))
        let transport = StubTransport([.http(200, discovery), .http(204, "")])
        let auth = authenticator(transport, store: store)
        await auth.signOut()
        #expect(await !auth.isSignedIn)
        #expect(store.load() == nil)
        #expect(form(await transport.requests.last) == ["client_id": "ac-technician-ios", "refresh_token": "r1"])
    }

    @Test func signOutWithoutASessionAsksNothing() async throws {
        let transport = StubTransport([])
        await authenticator(transport).signOut()
        #expect(await transport.requests.isEmpty)
    }

    @Test func providerWithoutAnEndSessionEndpointIsNotAsked() async throws {
        let plain = #"{"authorization_endpoint":"http://kc.test/a","token_endpoint":"http://kc.test/t"}"#
        let transport = StubTransport([.http(200, plain)])
        let auth = authenticator(transport, store: MemoryStore(signedIn(expiresAt: start.addingTimeInterval(300))))
        await auth.signOut()
        #expect(await transport.requests.count == 1)
    }

    @Test func callbackSchemeIsTheRedirectsScheme() {
        let configuration = OIDCConfiguration(issuer: issuer, clientId: "c", redirectURI: redirect)
        #expect(configuration.callbackScheme == "ac-technician")
        let relative = OIDCConfiguration(issuer: issuer, clientId: "c", redirectURI: URL(string: "oauth/callback") ?? redirect)
        #expect(relative.callbackScheme.isEmpty)
    }
}

@Suite struct SessionClaimsTests {
    @Test func readsTheTenantMembershipAndRole() {
        let claims = SessionClaims.of(accessToken: jwt(["tenant_id": "t1", "membership_id": "m1", "role": "client"]))
        #expect(claims == SessionClaims(tenantId: "t1", membershipId: "m1", role: "client"))
    }

    @Test(arguments: [
        "only.two",
        "a.%%%.c",
        "a.\(Data("[]".utf8).base64URLEncoded()).c",
    ])
    func malformedTokenHasNoClaims(token: String) {
        #expect(SessionClaims.of(accessToken: token) == nil)
    }

    @Test func emptyClaimIsNoClaim() {
        #expect(SessionClaims.of(accessToken: jwt(["tenant_id": "", "membership_id": "m1", "role": "client"])) == nil)
        #expect(SessionClaims.of(accessToken: jwt(["membership_id": "m1", "role": "client"])) == nil)
    }

    @Test func base64URLRoundTripsWithoutPadding() {
        for length in 0..<6 {
            let data = Data((0..<length).map { UInt8($0 * 61 % 256) })
            #expect(Data(base64URL: data.base64URLEncoded()) == data)
            #expect(!data.base64URLEncoded().contains("="))
        }
    }
}
