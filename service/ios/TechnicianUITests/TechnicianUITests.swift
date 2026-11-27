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
        XCTAssertTrue(app.descendants(matching: .any)["overview.tiles"].waitForExistence(timeout: 5), "the summary's tiles")
        try app.audit()
        keepScreenshot("jobs", of: app)
        // the open alert on the job's unit, under the list; it opens the job
        tapScrolling(app.buttons["overview.al1"], in: app)
        XCTAssertTrue(app.buttons["job.unit"].waitForExistence(timeout: 5))
        app.navigationBars.buttons.firstMatch.tap()
        scrollToTop(app)
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
        // the fix is taken as the sheet opens; the label is the one thing to scan
        XCTAssertTrue(
            wait(
                for: app.staticTexts["checkIn.positionHint"],
                label: "Found, accurate to ±12 m — the server checks you are within 200 m of the site"
            )
        )
        tapScrolling(submit, in: app)
        XCTAssertTrue(app.staticTexts["checkIn.error.qr"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.staticTexts["checkIn.error.position"].exists, "the fix is there")

        // another unit's label is refused under the QR field
        let qr = field("checkIn.qr", in: app)
        type("ac-unit:u2", into: qr, in: app)
        tapScrolling(submit, in: app)
        XCTAssertTrue(
            wait(
                for: app.staticTexts["checkIn.error.qr"],
                label: "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason"
            )
        )
        replace("ac-unit:u1", in: qr, in: app)
        tapScrolling(submit, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        XCTAssertTrue(app.staticTexts["In progress"].exists || app.navigationBars.staticTexts.firstMatch.label.contains("In progress"))

        // the time on site is paused and resumed
        pauseWork(in: app)
        // audited from the top, as the screen opens: an audit at the foot of a long screen scrolls it while it checks,
        // and its pictures of the moving text fail their contrast
        scrollToTop(app)
        try app.audit()
        keepScreenshot("job-paused", of: app)
        tapScrolling(app.buttons["job.resume"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work resumed"))
    }

    /// The stub's job accepted and checked in manually: the work report can be opened.
    @MainActor
    private func startedJob() -> XCUIApplication {
        let app = openJob()
        XCTAssertTrue(app.buttons["job.accept"].waitForExistence(timeout: 5))
        app.buttons["job.accept"].tap()
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Assignment accepted"))
        tapScrolling(app.buttons["job.checkIn"], in: app)
        XCTAssertTrue(app.buttons["checkIn.method.manual"].waitForExistence(timeout: 5))
        app.buttons["checkIn.method.manual"].tap()
        type("No GPS signal in the plant room", into: field("checkIn.reason", in: app), in: app)
        tapScrolling(app.buttons["checkIn.submit"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        return app
    }

    /// Pauses the work from the job's menu: its primary button is then Resume work.
    @MainActor
    private func pauseWork(in app: XCUIApplication) {
        tapScrolling(app.buttons["job.more"].firstMatch, in: app)
        let pause = app.buttons["Pause work"].firstMatch
        XCTAssertTrue(pause.waitForExistence(timeout: 5))
        pause.tap()
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work paused"))
        XCTAssertTrue(app.buttons["job.resume"].waitForExistence(timeout: 5), "resuming is the primary step")
    }

    /// Opens the job's unit and, from it, one of its screens.
    @MainActor
    private func openFromUnit(_ id: String, in app: XCUIApplication) {
        tapScrolling(app.buttons["job.unit"], in: app)
        let link = app.descendants(matching: .any)[id].firstMatch
        XCTAssertTrue(link.waitForExistence(timeout: 5))
        tapScrolling(link, in: app)
    }

    /// Chooses a result in a checklist row's menu.
    @MainActor
    private func choose(_ result: String, for key: String, in app: XCUIApplication) {
        tapScrolling(app.buttons["report.result.\(key)"], in: app)
        let option = app.buttons[result].firstMatch
        XCTAssertTrue(option.waitForExistence(timeout: 5))
        option.tap()
    }

    @MainActor
    func testAReportIsFilledSignedAndSubmitted() throws {
        let app = startedJob()
        tapScrolling(app.buttons["job.report"], in: app)
        let submit = app.buttons["report.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("report-new", of: app)

        // a submit first lists what blocks it: every empty result and the work text
        tapScrolling(submit, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["report.issues"], label: "9 issues block submit — they are listed above Submit report."))

        for key in ["evaporator_coil", "blower_motor", "blower_fan", "drain_pipe", "drain_pan", "outlet", "louver"] {
            choose("Normal", for: key, in: app)
        }
        choose("Attention", for: "filter", in: app)
        type("Clogged with dust", into: field("report.reason.filter", in: app), in: app)
        tapScrolling(app.buttons["report.reading.add"], in: app)
        type("12.5", into: field("report.reading.value", in: app), in: app)

        tapScrolling(app.buttons["report.add"].firstMatch, in: app)
        let addPart = app.buttons["Add part"].firstMatch
        XCTAssertTrue(addPart.waitForExistence(timeout: 5))
        addPart.tap()
        let catalog = app.buttons["part.catalog"]
        XCTAssertTrue(catalog.waitForExistence(timeout: 5))
        catalog.tap()
        app.buttons["Filter F-200 · F200 · 4 in van"].firstMatch.tap()
        app.buttons["part.save"].tap()
        XCTAssertTrue(app.staticTexts["Filter F-200 × 1 · Van stock"].waitForExistence(timeout: 5))

        type("Cleaned the filter and checked the drain.", into: field("report.workText", in: app), in: app)
        // no Save button: the sign-off saves the draft first (autosave may have saved a version before)

        // the customer signs on the phone
        tapScrolling(app.buttons["report.signOff"], in: app)
        type("Ms Tan", into: field("signOff.name", in: app), in: app)
        let pad = app.descendants(matching: .any)["signOff.pad"]
        XCTAssertTrue(pad.waitForExistence(timeout: 5))
        tapScrolling(pad, in: app)  // above the keyboard; the tap is a dot of the signature
        pad.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.6))
            .press(
                forDuration: 0.1,
                thenDragTo: pad.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.3)),
                withVelocity: .slow,
                thenHoldForDuration: 0.1
            )
        tapScrolling(app.buttons["signOff.save"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["report.note"], label: "Sign-off saved for this report version"))
        XCTAssertTrue(app.staticTexts["report.signed"].label.hasPrefix("Signed · Ms Tan · "))
        let version = app.descendants(matching: .any)["report.version"].firstMatch
        let isSaved = NSPredicate(format: "label MATCHES %@", "(?s)Draft v[0-9]+ · saved .*")
        let saved = XCTNSPredicateExpectation(predicate: isSaved, object: version)
        XCTAssertEqual(XCTWaiter().wait(for: [saved], timeout: 15), .completed, "the saved draft in the versions card: \(version.label)")

        tapScrolling(submit, in: app)
        let confirm = app.buttons["report.submit.confirm"].firstMatch
        XCTAssertTrue(confirm.waitForExistence(timeout: 5))
        confirm.tap()
        // the version signed is the one submitted: nothing was saved after the sign-off (the sign-off is kept, below); its
        // number is any, as autosave may have saved a draft version before the save above
        let note = app.staticTexts["report.note"]
        let isSubmitted = NSPredicate(format: "label MATCHES %@", "Report v[0-9]+ submitted")
        let submitted = XCTNSPredicateExpectation(predicate: isSubmitted, object: note)
        XCTAssertEqual(XCTWaiter().wait(for: [submitted], timeout: 15), .completed, "the submit note: \(note.label)")
        XCTAssertTrue(app.staticTexts["report.signed"].label.hasPrefix("Signed · Ms Tan · "), "the sign-off is kept")
        XCTAssertFalse(submit.exists, "a submitted version is read-only")
        scrollToTop(app)
        try app.audit()
        keepScreenshot("report-submitted", of: app)
    }

    @MainActor
    func testTheUnitShowsItsRegisterAndItsMonitoring() throws {
        let app = openJob()
        tapScrolling(app.buttons["job.unit"], in: app)
        XCTAssertTrue(app.buttons["Monitoring"].firstMatch.waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Location, Home A › Living room"].exists || app.staticTexts["Home A › Living room"].exists)
        XCTAssertTrue(app.descendants(matching: .any)["unit.alerts.al1"].waitForExistence(timeout: 5), "the open alert")
        try app.audit()
        keepScreenshot("unit-register", of: app)
        app.buttons["Monitoring"].firstMatch.tap()
        XCTAssertTrue(app.descendants(matching: .any)["unit.chart"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["24.5 °C"].exists, "the latest temperature")
        try app.audit()
        keepScreenshot("unit-monitoring", of: app)
        app.buttons["1h"].firstMatch.tap()
        XCTAssertTrue(app.staticTexts["Temperature (°C) · last 1 h (rolling)"].waitForExistence(timeout: 5))
    }

    @MainActor
    func testAnAlertIsAcknowledgedThenResolvedWithItsEvidence() throws {
        let app = openJob()
        openFromUnit("unit.alerts.al1", in: app)
        let acknowledge = app.buttons["alerts.acknowledge"]
        XCTAssertTrue(acknowledge.waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("alert-open", of: app)
        tapScrolling(acknowledge, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["alerts.note"], label: "Acknowledged"))
        XCTAssertFalse(app.buttons["alerts.acknowledge"].exists, "an acknowledged alert is not acknowledged again")

        // an alert without a policy resolves only with evidence
        type("Cooled down after the filter was cleaned.", into: field("alerts.reason", in: app), in: app)
        tapScrolling(app.buttons["alerts.resolve"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["alerts.problem"], label: "Pick at least one evidence record — this alert has no policy"))
        let evidence = app.switches["alerts.evidence.ev-r1"]
        tapScrolling(evidence, in: app)
        if evidence.value as? String != "1" { evidence.switches.firstMatch.tap() }
        tapScrolling(app.buttons["alerts.resolve"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["alerts.note"], label: "Resolved"))
        XCTAssertTrue(app.staticTexts["alerts.done"].label.hasPrefix("Resolved "))
        scrollToTop(app)
        try app.audit()
        keepScreenshot("alert-resolved", of: app)
    }

    @MainActor
    func testADiagnosticCommandAndATestRunAreSentForTheJob() throws {
        let app = openJob()
        openFromUnit("unit.diagnostics", in: app)
        let send = app.buttons["control.send"]
        XCTAssertTrue(send.waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("control", of: app)
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["control.error.reason"], label: "Reason is required (1–1000 characters)"))
        type("Check the compressor starts", into: field("control.reason", in: app), in: app)
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["control.note"], label: "dc1 sent — waiting for the device"))
        XCTAssertTrue(app.descendants(matching: .any)["control.history.dc1"].waitForExistence(timeout: 5))

        type("Cooling for five minutes", into: field("control.run.reason", in: app), in: app)
        tapScrolling(app.buttons["control.run.submit"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["control.note"], label: "Test run requested — waiting for the start acknowledgement"))
        XCTAssertTrue(
            app.staticTexts["End acknowledged — test run completed"].waitForExistence(timeout: 10),
            "stopped only after the end acknowledgement"
        )
        scrollToTop(app)
        try app.audit()
        keepScreenshot("control-run", of: app)
    }

    @MainActor
    func testADeviceIsCheckedCalibratedUpdatedAndAnswered() throws {
        let app = launch(["AC_UI_TEST": "stub"])
        app.buttons["signIn.button"].tap()
        app.tabBars.buttons["Devices"].firstMatch.tap()
        let row = app.buttons["device.d1"]
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        // the chosen filter is told to VoiceOver, not only by its style (the web's aria-pressed)
        let all = app.buttons["All"].firstMatch
        let online = app.buttons["Online"].firstMatch
        XCTAssertTrue(all.isSelected, "the filter All is chosen at first")
        XCTAssertFalse(online.isSelected)
        online.tap()
        XCTAssertTrue(online.isSelected, "the filter Online is chosen")
        XCTAssertFalse(all.isSelected)
        all.tap()
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        try app.audit()
        row.tap()
        let check = app.buttons["device.check"]
        XCTAssertTrue(check.waitForExistence(timeout: 5))
        try app.audit()
        keepScreenshot("device", of: app)
        tapScrolling(check, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["device.note"], label: "Connection check requested"))
        tapScrolling(app.buttons["device.update.2.0"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["device.note"], label: "Firmware update to 2.0 requested"))

        type("24.0", into: field("device.calibrate.reference", in: app), in: app)
        type("24.4", into: field("device.calibrate.measured", in: app), in: app)
        tapScrolling(app.buttons["device.calibrate"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["device.note"], label: "Calibration recorded — earlier readings unchanged"))

        type("Cover closed and sealed", into: field("device.note.text", in: app), in: app)
        tapScrolling(app.buttons["device.event.note.de1"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["device.note"], label: "Response note added — the device state is unchanged"))
        tapScrolling(app.buttons["device.alert.al1"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["device.note"], label: "Acknowledged"))
        scrollToTop(app)
        try app.audit()
        keepScreenshot("device-done", of: app)
    }

    @MainActor
    func testCantMakeIsSentWithItsReason() throws {
        let app = openJob()
        XCTAssertTrue(app.buttons["job.more"].firstMatch.waitForExistence(timeout: 5))
        app.buttons["job.more"].firstMatch.tap()
        let cantMake = app.buttons["Can’t make this time…"].firstMatch
        XCTAssertTrue(cantMake.waitForExistence(timeout: 5))
        cantMake.tap()
        let send = app.buttons["cantMake.send"]
        XCTAssertTrue(send.waitForExistence(timeout: 5))
        try app.audit()
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["cantMake.error"], label: "Tell your coordinator why (1–1000 characters)"))
        type("Medical appointment", into: field("cantMake.reason", in: app), in: app)
        tapScrolling(send, in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Sent to your coordinator"))
        let banner = app.staticTexts["job.banner"]
        XCTAssertTrue(banner.label.contains("“Medical appointment”"), banner.label)
        XCTAssertFalse(app.buttons["job.accept"].exists, "the answer is given")
        // back on the list, the job is read again: it no longer asks for an answer
        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(app.staticTexts["Can’t make it"].waitForExistence(timeout: 5), "the list shows the answer")
        XCTAssertFalse(app.staticTexts["Not accepted"].exists)
        XCTAssertFalse(app.staticTexts["jobs.answer"].exists, "no new assignment is announced")
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
        type("No GPS signal in the plant room", into: field("checkIn.reason", in: app), in: app)
        tapScrolling(app.buttons["checkIn.submit"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        pauseWork(in: app)
        keepScreenshot("live-job-paused", of: app)
        tapScrolling(app.buttons["job.resume"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Work resumed"))
    }

    /// A job assigned to tech-internal-a in the dev stack (inserted for the check): accepted, checked in, its report
    /// filled, saved, signed and submitted against the Core API.
    @MainActor
    func testLiveReportIsSavedSignedAndSubmitted() throws {
        guard let jobId = ProcessInfo.processInfo.environment["AC_UI_LIVE_REPORT"] else {
            throw XCTSkip("the dev stack's live report (TEST_RUNNER_AC_UI_LIVE_REPORT=<job id>)")
        }
        let app = launch([:])
        if app.navigationBars["Jobs"].waitForExistence(timeout: 3) { tapScrolling(app.buttons["signOut"], in: app) }
        XCTAssertTrue(app.buttons["signIn.button"].waitForExistence(timeout: 10))
        app.buttons["signIn.button"].tap()
        LiveSignIn.signIn(user: "tech-internal-a")
        let row = app.buttons["job.\(jobId)"]
        XCTAssertTrue(row.waitForExistence(timeout: 20))
        tapScrolling(row, in: app)
        XCTAssertTrue(app.buttons["job.accept"].waitForExistence(timeout: 20))
        app.buttons["job.accept"].tap()
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Assignment accepted"))
        tapScrolling(app.buttons["job.checkIn"], in: app)
        XCTAssertTrue(app.buttons["checkIn.method.manual"].waitForExistence(timeout: 5))
        app.buttons["checkIn.method.manual"].tap()
        type("No GPS signal in the plant room", into: field("checkIn.reason", in: app), in: app)
        tapScrolling(app.buttons["checkIn.submit"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["job.note"], label: "Checked in — the job has started"))
        tapScrolling(app.buttons["job.report"], in: app)
        XCTAssertTrue(app.buttons["report.submit"].waitForExistence(timeout: 10))
        let keys = [
            "filter", "evaporator_coil", "blower_motor", "blower_fan", "drain_pipe", "drain_pan", "outlet", "louver", "condenser_coil",
            "compressor", "fan", "blade", "refrigerant_pipe", "thermostat", "sensor", "capacitor", "contactor", "wiring",
        ]
        for key in keys { choose("Normal", for: key, in: app) }
        type("Cleaned the filters and checked every component.", into: field("report.workText", in: app), in: app)
        tapScrolling(app.buttons["report.signOff"], in: app)
        type("Ms Tan", into: field("signOff.name", in: app), in: app)
        let pad = app.descendants(matching: .any)["signOff.pad"]
        tapScrolling(pad, in: app)
        pad.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.6))
            .press(
                forDuration: 0.1,
                thenDragTo: pad.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.3)),
                withVelocity: .slow,
                thenHoldForDuration: 0.1
            )
        tapScrolling(app.buttons["signOff.save"], in: app)
        XCTAssertTrue(wait(for: app.staticTexts["report.note"], label: "Sign-off saved for this report version"))
        tapScrolling(app.buttons["report.submit"], in: app)
        let confirm = app.buttons["report.submit.confirm"].firstMatch
        XCTAssertTrue(confirm.waitForExistence(timeout: 5))
        confirm.tap()
        // any version: autosave (20 s after the last edit) may save a draft version first on a slow machine
        let note = app.staticTexts["report.note"]
        let isSubmitted = NSPredicate(format: "label MATCHES %@", "Report v[0-9]+ submitted")
        let submitted = XCTNSPredicateExpectation(predicate: isSubmitted, object: note)
        XCTAssertEqual(XCTWaiter().wait(for: [submitted], timeout: 15), .completed, "the submit note: \(note.label)")
        keepScreenshot("live-report-submitted", of: app)
    }
}
