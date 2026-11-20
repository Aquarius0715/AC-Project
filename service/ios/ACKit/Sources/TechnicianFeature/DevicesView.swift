import ACKit
import ACShared
import SwiftUI

/// The devices of the technician's assigned units, with their filter and the registration of a new one.
struct DevicesView: View {
    @State private var model: DevicesModel
    @State private var isRegistering = false

    init(model: DevicesModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        NavigationStack {
            Group {
                if let message = model.devices.message { Card { Text(verbatim: message).accessibilityIdentifier("devices.message") } }
                if let note = model.note { Card { Text(verbatim: note).accessibilityIdentifier("devices.note") } }
                Card {
                    HStack {
                        ForEach(DeviceFilter.allCases, id: \.self) { filter in
                            if model.filter == filter {
                                Button(filter.label) { model.filter = filter }.buttonStyle(.borderedProminent)
                            } else {
                                Button(filter.label) { model.filter = filter }.secondaryAction()
                            }
                        }
                    }
                    .accessibilityIdentifier("devices.filters")
                }
                Card(tr("Devices")) {
                    if model.rows.isEmpty { Text(verbatim: tr("No devices")).accessibilityIdentifier("devices.empty") }
                    ForEach(model.rows) { row in
                        NavigationLink {
                            DeviceView(model: DeviceModel(deviceId: row.id, client: model.client))
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(verbatim: row.title).font(.headline)
                                Text(verbatim: row.sub).font(.subheadline)
                            }
                            .multilineTextAlignment(.leading)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .foregroundStyle(.primary)
                        }
                        .accessibilityIdentifier("device.\(row.id)")
                    }
                }
                if model.canMaintain, !model.candidates.isEmpty {
                    Button(tr("Register device")) {
                        model.blank()
                        isRegistering = true
                    }
                    .buttonStyle(.borderedProminent)
                    .accessibilityIdentifier("devices.register")
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Devices")))
            .inlineTitle()
            .task { await model.load() }
            .refreshable { await model.load() }
            .sheet(isPresented: $isRegistering) { RegisterSheet(model: model) { isRegistering = false } }
        }
    }
}

/// Register a device on an assigned unit without one: its serial and the sensors of the unit's model it carries.
struct RegisterSheet: View {
    @Bindable var model: DevicesModel
    let close: () -> Void

    var body: some View {
        NavigationStack {
            Group {
                Card(tr("Register device")) {
                    Picker(selection: $model.unitId) {
                        ForEach(model.candidates) { Text(verbatim: $0.label).tag($0.id) }
                    } label: {
                        Text(verbatim: tr("Unit"))
                    }
                    .pickerStyle(.menu)
                    TextField(tr("Serial number"), text: $model.serial, axis: .vertical)
                        .textFieldStyle(.roundedBorder)
                        .codeEntry()
                        .accessibilityIdentifier("register.serial")
                    if let error = model.fieldErrors["serial"] { Text(verbatim: error).accessibilityIdentifier("register.error.serial") }
                    ForEach(model.candidates.first { $0.id == model.unitId }?.sensors ?? [], id: \.self) { metric in
                        Toggle(
                            isOn: Binding(
                                get: { model.sensorTypes.contains(metric) },
                                set: { on in if on { model.sensorTypes.insert(metric) } else { model.sensorTypes.remove(metric) } }
                            )
                        ) { Text(verbatim: UnitWords.metric(metric)) }
                    }
                    if let refusal = model.refusal { Text(verbatim: refusal).accessibilityIdentifier("register.refusal") }
                    HStack {
                        Button(tr("Register")) { Task { if await model.register() { close() } } }
                            .buttonStyle(.borderedProminent)
                            .disabled(model.isSending)
                            .accessibilityIdentifier("register.save")
                        Button(tr("Cancel"), action: close).secondaryAction()
                    }
                }
            }
            .cardScreen()
            .navigationTitle(Text(verbatim: tr("Register device")))
            .inlineTitle()
        }
    }
}

/// One device: its state, sensors, firmware, operations, calibrations and events, and the technician's maintenance.
struct DeviceView: View {
    @State private var model: DeviceModel

    init(model: DeviceModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let message = model.device.message { Card { Text(verbatim: message).accessibilityIdentifier("device.message") } }
            if let note = model.note {
                Card {
                    Label {
                        Text(verbatim: note)
                    } icon: {
                        Image(systemName: "checkmark.circle")
                    }.accessibilityIdentifier("device.note")
                }
            }
            if let refusal = model.refusal {
                Card {
                    Label {
                        Text(verbatim: refusal)
                    } icon: {
                        Image(systemName: "exclamationmark.triangle")
                    }
                    .accessibilityIdentifier("device.refusal")
                }
            }
            if model.device.value != nil { cards } else if model.device.message == nil { ProgressView() }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: model.device.value?.serial ?? tr("Device")))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        .refreshable { await model.load() }
    }

    @ViewBuilder private var cards: some View {
        Card(tr("Device")) {
            Facts(rows: model.tiles)
            if let unit = model.unit {
                Text(verbatim: "\(unit.displayName) · \(unit.location.pathLabels.joined(separator: " › "))").font(.subheadline)
            }
        }
        if let card = model.firmwareCard { Card(card.title) { Text(verbatim: card.text).accessibilityIdentifier("device.firmware") } }
        if model.canWork { maintenance }
        Card(tr("Sensors")) {
            Facts(rows: model.sensorRows)
            if model.canWork, !model.sensorRows.isEmpty { calibration }
        }
        if model.canWork {
            Card(tr("Operation history")) { rows(model.operationRows) }
            if !model.calibrationRows.isEmpty {
                Card(tr("Calibration history")) {
                    ForEach(Array(model.calibrationRows.enumerated()), id: \.offset) { Text(verbatim: $1).font(.subheadline) }
                }
            }
        }
        eventsCard
    }

    @ViewBuilder private var maintenance: some View {
        Card(tr("Maintenance")) {
            Button(tr("Check connection")) { Task { await model.check() } }
                .secondaryAction()
                .disabled(model.isSending)
                .accessibilityIdentifier("device.check")
            ForEach(model.newerFirmware, id: \.self) { version in
                Button(tr("Firmware update {from} → {to}", ["from": model.device.value?.firmwareVersion ?? "", "to": version])) {
                    Task { await model.update(to: version) }
                }
                .secondaryAction()
                .disabled(model.isSending)
                .accessibilityIdentifier("device.update.\(version)")
            }
            if !model.rebindTargets.isEmpty {
                Picker(selection: $model.bindUnitId) {
                    ForEach(model.rebindTargets) { Text(verbatim: $0.title).tag($0.id) }
                } label: {
                    Text(verbatim: tr("Unit"))
                }
                .pickerStyle(.menu)
                TextField(tr("Reason (required)"), text: $model.bindReason, axis: .vertical)
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("device.bind.reason")
                if let error = model.fieldErrors["reason"] { Text(verbatim: error) }
                Button(tr("Rebind…")) { Task { await model.bind() } }
                    .secondaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("device.bind")
            }
        }
    }

    @ViewBuilder private var calibration: some View {
        Picker(selection: $model.sensorId) {
            ForEach(model.device.value?.sensors ?? [], id: \.id) { Text(verbatim: UnitWords.metric($0.metric)).tag($0.id) }
        } label: {
            Text(verbatim: tr("Sensor"))
        }
        .pickerStyle(.menu)
        TextField(tr("Reference value"), text: $model.reference, axis: .vertical)
            .textFieldStyle(.roundedBorder)
            .accessibilityIdentifier("device.calibrate.reference")
        TextField(tr("Measured value"), text: $model.measured, axis: .vertical)
            .textFieldStyle(.roundedBorder)
            .accessibilityIdentifier("device.calibrate.measured")
        if let error = model.fieldErrors["referenceValue"] ?? model.fieldErrors["measuredValue"] {
            Text(verbatim: error).accessibilityIdentifier("device.calibrate.error")
        }
        Button(tr("Record calibration")) { Task { await model.calibrate() } }
            .secondaryAction()
            .disabled(model.isSending)
            .accessibilityIdentifier("device.calibrate")
    }

    @ViewBuilder private var eventsCard: some View {
        Card(tr("Device events")) {
            if model.eventRows.isEmpty { Text(verbatim: tr("No device events.")).accessibilityIdentifier("device.events.empty") }
            ForEach(model.eventRows) { row in
                VStack(alignment: .leading, spacing: 4) {
                    Text(verbatim: row.title).font(.headline)
                    Text(verbatim: row.detail).font(.subheadline)
                    Text(verbatim: row.state).font(.subheadline.weight(.semibold))
                    ForEach(Array(row.notes.enumerated()), id: \.offset) { Text(verbatim: $1).font(.footnote) }
                    Button(tr("Add response note")) { Task { await model.addNote(to: row) } }
                        .secondaryAction()
                        .disabled(model.isSending)
                        .accessibilityIdentifier("device.event.note.\(row.id)")
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            if !model.eventRows.isEmpty {
                TextField(tr("What you checked or did"), text: $model.noteText, axis: .vertical)
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("device.note.text")
                if let error = model.fieldErrors["responseNote"] { Text(verbatim: error) }
            }
            if model.canAcknowledge {
                ForEach(model.openAlerts, id: \.id) { alert in
                    Button(tr("Acknowledge {type} alert {id}", ["type": UnitWords.alertTitle(alert), "id": String(alert.id.prefix(8))])) {
                        Task { await model.acknowledge(alert) }
                    }
                    .secondaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("device.alert.\(alert.id)")
                }
            }
        }
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
