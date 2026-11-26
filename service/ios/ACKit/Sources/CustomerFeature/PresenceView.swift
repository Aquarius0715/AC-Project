import ACShared
import SwiftUI

/// The app's settings: this phone's home-or-away switch (DEC-109, IR53) and the sign-out. The one switch also records
/// the location consent when it is not granted yet; the consent is withdrawn on its own, below a switch that is off.
struct PresenceView: View {
    @State private var model: PresenceModel
    let signOut: () async -> Void
    @State private var isWithdrawing = false

    init(model: PresenceModel, signOut: @escaping () async -> Void) {
        _model = State(initialValue: model)
        self.signOut = signOut
    }

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Location")) { location }
                if let line = model.line {
                    Card { Text(verbatim: line).accessibilityIdentifier("presence.line") }
                }
                Button(tr("Sign out")) { Task { await signOut() } }
                    .secondaryAction()
                    .accessibilityIdentifier("signOut")
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Settings")))
            .inlineTitle()
            .task { await model.loadConsent() }
            .refreshable { await model.loadConsent() }
            .confirmationDialog(Text(verbatim: tr("Withdraw consent")), isPresented: $isWithdrawing, titleVisibility: .visible) {
                Button(tr("Withdraw consent"), role: .destructive) { Task { await model.setConsent(false) } }
                    .accessibilityIdentifier("consent.withdraw.confirm")
                Button(tr("Cancel"), role: .cancel) { isWithdrawing = false }
            } message: {
                Text(verbatim: tr("Location automations cannot run until you grant it again. Manual control and schedules are unaffected."))
            }
        }
    }

    @ViewBuilder private var location: some View {
        if !model.hasConsentRecord {
            Text(verbatim: tr("No location consent record exists for your account — location automations cannot be used."))
        } else {
            Toggle(isOn: Binding(get: { model.isInUse }, set: { on in Task { await model.use(on) } })) {
                Text(verbatim: tr("Use this device’s location"))
            }
            .disabled(model.consent == nil || model.isSavingConsent)
            .accessibilityIdentifier("presence.switch")
            Text(
                verbatim: tr(
                    "Your phone reports arriving and leaving your homes and offices, also while the app is closed. Its position is compared with their map pins, and only home or away is kept."
                )
            )
            .font(.subheadline)
            if !model.isConsentGranted {
                Text(verbatim: tr("Switching this on gives your location consent, for arrival/departure automations only."))
                    .font(.footnote)
            } else if !model.isOn {
                Button(tr("Withdraw consent")) { isWithdrawing = true }
                    .secondaryAction()
                    .disabled(model.isSavingConsent)
                    .accessibilityIdentifier("consent.withdraw")
            }
        }
    }
}
