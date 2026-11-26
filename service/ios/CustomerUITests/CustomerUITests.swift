import XCTest

/// AC Customer's screens (DEC-112). The stubbed tests run in CI without a server (AC_UI_TEST=stub); the live test signs
/// in to the dev stack's Keycloak and reads its Core API (run with TEST_RUNNER_AC_UI_LIVE=1, the stack up).
final class CustomerUITests: XCTestCase {
    override func setUp() {
        continueAfterFailure = false
    }

    /// The app in a display language (English unless the test says otherwise: the device's own may be any).
    @MainActor
    private func launch(_ environment: [String: String], language: String = "en") -> XCUIApplication {
        let app = XCUIApplication()
        app.launchEnvironment = environment
        app.launchArguments = ["-AppleLanguages", "(\(language))", "-AppleLocale", language]
        app.launch()
        return app
    }

    @MainActor
    func testSignInShowsTheCustomersACsAndSignOutReturns() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        XCTAssertTrue(app.staticTexts["Customer"].waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("sign-in", of: app)
        app.buttons["signIn.button"].tap()
        XCTAssertTrue(app.staticTexts["Living room AC"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Bedroom AC"].exists)
        XCTAssertTrue(app.staticTexts["On"].exists)
        XCTAssertTrue(app.staticTexts["Offline"].exists)
        try app.audit()
        keepScreenshot("units", of: app)
        app.tabBars.buttons["Settings"].firstMatch.tap()
        app.buttons["signOut"].tap()
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 5))
    }

    /// Signed in on the stub, the first AC opened.
    @MainActor
    private func openLivingRoom() -> XCUIApplication {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        let unit = app.buttons["unit.u1"]
        XCTAssertTrue(unit.waitForExistence(timeout: 5))
        unit.tap()
        XCTAssertTrue(app.staticTexts["temperature.value"].waitForExistence(timeout: 5))
        return app
    }

    @MainActor
    func testATemperatureIsSentOnceSettledAndUndone() throws {
        let app = openLivingRoom()
        XCTAssertEqual(app.staticTexts["temperature.value"].label, "26°C")
        XCTAssertFalse(app.staticTexts["pending"].exists, "nothing to send before a change")
        try app.audit()
        keepScreenshot("unit-control", of: app)
        app.buttons["temperature.lower"].tap()
        app.buttons["temperature.lower"].tap()
        XCTAssertEqual(app.staticTexts["temperature.value"].label, "24°C")
        // the two taps go as one command, without a confirmation
        let outcome = app.staticTexts["outcome"]
        XCTAssertTrue(outcome.waitForExistence(timeout: 15))
        XCTAssertEqual(outcome.label, "Living room AC: Set temperature 24°C — acknowledged by device")
        keepScreenshot("unit-sent", of: app)
        tapScrolling(app.buttons["outcome.undo"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["outcome"], label: "Living room AC: Set temperature 26°C — acknowledged by device"))
        XCTAssertFalse(app.buttons["outcome.undo"].exists, "an undo offers no undo of its own")
    }

    /// The largest text size: the screen grows instead of cutting its text (the audit checks it at each size).
    @MainActor
    func testTheLargestTextSizeKeepsTheControls() throws {
        let app = XCUIApplication()
        app.launchEnvironment = ["AC_UI_TEST": "stub"]
        app.launchArguments = [
            "-AppleLanguages", "(en)", "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL",
        ]
        app.launch()
        app.buttons["signIn.button"].tap()
        XCTAssertTrue(app.buttons["unit.u1"].waitForExistence(timeout: 5))
        app.buttons["unit.u1"].tap()
        XCTAssertTrue(app.staticTexts["temperature.value"].waitForExistence(timeout: 5))
        keepScreenshot("unit-largest-text", of: app)
        try app.audit()
    }

    @MainActor
    func testThePowerIsSentWithOneTap() {
        let app = openLivingRoom()
        let power = app.switches["power"]
        XCTAssertEqual(power.value as? String, "1")
        power.switches.firstMatch.tap()
        XCTAssertTrue(wait(for: app.staticTexts["outcome"], label: "Living room AC: Set power OFF — acknowledged by device"))
        XCTAssertEqual(app.switches["power"].value as? String, "0")
    }

    @MainActor
    func testNotificationsAreReadAndMarkedReadAllAtOnce() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        app.tabBars.buttons["Notifications"].firstMatch.tap()
        let cleaning = app.buttons["notification.n1"]
        XCTAssertTrue(cleaning.waitForExistence(timeout: 5))
        XCTAssertTrue(cleaning.label.contains("Filter cleaning due on Living room AC"), cleaning.label)
        XCTAssertTrue(cleaning.label.contains("Cleaning due. Not a fault."), cleaning.label)
        XCTAssertTrue(app.buttons["notification.n2"].label.contains("Work completed — Bedroom AC"))
        try app.audit()
        keepScreenshot("notifications", of: app)
        app.buttons["notifications.markAll"].firstMatch.tap()
        XCTAssertTrue(wait(for: app.staticTexts["notifications.message"], label: "2 notifications marked as read"))
        XCTAssertFalse(app.buttons["notifications.markAll"].exists, "nothing left unread")
    }

    @MainActor
    func testHomeOrAwayIsReportedOnceSwitchedOn() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        app.tabBars.buttons["Settings"].firstMatch.tap()
        let toggle = app.switches["presence.switch"]
        XCTAssertTrue(toggle.waitForExistence(timeout: 5))
        XCTAssertEqual(toggle.value as? String, "0")
        XCTAssertFalse(app.buttons["consent.withdraw"].exists, "nothing to withdraw before the consent")
        try app.audit()
        // one switch: it records the consent and starts the reports
        toggle.switches.firstMatch.tap()
        XCTAssertTrue(wait(for: app.staticTexts["presence.line"], label: "Arrived at Home A — 1 automation ran."))
        keepScreenshot("presence", of: app)
    }

    @MainActor
    func testTheDevicesLanguageIsShown() {
        let app = launch(["AC_UI_TEST": "stub", "AC_UI_TEST_ROLE": "other"], language: "ja")
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 5))
        XCTAssertEqual(app.buttons["signIn.button"].label, "サインイン")
        app.buttons["signIn.button"].tap()
        let message = app.staticTexts["signIn.message"]
        XCTAssertTrue(message.waitForExistence(timeout: 5))
        XCTAssertEqual(message.label, "このアカウントはこのアプリでは使えません — 別のアカウントでサインインしてください")
        keepScreenshot("sign-in-ja-refused", of: app)
    }

    @MainActor
    func testAnAccountOfAnotherAppIsRefused() {
        let app = launch(["AC_UI_TEST": "stub", "AC_UI_TEST_ROLE": "technician"])
        app.buttons["signIn.button"].tap()
        let message = app.staticTexts["signIn.message"]
        XCTAssertTrue(message.waitForExistence(timeout: 5))
        XCTAssertEqual(message.label, "This account is not for this app — sign in with another account")
    }

    /// The app signed in to the running stack as customer-a (a session a run before kept is signed out first).
    @MainActor
    private func liveSignedIn() throws -> XCUIApplication {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["AC_UI_LIVE"] == "1", "the dev stack's live test (TEST_RUNNER_AC_UI_LIVE=1)")
        let app = launch([:])
        // (the list may be long: its last row, Sign out, is loaded once scrolled to)
        if app.navigationBars["Air conditioners"].waitForExistence(timeout: 3) { signOut(app) }
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "customer-a")
        XCTAssertTrue(app.navigationBars["Air conditioners"].waitForExistence(timeout: 20))
        return app
    }

    /// The phone's real location services against the running stack: run with the simulator's location at a pin of
    /// customer-a's and its location permission granted "Always" (README, Checks). Nothing but home or away is kept.
    @MainActor
    func testLivePresenceIsReportedToTheDevStack() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["AC_UI_LIVE_PRESENCE"] == "1", "needs a pin and the simulator's location set")
        let app = try liveSignedIn()
        app.tabBars.buttons["Settings"].firstMatch.tap()
        let toggle = app.switches["presence.switch"]
        XCTAssertTrue(toggle.waitForExistence(timeout: 10))
        if toggle.value as? String == "1" { toggle.switches.firstMatch.tap() }
        // the account's consent as it was: granted by the switch for the run, withdrawn again after it when it was not
        let wasGranted = app.buttons["consent.withdraw"].waitForExistence(timeout: 5)
        toggle.switches.firstMatch.tap()
        let line = app.staticTexts["presence.line"]
        XCTAssertTrue(line.waitForExistence(timeout: 30))
        let said = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "label CONTAINS 'Home A' OR label CONTAINS 'nothing changed'"),
            object: line
        )
        XCTAssertEqual(XCTWaiter().wait(for: [said], timeout: 30), .completed, line.label)
        keepScreenshot("live-presence", of: app)
        toggle.switches.firstMatch.tap()
        if !wasGranted {
            app.buttons["consent.withdraw"].tap()
            app.buttons["consent.withdraw.confirm"].firstMatch.tap()
            XCTAssertTrue(app.buttons["consent.withdraw"].waitForNonExistence(timeout: 10))
        }
    }

    /// Signs out from the Settings tab.
    @MainActor
    private func signOut(_ app: XCUIApplication) {
        app.tabBars.buttons["Settings"].firstMatch.tap()
        tapScrolling(app.buttons["signOut"], in: app)
    }

    @MainActor
    func testLiveSignInToTheDevStack() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["AC_UI_LIVE"] == "1", "the dev stack's live test (TEST_RUNNER_AC_UI_LIVE=1)")
        let app = launch([:])
        // a session a run before kept in the Keychain: signed out first, so the sign-in is this run's
        // (the list may be long: its last row, Sign out, is loaded once scrolled to)
        if app.navigationBars["Air conditioners"].waitForExistence(timeout: 3) { signOut(app) }
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "customer-a")
        XCTAssertTrue(app.navigationBars["Air conditioners"].waitForExistence(timeout: 20))
        // the Core API took the list's query: a refusal would show its line instead of the rows (sort=name, 2026-11-20)
        XCTAssertFalse(app.staticTexts["units.message"].waitForExistence(timeout: 3), "the list shows a refusal")
        keepScreenshot("live-list", of: app)
        // an AC's detail from the dev stack's Core API: read only, nothing is sent to an AC from a live test
        let first = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'unit.'")).firstMatch
        XCTAssertTrue(first.waitForExistence(timeout: 20))
        first.tap()
        XCTAssertTrue(app.staticTexts["REMOTE CONTROL"].waitForExistence(timeout: 20))
        keepScreenshot("live-unit", of: app)
        app.navigationBars.buttons.firstMatch.tap()
        signOut(app)
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
    }
}
