import Foundation

/// Why a sign-in did not complete.
public enum SignInError: Error, Equatable {
    /// The provider or the network did not answer (its endpoints or its token endpoint).
    case unavailable
    /// The redirect was not this sign-in's, or the provider refused the code.
    case refused
    /// The account is not of this app's role (a technician signing in to the Customer app, an HQ account).
    case wrongRole(String)
}

/// The app's sign-in (DEC-112): the authorization code flow with PKCE against the app's public client, the session in
/// the Keychain, the access token refreshed shortly before it expires, and the session dropped when the provider or the
/// Core API no longer takes it. The interactive step (the provider's sign-in page) is the app's: it opens
/// ``PendingSignIn/url`` in a web authentication session and hands the redirect to ``complete(_:callback:)``.
public actor Authenticator: CredentialSource {
    let configuration: OIDCConfiguration
    let role: String
    let transport: any HTTPTransport
    let store: any SessionStore
    let now: @Sendable () -> Date
    let signedOut: (@Sendable () async -> Void)?
    private var endpoints: OIDCEndpoints?
    private var session: StoredSession?
    private var refreshing: Task<Result<StoredSession, CoreError>, Never>?

    /// The refresh starts this long before the access token expires.
    static let refreshMargin: TimeInterval = 30

    /// - Parameters:
    ///   - role: the `role` claim this app signs in (`client`, `technician`).
    ///   - signedOut: called when the session ends without the user signing out (refused, or the refresh failed).
    public init(
        configuration: OIDCConfiguration,
        role: String,
        transport: any HTTPTransport = URLSessionTransport(),
        store: any SessionStore,
        now: @escaping @Sendable () -> Date = { Date() },
        signedOut: (@Sendable () async -> Void)? = nil
    ) {
        self.configuration = configuration
        self.role = role
        self.transport = transport
        self.store = store
        self.now = now
        self.signedOut = signedOut
        session = store.load()
    }

    /// Whether a session is stored.
    public var isSignedIn: Bool { session != nil }

    /// The claims of the signed-in membership.
    public var claims: SessionClaims? { session?.claims }

    /// Starts a sign-in: the provider's page to open. `locale` is the display language the page opens in.
    public func begin(locale: String? = nil) async throws(SignInError) -> PendingSignIn {
        PendingSignIn.make(configuration, try await discovered(), locale: locale)
    }

    /// Completes a sign-in with the redirect the web authentication session returned.
    public func complete(_ pending: PendingSignIn, callback: URL) async throws(SignInError) {
        guard let code = pending.code(from: callback) else { throw .refused }
        let endpoints = try await discovered()
        let answer = try await token(
            endpoints,
            [
                "grant_type": "authorization_code",
                "client_id": configuration.clientId,
                "code": code,
                "redirect_uri": configuration.redirectURI.absoluteString,
                "code_verifier": pending.verifier,
            ]
        )
        guard let claims = SessionClaims.of(accessToken: answer.accessToken) else { throw .refused }
        guard claims.role == role else {
            // the provider's session ends too, so the next sign-in asks for an account again
            await endProviderSession(endpoints, refreshToken: answer.refreshToken)
            throw .wrongRole(claims.role)
        }
        keep(answer, claims: claims, previousRefresh: nil)
    }

    /// Signs out: the provider's session ends (best effort) and the stored session goes.
    public func signOut() async {
        let ending = session
        drop()
        if let ending, let endpoints = try? await discovered() {
            await endProviderSession(endpoints, refreshToken: ending.refreshToken)
        }
    }

    // MARK: - CredentialSource

    public func credentials() async throws(CoreError) -> Credentials {
        guard var current = session else { throw Self.unauthenticated }
        if current.expiresAt.timeIntervalSince(now()) < Self.refreshMargin {
            current = try await renewed().get()
        }
        return Credentials(
            accessToken: current.accessToken,
            tenantId: current.claims.tenantId,
            membershipId: current.claims.membershipId
        )
    }

    public func refused() async {
        guard session != nil else { return }
        drop()
        await signedOut?()
    }

    // MARK: - the provider

    private func discovered() async throws(SignInError) -> OIDCEndpoints {
        if let endpoints { return endpoints }
        let url = configuration.issuer.appending(path: ".well-known/openid-configuration")
        guard let (data, response) = try? await transport.send(URLRequest(url: url)), response.statusCode == 200,
            let found = try? JSONDecoder().decode(OIDCEndpoints.self, from: data)
        else { throw .unavailable }
        endpoints = found
        return found
    }

    private func token(_ endpoints: OIDCEndpoints, _ form: [String: String]) async throws(SignInError) -> TokenResponse {
        guard let (data, response) = try? await transport.send(Self.formRequest(endpoints.tokenEndpoint, form)) else {
            throw .unavailable
        }
        guard response.statusCode == 200, let answer = try? JSONDecoder().decode(TokenResponse.self, from: data) else {
            throw .refused
        }
        return answer
    }

    static let unauthenticated = CoreError.local(.unauthenticated, "error.unauthenticated", status: 401)

    /// One refresh at a time: the calls that need a token while it runs wait for the same answer.
    private func renewed() async -> Result<StoredSession, CoreError> {
        if let refreshing { return await refreshing.value }
        let task = Task { await refresh() }
        refreshing = task
        let result = await task.value
        refreshing = nil
        return result
    }

    /// A new access token. The session ends only when the provider refuses the refresh (or the new token names another
    /// membership); a provider the phone cannot reach (no network when the token expires) keeps it for the next call.
    private func refresh() async -> Result<StoredSession, CoreError> {
        guard let current = session, let refreshToken = current.refreshToken else {
            await refused()
            return .failure(Self.unauthenticated)
        }
        let form = ["grant_type": "refresh_token", "client_id": configuration.clientId, "refresh_token": refreshToken]
        let answer: TokenResponse
        do {
            answer = try await token(try await discovered(), form)
        } catch .unavailable {
            return .failure(CoreError.local(.unavailable, "error.unavailable"))
        } catch {
            await refused()
            return .failure(Self.unauthenticated)
        }
        guard let claims = SessionClaims.of(accessToken: answer.accessToken), claims.membershipId == current.claims.membershipId else {
            await refused()
            return .failure(Self.unauthenticated)
        }
        return .success(keep(answer, claims: claims, previousRefresh: refreshToken))
    }

    @discardableResult
    private func keep(_ answer: TokenResponse, claims: SessionClaims, previousRefresh: String?) -> StoredSession {
        let stored = StoredSession(
            accessToken: answer.accessToken,
            refreshToken: answer.refreshToken ?? previousRefresh,
            idToken: answer.idToken,
            expiresAt: now().addingTimeInterval(answer.expiresIn),
            claims: claims
        )
        session = stored
        // a session the Keychain did not take lasts until the app ends: the user is not signed out for it
        try? store.save(stored)
        return stored
    }

    private func drop() {
        session = nil
        store.clear()
    }

    /// Ends the provider's session with the refresh token (OpenID RP-initiated logout by POST, which Keycloak takes from
    /// a public client); a failure leaves only the provider's session, which expires by itself.
    private func endProviderSession(_ endpoints: OIDCEndpoints, refreshToken: String?) async {
        guard let logout = endpoints.endSessionEndpoint, let refreshToken else { return }
        _ = try? await transport.send(Self.formRequest(logout, ["client_id": configuration.clientId, "refresh_token": refreshToken]))
    }

    static func formRequest(_ url: URL, _ form: [String: String]) -> URLRequest {
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        let body = form.sorted { $0.key < $1.key }
            .map { "\(RestRequest.formEncoded($0.key))=\(RestRequest.formEncoded($0.value))" }
            .joined(separator: "&")
        request.httpBody = Data(body.utf8)
        return request
    }
}
