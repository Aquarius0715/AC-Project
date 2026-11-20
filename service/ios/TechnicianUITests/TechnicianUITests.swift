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
        XCTAssertTrue(app.staticTexts["Living room AC"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Not accepted"].exists)
        XCTAssertTrue(app.staticTexts["jobs.answer"].exists, "the new assignment is announced over the list")
        try app.audit()
        keepScreenshot("jobs", of: app)
        app.buttons["signOut"].tap()
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 5))
    }

    /// The stub's job j1, opened from the list.
    @MainActor
    private func openJob() -> XCUIApplication {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        let row = app.buttons["job.j1"]
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        row.tap()
        return app
    }

    @MainActor
    func testAJobIsAcceptedCheckedInPausedAndResumed() throws {
        let app = openJob()
        XCTAssertTrue(app.buttons["job.accept"].waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("job-new", of: app)
        app.buttons["job.accept"].tap()
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Assignment accepted"))
        tapScrolling(app.buttons["job.checkIn"], in: app)

        // the form's own checks, each under its field
        let submit = app.buttons["checkIn.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 5))
        try app.audit()
        tapScrolling(submit, in: app)
        XCTAssertTrue(app.staticTexts["checkIn.error.position"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["checkIn.error.qr"].exists)
        tapScrolling(app.buttons["checkIn.locate"], in: app)
        XCTAssertTrue(
            wait(
                for: app.staticTexts["checkIn.positionHint"],
                label: "Found, accurate to ±12 m — the server checks you are within 200 m of the site"
            )
        )

        // another unit's label is refused under the QR field
        let qr = app.textViews["checkIn.qr"]
        type("ac-unit:u2", into: qr, in: app)
        tapScrolling(submit, in: app)
        XCTAssertTrue(
            wait(
                for: app.staticTexts["checkIn.error.qr"],
                label: "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason"
            )
        )
        type(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 2) + "u1", into: qr, in: app)
        tapScrolling(submit, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        XCTAssertTrue(app.staticTexts["In progress"].exists || app.navigationBars.staticTexts.firstMatch.label.contains("In progress"))

        // the time on site is paused and resumed
        let pause = app.buttons["job.pause"]
        tapScrolling(pause, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work paused"))
        XCTAssertTrue(wait(for: pause, label: "Resume work"))
        // audited from the top, as the screen opens: an audit at the foot of a long screen scrolls it while it checks,
        // and its pictures of the moving text fail their contrast
        scrollToTop(app)
        try app.audit()
        keepScreenshot("job-paused", of: app)
        tapScrolling(pause, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work resumed"))
    }

    @MainActor
    func testCantMakeIsSentWithItsReason() throws {
        let app = openJob()
        XCTAssertTrue(app.buttons["job.cantMake"].waitForExistence(timeout: 5))
        app.buttons["job.cantMake"].tap()
        let send = app.buttons["cantMake.send"]
        XCTAssertTrue(send.waitForExistence(timeout: 5))
        try app.audit()
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["cantMake.error"], label: "Tell your coordinator why (1–1000 characters)"))
        type("Medical appointment", into: app.textViews["cantMake.reason"], in: app)
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Sent to your coordinator"))
        let banner = app.staticTexts["job.banner"]
        XCTAssertTrue(banner.label.contains("“Medical appointment”"), banner.label)
        XCTAssertFalse(app.buttons["job.accept"].exists, "the answer is given")
    }

    @MainActor
    func testNotificationsHaveTheirTab() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        app.tabBars.buttons["Notifications"].firstMatch.tap()
        XCTAssertTrue(app.buttons["notification.n1"].waitForExistence(timeout: 5))
        try app.audit()
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
        // a session a run before kept in the Keychain: signed out first, so the sign-in is this run's
        // (the list may be long: its last row, Sign out, is loaded once scrolled to)
        if app.navigationBars["Jobs"].waitForExistence(timeout: 3) { tapScrolling(app.buttons["signOut"], in: app) }
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "tech-external-a")
        XCTAssertTrue(app.navigationBars["Jobs"].waitForExistence(timeout: 20))
        // the Core API took the list's query: a refusal would show its line instead of the rows (sort=name, 2026-11-20)
        XCTAssertFalse(app.staticTexts["jobs.message"].waitForExistence(timeout: 3), "the list shows a refusal")
        let rows = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'job.'"))
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 20))
        tapScrolling(app.buttons["signOut"], in: app)
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
    }

    /// A job assigned to tech-internal-a in the dev stack, before its acknowledgement (inserted for the check, its window
    /// around the stack's time): accepted, checked in manually, paused and resumed against the Core API.
    @MainActor
    func testLiveJobIsAcceptedCheckedInAndPaused() throws {
        guard let jobId = ProcessInfo.processInfo.environment["AC_UI_LIVE_JOB"] else {
            throw XCTSkip("the dev stack's live job (TEST_RUNNER_AC_UI_LIVE_JOB=<job id>)")
        }
        let app = launch([:])
        if app.navigationBars["Jobs"].waitForExistence(timeout: 3) { tapScrolling(app.buttons["signOut"], in: app) }
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "tech-internal-a")
        let row = app.buttons["job.\(jobId)"]
        XCTAssertTrue(row.waitForExistence(timeout: 20))
        tapScrolling(row, in: app)
        let accept = app.buttons["job.accept"]
        XCTAssertTrue(accept.waitForExistence(timeout: 20))
        accept.tap()
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Assignment accepted"))
        tapScrolling(app.buttons["job.checkIn"], in: app)
        XCTAssertTrue(app.buttons["checkIn.method.manual"].waitForExistence(timeout: 5))
        app.buttons["checkIn.method.manual"].tap()
        type("No GPS signal in the plant room", into: app.textViews["checkIn.reason"], in: app)
        tapScrolling(app.buttons["checkIn.submit"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        let pause = app.buttons["job.pause"]
        tapScrolling(pause, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work paused"))
        XCTAssertTrue(wait(for: pause, label: "Resume work"))
        keepScreenshot("live-job-paused", of: app)
        tapScrolling(pause, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work resumed"))
    }
}
