import ACKit
import ACShared
import SwiftUI

/// One AC: its readings and confirmed setting, the remote control and the command history (the web's customer unit
/// page, FR-C03). A change is sent a moment after the last tap, without a confirmation; the result shows once the AC
/// answers, with its undo.
struct UnitDetailView: View {
    @State private var model: UnitControlModel

    init(model: UnitControlModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.unit.message {
                Card { StatusLabel(message, tone: .critical).accessibilityIdentifier("unit.message") }
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
        .loadingOverlay(model.unit.isLoading)
        // the AC's answer to a send is felt as well as read (the system's haptics follow the user's settings)
        .sensoryFeedback(trigger: model.outcome) { _, outcome in outcome.map { $0.isOk ? .success : .error } }
        .navigationTitle(Text(verbatim: model.unit.value?.displayName ?? ""))
        .inlineTitle()
        .refreshable { await model.load() }
        .task { await model.load() }
        .task { await refreshWhileShown() }
        // each change starts the wait again; the last one is sent
        .task(id: model.editCount) { if model.editCount > 0 { await model.sendWhenSettled() } }
        .task(id: model.outcome) { if let shown = model.outcome { await model.dismissWhenSeen(shown) } }
    }

    /// Others' changes (another user, an automation, the AC itself) appear without a reload, as on the web (IR809), and
    /// not while the customer is choosing a setting: a new report would put the choice back to the confirmed one.
    private func refreshWhileShown() async {
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(30))
            if !Task.isCancelled, !model.isSending, model.changes.isEmpty { await model.load() }
        }
    }

    @ViewBuilder
    private func overview(_ unit: UnitDetail) -> some View {
        Card {
            VStack(alignment: .leading, spacing: Spacing.xs) {
                Text(verbatim: unit.location.pathLabels.joined(separator: " › "))
                Text(verbatim: "\(Labels.power(unit.effectivePowerState)) · \(Labels.connection(unit.connection))").font(.headline)
            }
            .accessibilityElement(children: .combine)
            if unit.connection == .offline {
                StatusLabel(
                    tr(
                        "This AC is offline. Remote actions are disabled until the connection returns. Last seen {when}.",
                        ["when": unit.lastSeenAt.map { Format.dateTime($0) } ?? "—"]
                    ),
                    tone: .critical,
                    symbol: .offline
                )
            } else {
                reading(tr("Room temperature (measured)"), unit, .temperature)
                Fact(label: tr("Confirmed setting"), value: confirmed(unit.observedState), isEmphasized: true)
                reading(tr("Humidity"), unit, .humidity)
            }
        }
    }

    private func reading(_ label: String, _ unit: UnitDetail, _ metric: Metric) -> some View {
        let measurement = unit.latestMeasurements.first { $0.metric == metric }
        let value = measurement.flatMap(Words.reading) ?? "—"
        let at = measurement.map { tr("observed {at}", ["at": Format.dateTime($0.observedAt)]) } ?? tr("no recent measurement")
        return Fact(label: label, value: value, note: at, isEmphasized: true)
    }

    private func confirmed(_ state: ObservedState) -> String {
        let celsius = state.celsius.map { "\(Words.celsius($0))°C" } ?? "—"
        return "\(celsius) · \(state.mode.map(Words.mode) ?? "—") · \(state.fanLevel.map(Words.fan) ?? "—")"
    }

    @ViewBuilder private var banners: some View {
        if model.isBlocked {
            Card {
                StatusLabel(tr("Remote control is paused while HQ checks this AC’s restriction state. Try again later."), tone: .warn)
                    .accessibilityIdentifier("banner.blocked")
            }
        }
        switch model.restriction {
        case .powerOff:
            Card {
                StatusLabel(
                    tr("Service restriction active — payment-related: this AC is kept off until the release is confirmed."),
                    tone: .critical
                )
                .accessibilityIdentifier("banner.restriction")
            }
        case .temperatureLimit:
            Card {
                StatusLabel(
                    tr(
                        "Cooling restriction active — payment-related: setpoints below {min} °C are unavailable until the release is confirmed.",
                        ["min": Words.celsius(model.minimum)]
                    ),
                    tone: .warn
                )
                .accessibilityIdentifier("banner.restriction")
            }
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
                if let range = model.layout.temperature { temperature(range) }
                if !model.layout.modes.isEmpty {
                    let mode = Binding(get: { model.mode }, set: { model.choose(mode: $0) })
                    choice(tr("Mode"), id: "mode", options: model.layout.modes, selection: mode, words: Words.mode)
                }
                if !model.layout.fans.isEmpty {
                    let fan = Binding(get: { model.fan }, set: { model.choose(fan: $0) })
                    choice(tr("Fan speed"), id: "fan", options: model.layout.fans, selection: fan, words: Words.fan)
                }
                pending
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
        VStack(alignment: .leading, spacing: Spacing.s) {
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
            isOn: Binding(get: { model.isOn }, set: { power in Task { await model.sendPower(power) } })
        ) {
            VStack(alignment: .leading) {
                Text(verbatim: tr("Power"))
                if model.isPowerUnknown {
                    Text(verbatim: tr("Current power unknown · switching sends that command")).font(.footnote)
                }
            }
        }
        .disabled(model.isPowerLocked)
        .accessibilityIdentifier("power")
    }

    private func temperature(_ range: CapabilityTemperature) -> some View {
        VStack(alignment: .leading, spacing: Spacing.s) {
            Text(verbatim: tr("Set temperature"))
            HStack {
                Button {
                    model.lower()
                } label: {
                    Image(symbol: .decrease).frame(minWidth: Size.minTap, minHeight: Size.minTap)
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
                    Image(symbol: .increase).frame(minWidth: Size.minTap, minHeight: Size.minTap)
                }
                .buttonStyle(.bordered)
                .disabled(!model.canRaise || model.isSending)
                .accessibilityLabel(Text(verbatim: tr("Raise the temperature")))
                .accessibilityIdentifier("temperature.raise")
            }
            Text(verbatim: tr("{min}–{max}°C", ["min": Words.celsius(model.minimum), "max": Words.celsius(range.max)]))
                .font(.footnote)
                .fixedSize(horizontal: false, vertical: true)
            if let hint = model.coolingHint {
                StatusLabel(hint, tone: .info).font(.footnote)
            }
        }
    }

    /// The changes about to be sent, while the customer may still change them (they go a moment after the last tap), or
    /// the retry of changes a send did not take.
    @ViewBuilder private var pending: some View {
        if model.isSettling, !model.changes.isEmpty, !model.isDisabled, !model.isSending {
            StatusLabel(tr("Sending: {changes}", ["changes": model.changeSummary]), tone: .info, symbol: .sending)
                .font(.footnote)
                .accessibilityIdentifier("pending")
        } else if model.isLeftBehind {
            // a send that was refused leaves its changes: they go again only when asked
            Button(tr("Try again")) { Task { await model.sendChanges() } }
                .primaryAction()
                .accessibilityIdentifier("retry")
        }
    }

    @ViewBuilder private var state: some View {
        if model.isSending {
            // a new countdown for each command of the send (the web keys its wait banner by the command id)
            WaitingText(seconds: model.waiting?.seconds ?? 30).id(model.waiting?.commandId)
        } else if let outcome = model.outcome {
            VStack(alignment: .leading, spacing: Spacing.s) {
                StatusLabel(outcome.text, tone: outcome.isOk ? .ok : .critical).accessibilityIdentifier("outcome")
                if !outcome.undo.isEmpty {
                    Button(tr("Undo")) { Task { await model.undo() } }.secondaryAction().accessibilityIdentifier("outcome.undo")
                }
            }
        }
        if model.isOffline {
            StatusLabel(tr("Controls disabled while offline"), tone: .critical, symbol: .offline).font(.headline)
        }
    }

    /// The history, folded away: most visits only change the setting.
    @ViewBuilder private var history: some View {
        Card {
            DisclosureGroup {
                historyRows
            } label: {
                Text(verbatim: tr("Command history"))
            }
            .accessibilityIdentifier("history")
        }
    }

    @ViewBuilder private var historyRows: some View {
        VStack(alignment: .leading, spacing: Spacing.s) {
            if model.historyRows.isEmpty {
                Text(verbatim: tr("No commands for this AC yet"))
            }
            ForEach(model.historyRows) { row in
                VStack(alignment: .leading, spacing: Spacing.xxs) {
                    Label {
                        Text(verbatim: row.text)
                    } icon: {
                        if row.isBad {
                            Image(symbol: .failed).foregroundStyle(Palette.critical).accessibilityLabel(Text(verbatim: tr("Failed")))
                        }
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
