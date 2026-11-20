import XCTest

extension XCUIApplication {
    /// The accessibility audit of the screen shown (contrast, labels, hit areas, Dynamic Type…), as jsx-a11y strict is for
    /// the web apps: every issue fails the test, named with its element so the failure says what to fix.
    @MainActor
    func audit() throws {
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
        // hittable is not enough: an element at the very bottom is under the home indicator, which takes the tap
        let bottom = app.windows.firstMatch.frame.maxY - 100
        for _ in 0..<5 where !(element.exists && element.isHittable && element.frame.maxY < bottom) {
            app.swipeUp()
        }
        XCTAssertTrue(element.isHittable, "\(element) is on the screen")
        element.tap()
    }
}
