#if DEBUG
import ACKit
import Foundation

/// The UI tests' stand-in for the identity provider and the Core API (Debug builds only): an app started with the
/// environment `AC_UI_TEST=stub` signs in without a web page and reads the answers below, built from the contract's
/// types so they cannot drift from the API's. The UI tests in CI need no server; the live UI tests run against the dev
/// stack instead.
public enum UITestStub {
    /// Whether a launch with this environment is a stubbed UI test.
    public static func isActive(_ environment: [String: String]) -> Bool { environment["AC_UI_TEST"] == "stub" }

    /// The provider's page: returns the redirect with the request's state at once.
    public static let signIn: WebSignIn = { url, scheme in
        let state = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "state" }?.value ?? ""
        return URL(string: "\(scheme)://oauth/callback?code=stub&state=\(state)") ?? url
    }

    /// A session model on the stub transport and an empty in-memory store. The stubbed sign-in signs in with the role
    /// `AC_UI_TEST_ROLE` names (the app's own by default): another role shows the refusal of an account of another app.
    @MainActor
    public static func session(configuration: AppConfiguration, environment: [String: String] = [:]) -> SessionModel {
        let role = environment["AC_UI_TEST_ROLE"] ?? configuration.role
        return SessionModel(configuration: configuration, store: Memory(), transport: Transport(role: role))
    }

    final class Memory: SessionStore, @unchecked Sendable {
        // one UI test drives one app: the lock only keeps the type honest about Sendable
        private let lock = NSLock()
        private var stored: StoredSession?

        func load() -> StoredSession? { lock.withLock { stored } }
        func save(_ session: StoredSession) throws { lock.withLock { stored = session } }
        func clear() { lock.withLock { stored = nil } }
    }

    struct Transport: HTTPTransport {
        let role: String

        func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
            let url = request.url ?? URL(filePath: "/")
            let body = try answer(url.path())
            let response = HTTPURLResponse(url: url, statusCode: body == nil ? 404 : 200, httpVersion: nil, headerFields: nil)
            return (body ?? Data("{}".utf8), response ?? HTTPURLResponse())
        }

        private func answer(_ path: String) throws -> Data? {
            let encoder = JSONEncoder()
            // the discovery is under the app's own issuer (…/realms/ac), the other endpoints are the stub's
            if path.hasSuffix("/.well-known/openid-configuration") { return try encoder.encode(StubEndpoints()) }
            switch path {
            case "/token": return try encoder.encode(StubTokens(accessToken: token))
            case "/logout": return Data("{}".utf8)
            case "/v1/units": return try encoder.encode(StubAnswer(data: page(Self.units)))
            case "/v1/jobs": return try encoder.encode(StubAnswer(data: page(Self.jobs)))
            default: return nil
            }
        }

        private var token: String {
            let claims = ["tenant_id": "t1", "membership_id": "m1", "role": role]
            let payload = (try? JSONEncoder().encode(claims)) ?? Data()
            return "e30.\(payload.base64EncodedString().replacingOccurrences(of: "=", with: "")).sig"
        }

        private func page<Item>(_ items: [Item]) -> Page<Item> {
            Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1)
        }

        private static let day = "2026-12-01T00:00:00Z"

        private static let units = [
            unit(id: "u1", name: "Living room AC", power: .on, connection: .online),
            unit(id: "u2", name: "Bedroom AC", power: .off, connection: .offline),
        ]

        private static func unit(id: String, name: String, power: EffectivePowerState, connection: Connection) -> UnitSummary {
            UnitSummary(
                id: id,
                tenantId: "t1",
                version: 1,
                createdAt: day,
                updatedAt: day,
                customerOrgId: "c1",
                propertyId: "p1",
                spaceId: nil,
                displayName: name,
                modelId: "m1",
                installState: .installed,
                installedAt: nil,
                serviceScope: [],
                warrantyEndsAt: nil,
                alertPolicyIds: [],
                archived: false,
                capabilityVersion: 1,
                iotModelId: nil,
                iotModelVersion: nil,
                connection: connection,
                observedState: ObservedState(power: nil, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil),
                observedRestriction: nil,
                lastSeenAt: nil,
                effectivePowerState: power,
                latestMeasurements: [],
                activeAlertCount: 0
            )
        }

        private static let jobs = [
            JobsListResultItem.summary(
                JobSummary(
                    id: "j1",
                    version: 1,
                    status: .assigned,
                    createdAt: day,
                    dueAt: day,
                    type: .reactive,
                    origin: .clientRequest,
                    unitId: "u1",
                    requestedSlot: Slot(startAt: day, endAt: day),
                    scheduledSlot: nil,
                    assignmentId: "a1",
                    preferredSlots: [],
                    followUpOfJobId: nil,
                    followUpClass: nil,
                    severity: .normal,
                    displayStatus: .assigned,
                    assignmentAcknowledgement: nil,
                    technicianMembershipId: "m1",
                    accessValidFrom: nil,
                    accessValidUntil: nil,
                    eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
                )
            )
        ]
    }
}

private struct StubEndpoints: Encodable {
    let authorizationEndpoint = "http://stub/auth"
    let tokenEndpoint = "http://stub/token"
    let endSessionEndpoint = "http://stub/logout"

    enum CodingKeys: String, CodingKey {
        case authorizationEndpoint = "authorization_endpoint"
        case tokenEndpoint = "token_endpoint"
        case endSessionEndpoint = "end_session_endpoint"
    }
}

private struct StubTokens: Encodable {
    let accessToken: String
    let refreshToken = "r"
    let expiresIn = 3600

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case expiresIn = "expires_in"
    }
}

private struct StubAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
