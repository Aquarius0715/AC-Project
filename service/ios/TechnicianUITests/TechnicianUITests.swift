import XCTest

/// AC Technician's screens (DEC-112). The stubbed tests run in CI without a server (AC_UI_TEST=stub); the live test signs
/// in to the dev stack's Keycloak and reads its Core API (run with TEST_RUNNER_AC_UI_LIVE=1, the stack up).
final class TechnicianUITests: XCTestCase {
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
    func testSignInShowsTheTechniciansJobsAndSignOutReturns() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        XCTAssertTrue(app.staticTexts["Technician"].waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("sign-in", of: app)
        app.buttons["signIn.button"].tap()
        XCTAssertTrue(app.staticTexts["Repair"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Assigned"].exists)
        try app.audit()
        keepScreenshot("jobs", of: app)
        app.buttons["signOut"].tap()
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 5))
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
        let app = launch(["AC_UI_TEST": "stub", "AC_UI_TEST_ROLE": "client"])
        app.buttons["signIn.button"].tap()
        let message = app.staticTexts["signIn.message"]
        XCTAssertTrue(message.waitForExistence(timeout: 5))
        XCTAssertEqual(message.label, "This account is not for this app — sign in with another account")
    }

    @MainActor
    func testLiveSignInToTheDevStack() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["AC_UI_LIVE"] == "1", "the dev stack's live test (TEST_RUNNER_AC_UI_LIVE=1)")
        let app = launch([:])
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "tech-external-a")
        XCTAssertTrue(app.navigationBars["Jobs"].waitForExistence(timeout: 20))
        XCTAssertTrue(app.cells.firstMatch.waitForExistence(timeout: 20))
        app.buttons["signOut"].tap()
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
    }
}
