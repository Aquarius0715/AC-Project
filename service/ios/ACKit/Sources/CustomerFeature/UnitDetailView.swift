import ACKit
import ACShared
import SwiftUI

/// One AC: its readings and confirmed setting, the remote control and the command history (the web's customer unit
/// page, FR-C03). Nothing is sent without a confirmation; the result shows once the AC answers.
struct UnitDetailView: View {
    @State private var model: UnitControlModel
    /// The power the customer switched to, waiting for the confirmation.
    @State private var isAskingPower = false
    @State private var askedPower = false
    @State private var isConfirming = false

    init(model: UnitControlModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.unit.message {
                Text(verbatim: message).accessibilityIdentifier("unit.message")
            }
            if let unit = model.unit.value {
                overview(unit)
                banners
                control(unit)
                history
            }
        }
        .cardScreen()
        .hideTabBar()
        .overlay { if case .loading = model.unit { ProgressView() } }
        .navigationTitle(Text(verbatim: model.unit.value?.displayName ?? ""))
        .inlineTitle()
        .refreshable { await model.load() }
        .task { await model.load() }
        .task { await refreshWhileShown() }
        .confirmationDialog(
            Text(verbatim: askedPower ? tr("Turn this AC on?") : tr("Turn this AC off?")),
            isPresented: $isAskingPower,
            titleVisibility: .visible
        ) {
            if isAskingPower {
                Button(askedPower ? tr("Power ON") : tr("Power OFF")) { Task { await model.sendPower(askedPower) } }
                    .accessibilityIdentifier("power.confirm")
            }
            Button(tr("Cancel"), role: .cancel) { isAskingPower = false }
        }
        .alert(Text(verbatim: tr("Confirm command")), isPresented: $isConfirming) {
            Button(tr("Cancel"), role: .cancel) { isConfirming = false }
            Button(tr("Send to {name}", ["name": model.unit.value?.displayName ?? ""])) { Task { await model.sendChanges() } }
                .accessibilityIdentifier("send.confirm")
        } message: {
            Text(verbatim: confirmMessage)
        }
    }

    /// What the confirmation says: the changes, and that only this AC changes once it answers.
    private var confirmMessage: String {
        let name = model.unit.value?.displayName ?? ""
        let only = tr("Only {name} changes. The result shows after the device acknowledges.", ["name": name])
        return "\(model.changeSummary)\n\(only)"
    }

    /// Others' changes (another user, an automation, the AC itself) appear without a reload, as on the web (IR809), and
    /// not while the customer is choosing a setting: a new report would put the choice back to the confirmed one.
    private func refreshWhileShown() async {
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(30))
            if !Task.isCancelled, !model.isSending, !isAskingPower, !isConfirming, model.changes.isEmpty { await model.load() }
        }
    }

    @ViewBuilder
    private func overview(_ unit: UnitDetail) -> some View {
        Card {
            VStack(alignment: .leading, spacing: 4) {
                Text(verbatim: unit.location.pathLabels.joined(separator: " › "))
                Text(verbatim: "\(Labels.power(unit.effectivePowerState)) · \(Labels.connection(unit.connection))").font(.headline)
            }
            .accessibilityElement(children: .combine)
            if unit.connection == .offline {
                Text(
                    verbatim: tr(
                        "This AC is offline. Remote actions are disabled until the connection returns. Last seen {when}.",
                        ["when": unit.lastSeenAt.map { Format.dateTime($0) } ?? "—"]
                    )
                )
            } else {
                reading(tr("Room temperature (measured)"), unit, .temperature)
                Fact(label: tr("Confirmed setting"), value: confirmed(unit.observedState), note: nil)
                reading(tr("Humidity"), unit, .humidity)
            }
        }
    }

    private func reading(_ label: String, _ unit: UnitDetail, _ metric: Metric) -> some View {
        let measurement = unit.latestMeasurements.first { $0.metric == metric }
        let value = measurement.flatMap(Words.reading) ?? "—"
        let at = measurement.map { tr("observed {at}", ["at": Format.dateTime($0.observedAt)]) } ?? tr("no recent measurement")
        return Fact(label: label, value: value, note: at)
    }

    private func confirmed(_ state: ObservedState) -> String {
        let celsius = state.celsius.map { "\(Words.celsius($0))°C" } ?? "—"
        return "\(celsius) · \(state.mode.map(Words.mode) ?? "—") · \(state.fanLevel.map(Words.fan) ?? "—")"
    }

    @ViewBuilder private var banners: some View {
        if model.isBlocked {
            Text(verbatim: tr("Remote control is paused while HQ checks this AC’s restriction state. Try again later."))
                .accessibilityIdentifier("banner.blocked")
        }
        switch model.restriction {
        case .powerOff:
            Text(verbatim: tr("Service restriction active — payment-related: this AC is kept off until the release is confirmed."))
                .accessibilityIdentifier("banner.restriction")
        case .temperatureLimit:
            Text(
                verbatim: tr(
                    "Cooling restriction active — payment-related: setpoints below {min} °C are unavailable until the release is confirmed.",
                    ["min": Words.celsius(model.minimum)]
                )
            )
            .accessibilityIdentifier("banner.restriction")
        case nil:
            EmptyView()
        }
    }

    @ViewBuilder
    private func control(_ unit: UnitDetail) -> some View {
        Card(tr("REMOTE CONTROL")) {
            if model.layout.control {
                // the wait and the result first: a send's answer is seen where the customer looks, not below the fold
                state
                power
                if let range = model.layout.temperature { temperature(range, unit) }
                if !model.layout.modes.isEmpty {
                    let mode = Binding(get: { model.mode }, set: { model.choose(mode: $0) })
                    choice(tr("Mode"), id: "mode", options: model.layout.modes, selection: mode, words: Words.mode)
                }
                if !model.layout.fans.isEmpty {
                    let fan = Binding(get: { model.fan }, set: { model.choose(fan: $0) })
                    choice(tr("Fan speed"), id: "fan", options: model.layout.fans, selection: fan, words: Words.fan)
                }
                review(unit)
            } else {
                Text(
                    verbatim: tr(
                        "This AC’s module only measures: its readings are shown below, and settings are changed on the AC itself."
                    )
                )
            }
        }
    }

    /// A setting with a few values: its name above every value side by side (three at most on every model).
    private func choice<Value: Hashable>(
        _ title: String,
        id: String,
        options: [Value],
        selection: Binding<Value>,
        words: @escaping (Value) -> String
    ) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(verbatim: title)
            Picker(title, selection: selection) {
                ForEach(options, id: \.self) { Text(verbatim: words($0)).tag($0) }
            }
            .pickerStyle(.segmented)
            .disabled(model.isDisabled || model.isSending)
            .accessibilityIdentifier(id)
        }
    }

    private var power: some View {
        Toggle(
            isOn: Binding(
                get: { model.isOn },
                set: { power in
                    askedPower = power
                    isAskingPower = true
                }
            )
        ) {
            VStack(alignment: .leading) {
                Text(verbatim: tr("Power"))
                Text(
                    verbatim: model.isPowerUnknown
                        ? tr("Current power unknown · switching sends that command") : tr("Sent as its own command")
                )
                .font(.footnote)
            }
        }
        .disabled(model.isPowerLocked)
        .accessibilityIdentifier("power")
    }

    private func temperature(_ range: CapabilityTemperature, _ unit: UnitDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(verbatim: tr("Set temperature"))
            HStack {
                Button {
                    model.lower()
                } label: {
                    Image(systemName: "minus").frame(minWidth: 44, minHeight: 44)
                }
                .buttonStyle(.bordered)
                .disabled(!model.canLower || model.isSending)
                .accessibilityLabel(Text(verbatim: tr("Lower the temperature")))
                .accessibilityIdentifier("temperature.lower")
                Text(verbatim: "\(Words.celsius(model.setpoint))°C")
                    .font(.title3.bold())
                    .frame(minWidth: 64)
                    .accessibilityIdentifier("temperature.value")
                Button {
                    model.raise()
                } label: {
                    Image(systemName: "plus").frame(minWidth: 44, minHeight: 44)
                }
                .buttonStyle(.bordered)
                .disabled(!model.canRaise || model.isSending)
                .accessibilityLabel(Text(verbatim: tr("Raise the temperature")))
                .accessibilityIdentifier("temperature.raise")
            }
            Text(
                verbatim: tr(
                    "{min}–{max}°C · step {step}°C · confirmed {confirmed}°C",
                    [
                        "min": Words.celsius(model.minimum), "max": Words.celsius(range.max), "step": Words.celsius(range.step),
                        "confirmed": unit.observedState.celsius.map(Words.celsius) ?? "—",
                    ]
                )
            )
            .font(.footnote)
            .fixedSize(horizontal: false, vertical: true)
            if let hint = model.coolingHint {
                Label {
                    Text(verbatim: hint)
                } icon: {
                    Image(systemName: "info.circle")
                }.font(.footnote)
            }
        }
    }

    private func review(_ unit: UnitDetail) -> some View {
        let count = model.changes.count
        return VStack(alignment: .leading, spacing: 8) {
            Text(
                verbatim: "\(count == 1 ? tr("{n} change:", ["n": "1"]) : tr("{n} changes:", ["n": String(count)])) \(model.changeSummary)"
            )
            .accessibilityIdentifier("review.summary")
            Text(verbatim: tr("Target: {name} only · you will confirm before sending", ["name": unit.displayName])).font(.footnote)
            // only a change there is to send has its button: a greyed-out button is below the contrast the audit asks
            // for, and the line above already says there is no change (the wait shows below while sending)
            if count > 0, !model.isDisabled, !model.isSending {
                Button(tr("Review & send")) { isConfirming = true }
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("review.send")
            }
        }
    }

    @ViewBuilder private var state: some View {
        if model.isSending {
            WaitingText(seconds: model.answerSeconds ?? 30)
        } else if let outcome = model.outcome {
            VStack(alignment: .leading, spacing: 8) {
                Label {
                    Text(verbatim: outcome.text)
                } icon: {
                    Image(systemName: outcome.isOk ? "checkmark.circle" : "exclamationmark.triangle")
                }
                .accessibilityIdentifier("outcome")
                HStack {
                    if !outcome.undo.isEmpty {
                        Button(tr("Undo")) { Task { await model.undo() } }.secondaryAction().accessibilityIdentifier("outcome.undo")
                    }
                    Button(tr("Dismiss")) { model.outcome = nil }.secondaryAction()
                }
            }
        }
        if model.isOffline {
            Text(verbatim: tr("Controls disabled while offline")).font(.headline)
        }
    }

    @ViewBuilder private var history: some View {
        Card(tr("Command history")) {
            if model.historyRows.isEmpty {
                Text(verbatim: tr("No commands for this AC yet"))
            }
            ForEach(model.historyRows) { row in
                VStack(alignment: .leading, spacing: 2) {
                    Label {
                        Text(verbatim: row.text)
                    } icon: {
                        if row.isBad { Image(systemName: "exclamationmark.circle").accessibilityLabel(Text(verbatim: tr("Failed"))) }
                    }
                    Text(verbatim: row.when).font(.footnote)
                }
                .accessibilityElement(children: .combine)
            }
        }
    }
}

/// The wait for the AC's answer, with the seconds it has left (IR806); the sentence stays the same for VoiceOver.
private struct WaitingText: View {
    let seconds: Int
    @State private var started = Date()

    var body: some View {
        TimelineView(.periodic(from: started, by: 1)) { context in
            let left = max(0, seconds - Int(context.date.timeIntervalSince(started)))
            HStack {
                ProgressView()
                if left > 0 {
                    Text(verbatim: tr("Waiting for the AC to confirm… The result shows only after it acknowledges."))
                    Text(verbatim: tr("{s} s left", ["s": String(left)])).accessibilityHidden(true)
                } else {
                    Text(verbatim: tr("No answer yet — checking the result…"))
                }
            }
        }
        .accessibilityIdentifier("waiting")
    }
}

/// A reading or a setting: its name, its value and when it was observed, stacked so that every size of text fits
/// (a label beside its value is cut at the largest sizes).
private struct Fact: View {
    let label: String
    let value: String
    let note: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(verbatim: label).font(.subheadline)
            Text(verbatim: value).font(.headline)
            if let note { Text(verbatim: note).font(.footnote) }
        }
        .accessibilityElement(children: .combine)
    }
}
