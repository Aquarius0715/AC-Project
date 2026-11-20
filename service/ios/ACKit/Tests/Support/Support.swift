import ACKit
import Foundation

/// A transport that answers each request with the next queued answer and keeps the requests it got.
public actor StubTransport: HTTPTransport {
    public enum Answer: Sendable {
        case http(Int, String)
        case failure(any Error & Sendable)
    }

    private var answers: [Answer]
    public private(set) var requests: [URLRequest] = []
    /// Waits this long before answering, so that calls can overlap.
    let delay: Duration

    public init(_ answers: [Answer], delay: Duration = .zero) {
        self.answers = answers
        self.delay = delay
    }

    public func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        requests.append(request)
        if delay > .zero { try await Task.sleep(for: delay) }
        guard !answers.isEmpty else { throw URLError(.cannotConnectToHost) }
        switch answers.removeFirst() {
        case let .http(status, body):
            let url = request.url ?? URL(filePath: "/")
            let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil)
            return (Data(body.utf8), response ?? HTTPURLResponse())
        case let .failure(error):
            throw error
        }
    }
}

/// Fixed credentials; counts the refusals.
public actor FixedCredentials: CredentialSource {
    public private(set) var refusals = 0
    let result: Result<Credentials, CoreError>

    public init(_ result: Result<Credentials, CoreError> = .success(Credentials(accessToken: "tok", tenantId: "t1", membershipId: "m1"))) {
        self.result = result
    }

    public func credentials() async throws(CoreError) -> Credentials {
        try result.get()
    }

    public func refused() async {
        refusals += 1
    }
}

/// A session store in memory.
public final class MemoryStore: SessionStore, @unchecked Sendable {
    // the tests use one store from one task at a time; the lock keeps the type honest about Sendable
    private let lock = NSLock()
    private var stored: StoredSession?
    public private(set) var saves = 0

    public init(_ session: StoredSession? = nil) {
        stored = session
    }

    public func load() -> StoredSession? {
        lock.withLock { stored }
    }

    public func save(_ session: StoredSession) throws {
        lock.withLock {
            stored = session
            saves += 1
        }
    }

    public func clear() {
        lock.withLock { stored = nil }
    }
}

/// A JWT with the given payload (unsigned: the app reads claims, the Core API checks signatures).
public func jwt(_ claims: [String: String]) -> String {
    // sorted keys: the same claims are the same token (a dictionary's order changes from run to run)
    let encoder = JSONEncoder()
    encoder.outputFormatting = .sortedKeys
    let payload = (try? encoder.encode(claims)) ?? Data()
    return "e30.\(base64URL(payload)).sig"
}

/// The query of a path as name → values, decoded as URLSearchParams does (a `+` is a space).
public func query(_ path: String) -> (path: String, values: [String: [String]]) {
    let components = URLComponents(string: "http://core\(path.replacingOccurrences(of: "+", with: "%20"))")
    var values: [String: [String]] = [:]
    for item in components?.queryItems ?? [] {
        values[item.name, default: []].append(item.value ?? "")
    }
    return (components?.path ?? "", values)
}

/// Base64url without padding (RFC 4648 §5), as a JWT writes its parts.
public func base64URL(_ data: Data) -> String {
    data.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_")
        .replacingOccurrences(of: "=", with: "")
}

/// Counts calls made from any task.
public actor Counter {
    public private(set) var value = 0

    public init() {
        // nothing counted yet
    }

    public func add() {
        value += 1
    }
}
