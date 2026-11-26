import SwiftUI

/// The web apps' colours (shared/globals.css), so an app and a page look like one service.
public enum Brand {
    /// `--primary` #005bea: white text on it has a contrast of 5.6:1 (the system blue's 4.0:1 fails the accessibility
    /// audit for body text).
    public static let primary = Color(red: 0, green: 0x5B / 255.0, blue: 0xEA / 255.0)
}
