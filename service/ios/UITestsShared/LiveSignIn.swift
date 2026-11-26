import XCTest

/// The dev stack's sign-in page (Keycloak, themed ac-customer / ac-technician), which the web authentication session
/// shows in Safari's view service: the demo users of docker/keycloak/realm-ac.json.
enum LiveSignIn {
    static let password = "AcDemoLocal-2026"

    /// Types into the page as a person with a keyboard does: the user name the page has focused, a tab to the password,
    /// a return to submit. A tap on the password field can land under the keyboard, or be taken back by the page's
    /// autofocus while it loads.
    @MainActor
    static func signIn(user: String) {
        let safari = XCUIApplication(bundleIdentifier: "com.apple.SafariViewService")
        let username = safari.textFields.firstMatch
        XCTAssertTrue(username.waitForExistence(timeout: 20), "the sign-in page opens")
        username.tap()
        username.typeText(user + "\t")
        safari.typeText(password + "\n")
    }
}
