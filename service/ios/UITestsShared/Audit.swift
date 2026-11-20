import XCTest

extension XCUIApplication {
    /// The accessibility audit of the screen shown (contrast, labels, hit areas, Dynamic Type…), as jsx-a11y strict is for
    /// the web apps: every issue fails the test, named with its element so the failure says what to fix.
    @MainActor
    func audit() throws {
        try performAccessibilityAudit { issue in
            let element = issue.element.map { "\($0.elementType) '\($0.label)' (\($0.identifier))" } ?? "the screen"
            XCTFail("\(issue.compactDescription): \(element) — \(issue.detailedDescription)")
            return true  // reported above, with its element
        }
    }
}
