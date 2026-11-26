import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import ACShared

@Suite(.serialized) struct LocalizerTests {
    @Test(arguments: [("ja-JP", DisplayLanguage.ja), ("ms", .ms), ("id_ID", .id), ("EN-gb", .en)])
    func languageOfACode(code: String, language: DisplayLanguage) {
        #expect(DisplayLanguage(code: code) == language)
    }

    @Test func languageTheAppsDoNotHaveIsNone() {
        #expect(DisplayLanguage(code: "fr-FR") == nil)
        #expect(DisplayLanguage(code: "") == nil)
        #expect(DisplayLanguage.preferred(["fr", "de"]) == .en)
        #expect(DisplayLanguage.preferred(["fr", "ja-JP", "ms"]) == .ja)
    }

    @Test func textsComeFromTheChosenLanguagesTable() throws {
        // a bundle with the Japanese table the String Catalog compiles to
        let bundle = FileManager.default.temporaryDirectory.appending(path: "ackit-\(UUID().uuidString).bundle")
        let ja = bundle.appending(path: "ja.lproj")
        try FileManager.default.createDirectory(at: ja, withIntermediateDirectories: true)
        let table = #""Sign in" = "サインイン";"# + "\n" + #""Hello {name}" = "{name} さん";"#
        try Data(table.utf8).write(to: ja.appending(path: "Localizable.strings"))
        defer { try? FileManager.default.removeItem(at: bundle) }
        let localized = try #require(Bundle(url: bundle))
        Localizer.with(.ja, in: localized) {
            #expect(Localizer.language == .ja)
            #expect(tr("Sign in") == "サインイン")
            #expect(tr("Hello {name}", ["name": "Aiko"]) == "Aiko さん")
            #expect(tr("Not in the table") == "Not in the table")
        }
        // a language without a table shows the keys, never another language's texts
        Localizer.with(.ms, in: localized) { #expect(tr("Sign in") == "Sign in") }
    }

    @Test func placeholdersAreFilledOnceAsOnTheWeb() {
        // a value is never read for placeholders (a unit named "{b}"), whatever order the values come in
        #expect(tr("{a} and {b}", ["a": "{b}", "b": "{a}"]) == "{b} and {a}")
        #expect(tr("{a}{a} {unknown}", ["a": "x"]) == "xx {unknown}")
        let device = tr("Device {serial} under a restriction ({restriction})", ["serial": "{restriction}", "restriction": "Power off"])
        #expect(device == "Device {restriction} under a restriction (Power off)")
    }

    @Test func taskSeesItsOwnLanguage() {
        // the app's own choice is Launch's (LaunchTests); a test that needs a language sets its own
        #expect(Localizer.with(.ms) { Localizer.language } == .ms)
        #expect(Localizer.with(.id) { Localizer.with(.ja) { Localizer.language } } == .ja)
    }
}

@Suite struct ErrorTextTests {
    private func refusal(_ code: ErrorCode, _ key: String, fields: [String: String] = [:], retry: Double? = nil) -> DomainError {
        DomainError(code: code, messageKey: key, fieldErrors: fields, correlationId: "c", retryAfterSeconds: retry)
    }

    @Test func fieldErrorNamesTheField() {
        #expect(ErrorText.of(refusal(.validation, "error.validation", fields: ["newUser.email": "error.required"])) == "Email is required")
        #expect(ErrorText.of(refusal(.validation, "error.validation", fields: ["serial": "error.weird_thing"])) == "Serial — weird thing")
        // of several fields, the first by name is shown (the same one every time)
        let two = refusal(.validation, "error.validation", fields: ["zone": "error.required", "name": "error.length"])
        #expect(ErrorText.of(two) == "Name is too short or too long")
    }

    @Test func messageKeysAndCodesAreWordedAsOnTheWeb() {
        #expect(
            ErrorText.of(refusal(.conflict, "error.versionConflict"))
                == "Someone changed this in the meantime — the screen now shows the latest version"
        )
        #expect(
            ErrorText.of(refusal(.rateLimited, "error.rateLimited", retry: 12)) == "Too many changes in a short time — try again in 12 s"
        )
        #expect(ErrorText.of(refusal(.rateLimited, "error.rateLimited")) == "Too many changes in a short time — try again shortly")
        #expect(ErrorText.of(refusal(.offline, "ios.noConnection")) == "No connection — check the network and try again")
        #expect(ErrorText.of(refusal(.offline, "error.offline")) == "The device is offline — try again when it is back online")
        #expect(ErrorText.of(refusal(.conflict, "errors.some_new_rule")) == "Some new rule")
        #expect(ErrorText.of(refusal(.forbidden, "error.forbidden")) == "You don't have permission for this action")
        #expect(ErrorText.of(refusal(.timeout, "error.somethingNew")) == "TIMEOUT: Something new")
        let error = CoreError(status: 503, error: refusal(.unavailable, "error.unavailable"))
        #expect(ErrorText.of(error) == "The service is unavailable — try again shortly")
    }

    @Test func signInRefusals() {
        #expect(ErrorText.of(SignInError.unavailable) == "The sign-in service is unavailable — try again shortly")
        #expect(ErrorText.of(SignInError.refused) == "The sign-in did not complete — try again")
        #expect(ErrorText.of(SignInError.wrongRole("client")) == "This account is not for this app — sign in with another account")
    }

    @Test func keysInWords() {
        #expect(ErrorText.humanized("paymentReference") == "Payment reference")
        #expect(ErrorText.humanized("error.a_b") == "A b")
        #expect(ErrorText.fieldName("a.b.unitId") == "Unit id")
        #expect(ErrorText.fieldName("").isEmpty)
    }
}

@Suite struct AppConfigurationTests {
    private let info: [String: Any] = [
        "ACCoreAPIURL": "http://127.0.0.1:8080", "ACIssuerURL": "http://localhost:8081/realms/ac",
        "ACClientID": "ac-customer-ios", "ACRedirectURI": "ac-customer://oauth/callback",
    ]

    @Test func readsTheInfoPlist() throws {
        let configuration = try AppConfiguration(info: info, role: "client")
        #expect(configuration.coreAPI.absoluteString == "http://127.0.0.1:8080")
        #expect(configuration.oidc.issuer.absoluteString == "http://localhost:8081/realms/ac")
        #expect(configuration.oidc.clientId == "ac-customer-ios")
        #expect(configuration.oidc.callbackScheme == "ac-customer")
        #expect(configuration.role == "client")
    }

    @Test(arguments: ["ACCoreAPIURL", "ACIssuerURL", "ACClientID", "ACRedirectURI"])
    func everyKeyIsRequired(key: String) {
        var missing = info
        missing[key] = nil
        #expect(throws: AppConfiguration.Invalid(key: key)) { try AppConfiguration(info: missing, role: "client") }
        var empty = info
        empty[key] = key == "ACClientID" ? "" : "no scheme"
        #expect(throws: AppConfiguration.Invalid(key: key)) { try AppConfiguration(info: empty, role: "client") }
    }
}

@Suite struct LoadableTests {
    @Test func keepsTheLastValueWhenAReloadFails() async {
        let first = await Loadable<Int>.loading.after { () async throws(CoreError) in 1 }
        #expect(first.value == 1)
        #expect(first.message == nil)
        let failed = await first.after { () async throws(CoreError) -> Int in
            throw CoreError(
                status: 503,
                error: DomainError(
                    code: .unavailable,
                    messageKey: "error.unavailable",
                    fieldErrors: [:],
                    correlationId: "",
                    retryAfterSeconds: nil
                )
            )
        }
        #expect(failed.value == 1)
        #expect(failed.message == "The service is unavailable — try again shortly")
        #expect(Loadable<Int>.loading.value == nil)
        #expect(Loadable<Int>.loading.message == nil)
    }
}

@Suite struct LabelsTests {
    @Test func everyStateHasWords() throws {
        for status in JobDisplayStatus.allCases { #expect(!Labels.job(status).isEmpty) }
        for status in JobStatus.allCases {
            #expect(Labels.job(status) == Labels.job(try #require(JobDisplayStatus(rawValue: status.rawValue))))
        }
        for type in MaintenanceJobType.allCases { #expect(!Labels.jobType(type).isEmpty) }
        for state in EffectivePowerState.allCases { #expect(!Labels.power(state).isEmpty) }
        for connection in Connection.allCases { #expect(!Labels.connection(connection).isEmpty) }
        #expect(Labels.job(JobDisplayStatus.inProgress) == "In progress")
        #expect(Labels.jobType(.reactive) == "Repair")
    }
}

@MainActor @Suite struct SessionModelTests {
    private let discovery =
        #"""
        {"authorization_endpoint":"http://kc.test/auth","token_endpoint":"http://kc.test/token",
         "end_session_endpoint":"http://kc.test/logout"}
        """#
    private func model(_ answers: [StubTransport.Answer], store: MemoryStore = MemoryStore()) throws -> SessionModel {
        let info: [String: Any] = [
            "ACCoreAPIURL": "http://core.test", "ACIssuerURL": "http://kc.test", "ACClientID": "ac-customer-ios",
            "ACRedirectURI": "ac-customer://oauth/callback",
        ]
        return SessionModel(
            configuration: try AppConfiguration(info: info, role: "client"),
            store: store,
            transport: StubTransport(answers)
        )
    }

    private func tokens(_ role: String) -> String {
        let access = jwt(["tenant_id": "t1", "membership_id": "m1", "role": role])
        return #"{"access_token":"\#(access)","refresh_token":"r1","expires_in":300}"#
    }

    /// The provider's page: returns the redirect with the request's state and a code.
    private func provider(_ url: URL, _ scheme: String) async throws -> URL {
        #expect(scheme == "ac-customer")
        let state = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "state" }?.value ?? ""
        return URL(string: "ac-customer://oauth/callback?code=c1&state=\(state)") ?? url
    }

    @Test func storedSessionStartsSignedIn() throws {
        let stored = StoredSession(
            accessToken: "a",
            refreshToken: nil,
            idToken: nil,
            expiresAt: .distantFuture,
            claims: SessionClaims(tenantId: "t1", membershipId: "m1", role: "client")
        )
        let model = try model([], store: MemoryStore(stored))
        #expect(model.state == .signedIn(stored.claims))
    }

    @Test func signInThroughTheProvidersPage() async throws {
        let model = try model([.http(200, discovery), .http(200, tokens("client"))])
        #expect(model.state == .signedOut(message: nil))
        await model.signIn(using: provider)
        #expect(model.state == .signedIn(SessionClaims(tenantId: "t1", membershipId: "m1", role: "client")))
    }

    @Test func closedPageIsNoRefusal() async throws {
        let model = try model([.http(200, discovery)])
        await model.signIn { _, _ in throw CancellationError() }
        #expect(model.state == .signedOut(message: nil))
    }

    @Test func accountOfAnotherRoleIsRefusedWithItsLine() async throws {
        let model = try model([.http(200, discovery), .http(200, tokens("technician")), .http(204, "")])
        await model.signIn(using: provider)
        #expect(model.state == .signedOut(message: "This account is not for this app — sign in with another account"))
    }

    @Test func signOutAndASessionTheAPIRefuses() async throws {
        let refused =
            #"""
            {"code":"UNAUTHENTICATED","messageKey":"error.unauthenticated","fieldErrors":{},"correlationId":"c",
             "retryAfterSeconds":null}
            """#
        let model = try model([.http(200, discovery), .http(200, tokens("client")), .http(401, refused)])
        await model.signIn(using: provider)
        _ = try? await model.client.call(Ops.UnitsList.self, Query())
        #expect(model.state == .signedOut(message: "Your session ended — sign in again"))
        await model.signIn { _, _ in throw CancellationError() }
        await model.signOut()
        #expect(model.state == .signedOut(message: nil))
    }
}

@MainActor @Suite struct UITestStubTests {
    private func configuration(role: String) throws -> AppConfiguration {
        try AppConfiguration(
            info: [
                "ACCoreAPIURL": "http://stub", "ACIssuerURL": "http://stub", "ACClientID": "c",
                "ACRedirectURI": "ac-customer://oauth/callback",
            ],
            role: role
        )
    }

    /// The stub's answers are answers of the contract: the UI tests show what the real API would make the screens show.
    @Test func stubSignsInAndAnswersInTheContractsTypes() async throws {
        #expect(!UITestStub.isActive([:]))
        #expect(UITestStub.isActive(["AC_UI_TEST": "stub"]))
        let session = UITestStub.session(configuration: try configuration(role: "client"))
        await session.signIn(using: UITestStub.signIn)
        #expect(session.state == .signedIn(SessionClaims(tenantId: "t1", membershipId: "m1", role: "client")))
        let units = try await session.client.call(Ops.UnitsList.self, Query())
        #expect(units.data.items.map(\.displayName) == ["Living room AC", "Bedroom AC"])
        let jobs = try await session.client.call(Ops.JobsList.self, Query())
        #expect(jobs.data.items.count == 1)
        // a route the stub does not answer is refused
        await #expect(throws: CoreError.self) { try await session.client.call(Ops.DevicesGet.self, DevicesGetInput(id: "none")) }
        await session.signOut()
        #expect(session.state == .signedOut(message: nil))
    }
}

@Suite(.serialized) struct FormatTests {
    private let malaysia = TimeZone(identifier: "Asia/Kuala_Lumpur") ?? .gmt

    @Test func instantInTheDisplayLanguageAndTimeZone() {
        Localizer.with(.en) {
            // 01:00 UTC is 09:00 in Kuala Lumpur; the words around it are the locale's (CLDR), so only the parts are checked
            let english = Format.dateTime("2026-12-01T01:00:00Z", timeZone: malaysia)
            #expect(english.contains("Dec") && english.contains("2026") && english.contains("9:00"))
            #expect(
                Format.dateTime("2026-12-01T01:00:00.250Z", timeZone: malaysia)
                    == Format.dateTime("2026-12-01T01:00:00Z", timeZone: malaysia)
            )
        }
        Localizer.with(.ja) {
            let japanese = Format.dateTime("2026-12-01T01:00:00Z", timeZone: malaysia)
            #expect(japanese.contains("2026") && japanese.contains("9:00") && !japanese.contains("Dec"))
        }
    }

    @Test func textThatIsNoInstantIsShownAsItIs() {
        #expect(Format.dateTime("2026-12-02") == "2026-12-02")
        #expect(Format.parse("soon") == nil)
    }
}

@MainActor @Suite struct LaunchTests {
    private let configured = [
        "ACCoreAPIURL": "http://127.0.0.1:8080", "ACIssuerURL": "http://localhost:8081/realms/ac", "ACClientID": "ac-customer-ios",
        "ACRedirectURI": "ac-customer://oauth/callback",
    ]

    /// A bundle with this Info.plist, or with none (nil).
    private func bundle(_ info: [String: String]?) throws -> Bundle {
        let url = FileManager.default.temporaryDirectory.appending(path: "ackit-launch-\(UUID().uuidString).bundle")
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        if let info {
            let plist = try PropertyListSerialization.data(fromPropertyList: info, format: .xml, options: 0)
            try plist.write(to: url.appending(path: "Info.plist"))
        }
        return try #require(Bundle(url: url))
    }

    @Test func stubbedUITestSignsInWithTheStub() async throws {
        let launch = Launch.start(
            role: "client",
            bundle: try bundle(configured),
            environment: ["AC_UI_TEST": "stub", "AC_UI_TEST_ROLE": "technician"]
        )
        guard case let .ready(session, web?) = launch else {
            Issue.record("a stubbed UI test signs in with the stub")
            return
        }
        await session.signIn(using: web)
        #expect(session.state == .signedOut(message: "This account is not for this app — sign in with another account"))
    }

    @Test func bundleWithoutAnInfoPlistIsMisconfigured() throws {
        guard case .misconfigured = Launch.start(role: "client", bundle: try bundle(nil), environment: [:]) else {
            Issue.record("an app without an Info.plist says so")
            return
        }
    }

    @Test func appWithItsConfigurationIsReadyAndSignedOut() throws {
        var identified = configured
        identified["CFBundleIdentifier"] = "ackit.launch.tests"
        let ready = Launch.start(role: "client", bundle: try bundle(identified), environment: [:])
        guard case let .ready(session, web) = ready else {
            Issue.record("a configured app is ready")
            return
        }
        #expect(web == nil)
        #expect(session.state == .signedOut(message: nil))
    }

    @Test func appWithoutItsConfigurationNamesTheKey() throws {
        guard case let .misconfigured(key) = Launch.start(role: "client", bundle: try bundle([:]), environment: [:]) else {
            Issue.record("an app without its configuration says so")
            return
        }
        #expect(key == "ACClientID")
    }
}

extension LaunchTests {
    @Test func appWithoutABundleIdentifierStillKeepsItsSession() throws {
        guard case .ready = Launch.start(role: "client", bundle: try bundle(configured), environment: [:]) else {
            Issue.record("a configured app is ready")
            return
        }
    }
}

@Suite struct WordsTests {
    private let day = "2026-11-20T00:00:00Z"

    private func command(
        _ action: CommandAction,
        _ status: CommandStatus = .acknowledged,
        failure: CommandFailureCode? = nil,
        source: CommandSource = .ui,
        actor: String = "m1"
    )
        -> Command
    {
        Command(
            id: "c1",
            tenantId: "t1",
            version: 1,
            createdAt: day,
            updatedAt: day,
            unitId: "u1",
            actorMembershipId: actor,
            action: action,
            diagnosticRunId: nil,
            jobId: nil,
            reason: nil,
            status: status,
            delivery: .sent,
            requestedAt: day,
            sentAt: nil,
            acknowledgedAt: nil,
            expiresAt: day,
            failureCode: failure,
            correlationId: "c1",
            source: source
        )
    }

    @Test func settingsAndCommandsInWords() {
        #expect(Words.celsius(24) == "24")
        #expect(Words.celsius(24.5) == "24.5")
        #expect(Mode.allCases.map(Words.mode) == ["Cool", "Dry", "Fan"])
        #expect(Fan.allCases.map(Words.fan) == ["Low", "Mid", "High"])
        #expect(Words.action(.setPower(UnitActionSetPower(power: true))) == "Set power ON")
        #expect(Words.action(.setPower(UnitActionSetPower(power: false))) == "Set power OFF")
        #expect(Words.action(.setTemperature(UnitActionSetTemperature(celsius: 24))) == "Set temperature 24°C")
        #expect(Words.action(.setMode(UnitActionSetMode(mode: .dry))) == "Set mode DRY")
        #expect(Words.action(.setFan(UnitActionSetFan(fanLevel: .high))) == "Set fan HIGH")
        #expect(Words.action(.ventilate(UnitActionVentilate(level: .low))) == "Set ventilation LOW")
    }

    @Test func historyCommandsIncludeTheRestrictionsOwn() {
        let limit = RestrictionPolicy.temperatureLimit(RestrictionPolicyTemperatureLimit(minimumCoolingSetpoint: 26))
        let apply = CommandAction.applyRestriction(CommandActionApplyRestriction(restrictionId: "r1", rulesVersion: "v1", policy: limit))
        #expect(Words.command(apply) == "Apply restriction: Temperature limit — cooling setpoint ≥ 26 °C")
        let off = CommandAction.applyRestriction(
            CommandActionApplyRestriction(restrictionId: "r1", rulesVersion: "v1", policy: .powerOff(RestrictionPolicyPowerOff()))
        )
        #expect(Words.command(off) == "Apply restriction: Power off")
        #expect(
            Words.command(.removeRestriction(CommandActionRemoveRestriction(restrictionId: "r1", rulesVersion: "v1")))
                == "Remove restriction"
        )
        for action: CommandAction in [
            .setPower(UnitActionSetPower(power: true)), .setTemperature(UnitActionSetTemperature(celsius: 20)),
            .setMode(UnitActionSetMode(mode: .fan)), .setFan(UnitActionSetFan(fanLevel: .low)),
            .ventilate(UnitActionVentilate(level: .mid)),
        ] {
            #expect(!Words.command(action).isEmpty)
        }
    }

    @Test func statesAndFailuresInWords() {
        #expect(CommandStatus.allCases.map(Words.status).count == 6)
        #expect(Words.status(.expired) == "no device response (expired)")
        #expect(Words.failure(.deviceRejected) == "the AC refused it")
        #expect(Words.failure(.timeout) == "no answer from the AC in time")
        #expect(Words.failure(.offline) == "the AC was offline")
        #expect(Words.failure(.unavailable) == "the AC reported a fault")
        #expect(Words.failure(.conflict) == "another command was still running")
    }

    @Test func historyLineSaysWhoSentIt() {
        let power = CommandAction.setPower(UnitActionSetPower(power: true))
        #expect(Words.history(command(power), me: "m1") == "Set power ON — acknowledged by device · by you")
        #expect(Words.history(command(power, actor: "m2"), me: "m1") == "Set power ON — acknowledged by device · by someone else")
        #expect(Words.history(command(power), me: nil) == "Set power ON — acknowledged by device")
        #expect(Words.history(command(power, source: .group), me: "m1") == "Set power ON — acknowledged by device · by you")
        #expect(
            Words.history(command(power, source: .diagnostic), me: "m1") == "Set power ON — acknowledged by device · technician test run"
        )
        #expect(
            Words.history(command(power, .failed, failure: .timeout), me: nil) == "Set power ON — failed: no answer from the AC in time"
        )
        #expect(Words.history(command(power, .expired, failure: .timeout), me: nil) == "Set power ON — no device response (expired)")
    }

    @Test func readingsKeepTheirQuality() throws {
        let base = """
            {"id":"m1","tenantId":"t1","version":1,"createdAt":"x","updatedAt":"x","boundaryId":null,"qualityReason":null,"rawUnit":null,
             "unitId":"u1","sensorId":"s1","metric":"METRIC","value":VALUE,"unit":"UNIT","observedAt":"x","receivedAt":"x",
             "origin":"measured","quality":"QUALITY","isDemo":true,"sequence":1,"eventId":"e1"}
            """
        func reading(_ metric: String, _ value: String, _ unit: String, _ quality: String = "valid") throws -> String? {
            let text = base.replacingOccurrences(of: "METRIC", with: metric).replacingOccurrences(of: "VALUE", with: value)
                .replacingOccurrences(of: "UNIT", with: unit).replacingOccurrences(of: "QUALITY", with: quality)
            return Words.reading(try JSONDecoder().decode(CoreMeasurement.self, from: Data(text.utf8)))
        }
        #expect(try reading("temperature", "28", "°C") == "28.0 °C")
        #expect(try reading("humidity", "60.04", "%", "stale") == "60.0 % (stale)")
        #expect(try reading("co2", "812.6", "ppm", "suspect") == "813 ppm (suspect)")
        #expect(try reading("pm25", "12", "µg/m³", "missing") == "12 µg/m³ (missing)")
        #expect(try reading("power", "0.75", "kW") == "0.75 kW")
        #expect(try reading("temperature", "null", "°C") == nil)
    }
}

@MainActor @Suite struct SessionPreferencesTests {
    private let discovery = #"{"authorization_endpoint":"http://kc.test/auth","token_endpoint":"http://kc.test/token"}"#

    private func session(
        _ answers: [StubTransport.Answer],
        shown: @escaping @MainActor (DisplayLanguage, TimeZone) -> Void
    ) throws -> SessionModel {
        let info: [String: Any] = [
            "ACCoreAPIURL": "http://core.test", "ACIssuerURL": "http://kc.test", "ACClientID": "ac-customer-ios",
            "ACRedirectURI": "ac-customer://oauth/callback",
        ]
        let access = jwt(["tenant_id": "t1", "membership_id": "m1", "role": "client"])
        let tokens = #"{"access_token":"\#(access)","refresh_token":"r1","expires_in":300}"#
        return SessionModel(
            configuration: try AppConfiguration(info: info, role: "client"),
            store: MemoryStore(),
            transport: StubTransport([.http(200, discovery), .http(200, tokens)] + answers),
            display: shown
        )
    }

    private func signIn(_ session: SessionModel) async {
        await session.signIn { url, _ in
            let state = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first { $0.name == "state" }?.value ?? ""
            return URL(string: "ac-customer://oauth/callback?code=c&state=\(state)") ?? url
        }
    }

    @Test func membershipsLanguageAndTimeZoneAreShownAfterSignIn() async throws {
        var shown: [(DisplayLanguage, String)] = []
        let preferences = #"""
            {"data":{"locale":"ja","timezone":"Asia/Tokyo","currency":"MYR","monthlyReportEmail":false,
             "twoFactorEnabled":false,"pinnedUnitIds":[]},
             "meta":{"correlationId":"c"}}
            """#
        let session = try session([.http(200, preferences)]) { shown.append(($0, $1.identifier)) }
        #expect(session.membershipId == nil)
        await session.applyPreferences()
        #expect(shown.isEmpty, "signed out: nothing to read")
        await signIn(session)
        #expect(session.membershipId == "m1")
        await session.applyPreferences()
        #expect(shown.map(\.0) == [.ja])
        #expect(shown.map(\.1) == ["Asia/Tokyo"])
        #expect(session.language == .ja)
    }

    @Test func unreadablePreferencesKeepTheDevicesChoice() async throws {
        var shown = 0
        let session = try session([.http(503, "down")]) { _, _ in shown += 1 }
        await signIn(session)
        await session.applyPreferences()
        #expect(shown == 0)
    }

    @Test func unknownTimeZoneIsTheDevicesOne() async throws {
        var zones: [TimeZone] = []
        let preferences = #"""
            {"data":{"locale":"en","timezone":"Mars/Olympus","currency":"MYR","monthlyReportEmail":false,
             "twoFactorEnabled":false,"pinnedUnitIds":[]},
             "meta":{"correlationId":"c"}}
            """#
        let session = try session([.http(200, preferences)]) { _, zone in zones.append(zone) }
        await signIn(session)
        await session.applyPreferences()
        #expect(zones == [.current])
    }
}

@Suite struct LocalizerTimeZoneTests {
    @Test func timesFollowTheChosenTimeZone() {
        // the app's own setting is Launch's; this reads it without changing it
        #expect(Localizer.timeZone.identifier.isEmpty == false)
        #expect(Format.dateTime("2026-12-01T01:00:00Z") == Format.dateTime("2026-12-01T01:00:00Z", timeZone: Localizer.timeZone))
    }
}

extension SessionPreferencesTests {
    @Test func everyLocaleOfTheContractIsADisplayLanguage() {
        for locale in CoreLocale.allCases { #expect(SessionModel.language(locale).rawValue == locale.rawValue) }
    }

    @Test func appShowsTheChosenLanguageAndTimeZone() {
        // the app's own display, back to the device's afterwards (the other tests set their own with Localizer.with)
        let zone = TimeZone(identifier: "Asia/Tokyo") ?? .current
        SessionModel.show(.en, in: zone)
        #expect(Localizer.timeZone == zone)
        SessionModel.show(.preferred(), in: .current)
    }
}

extension SessionPreferencesTests {
    /// The app's own display (no recorder): preferences that name the display already shown, so no other test sees a
    /// change while they run side by side.
    @Test func appsDisplayTakesThePreferences() async throws {
        let info: [String: Any] = [
            "ACCoreAPIURL": "http://core.test", "ACIssuerURL": "http://kc.test", "ACClientID": "ac-customer-ios",
            "ACRedirectURI": "ac-customer://oauth/callback",
        ]
        let access = jwt(["tenant_id": "t1", "membership_id": "m1", "role": "client"])
        let tokens = #"{"access_token":"\#(access)","refresh_token":"r1","expires_in":300}"#
        let language = Localizer.language.rawValue
        let zone = Localizer.timeZone.identifier
        let preferences = #"""
            {"data":{"locale":"\#(language)","timezone":"\#(zone)","currency":"MYR","monthlyReportEmail":false,"twoFactorEnabled":false,
             "pinnedUnitIds":[]},"meta":{"correlationId":"c"}}
            """#
        let session = SessionModel(
            configuration: try AppConfiguration(info: info, role: "client"),
            store: MemoryStore(),
            transport: StubTransport([.http(200, discovery), .http(200, tokens), .http(200, preferences)])
        )
        await signIn(session)
        await session.applyPreferences()
        #expect(Localizer.language.rawValue == language)
        #expect(Localizer.timeZone.identifier == zone)
    }
}
