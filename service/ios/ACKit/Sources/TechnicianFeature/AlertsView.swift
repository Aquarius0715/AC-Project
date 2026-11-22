import ACKit
import ACShared
import SwiftUI

/// The alert evidence of one unit: the unit's alerts, the selected one with its evidence, resolution, history and what it
/// relates to; acknowledging it and resolving it with a reason and the evidence it cites.
struct AlertsView: View {
    @State private var model: AlertsModel

    init(model: AlertsModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.content.message {
                Card { Text(verbatim: message).accessibilityIdentifier("alerts.message") }
            }
            switch model.content.value {
            case .alerts?: alertCards
            case let .unavailable(title, text)?: Card(title) { Text(verbatim: text).accessibilityIdentifier("alerts.unavailable") }
            // a failed first read shows only its message, not a spinner that never ends
            case nil: if case .loading = model.content { ProgressView() }
            }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: tr("Alert evidence")))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
    }

    @ViewBuilder private var alertCards: some View {
        if let note = model.note {
            Card {
                Label {
                    Text(verbatim: note)
                } icon: {
                    Image(systemName: "checkmark.circle")
                }.accessibilityIdentifier("alerts.note")
            }
        }
        if let refusal = model.refusal {
            Card {
                Label {
                    Text(verbatim: refusal)
                } icon: {
                    Image(systemName: "exclamationmark.triangle")
                }
                .accessibilityIdentifier("alerts.refusal")
            }
        }
        if let view = model.view, let alert = model.selected {
            Card(view.title) {
                Text(verbatim: view.subtitle).font(.subheadline)
                if let state = view.state {
                    Text(verbatim: state).font(.subheadline.weight(.semibold)).accessibilityIdentifier("alerts.state")
                }
                Facts(rows: view.evidence)
            }
            resolutionCard(view, alert)
            Card(tr("Alert history")) { rows(view.history) }
            Card(tr("Related")) { Facts(rows: view.related) }
        } else {
            Card(tr("No alerts on this unit")) { Text(verbatim: tr("Alerts raised on this unit appear here with their evidence.")) }
        }
        if model.alerts.count > 1 {
            Card(tr("Alerts on this unit ({n})", ["n": String(model.alerts.count)])) {
                ForEach(model.choices) { choice in
                    Button {
                        Task { await model.select(choice.id) }
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(verbatim: choice.title).font(.headline)
                            Text(verbatim: "\(choice.badge) · \(choice.sub)").font(.subheadline)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .foregroundStyle(.primary)
                    }
                    .accessibilityIdentifier("alerts.choice.\(choice.id)")
                    .accessibilityAddTraits(choice.id == model.selected?.id ? .isSelected : [])
                }
            }
        }
    }

    @ViewBuilder
    private func resolutionCard(_ view: AlertWords.View, _ alert: CoreAlert) -> some View {
        Card(tr("Resolution")) {
            if let done = view.done { Text(verbatim: done).accessibilityIdentifier("alerts.done") }
            if let information = view.resolution { Text(verbatim: information) }
            if alert.status == .open {
                Button(tr("Acknowledge")) { Task { await model.acknowledge() } }
                    .secondaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("alerts.acknowledge")
            }
            if alert.status != .resolved {
                if model.canResolve { resolveForm(alert) } else { Text(verbatim: view.forbidden).font(.subheadline) }
            }
            Text(verbatim: view.note).font(.footnote)
        }
    }

    @ViewBuilder
    private func resolveForm(_ alert: CoreAlert) -> some View {
        TextField(
            tr("Resolution reason (required, 1–1000 characters)"),
            text: $model.reason,
            prompt: Text(verbatim: tr("e.g. Confirmed on site that airflow is restored after filter cleaning.")),
            axis: .vertical
        )
        .lineLimit(2...6)
        .textFieldStyle(.roundedBorder)
        .accessibilityIdentifier("alerts.reason")
        let required = alert.policyId == nil
        Text(verbatim: required ? tr("Resolution evidence · required") : tr("Resolution evidence · optional")).font(
            .subheadline.weight(.semibold)
        )
        Text(
            verbatim: required
                ? tr("At least one · measurements after detection") : tr("A policy alert may also be resolved by hand without evidence")
        )
        .font(.footnote)
        ForEach(model.evidenceRows) { row in
            Toggle(isOn: Binding(get: { model.picked.contains(row.id) }, set: { model.pick(row.id, $0) })) {
                VStack(alignment: .leading) {
                    Text(verbatim: row.title)
                    Text(verbatim: row.sub).font(.footnote)
                }
            }
            .accessibilityIdentifier("alerts.evidence.\(row.id)")
        }
        if let problem = model.problem { Text(verbatim: problem).accessibilityIdentifier("alerts.problem") }
        Button(tr("Resolve alert")) { Task { await model.resolve() } }
            .buttonStyle(.borderedProminent)
            .disabled(model.isSending)
            .accessibilityIdentifier("alerts.resolve")
    }

    private func rows(_ rows: [UnitRow]) -> some View {
        ForEach(rows) { row in
            VStack(alignment: .leading, spacing: 2) {
                Text(verbatim: row.title).font(.headline)
                Text(verbatim: row.sub).font(.subheadline)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .combine)
        }
    }
}
