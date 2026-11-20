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
    func testATemperatureIsReviewedSentConfirmedAndUndone() throws {
        let app = openLivingRoom()
        XCTAssertEqual(app.staticTexts["temperature.value"].label, "26°C")
        XCTAssertFalse(app.buttons["review.send"].exists, "nothing to send before a change")
        try app.audit()
        keepScreenshot("unit-control", of: app)
        app.buttons["temperature.lower"].tap()
        app.buttons["temperature.lower"].tap()
        XCTAssertEqual(app.staticTexts["temperature.value"].label, "24°C")
        XCTAssertEqual(app.staticTexts["review.summary"].label, "1 change: Set temperature 26°C → 24°C")
        tapScrolling(app.buttons["review.send"], in: app)
        let send = app.alerts.buttons["send.confirm"]
        XCTAssertTrue(send.waitForExistence(timeout: 5))
        send.tap()
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
    func testThePowerIsSentAfterItsConfirmation() {
        let app = openLivingRoom()
        let power = app.switches["power"]
        XCTAssertEqual(power.value as? String, "1")
        power.switches.firstMatch.tap()
        let confirm = app.buttons["power.confirm"]
        XCTAssertTrue(confirm.waitForExistence(timeout: 5))
        XCTAssertEqual(confirm.label, "Power OFF")
        confirm.tap()
        XCTAssertTrue(wait(for: app.staticTexts["outcome"], label: "Living room AC: Set power OFF — acknowledged by device"))
        XCTAssertEqual(app.switches["power"].value as? String, "0")
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

    @MainActor
    func testLiveSignInToTheDevStack() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["AC_UI_LIVE"] == "1", "the dev stack's live test (TEST_RUNNER_AC_UI_LIVE=1)")
        let app = launch([:])
        // a session a run before kept in the Keychain: signed out first, so the sign-in is this run's
        // (the list may be long: its last row, Sign out, is loaded once scrolled to)
        if app.navigationBars["Air conditioners"].waitForExistence(timeout: 3) { tapScrolling(app.buttons["signOut"], in: app) }
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
        tapScrolling(app.buttons["signOut"], in: app)
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
    }
}
