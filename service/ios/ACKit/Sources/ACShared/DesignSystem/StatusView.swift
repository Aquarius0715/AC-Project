import SwiftUI

/// A status line: its words with the symbol and colour of its tone (Tone). The words say it all; the symbol is a second
/// sign and VoiceOver does not read it. For a field's error, the critical tone with the smaller `failed` symbol.
public struct StatusLabel: View {
    let text: String
    let tone: Tone
    let symbol: Symbol

    /// `symbol` replaces the tone's own where a status has a more precise one (offline, a message waiting).
    public init(_ text: String, tone: Tone, symbol: Symbol? = nil) {
        self.text = text
        self.tone = tone
        self.symbol = symbol ?? tone.symbol
    }

    public var body: some View {
        Label {
            Text(verbatim: text)
        } icon: {
            Image(symbol: symbol).foregroundStyle(tone.color).accessibilityHidden(true)
        }
    }
}

extension StatusLabel {
    /// The error of one field, under the field.
    public static func fieldError(_ text: String) -> Self {
        Self(text, tone: .critical, symbol: .failed)
    }
}

extension View {
    /// The spinner of a first read, over the screen: a reload keeps the screen and shows the system's refresh control.
    public func loadingOverlay(_ isLoading: Bool) -> some View {
        overlay { if isLoading { ProgressView() } }
    }
}

extension View {
    /// The system's success haptic when a write's note appears and its error haptic when a refusal does (the haptics
    /// follow the user's settings). One modifier with plain conditions, so a long screen's body stays quick to type-check.
    public func writeFeedback(note: String?, refusal: String?) -> some View {
        sensoryFeedback(.success, trigger: note) { _, new in new != nil }
            .sensoryFeedback(.error, trigger: refusal) { _, new in new != nil }
    }
}

#Preview("Status labels") {
    Card {
        ForEach(Tone.allCases, id: \.self) { tone in StatusLabel("A status in the \(tone) tone", tone: tone) }
        StatusLabel("Offline since 10:42", tone: .critical, symbol: .offline)
        StatusLabel.fieldError("Enter a reason.")
    }
    .padding(Spacing.l)
    .background(Palette.background)
}

#Preview("Status labels, dark, largest text") {
    Card {
        ForEach(Tone.allCases, id: \.self) { tone in StatusLabel("A status in the \(tone) tone", tone: tone) }
        StatusLabel("Offline since 10:42", tone: .critical, symbol: .offline)
        StatusLabel.fieldError("Enter a reason.")
    }
    .padding(Spacing.l)
    .background(Palette.background)
    .preferredColorScheme(.dark)
    .dynamicTypeSize(.accessibility3)
}
