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

    final class Transport: HTTPTransport, @unchecked Sendable {
        let role: String
        // the AC the stub's commands change: one UI test drives one app, the lock only keeps the type honest
        private let lock = NSLock()
        private var observed = ObservedState(power: true, celsius: 26, mode: .cool, fanLevel: .mid, observedAt: day)
        private var commands: [String: Command] = [:]
        private var version = 1.0
        private var notifications = Transport.inbox
        private let jobs = StubJobs()
        private var consent = Consent(
            id: "consent1",
            tenantId: "t1",
            version: 1,
            createdAt: Transport.day,
            updatedAt: Transport.day,
            membershipId: "m1",
            granted: false,
            grantedAt: nil,
            revokedAt: nil
        )

        init(role: String) {
            self.role = role
        }

        func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
            let url = request.url ?? URL(filePath: "/")
            let method = request.httpMethod ?? "GET"
            let (status, body) = try lock.withLock {
                try jobs.answer(method, url.path(), request.httpBody)
                    ?? answer(method, url.path(), request.httpBody).map { (200, $0) } ?? (404, Data("{}".utf8))
            }
            let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil)
            return (body, response ?? HTTPURLResponse())
        }

        private func answer(_ method: String, _ path: String, _ body: Data?) throws -> Data? {
            let encoder = JSONEncoder()
            // the discovery is under the app's own issuer (…/realms/ac), the other endpoints are the stub's
            if path.hasSuffix("/.well-known/openid-configuration") { return try encoder.encode(StubEndpoints()) }
            if path.hasPrefix("/v1/units/") { return try encoder.encode(StubAnswer(data: detail(String(path.dropFirst(10))))) }
            if path.hasPrefix("/v1/commands/") { return try encoder.encode(StubAnswer(data: acknowledge(String(path.dropFirst(13))))) }
            if path.hasPrefix("/v1/notifications/"), path.hasSuffix("/mark-read") {
                let id = String(path.dropFirst(18).dropLast(10))
                return try encoder.encode(StubAnswer(data: markRead(id)))
            }
            switch (method, path) {
            case (_, "/token"): return try encoder.encode(StubTokens(accessToken: token))
            case (_, "/logout"): return Data("{}".utf8)
            case (_, "/v1/units"): return try encoder.encode(StubAnswer(data: page(Self.units)))
            case (_, "/v1/preferences"): return try encoder.encode(StubAnswer(data: Self.preferences))
            case (_, "/v1/notifications"): return try encoder.encode(StubAnswer(data: page(notifications)))
            case ("POST", "/v1/notifications/mark-all-read"):
                let ids = notifications.filter { $0.readAt == nil }.map(\.id)
                for id in ids { _ = markRead(id) }
                return try encoder.encode(StubAnswer(data: NotificationsMarked(markedIds: ids, skippedIds: [])))
            case (_, "/v1/properties"): return try encoder.encode(StubAnswer(data: page([Self.home])))
            case ("GET", "/v1/consents/location_automation"): return try encoder.encode(StubAnswer(data: consent))
            case ("PUT", "/v1/consents/location_automation"):
                // the purpose is the route's ({purpose}): the body holds only the choice
                let input = try JSONDecoder().decode(StubConsentBody.self, from: body ?? Data())
                consent.granted = input.granted
                consent.version += 1
                return try encoder.encode(StubAnswer(data: consent))
            case ("POST", "/v1/presence"):
                let arrival = PresenceResultTransitions(propertyId: Self.home.id, event: .arrival, commandIds: ["c-auto"])
                return try encoder.encode(StubAnswer(data: PresenceResult(transitions: [arrival])))
            case ("GET", "/v1/commands"):
                return try encoder.encode(StubAnswer(data: page(commands.values.sorted { $0.id > $1.id })))
            case ("POST", "/v1/commands"):
                let input = try JSONDecoder().decode(CommandsCreateInput.self, from: body ?? Data())
                return try encoder.encode(StubAnswer(data: create(input)))
            default: return nil
            }
        }

        /// A command the AC has not answered yet; the first commands.get answers it (the AC takes every setting).
        private func create(_ input: CommandsCreateInput) -> Command {
            let id = "c\(commands.count + 1)"
            let command = Command(
                id: id,
                tenantId: "t1",
                version: 1,
                createdAt: Self.day,
                updatedAt: Self.day,
                unitId: input.unitId,
                actorMembershipId: "m1",
                action: Self.commandAction(input.action),
                diagnosticRunId: nil,
                jobId: nil,
                reason: nil,
                status: .sent,
                delivery: .sent,
                requestedAt: Self.day,
                sentAt: Self.day,
                acknowledgedAt: nil,
                expiresAt: "2026-12-01T00:00:30Z",
                failureCode: nil,
                correlationId: id,
                source: .ui
            )
            commands[id] = command
            return command
        }

        private func acknowledge(_ id: String) -> Command? {
            guard var command = commands[id] else { return nil }
            if command.status == .sent {
                command.status = .acknowledged
                command.acknowledgedAt = Self.day
                commands[id] = command
                apply(command.action)
            }
            return command
        }

        private func apply(_ action: CommandAction) {
            switch action {
            case let .setPower(change): observed.power = change.power
            case let .setTemperature(change): observed.celsius = change.celsius
            case let .setMode(change): observed.mode = change.mode
            case let .setFan(change): observed.fanLevel = change.fanLevel
            default: break
            }
            version += 1
        }

        private func markRead(_ id: String) -> CoreNotification? {
            guard let index = notifications.firstIndex(where: { $0.id == id }) else { return nil }
            notifications[index].readAt = Self.day
            notifications[index].version += 1
            return notifications[index]
        }

        private static let home = Property(
            id: "p1",
            tenantId: "t1",
            version: 1,
            createdAt: day,
            updatedAt: day,
            customerOrgId: "c1",
            kind: .home,
            name: "Home A",
            address: nil,
            accessInstructions: nil,
            location: GeoPoint(latitude: 3.139, longitude: 101.6869),
            geofenceRadiusMeters: nil,
            archived: false
        )

        private static let inbox = [
            notice(id: "n1", type: .cleaningDue, template: .alert, name: "Living room AC", message: "Cleaning due. Not a fault."),
            notice(id: "n2", type: .completion, template: .completion, name: "Bedroom AC", message: nil, status: "completed"),
        ]

        private static func notice(
            id: String,
            type: NotificationType,
            template: CoreNotificationTemplateKey,
            name: String,
            message: String?,
            status: String = ""
        ) -> CoreNotification {
            CoreNotification(
                id: id,
                tenantId: "t1",
                version: 1,
                createdAt: day,
                updatedAt: day,
                sourceAlertId: nil,
                type: type,
                recipientMembershipId: "m1",
                scopeVersionAtCreation: 1,
                target: Target(kind: .unit, id: "u1"),
                templateKey: template,
                params: CoreNotificationParams(
                    targetName: name,
                    at: day,
                    status: status.isEmpty ? type.rawValue : status,
                    reason: nil,
                    amountMinor: nil,
                    currency: nil,
                    method: nil,
                    message: message
                ),
                channel: .inApp,
                deliveryState: .queued,
                severity: .normal,
                occurredAt: day,
                readAt: nil,
                targetOpen: true
            )
        }

        private static func commandAction(_ action: UnitAction) -> CommandAction {
            switch action {
            case let .setPower(change): .setPower(change)
            case let .setTemperature(change): .setTemperature(change)
            case let .setMode(change): .setMode(change)
            case let .setFan(change): .setFan(change)
            case let .ventilate(change): .ventilate(change)
            }
        }

        private func detail(_ id: String) -> UnitDetail? {
            guard let unit = Self.units.first(where: { $0.id == id }) else { return nil }
            let isLiving = id == "u1"
            return UnitDetail(
                id: unit.id,
                tenantId: unit.tenantId,
                version: version,
                createdAt: unit.createdAt,
                updatedAt: unit.updatedAt,
                customerOrgId: unit.customerOrgId,
                propertyId: unit.propertyId,
                spaceId: unit.spaceId,
                displayName: unit.displayName,
                modelId: unit.modelId,
                installState: unit.installState,
                installedAt: unit.installedAt,
                serviceScope: unit.serviceScope,
                warrantyEndsAt: unit.warrantyEndsAt,
                alertPolicyIds: unit.alertPolicyIds,
                archived: unit.archived,
                capabilityVersion: unit.capabilityVersion,
                iotModelId: nil,
                iotModelVersion: nil,
                connection: unit.connection,
                observedState: isLiving ? observed : unit.observedState,
                observedRestriction: nil,
                lastSeenAt: Self.day,
                effectivePowerState: isLiving ? (observed.power == true ? .on : .off) : unit.effectivePowerState,
                latestMeasurements: [],
                activeAlertCount: 0,
                capabilities: Self.capability,
                iotModel: nil,
                effectiveControlPolicy: .unrestricted(EffectiveControlPolicyUnrestricted()),
                controlAvailability: .available(ControlAvailabilityAvailable()),
                components: [],
                pendingCommands: [],
                pendingCommandIds: [],
                location: UnitDetailLocation(pathLabels: ["Home A", "Living room"], address: nil, accessInstructions: nil)
            )
        }

        private static let capability = Capability(
            id: "cap1",
            tenantId: "t1",
            version: 1,
            createdAt: day,
            updatedAt: day,
            manufacturer: "Daikin",
            model: "FTKF",
            control: true,
            modeControl: true,
            fanControl: true,
            temperature: CapabilityTemperature(min: 16, max: 30, step: 1),
            modes: [.cool, .dry, .fan],
            fanLevels: [.low, .mid, .high],
            ventilation: false,
            ventilationLevels: []
        )

        private static let preferences = Preferences(
            locale: .en,
            timezone: "Asia/Kuala_Lumpur",
            monthlyReportEmail: false,
            twoFactorEnabled: false,
            pinnedUnitIds: []
        )

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

private struct StubConsentBody: Decodable {
    let granted: Bool
}

private struct StubAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
