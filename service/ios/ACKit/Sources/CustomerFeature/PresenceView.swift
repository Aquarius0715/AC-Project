import ACShared
import SwiftUI

/// The location consent and the home-or-away switch of this phone (DEC-109, IR53): the switch is offered once the
/// consent is granted, as on the web's automations page.
struct PresenceView: View {
    @State private var model: PresenceModel
    @State private var isWithdrawing = false

    init(model: PresenceModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Location consent — for arrival/departure automations only")) {
                    consent
                }
                if model.isConsentGranted {
                    Card { presence }
                }
                if let line = model.line {
                    Card { Text(verbatim: line).accessibilityIdentifier("presence.line") }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Location")))
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

    @ViewBuilder private var consent: some View {
        if !model.hasConsentRecord {
            Text(verbatim: tr("No location consent record exists for your account — location automations cannot be used."))
        } else if model.isConsentGranted {
            Button(tr("Withdraw consent")) { isWithdrawing = true }.secondaryAction().disabled(model.isSavingConsent)
                .accessibilityIdentifier("consent.withdraw")
        } else if model.consent != nil {
            Text(
                verbatim: tr(
                    "Location automations need your consent. It is used only for arrival/departure automations: your device’s location is compared with the map pins of your homes and offices, and only home or away is kept."
                )
            )
            .font(.subheadline)
            Button(tr("Grant consent")) { Task { await model.setConsent(true) } }
                .buttonStyle(.borderedProminent)
                .disabled(model.isSavingConsent)
                .accessibilityIdentifier("consent.grant")
        }
    }

    @ViewBuilder private var presence: some View {
        Toggle(isOn: Binding(get: { model.isOn }, set: { on in Task { await model.set(on) } })) {
            Text(verbatim: tr("Use this device’s location"))
        }
        .accessibilityIdentifier("presence.switch")
        Text(
            verbatim: tr(
                "Your phone reports arriving and leaving your homes and offices, also while the app is closed. Its position is compared with their map pins, and only home or away is kept."
            )
        )
        .font(.subheadline)
    }
}
