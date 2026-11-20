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
        await #expect(throws: CoreError.self) { try await session.client.call(Ops.SessionGet.self, Empty()) }
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
