import SwiftUI

/// What a status says of itself, the same on every screen (the web apps' badge tones): its colour and its symbol come
/// from here. The colour never says it alone: a status always has its words, and its symbol is a second sign.
public enum Tone: Sendable, CaseIterable {
    case info, ok, warn, critical, muted

    public var color: Color {
        switch self {
        case .info: Palette.accent
        case .ok: Palette.ok
        case .warn: Palette.warn
        case .critical: Palette.critical
        case .muted: Palette.unknown
        }
    }

    public var symbol: Symbol {
        switch self {
        case .info, .muted: .info
        case .ok: .success
        case .warn: .warning
        case .critical: .critical
        }
    }
}
