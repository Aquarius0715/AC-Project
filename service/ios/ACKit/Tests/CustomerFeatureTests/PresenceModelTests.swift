import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import CustomerFeature

/// A phone's location services with set answers, and the circles it was asked to watch.
actor ScriptedLocation: LocationSource {
    let given: LocationPermission
    let fixes: [Fix?]
    private var next = 0
    private(set) var watched: [[Geofence]] = []
    private(set) var stops = 0
    private var crossings: AsyncStream<String>.Continuation?
    /// Held until the test opens them: the permission prompt on the screen, the system setting up its monitor.
    let asking: Gate?
    let settingUp: Gate?

    init(
        _ permission: LocationPermission = .always,
        fixes: [Fix?] = [ScriptedLocation.home],
        asking: Gate? = nil,
        settingUp: Gate? = nil
    ) {
        given = permission
        self.fixes = fixes
        self.asking = asking
        self.settingUp = settingUp
    }

    static let home = Fix(latitude: 3.139, longitude: 101.6869, accuracyMeters: 10, at: Date(timeIntervalSince1970: 0))

    func permission() async -> LocationPermission { given }

    func requestAlways() async -> LocationPermission {
        await asking?.pass()
        return given
    }

    func fix() async -> Fix? {
        defer { next += 1 }
        return next < fixes.count ? fixes[next] : fixes.last.flatMap { $0 }
    }

    func watch(_ fences: [Geofence]) async -> AsyncStream<String> {
        watched.append(fences)
        await settingUp?.pass()
        let (stream, continuation) = AsyncStream<String>.makeStream()
        crossings = continuation
        return stream
    }

    func stop() async {
        stops += 1
        crossings?.finish()
    }

    /// The phone crossing a circle.
    func cross(_ id: String) {
        crossings?.yield(id)
    }
}

final class Switch: PresenceSettings, @unchecked Sendable {
    private let lock = NSLock()
    private var on: Bool

    init(_ on: Bool = false) {
        self.on = on
    }

    var isOn: Bool { lock.withLock { on } }
    func set(_ isOn: Bool) { lock.withLock { on = isOn } }
}

@MainActor @Suite struct PresenceModelTests {
    private struct Envelope<Value: Encodable>: Encodable {
        let data: Value
        let meta = ["correlationId": "c"]
    }

    private let day = "2026-11-20T00:00:00Z"

    private func answer(_ value: some Encodable) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try JSONEncoder().encode(Envelope(data: value)), encoding: .utf8)))
    }

    private func property(_ id: String, _ name: String, pin: GeoPoint?, radius: Double? = nil, archived: Bool = false) -> Property {
        Property(
            id: id,
            tenantId: "t1",
            version: 1,
            createdAt: day,
            updatedAt: day,
            customerOrgId: "c1",
            kind: .home,
            name: name,
            address: nil,
            accessInstructions: nil,
            location: pin,
            geofenceRadiusMeters: radius,
            archived: archived
        )
    }

    private var properties: StubTransport.Answer {
        get throws {
            try answer(
                Page(
                    items: [
                        property("p1", "Home A", pin: GeoPoint(latitude: 3.1, longitude: 101.6)),
                        property("p2", "Office", pin: GeoPoint(latitude: 3.2, longitude: 101.7), radius: 300),
                        property("p3", "No pin yet", pin: nil),
                        property("p4", "Old", pin: GeoPoint(latitude: 0, longitude: 0), archived: true),
                    ],
                    nextCursor: nil,
                    total: 4,
                    snapshotVersion: 1
                )
            )
        }
    }

    private func result(_ event: PresenceResultTransitionsEvent?, ran: Int = 0, property: String = "p1") throws -> StubTransport.Answer {
        let transitions =
            event.map { [PresenceResultTransitions(propertyId: property, event: $0, commandIds: (0..<ran).map { "c\($0)" })] } ?? []
        return try answer(PresenceResult(transitions: transitions))
    }

    private func model(
        _ answers: [StubTransport.Answer],
        location: ScriptedLocation = ScriptedLocation(),
        on: Bool = false,
        gate: Gate? = nil
    ) -> (PresenceModel, StubTransport) {
        let transport = StubTransport(answers, gate: gate)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let client = CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())
        return (PresenceModel(client: client, location: location, settings: Switch(on)), transport)
    }

    @Test func switchedOnItWatchesThePinsAndReportsWhereThePhoneIs() async throws {
        let location = ScriptedLocation()
        let (model, transport) = model([try properties, try result(.arrival, ran: 1)], location: location)
        await model.set(true)
        #expect(model.isOn)
        #expect(model.permission == .always)
        #expect(model.line == "Arrived at Home A — 1 automation ran.")
        #expect(
            await location.watched == [
                [
                    Geofence(propertyId: "p1", latitude: 3.1, longitude: 101.6, radiusMeters: 150),
                    Geofence(propertyId: "p2", latitude: 3.2, longitude: 101.7, radiusMeters: 300),
                ]
            ]
        )
        let report = try #require(await transport.requests.last)
        #expect(report.url?.path() == "/v1/presence")
        #expect(report.httpBody == Data(#"{"accuracyMeters":10,"latitude":3.139,"longitude":101.6869}"#.utf8))
    }

    @Test func crossingAPinReportsAgain() async throws {
        let location = ScriptedLocation()
        let (model, transport) = model(
            [try properties, try result(nil), try result(.departure, ran: 2, property: "p2")],
            location: location
        )
        await model.set(true)
        #expect(model.line == "Location sent · nothing changed.")
        await location.cross("p2")
        // the report goes through other actors, which yields alone do not move on: poll with a bound instead
        for _ in 0..<1000 where model.line == "Location sent · nothing changed." { try await Task.sleep(for: .milliseconds(5)) }
        #expect(model.line == "Left Office — 2 automations ran.")
        #expect(await transport.requests.count == 3)
    }

    @Test func switchedOffItWatchesNothing() async throws {
        let location = ScriptedLocation()
        let (model, _) = model([try properties, try result(nil)], location: location, on: true)
        await model.resume()
        await model.set(false)
        #expect(!model.isOn)
        #expect(model.line == nil)
        #expect(await location.stops == 1)
        await model.pause()
        #expect(await location.stops == 2)
    }

    @Test func resumingWhileWatchingAsksNothingAgain() async throws {
        let location = ScriptedLocation()
        let (model, transport) = model([try properties, try result(nil)], location: location, on: true)
        await model.resume()
        await model.resume()
        #expect(await location.watched.count == 1)
        #expect(await transport.requests.count == 2)
    }

    @Test func theAppAndTheScreenResumingTogetherStartOnce() async throws {
        let asking = Gate()
        let location = ScriptedLocation(asking: asking)
        let (model, transport) = model([try properties, try result(nil)], location: location, on: true)
        let atLaunch = Task { await model.resume() }
        await asking.until()
        let onScreen = Task { await model.resume() }
        await asking.until(2)
        #expect(await asking.waiting == 1, "the screen's resume asks nothing while the launch's start waits")
        await asking.open()
        await onScreen.value
        await atLaunch.value
        await onScreen.value
        #expect(await location.watched.count == 1)
        #expect(await transport.requests.count == 2, "one read of the pins and one report")
        #expect(model.line == "Location sent · nothing changed.")
    }

    @Test(arguments: [true, false])
    func offWhileThePermissionIsAskedStartsNothing(signedOut: Bool) async throws {
        let asking = Gate()
        let location = ScriptedLocation(asking: asking)
        let (model, transport) = model([try properties, try result(nil)], location: location)
        let starting = Task { await model.set(true) }
        await asking.until()
        if signedOut { await model.pause() } else { await model.set(false) }
        await asking.open()
        await starting.value
        #expect(await transport.requests.isEmpty, "no pins read and no position sent")
        #expect(await location.watched.isEmpty)
        #expect(model.line == nil)
    }

    @Test func offWhileThePinsAreReadSendsNothing() async throws {
        let location = ScriptedLocation()
        let reading = Gate()
        let (model, transport) = model([try properties, try result(nil)], location: location, gate: reading)
        let starting = Task { await model.set(true) }
        await reading.until()
        await model.set(false)
        await reading.open()
        await starting.value
        #expect(await transport.requests.count == 1, "no position sent")
        #expect(await location.watched.isEmpty)
        #expect(model.line == nil)
        let failing = Gate()
        let (refused, _) = self.model([.failure(URLError(.timedOut))], location: ScriptedLocation(), gate: failing)
        let failed = Task { await refused.set(true) }
        await failing.until()
        await refused.set(false)
        await failing.open()
        await failed.value
        #expect(refused.line == nil, "the switched-off start does not say why the pins could not be read")
    }

    @Test func offWhileThePositionIsSentSaysNothingAndWatchesNothing() async throws {
        let location = ScriptedLocation()
        let sending = Gate(after: 1)
        let (model, transport) = model([try properties, try result(.arrival)], location: location, gate: sending)
        let starting = Task { await model.set(true) }
        await sending.until()
        #expect(await transport.requests.count == 2)
        await model.set(false)
        await sending.open()
        await starting.value
        #expect(model.line == nil, "the arrival is not said under a switch that is off")
        #expect(await location.watched.isEmpty)
    }

    @Test func offWhileTheCirclesAreSetUpWatchesNothing() async throws {
        let settingUp = Gate()
        let location = ScriptedLocation(settingUp: settingUp)
        let (model, transport) = model([try properties, try result(nil), try result(.arrival)], location: location)
        let starting = Task { await model.set(true) }
        await settingUp.until()
        await model.set(false)
        await settingUp.open()
        await starting.value
        #expect(await location.stops == 2, "stopped by the switch and again once the circles were set up")
        await location.cross("p1")
        for _ in 0..<50 { await Task.yield() }
        #expect(await transport.requests.count == 2, "a crossing after the switch went off reports nothing")
        #expect(model.line == nil)
    }

    @Test func switchedOnAgainWhileStartingTheLaterStartWins() async throws {
        let asking = Gate()
        let location = ScriptedLocation(asking: asking)
        let (model, transport) = model([try properties, try result(nil)], location: location)
        let first = Task { await model.set(true) }
        await asking.until()
        let again = Task { await model.set(true) }
        await asking.until(2)
        await asking.open()
        await first.value
        await again.value
        #expect(await location.watched.count == 1)
        #expect(await location.stops == 0, "the earlier start leaves the later one's circles alone")
        #expect(await transport.requests.count == 2)
    }

    @Test func offAtLaunchAsksNothing() async {
        let location = ScriptedLocation()
        let (model, transport) = model([], location: location)
        await model.resume()
        #expect(await transport.requests.isEmpty)
        #expect(await location.watched.isEmpty)
    }

    @Test(arguments: [LocationPermission.denied, .notDetermined])
    func noPermissionSaysWhereToAllowIt(permission: LocationPermission) async {
        let (model, transport) = model([], location: ScriptedLocation(permission))
        await model.set(true)
        #expect(model.line == "Location is off for this app — allow it in Settings, or switch this off.")
        #expect(await transport.requests.isEmpty)
    }

    @Test func onlyWhileInUseAsksForAlways() async throws {
        let (model, _) = model([try properties, try result(.arrival)], location: ScriptedLocation(.whenInUse))
        await model.set(true)
        #expect(model.line == "Allow location “Always” so that arriving and leaving are reported while the app is closed.")
    }

    @Test func noPinsYetSaysWhoSetsThem() async throws {
        let none = try answer(Page(items: [property("p3", "No pin yet", pin: nil)], nextCursor: nil, total: 1, snapshotVersion: 1))
        let (model, _) = model([none])
        await model.set(true)
        #expect(model.line == "None of your homes or offices has a map pin yet — HQ sets them.")
    }

    @Test func unreadablePlacesSayWhy() async {
        let (model, _) = model([.failure(URLError(.notConnectedToInternet))])
        await model.set(true)
        #expect(model.line == "No connection — check the network and try again")
    }

    @Test(arguments: [nil, Fix(latitude: 3, longitude: 101, accuracyMeters: 250, at: Date())])
    func coarseOrNoFixIsNotSent(fix: Fix?) async throws {
        let (model, transport) = model([try properties], location: ScriptedLocation(fixes: [fix]))
        await model.set(true)
        #expect(model.line == "This phone’s position is not precise enough to send — it is tried again at the next crossing.")
        #expect(await transport.requests.count == 1, "only the places were read")
    }

    @Test func refusedReportIsTriedAtTheNextCrossing() async throws {
        let (model, _) = model([try properties, .http(503, "down")])
        await model.set(true)
        #expect(model.line == "This device’s location could not be sent — the next one is tried in a moment.")
    }

    @Test func reportsInWords() {
        let names = ["p1": "Home A"]
        func text(_ event: PresenceResultTransitionsEvent, _ ran: Int, _ id: String = "p1") -> String? {
            PresenceModel.text(
                PresenceResult(transitions: [.init(propertyId: id, event: event, commandIds: (0..<ran).map(String.init))]),
                names: names
            )
        }
        #expect(text(.arrival, 0) == "Arrived at Home A.")
        #expect(text(.departure, 0) == "Left Home A.")
        #expect(text(.arrival, 1) == "Arrived at Home A — 1 automation ran.")
        #expect(text(.departure, 1) == "Left Home A — 1 automation ran.")
        #expect(text(.arrival, 3) == "Arrived at Home A — 3 automations ran.")
        #expect(text(.departure, 3, "p9") == "Left your property — 3 automations ran.")
        #expect(PresenceModel.text(PresenceResult(transitions: []), names: names) == nil)
    }

    @Test func choiceIsKeptOnThisPhone() throws {
        let suite = "ackit.presence.\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let settings = UserDefaultsPresenceSettings(defaults: defaults)
        #expect(!settings.isOn)
        settings.set(true)
        #expect(UserDefaultsPresenceSettings(defaults: defaults).isOn)
        #expect(UserDefaultsPresenceSettings().isOn == UserDefaults.standard.bool(forKey: UserDefaultsPresenceSettings.key))
    }
}

extension PresenceModelTests {
    private func consent(granted: Bool, version: Double = 4) -> Consent {
        Consent(
            id: "k1",
            tenantId: "t1",
            version: version,
            createdAt: "2026-11-20T00:00:00Z",
            updatedAt: "2026-11-20T00:00:00Z",
            membershipId: "m1",
            granted: granted,
            grantedAt: nil,
            revokedAt: nil
        )
    }

    @Test func consentIsReadGrantedAndWithdrawn() async throws {
        let location = ScriptedLocation()
        let (model, transport) = model(
            [
                try answer(consent(granted: false)), try answer(consent(granted: true, version: 5)), try properties, try result(nil),
                try answer(consent(granted: false, version: 6)),
            ],
            location: location
        )
        await model.loadConsent()
        #expect(model.hasConsentRecord && !model.isConsentGranted)
        await model.setConsent(true)
        #expect(model.isConsentGranted)
        #expect(model.line == "Location consent granted")
        let grant = try #require(await transport.requests.dropFirst().first)
        #expect(grant.httpMethod == "PUT" && grant.url?.path() == "/v1/consents/location_automation")
        #expect(grant.value(forHTTPHeaderField: "X-Expected-Version") == "4")
        #expect(grant.httpBody == Data(#"{"granted":true}"#.utf8))
        await model.set(true)
        await model.setConsent(false)
        #expect(!model.isConsentGranted)
        #expect(!model.isOn, "withdrawing turns this phone's reports off")
        #expect(model.line == nil)
        #expect(await location.stops == 1)
    }

    /// A second tap of "Grant consent" while the first is saved sends nothing: it would send the version already used and
    /// come back as "Someone changed this in the meantime".
    @Test func aSecondTapWhileTheConsentIsSavedSendsNothing() async throws {
        let saving = Gate(after: 1)
        let (model, transport) = model([try answer(consent(granted: false)), try answer(consent(granted: true, version: 5))], gate: saving)
        await model.loadConsent()
        #expect(!model.isSavingConsent)
        let first = Task { await model.setConsent(true) }
        await saving.until()
        #expect(model.isSavingConsent, "Grant consent and Withdraw consent are disabled while it is saved")
        // the second tap runs on its own: had it sent a request, that would wait at the gate too and the count say so
        let second = Task { await model.setConsent(true) }
        await saving.until(2)
        #expect(await saving.waiting == 1, "only the first tap's update waits")
        await saving.open()
        await first.value
        await second.value
        #expect(!model.isSavingConsent)
        #expect(await transport.requests.count == 2, "the read and one update, nothing for the second tap")
        #expect(model.line == "Location consent granted")
        #expect(model.isConsentGranted)
    }

    @Test func withdrawingWhileOffChangesNothingElse() async throws {
        let (model, _) = model([try answer(consent(granted: true)), try answer(consent(granted: false, version: 5))])
        await model.loadConsent()
        await model.setConsent(false)
        #expect(model.line == "Location consent withdrawn — location automations are off")
    }

    @Test func accountWithoutAConsentRecordCannotUseLocation() async {
        let missing = StubTransport.Answer.http(
            404,
            #"{"code":"NOT_FOUND","messageKey":"error.notFound","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#
        )
        let (model, transport) = model([missing])
        await model.loadConsent()
        #expect(!model.hasConsentRecord && model.consent == nil && model.line == nil)
        #expect(!model.isConsentGranted)
        await model.setConsent(true)
        #expect(await transport.requests.count == 1, "nothing to grant without a record")
    }

    @Test func unreadableConsentSaysWhy() async {
        let (model, _) = model([.failure(URLError(.timedOut))])
        await model.loadConsent()
        #expect(model.hasConsentRecord)
        #expect(model.line == "The request took too long — check whether the change was saved, then try again")
    }

    @Test func theSwitchGrantsTheConsentFirstAndOffKeepsIt() async throws {
        let location = ScriptedLocation()
        let (model, transport) = model(
            [try answer(consent(granted: false)), try answer(consent(granted: true, version: 5)), try properties, try result(nil)],
            location: location
        )
        await model.loadConsent()
        #expect(!model.isInUse)
        await model.use(true)
        #expect(model.isConsentGranted && model.isOn && model.isInUse)
        let grant = try #require(await transport.requests.dropFirst().first)
        #expect(grant.httpMethod == "PUT" && grant.httpBody == Data(#"{"granted":true}"#.utf8))
        await model.use(false)
        #expect(!model.isOn && !model.isInUse)
        #expect(model.isConsentGranted, "switching off keeps the consent")
        #expect(await transport.requests.count == 4, "nothing sent for the switch going off")
    }

    @Test func theSwitchWithTheConsentGrantedAsksNoConsent() async throws {
        let (model, transport) = model([try answer(consent(granted: true)), try properties, try result(nil)])
        await model.loadConsent()
        await model.use(true)
        #expect(model.isInUse)
        #expect(await transport.requests.count == 3, "the read, the pins and the report: no consent update")
    }

    @Test func aRefusedGrantLeavesTheSwitchOff() async throws {
        let conflict = StubTransport.Answer.http(
            409,
            #"{"code":"CONFLICT","messageKey":"error.versionConflict","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#
        )
        let (model, _) = model([try answer(consent(granted: false)), conflict])
        await model.loadConsent()
        await model.use(true)
        #expect(!model.isOn && !model.isInUse)
        #expect(model.line == "Someone changed this in the meantime — the screen now shows the latest version")
    }

    @Test func refusedConsentChangeSaysWhy() async throws {
        let conflict = StubTransport.Answer.http(
            409,
            #"{"code":"CONFLICT","messageKey":"error.versionConflict","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#
        )
        let (model, _) = model([try answer(consent(granted: false)), conflict])
        await model.loadConsent()
        await model.setConsent(true)
        #expect(model.line == "Someone changed this in the meantime — the screen now shows the latest version")
        #expect(!model.isConsentGranted)
        #expect(!model.isSavingConsent, "a refused change can be tried again")
    }

    @Test func reportWithoutTheConsentSaysWhatItIsFor() async throws {
        let required = StubTransport.Answer.http(
            409,
            #"{"code":"CONFLICT","messageKey":"errors.consent_required","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#
        )
        let (model, _) = model([try properties, required])
        await model.set(true)
        #expect(model.line?.hasPrefix("Location automations need your consent.") == true)
    }
}
