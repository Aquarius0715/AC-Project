#if DEBUG
import ACKit
import Foundation

/// The alert of the UI tests' AC u1 (AC_UI_TEST=stub): open, then acknowledged and resolved as the Core API would; its
/// one evidence candidate; and the technician's session, who may resolve alerts.
final class StubAlerts {
    private var alert = StubUnitData.alert

    /// The answer of an alert or session route, or nil for a route that is not one.
    func answer(_ method: String, _ path: String, _ query: String?, _ body: Data?) throws -> (Int, Data)? {
        let encoder = JSONEncoder()
        switch (method, path) {
        case ("GET", "/v1/alerts"):
            // the screens read the alerts per status (IR712)
            let status = URLComponents(string: "?\(query ?? "")")?.queryItems?.first { $0.name == "status" }?.value
            let items = status.map { $0 == alert.status.rawValue } ?? true ? [alert] : []
            return (
                200,
                try encoder.encode(
                    StubAlertAnswer(data: Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1))
                )
            )
        case ("GET", "/v1/alerts/al1/evidence"):
            let candidate = EvidenceCandidate(
                id: "ev-r1",
                kind: .remeasurement,
                observedAt: StubUnitData.at(300),
                metric: .temperature,
                value: 24.5,
                unit: .c,
                origin: .measured,
                quality: .valid,
                eventType: nil
            )
            return (200, try encoder.encode(StubAlertAnswer(data: Page(items: [candidate], nextCursor: nil, total: 1, snapshotVersion: 1))))
        case ("POST", "/v1/alerts/al1/acknowledge"):
            alert.status = .acknowledged
            alert.acknowledgedAt = StubUnitData.at(0)
            alert.version += 1
            return (200, try encoder.encode(StubAlertAnswer(data: alert)))
        case ("POST", "/v1/alerts/al1/resolve"):
            let input = try JSONDecoder().decode(StubResolve.self, from: body ?? Data())
            alert.status = .resolved
            alert.resolvedAt = StubUnitData.at(0)
            alert.resolutionReason = input.resolutionReason
            alert.version += 1
            return (200, try encoder.encode(StubAlertAnswer(data: alert)))
        case ("GET", "/v1/session"):
            let session = Session(
                tenantId: "t1",
                membershipId: "m1",
                scopeVersion: 1,
                userId: "u-tech",
                role: .technician,
                clientRole: nil,
                displayName: "Tech",
                organizationName: "HQ",
                organizationId: "o1",
                customerId: nil,
                permissions: [.alertResolve],
                generation: 1,
                viewEpoch: 1,
                issuedAt: StubUnitData.at(3600),
                expiresAt: StubUnitData.at(-3600)
            )
            return (200, try encoder.encode(StubAlertAnswer(data: session)))
        default:
            return nil
        }
    }
}

private struct StubResolve: Decodable {
    let resolutionReason: String
}

private struct StubAlertAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
