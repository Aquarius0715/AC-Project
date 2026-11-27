import SwiftUI

/// The SF Symbols the apps show, named by what they mean (docs/03-uiux/ios-design-system.md), as the web apps' Icon map
/// names their Lucide icons: one meaning has one symbol on every screen. A screen writes `Image(symbol: .warning)` or
/// `Label(title, symbol: .photo)`, never a symbol's name (SwiftLint's `sf_symbol_name` rule). The outline style is the
/// default; a filled symbol only marks a state (unread).
public enum Symbol: String, CaseIterable, Sendable {
    // the tabs
    case airConditioner = "air.conditioner.horizontal"
    case jobs = "wrench.and.screwdriver"
    case devices = "sensor"
    case notifications = "bell"
    case settings = "gearshape"

    // the tones of a status (Tone)
    case info = "info.circle"
    case success = "checkmark.circle"
    case warning = "exclamationmark.triangle"
    case critical = "exclamationmark.octagon"
    /// A failed item or a field's error (smaller than a warning: one line among others).
    case failed = "exclamationmark.circle"

    // states and actions
    case unread = "circle.fill"
    case offline = "wifi.slash"
    case message = "envelope"
    case sending = "paperplane"
    case decrease = "minus"
    case increase = "plus"
    case disclosure = "chevron.right"
    /// The menu of a screen's other actions, beside its primary button.
    case more = "ellipsis.circle"
    case history = "clock.arrow.circlepath"

    // the technician's work
    case alertEvidence = "exclamationmark.bubble"
    case diagnostics = "stethoscope"
    case report = "doc.text"
    case photo = "photo"
    case camera = "camera"
    case signature = "signature"
    case location = "location"
    case scan = "qrcode.viewfinder"
}

extension Image {
    /// The symbol of a meaning.
    nonisolated public init(symbol: Symbol) {
        self.init(systemName: symbol.rawValue)
    }
}

extension Label where Title == Text, Icon == Image {
    /// A title with the symbol of a meaning.
    nonisolated public init(_ title: String, symbol: Symbol) {
        self.init(title, systemImage: symbol.rawValue)
    }
}
