import Foundation

/// What a call is made as: the access token and the membership it names (the token's `tenant_id` and `membership_id`).
public struct Credentials: Sendable, Equatable {
    public let accessToken: String
    public let tenantId: String
    public let membershipId: String

    public init(accessToken: String, tenantId: String, membershipId: String) {
        self.accessToken = accessToken
        self.tenantId = tenantId
        self.membershipId = membershipId
    }
}

/// Where the client gets its credentials (the signed-in session, refreshed when it is about to expire).
public protocol CredentialSource: Sendable {
    /// The credentials of the current sign-in; throws UNAUTHENTICATED when there is none.
    func credentials() async throws(CoreError) -> Credentials
    /// The Core API no longer takes the sign-in (HQ signed the user out everywhere, IR384, or the membership ended).
    func refused() async
}

/// Sends one request: URLSession in the apps, a stub in the tests.
public protocol HTTPTransport: Sendable {
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}

/// The transport of the apps.
public struct URLSessionTransport: HTTPTransport {
    let session: URLSession

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        return (data, http)
    }
}

/// A refusal of the Core API (its DomainError body), or one the client says for a call that got no answer.
public struct CoreError: Error, Sendable, Equatable {
    /// The HTTP status; 0 for a call that got no answer.
    public let status: Int
    public let error: DomainError

    public init(status: Int, error: DomainError) {
        self.status = status
        self.error = error
    }

    static func local(_ code: ErrorCode, _ key: String, status: Int = 0) -> Self {
        Self(
            status: status,
            error: DomainError(code: code, messageKey: key, fieldErrors: [:], correlationId: "", retryAfterSeconds: nil)
        )
    }

    /// The answer of a call the network did not carry: no connection is OFFLINE, a timeout is TIMEOUT, anything else
    /// (and a body that is no answer of the contract) UNAVAILABLE.
    static func network(_ error: any Error) -> Self {
        switch (error as? URLError)?.code {
        case .notConnectedToInternet, .networkConnectionLost, .dataNotAllowed, .internationalRoamingOff:
            local(.offline, "ios.noConnection")  // the phone has no network: not "error.offline", which is an AC's
        case .timedOut:
            local(.timeout, "error.timeout")
        default:
            local(.unavailable, "error.unavailable")
        }
    }
}

/// The `meta` of an answer, the part the apps read.
public struct AnswerMeta: Decodable, Sendable, Equatable {
    public let correlationId: String
    public let snapshotAt: String?
}

/// An answer of the Core API: the operation's result and its meta.
public struct CoreAnswer<Result: Sendable>: Sendable {
    public let data: Result
    public let meta: AnswerMeta
}

private struct Envelope<Result: Decodable>: Decodable {
    let data: Result
    let meta: AnswerMeta
}

/// Calls the Core API's REST routes with the signed-in membership's bearer token (DEC-112, IR860).
public struct CoreClient: Sendable {
    let baseURL: URL
    let transport: any HTTPTransport
    let source: any CredentialSource

    public init(baseURL: URL, transport: any HTTPTransport = URLSessionTransport(), credentials: any CredentialSource) {
        self.baseURL = baseURL
        self.transport = transport
        source = credentials
    }

    /// Calls the operation. A write sends `idempotencyKey` (a retry of the same write passes the same key, D04);
    /// `expectedVersion` is the version the caller saw (`X-Expected-Version`, a CONFLICT when it changed).
    public func call<Operation: CoreOperation>(
        _ operation: Operation.Type,
        _ input: Operation.Input,
        expectedVersion: Int? = nil,
        idempotencyKey: UUID = UUID()
    ) async throws(CoreError) -> CoreAnswer<Operation.Result> {
        let built: RestRequest?
        do {
            built = try RestRequest.of(operation, input)
        } catch {
            throw CoreError.local(.validation, "error.malformedInput")
        }
        guard let rest = built, let url = URL(string: rest.path, relativeTo: baseURL) else {
            throw CoreError.local(.notFound, "error.unknownOperation", status: 404)
        }
        let credentials = try await source.credentials()
        var request = URLRequest(url: url)
        request.httpMethod = rest.method.rawValue
        request.httpBody = rest.body
        request.setValue("Bearer \(credentials.accessToken)", forHTTPHeaderField: "Authorization")
        request.setValue(credentials.tenantId, forHTTPHeaderField: "X-Tenant-Id")
        request.setValue(credentials.membershipId, forHTTPHeaderField: "X-Membership-Id")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if rest.body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        if Operation.isWrite { request.setValue(idempotencyKey.uuidString.lowercased(), forHTTPHeaderField: "Idempotency-Key") }
        if let expectedVersion { request.setValue(String(expectedVersion), forHTTPHeaderField: "X-Expected-Version") }

        let data: Data
        let response: HTTPURLResponse
        do {
            (data, response) = try await transport.send(request)
        } catch {
            throw CoreError.network(error)
        }
        if response.statusCode == 401 { await source.refused() }
        guard (200..<300).contains(response.statusCode) else {
            let refusal = try? JSONDecoder().decode(DomainError.self, from: data)
            throw CoreError(status: response.statusCode, error: refusal ?? CoreError.local(.unavailable, "error.unavailable").error)
        }
        do {
            let envelope = try JSONDecoder().decode(Envelope<Operation.Result>.self, from: data)
            return CoreAnswer(data: envelope.data, meta: envelope.meta)
        } catch {
            // a success that is no answer of the contract reads as unavailable, as in the web apps
            throw CoreError.local(.unavailable, "error.unavailable", status: response.statusCode)
        }
    }
}
