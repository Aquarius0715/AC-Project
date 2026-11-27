import AppKit
import SwiftUI
import Testing

@testable import ACShared

/// The design system's tokens (docs/03-uiux/ios-design-system.md): the colours keep their contrast in both appearances,
/// every symbol exists, every tone has its colour and symbol.
@Suite struct PaletteTests {
    /// The backgrounds a text colour is drawn on: the card and the grouped screen, in each appearance (the system's
    /// secondarySystemGroupedBackground and systemGroupedBackground).
    private let lightBackgrounds: [UInt32] = [0xFFFFFF, 0xF2F2F7]
    private let darkBackgrounds: [UInt32] = [0x1C1C1E, 0x000000, 0x2C2C2E]

    /// The colours of words and symbols: each one's contrast is checked.
    private let textSwatches: [(String, Swatch)] = [
        ("accent", .accent), ("ok", .ok), ("warn", .warn), ("critical", .critical), ("unknown", .unknown),
    ]

    @Test func everyTextColourKeepsItsContrastInBothAppearances() {
        for (name, swatch) in textSwatches {
            for background in lightBackgrounds {
                let ratio = contrast(swatch.light, background)
                #expect(ratio >= 4.5, "\(name) on \(String(background, radix: 16)) is \(ratio):1 in the light")
            }
            for background in darkBackgrounds {
                let ratio = contrast(swatch.dark, background)
                #expect(ratio >= 4.5, "\(name) on \(String(background, radix: 16)) is \(ratio):1 in the dark")
            }
        }
    }

    @Test func whiteWordsOnThePrimaryFillKeepTheirContrast() {
        #expect(contrast(0xFFFFFF, Swatch.primaryFill.light) >= 4.5)
        #expect(contrast(0xFFFFFF, Swatch.primaryFill.dark) >= 4.5)
        #expect(Swatch.primaryFill.light == Swatch.primaryFill.dark, "the same blue in both appearances")
        #expect(Swatch.accent.light == 0x005BEA, "the web apps' --primary")
    }

    @Test func aSwatchResolvesToItsValueInEachAppearance() {
        for (_, swatch) in textSwatches {
            #expect(resolved(swatch.platformColor, .aqua) == swatch.light)
            #expect(resolved(swatch.platformColor, .darkAqua) == swatch.dark)
            #expect(swatch.value(isDark: false) == swatch.light && swatch.value(isDark: true) == swatch.dark)
        }
    }

    @Test func thePaletteNamesEveryUse() {
        let colours = [Palette.accent, Palette.primaryFill, Palette.ok, Palette.warn, Palette.critical, Palette.unknown]
        #expect(Set(colours.map(\.description)).count == colours.count, "each use has its own colour")
        #expect(Palette.background != Palette.surface)
        #expect(Swatch.Channels(0x336699) != Swatch.Channels(0x336698))
        #expect(Swatch.Channels(0xFF8000).red == 1 && Swatch.Channels(0xFF8000).blue == 0)
    }

    /// The WCAG 2 contrast ratio of two sRGB colours.
    private func contrast(_ a: UInt32, _ b: UInt32) -> Double {
        let (l1, l2) = (luminance(a), luminance(b))
        return (max(l1, l2) + 0.05) / (min(l1, l2) + 0.05)
    }

    private func luminance(_ value: UInt32) -> Double {
        let c = Swatch.Channels(value)
        func linear(_ v: Double) -> Double { v <= 0.039_28 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4) }
        return 0.212_6 * linear(c.red) + 0.715_2 * linear(c.green) + 0.072_2 * linear(c.blue)
    }

    private func resolved(_ color: NSColor, _ name: NSAppearance.Name) -> UInt32? {
        var value: UInt32?
        NSAppearance(named: name)?.performAsCurrentDrawingAppearance {
            guard let rgb = color.usingColorSpace(.sRGB) else { return }
            let parts = [rgb.redComponent, rgb.greenComponent, rgb.blueComponent].map { UInt32(($0 * 255).rounded()) }
            value = parts[0] << 16 | parts[1] << 8 | parts[2]
        }
        return value
    }
}

@Suite struct SymbolAndToneTests {
    @Test func everySymbolIsOneTheSystemHas() {
        for symbol in Symbol.allCases {
            #expect(NSImage(systemSymbolName: symbol.rawValue, accessibilityDescription: nil) != nil, "\(symbol.rawValue)")
        }
        // the views take a symbol by its meaning
        _ = Image(symbol: .warning)
        _ = Label("Photo", symbol: .photo)
    }

    @Test func everyToneHasItsColourAndSymbol() {
        let expected: [Tone: (Color, Symbol)] = [
            .info: (Palette.accent, .info), .ok: (Palette.ok, .success), .warn: (Palette.warn, .warning),
            .critical: (Palette.critical, .critical), .muted: (Palette.unknown, .info),
        ]
        for tone in Tone.allCases {
            #expect(tone.color == expected[tone]?.0 && tone.symbol == expected[tone]?.1, "\(tone)")
        }
    }

    @Test func theSpacingScaleIsOnTheGrid() {
        let scale = [Spacing.xxs, Spacing.xs, Spacing.s, Spacing.m, Spacing.l, Spacing.xl]
        #expect(scale == scale.sorted() && Set(scale).count == scale.count, "ascending, no step twice")
        #expect(scale.dropFirst().allSatisfy { $0.truncatingRemainder(dividingBy: 4) == 0 }, "4 pt steps above the 2 pt one")
        #expect(Radius.control < Radius.card && Size.minTap == 44)
        #expect(Font.factLabel == .subheadline.weight(.semibold))
    }
}
