import SwiftUI

/// The kinds of button (docs/03-uiux/ios-design-system.md): the system's Button with one of these styles, so every screen
/// shows the same few. A screen has at most one primary action in view; the rest are secondary.
extension View {
    /// The action a screen or a card is for (Sign in, Start job, Submit, Try again): the bordered prominent button on the
    /// brand blue, the same blue in both appearances so its white words keep 5.6:1.
    public func primaryAction() -> some View {
        buttonStyle(.borderedProminent).tint(Palette.primaryFill)
    }

    /// A secondary button (Mark all as read, Undo, Dismiss, a filter not chosen): the body colour on the grey of a
    /// bordered button; the accent on its light tint is below the contrast the accessibility audit asks for.
    public func secondaryAction() -> some View {
        buttonStyle(.bordered).tint(.primary)
    }

    /// A link to another screen shown as a secondary button: its words in the body colour too (iOS 26 draws a link's
    /// label in the accent colour, below the contrast the accessibility audit asks for on the button's grey).
    public func linkAction() -> some View {
        secondaryAction().foregroundStyle(.primary)
    }

    /// An action that cannot be taken back or ends something (Withdraw consent, Can't make this time): a bordered button
    /// in the critical colour. The button also takes `role: .destructive`, which VoiceOver reads; one that removes data
    /// asks first (a confirmation dialog whose safe choice is Cancel).
    public func destructiveAction() -> some View {
        buttonStyle(.bordered).tint(Palette.critical)
    }
}
