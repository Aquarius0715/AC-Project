import ACKit
import ACShared
import SwiftUI

/// Diagnostic control: the unit's current state, a diagnostic command, a test run and the job's command history.
struct ControlView: View {
    @State private var model: ControlModel
    /// The running send or test run with its polls, cancelled when the screen closes.
    @State private var writing: Task<Void, Never>?

    init(model: ControlModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.content.message {
                Card { Text(verbatim: message).accessibilityIdentifier("control.message") }
            }
            switch model.content.value {
            case .unit?: cards
            case let .notStarted(text)?: Card(tr("Not started yet")) { Text(verbatim: text).accessibilityIdentifier("control.notStarted") }
            case .notFound?:
                Card(tr("This page isn’t available")) {
                    Text(verbatim: tr("The page doesn’t exist, or your account can’t open it. Nothing was navigated automatically."))
                }
            // a failed first read shows only its message, not a spinner that never ends
            case nil: if case .loading = model.content { ProgressView() }
            }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: tr("Diagnostic control")))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
        .onDisappear { writing?.cancel() }
    }

    @ViewBuilder private var cards: some View {
        if let note = model.note {
            Card {
                Label {
                    Text(verbatim: note)
                } icon: {
                    Image(systemName: "checkmark.circle")
                }.accessibilityIdentifier("control.note")
            }
        }
        if let refusal = model.refusal {
            Card {
                Label {
                    Text(verbatim: refusal)
                } icon: {
                    Image(systemName: "exclamationmark.triangle")
                }
                .accessibilityIdentifier("control.refusal")
            }
        }
        if let banner = model.banner {
            Card(banner.title) { Text(verbatim: banner.text).accessibilityIdentifier("control.run") }
        }
        Card(tr("Current unit state")) {
            Facts(rows: model.tiles)
            if let capability = model.capability { Text(verbatim: capability).font(.footnote) }
            if let restriction = model.restriction {
                Text(verbatim: tr("Restriction active — {restriction} applies to diagnostics too.", ["restriction": restriction])).font(
                    .subheadline
                )
            }
        }
        if let unavailable = model.unavailable {
            Card { Text(verbatim: unavailable).accessibilityIdentifier("control.unavailable") }
        } else {
            commandCard
            runCard
        }
        if model.canDiagnose, model.job != nil {
            Card(tr("Command history — this job")) {
                if model.history.isEmpty { Text(verbatim: tr("No commands for this job yet.")) }
                ForEach(model.history) { row in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(verbatim: row.title).font(.headline)
                        Text(verbatim: "\(row.badge) · \(row.sub)").font(.subheadline)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("control.history.\(row.id)")
                }
                Text(verbatim: tr("Newest first · the device's acknowledgement is the only proof a setting changed")).font(.footnote)
            }
        }
    }

    private func picker(_ title: String, selection: Binding<String>, id: String) -> some View {
        Picker(selection: selection) {
            ForEach(model.options) { Text(verbatim: $0.label).tag($0.id) }
        } label: {
            Text(verbatim: title)
        }
        .pickerStyle(.menu)
        .accessibilityIdentifier(id)
    }

    @ViewBuilder private var commandCard: some View {
        Card(tr("Normal diagnostic action")) {
            picker(tr("Action"), selection: $model.actionId, id: "control.action")
            TextField(
                tr("Reason (required)"),
                text: $model.reason,
                prompt: Text(verbatim: tr("Why this command is needed for the diagnosis")),
                axis: .vertical
            )
            .lineLimit(2...5)
            .textFieldStyle(.roundedBorder)
            .accessibilityIdentifier("control.reason")
            if let error = model.fieldErrors["reason"] { Text(verbatim: error).accessibilityIdentifier("control.error.reason") }
            Button(tr("Review & send")) { writing = Task { await model.send() } }
                .buttonStyle(.borderedProminent)
                .disabled(model.isSending || model.isActive)
                .accessibilityIdentifier("control.send")
            Text(
                verbatim: tr(
                    "Permission, work window, capability and restriction are checked again just before sending. The setting changes only after the device acknowledges the command."
                )
            )
            .font(.footnote)
        }
    }

    @ViewBuilder private var runCard: some View {
        Card(tr("Test run")) {
            picker(tr("Start action"), selection: $model.startId, id: "control.run.start")
            picker(tr("End action"), selection: $model.endId, id: "control.run.end")
            Stepper(value: $model.minutes, in: 1...15) { Text(verbatim: "\(tr("Duration (min)")) \(model.minutes)") }
                .accessibilityIdentifier("control.run.minutes")
            TextField(
                tr("Reason (required)"),
                text: $model.runReason,
                prompt: Text(verbatim: tr("What the test run should show")),
                axis: .vertical
            )
            .lineLimit(2...5)
            .textFieldStyle(.roundedBorder)
            .accessibilityIdentifier("control.run.reason")
            if let error = model.fieldErrors["runReason"] ?? model.fieldErrors["durationMinutes"] {
                Text(verbatim: error).accessibilityIdentifier("control.error.run")
            }
            Button(tr("Test run")) { writing = Task { await model.startRun() } }
                .buttonStyle(.borderedProminent)
                .disabled(model.isSending || model.isActive)
                .accessibilityIdentifier("control.run.submit")
            Text(verbatim: tr("1–15 minutes only. Reason required. Cannot start during firmware updates or an unfinished command.")).font(
                .footnote
            )
        }
    }
}
