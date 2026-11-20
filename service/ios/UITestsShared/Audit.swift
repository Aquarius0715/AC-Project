import XCTest

extension XCUIApplication {
    /// The accessibility audit of the screen shown (contrast, labels, hit areas, Dynamic Type…), as jsx-a11y strict is for
    /// the web apps: every issue fails the test, named with its element so the failure says what to fix.
    @MainActor
    func audit() throws {
        // a scroll the test made a moment ago may still be gliding: the audit's pictures of a moving screen fail the
        // contrast of still text
        _ = XCTWaiter().wait(for: [XCTestExpectation(description: "the screen settles")], timeout: 1)
        var issues: [String] = []
        try performAccessibilityAudit { issue in
            let element = issue.element.map { "\($0.elementType) '\($0.label)' (\($0.identifier))" } ?? "the screen"
            issues.append("\(issue.compactDescription): \(element) — \(issue.detailedDescription)")
            return true  // reported below, all of them at once
        }
        if !issues.isEmpty { XCTFail("accessibility audit:\n" + issues.joined(separator: "\n")) }
    }
}

extension XCTestCase {
    /// A screenshot kept in the test's result (`-resultBundlePath`), for a reviewer to see the screen the test checked.
    @MainActor
    func keepScreenshot(_ name: String, of app: XCUIApplication) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }

    /// Waits until the element says this (a line that changes in place, as a send's outcome does).
    @MainActor
    func wait(for element: XCUIElement, label: String, timeout: TimeInterval = 15) -> Bool {
        let said = XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == %@", label), object: element)
        let isSaid = XCTWaiter().wait(for: [said], timeout: timeout) == .completed
        if !isSaid { XCTFail("waited for \"\(label)\", it says \"\(element.label)\"") }
        return isSaid
    }

    /// Scrolls the screen until the element can be tapped, then taps it: as a person scrolls to a button below the fold.
    @MainActor
    func tapScrolling(_ element: XCUIElement, in app: XCUIApplication) {
        // hittable is not enough: an element at the very bottom is under the home indicator, one at the very top under
        // the navigation bar, and either takes the tap
        let window = app.windows.firstMatch.frame
        let top = window.minY + 120
        // the on-screen keyboard, when it is up, covers the screen's lower part as the home indicator does
        let keyboard = app.keyboards.firstMatch
        let bottom = keyboard.exists ? keyboard.frame.minY - 20 : window.maxY - 100
        for _ in 0..<6 {
            guard element.exists else {
                app.swipeUp()
                continue
            }
            if element.frame.maxY > bottom {
                // above the keyboard when it is up: a swipe on it does not move the screen
                if keyboard.exists { drag(app, from: 0.35, to: 0.1) } else { app.swipeUp() }
            } else if element.frame.minY < top {
                app.swipeDown()
            } else {
                break
            }
        }
        XCTAssertTrue(element.isHittable, "\(element) is on the screen")
        element.tap()
    }

    /// Types into a field as a person does: taps it (again, on its first line, when the first tap did not give it the
    /// focus), then types.
    @MainActor
    func type(_ text: String, into field: XCUIElement, in app: XCUIApplication) {
        tapScrolling(field, in: app)
        // the simulator may take the Mac's keyboard, so no on-screen keyboard tells whether the field has the focus
        let focused = NSPredicate(format: "hasKeyboardFocus == true")
        let hasFocus = { XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: focused, object: field)], timeout: 2) == .completed }
        // a field that grows takes the tap on its first line, not in the space it keeps for more lines
        if !hasFocus() { field.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.2)).tap() }
        XCTAssertTrue(hasFocus(), "\(field) has the keyboard focus")
        field.typeText(text)
    }

    /// A drag on the screen from one height to another (fractions of the window, from the top).
    @MainActor
    private func drag(_ app: XCUIApplication, from start: CGFloat, to end: CGFloat) {
        let window = app.windows.firstMatch
        window.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: start))
            .press(forDuration: 0.05, thenDragTo: window.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: end)))
    }

    /// Scrolls the screen back to its top.
    @MainActor
    func scrollToTop(_ app: XCUIApplication) {
        for _ in 0..<3 { drag(app, from: 0.3, to: 0.9) }
    }
}
