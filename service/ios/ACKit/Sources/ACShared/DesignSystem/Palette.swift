import SwiftUI

#if os(iOS)
import UIKit
#else
import AppKit
#endif

/// The one place the apps' colours are defined (docs/03-uiux/ios-design-system.md): a screen names a colour by its use,
/// never by its value. The values follow the web apps' tokens (service/web/shared/globals.css) in the light appearance,
/// and each has a lighter one for the dark appearance; the unit tests hold every text colour at 4.5:1 or more on the
/// backgrounds it is drawn on, in both.
public enum Palette {
    /// The brand blue of links, icons, switches and the selected item (`--primary`). In the dark it is lighter: #005bea
    /// on black is 3.4:1.
    public static let accent = Color(Swatch.accent)
    /// The fill of a primary button, the same blue in both appearances: its white words are 5.6:1 on it (the lighter blue
    /// of the dark appearance would leave them at 2.9:1).
    public static let primaryFill = Color(Swatch.primaryFill)
    /// Done, confirmed, connected (`--color-ok`).
    public static let ok = Color(Swatch.ok)
    /// Needs attention soon (`--color-warn`).
    public static let warn = Color(Swatch.warn)
    /// Failed, blocked, destructive (`--color-crit`, `--destructive`).
    public static let critical = Color(Swatch.critical)
    /// Unknown or missing data, never a normal state (`--color-unknown`).
    public static let unknown = Color(Swatch.unknown)

    /// The screen's grouped background, under the cards (the system's: it follows Increase Contrast too).
    public static var background: Color {
        #if os(iOS)
        Color(uiColor: .systemGroupedBackground)
        #else
        Color(nsColor: .windowBackgroundColor)
        #endif
    }

    /// A card on the grouped background: white in the light appearance, the raised grey in the dark one (the plain
    /// background is black there, the same as the screen's, and a card on it would not show).
    public static var surface: Color {
        #if os(iOS)
        Color(uiColor: .secondarySystemGroupedBackground)
        #else
        Color(nsColor: .controlBackgroundColor)
        #endif
    }
}

/// A colour of both appearances as 0xRRGGBB sRGB values, so the tests can measure their contrast.
public struct Swatch: Sendable, Equatable {
    public let light: UInt32
    public let dark: UInt32

    public static let accent = Self(light: 0x005BEA, dark: 0x5A9BFF)
    public static let primaryFill = Self(light: 0x005BEA, dark: 0x005BEA)
    public static let ok = Self(light: 0x166534, dark: 0x4ADE80)
    public static let warn = Self(light: 0x9A3412, dark: 0xFB923C)
    public static let critical = Self(light: 0xB91C1C, dark: 0xF87171)
    public static let unknown = Self(light: 0x475569, dark: 0xA3B1C4)

    /// The value in one appearance.
    public func value(isDark: Bool) -> UInt32 {
        isDark ? dark : light
    }

    /// The red, green and blue of a value, each 0…1.
    public struct Channels: Sendable, Equatable {
        public let red: Double
        public let green: Double
        public let blue: Double

        public init(_ value: UInt32) {
            red = Double((value >> 16) & 0xFF) / 255
            green = Double((value >> 8) & 0xFF) / 255
            blue = Double(value & 0xFF) / 255
        }
    }

    #if os(iOS)
    var platformColor: UIColor {
        UIColor { traits in Self.color(value(isDark: traits.userInterfaceStyle == .dark)) }
    }

    private static func color(_ value: UInt32) -> UIColor {
        let c = Channels(value)
        return UIColor(red: c.red, green: c.green, blue: c.blue, alpha: 1)
    }
    #else
    var platformColor: NSColor {
        NSColor(name: nil) { appearance in
            Self.color(value(isDark: appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua))
        }
    }

    private static func color(_ value: UInt32) -> NSColor {
        let c = Channels(value)
        return NSColor(srgbRed: c.red, green: c.green, blue: c.blue, alpha: 1)
    }
    #endif
}

extension Color {
    /// The colour that resolves to the swatch's value in each appearance.
    init(_ swatch: Swatch) {
        #if os(iOS)
        self.init(uiColor: swatch.platformColor)
        #else
        self.init(nsColor: swatch.platformColor)
        #endif
    }
}
