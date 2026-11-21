import XCTest

extension XCUIApplication {
    /// The height of the blur over the tab bar's top edge (iOS 26).
    static let tabBarBlur: CGFloat = 40

    /// The accessibility audit of the screen shown (contrast, labels, hit areas, Dynamic Type…), as jsx-a11y strict is for
    /// the web apps: every issue fails the test, named with its element so the failure says what to fix.
    @MainActor
    func audit() throws {
        // a scroll the test made a moment ago may still be gliding: the audit's pictures of a moving screen fail the
        // contrast of still text
        _ = XCTWaiter().wait(for: [XCTestExpectation(description: "the screen settles")], timeout: 1)
        // a fresh simulator may show a system banner over the app, or answer the audit too slowly, once: a screen that
        // fails is audited once more a moment later, and that audit is the one reported (an issue of the app stays)
        if (try? issues()).map(\.isEmpty) == true { return }
        _ = XCTWaiter().wait(for: [XCTestExpectation(description: "the system banner goes")], timeout: 6)
        let issues = try issues()
        if !issues.isEmpty { XCTFail("accessibility audit:\n" + issues.joined(separator: "\n")) }
    }

    @MainActor
    private func issues() throws -> [String] {
        var issues: [String] = []
        var window = windows.firstMatch.frame
        // iOS 26 blurs what scrolls under its floating tab bar, and a band above it: what lies there is behind the bar,
        // as an element the edge cuts off, and its contrast is checked once it is scrolled above
        let tabBar = tabBars.firstMatch
        if tabBar.exists, tabBar.frame.minY > window.minY {
            window.size.height = tabBar.frame.minY - Self.tabBarBlur - window.minY
        }
        try performAccessibilityAudit { issue in
            // an element the screen's edge cuts off is judged by the part that shows: its contrast is checked once it is
            // scrolled into view
            if issue.auditType == .contrast, let frame = issue.element?.frame, !window.contains(frame) { return true }
            // where it is, so a finding names the one of several elements with that label
            let element = issue.element.map { "\($0.elementType) '\($0.label)' (\($0.identifier)) at \($0.frame.integral)" } ?? "the screen"
            issues.append("\(issue.compactDescription): \(element) — \(issue.detailedDescription)")
            return true  // reported below, all of them at once
        }
        return issues
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
        var bottom: CGFloat { keyboard.exists ? keyboard.frame.minY : window.maxY - 100 }
        // a tap on anything but a field puts the keyboard away first, as a person does: on iOS 26 its Done bar floats
        // over the content just above it
        let done = { app.buttons.matching(identifier: "keyboard.done").allElementsBoundByIndex.first { $0.isHittable } }
        if keyboard.exists, ![.textField, .textView].contains(element.elementType), let button = done() {
            button.tap()
            _ = XCTWaiter().wait(
                for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: keyboard)],
                timeout: 2
            )
        }
        for _ in 0..<6 {
            guard element.exists else {
                app.swipeUp()
                continue
            }
            if element.frame.maxY > bottom {
                // the keyboard covers it: put away first with its Done button (a drag above it can start on a field,
                // which selects text instead of scrolling)
                // the screen under a sheet keeps its own Done button: the one that can be tapped is the sheet's
                if keyboard.exists, let button = done() { button.tap() } else { app.swipeUp() }
            } else if element.frame.minY < top {
                // a short, slow pull: a swipe down on a sheet scrolled to its top closes the sheet
                drag(app, from: 0.45, to: 0.6)
            } else {
                break
            }
        }
        // a field that grows is not always reported hittable (iOS 26) while it is on the screen: it is tapped where it is
        let isShown = element.exists && element.frame.minY >= window.minY && element.frame.maxY <= window.maxY
        XCTAssertTrue(element.isHittable || isShown, "\(element) is on the screen")
        if element.isHittable { element.tap() } else { element.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.2)).tap() }
    }

    /// Types into a field as a person does: taps it (again, on its first line, when the first tap did not give it the
    /// focus), then types.
    @MainActor
    func type(_ text: String, into field: XCUIElement, in app: XCUIApplication) {
        tapScrolling(field, in: app)
        // the simulator may take the Mac's keyboard, so no on-screen keyboard tells whether the field has the focus
        // iOS 26's growing field does not report its focus while it has it: the on-screen keyboard then says so
        let focused = NSPredicate(format: "hasKeyboardFocus == true")
        let hasFocus = {
            XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: focused, object: field)], timeout: 2) == .completed
                || app.keyboards.firstMatch.exists
        }
        // a field that grows takes the tap on its first line, not in the space it keeps for more lines
        if !hasFocus() { field.coordinate(withNormalizedOffset: CGVector(dx: 0.2, dy: 0.2)).tap() }
        XCTAssertTrue(hasFocus(), "\(field) has the keyboard focus")
        field.typeText(text)
    }

    /// Replaces a field's text: the cursor to its end, every character deleted, then the new text typed (a tap puts the
    /// cursor where it lands, so deleting a few characters could take the wrong ones).
    @MainActor
    func replace(_ text: String, in field: XCUIElement, in app: XCUIApplication) {
        let old = (field.value as? String) ?? ""
        type("", into: field, in: app)
        field.coordinate(withNormalizedOffset: CGVector(dx: 0.98, dy: 0.5)).tap()
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: old.count) + text)
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

    /// A text field by its identifier: a field that grows is a text view on iOS 17 and a text field on iOS 26.
    @MainActor
    func field(_ identifier: String, in app: XCUIApplication) -> XCUIElement {
        let kinds = [XCUIElement.ElementType.textField, .textView].map(\.rawValue)
        let predicate = NSPredicate(format: "identifier == %@ AND elementType IN %@", identifier, kinds)
        return app.descendants(matching: .any).matching(predicate).firstMatch
    }
}
