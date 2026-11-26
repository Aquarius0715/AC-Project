import ACKit
import ACShared
import Foundation
import Observation

/// One device of the technician's assigned units (FR-T11, FR-T12, DD-T11, DD-T12): its connection, power signal,
/// tamper and firmware with their evidence, its sensors, operations and calibrations, its events with their alerts and
/// response notes; and the maintenance a technician does under the unit's job (IR94, device.maintain): a connection check,
/// a calibration, a firmware update to a newer candidate, binding it to another assigned unit, a response note on an event
/// and acknowledging an event's alert (alert.read). Every write carries the version it changes.
@MainActor @Observable
public final class DeviceModel {
    public let deviceId: String
    public private(set) var device: Loadable<DeviceDetail> = .loading
    public private(set) var unit: UnitDetail?
    public private(set) var operations: [DeviceOperation] = []
    public private(set) var calibrations: [CalibrationRecord] = []
    public private(set) var events: [DeviceEvent] = []
    public private(set) var alerts: [String: CoreAlert] = [:]
    public private(set) var jobs: [JobSummary] = []
    public private(set) var units: [UnitSummary] = []
    public private(set) var canMaintain = false
    public private(set) var canAcknowledge = false
    public private(set) var now = Date()
    public private(set) var isSending = false
    public private(set) var note: String?
    public private(set) var refusal: String?
    public private(set) var fieldErrors: [String: String] = [:]
    /// The calibration form: the sensor, the reference and the measured value.
    public var sensorId = ""
    public var reference = ""
    public var measured = ""
    /// The binding form: the other unit and the reason.
    public var bindUnitId = ""
    public var bindReason = ""
    /// The response note and the event it is for.
    public var noteText = ""

    let client: CoreClient

    public init(deviceId: String, client: CoreClient) {
        self.deviceId = deviceId
        self.client = client
    }

    /// The unit the device serves: its bound unit, or the one it is planned for (D05, IR722).
    public var home: String? { device.value.flatMap { $0.unitId ?? $0.targetUnitId } }

    /// The job device writes run under: the unit's job in progress, else assigned, else returned for rework.
    public var jobId: String? { DeviceWords.jobs(for: home, jobs).first?.id }

    public var canWork: Bool { canMaintain && jobId != nil }

    public func load() async {
        device = await device.after { () async throws(CoreError) in
            let answer = try await client.call(Ops.DevicesGet.self, DevicesGetInput(id: deviceId))
            now = answer.meta.snapshotAt.flatMap(Format.parse) ?? Date()
            return answer.data
        }
        guard let device = device.value else { return }
        let session = try? await client.call(Ops.SessionGet.self, Empty())
        canMaintain = session?.data.permissions.contains(.deviceMaintain) ?? false
        canAcknowledge = session?.data.permissions.contains(.alertRead) ?? false
        jobs = ((try? await client.call(Ops.JobsList.self, Query(limit: 100)).data.items) ?? []).compactMap { item in
            if case let .summary(job) = item { job } else { nil }
        }
        units = (try? await client.call(Ops.UnitsList.self, Query(limit: 100)).data.items) ?? []
        unit = nil
        if let home { unit = try? await client.call(Ops.UnitsGet.self, UnitsGetInput(id: home)).data }
        let byNewest = Query(limit: 20, sort: QuerySort(field: .createdAt, direction: .desc))
        if canWork {
            let operationsInput = DevicesOperationsInput(deviceId: device.id, query: byNewest)
            operations = (try? await client.call(Ops.DevicesOperations.self, operationsInput).data.items) ?? []
            calibrations =
                (try? await client.call(Ops.DevicesCalibrations.self, DevicesCalibrationsInput(deviceId: device.id, query: byNewest)).data
                    .items) ?? []
        }
        // the events need an assignment on the unit; without one they are not shown
        events =
            (try? await client.call(Ops.DevicesEvents.self, DevicesEventsInput(id: device.id, query: Query(limit: 50))).data.items) ?? []
        var alerts: [String: CoreAlert] = [:]
        for id in Set(events.flatMap(\.alertIds)) {
            if let alert = try? await client.call(Ops.AlertsGet.self, AlertsGetInput(id: id)).data { alerts[id] = alert }
        }
        self.alerts = alerts
        if !device.sensors.contains(where: { $0.id == sensorId }) { sensorId = device.sensors.first?.id ?? "" }
        if !rebindTargets.contains(where: { $0.id == bindUnitId }) { bindUnitId = rebindTargets.first?.id ?? "" }
    }

    // MARK: - What the screen shows

    /// The firmware candidates above the installed version (the IoT model the device was registered with).
    public var newerFirmware: [String] {
        guard let device = device.value, let model = unit?.iotModel, model.id == device.iotModelId else { return [] }
        return DeviceWords.newer(device.firmwareVersion, model.firmwareCandidates)
    }

    public var tiles: [(String, String)] {
        device.value.map { DeviceWords.tiles($0, newest: newerFirmware.last, faults: DeviceWords.openFaults(events), now: now) } ?? []
    }

    public var firmwareCard: (title: String, text: String)? { device.value.flatMap { DeviceWords.firmwareCard($0, operations) } }
    public var operationRows: [UnitRow] { device.value.map { DeviceWords.operations(operations, current: $0.firmwareVersion) } ?? [] }
    public var calibrationRows: [String] { calibrations.map(DeviceWords.calibration) }
    public var sensorRows: [(String, String)] { device.value.map(DeviceWords.sensors) ?? [] }
    public var eventRows: [DeviceWords.EventRow] { DeviceWords.events(events, alerts: alerts, now: now) }

    /// The open alerts of the events, once each.
    public var openAlerts: [CoreAlert] {
        alerts.values.filter { $0.status == .open }.sorted { $0.id < $1.id }
    }

    /// The other assigned units the device can be bound to, each with its job.
    public var rebindTargets: [UnitRow] {
        units.filter { !$0.archived && $0.id != home }.compactMap { unit in
            DeviceWords.jobs(for: unit.id, jobs).first.map { UnitRow(id: unit.id, title: unit.displayName, sub: $0.id, badge: "") }
        }
    }

    // MARK: - The steps

    /// Checks the device's connection: the device answers with a heartbeat.
    public func check() async {
        await write(tr("Connection check requested")) { [client] device, jobId throws(CoreError) in
            _ = try await client.call(
                Ops.DevicesCheck.self,
                DevicesCheckInput(id: device.id, jobId: jobId),
                expectedVersion: Int(device.version)
            )
        }
    }

    /// Updates the firmware to a newer candidate.
    public func update(to version: String) async {
        await write(tr("Firmware update to {version} requested", ["version": version])) { [client] device, jobId throws(CoreError) in
            let input = DevicesUpdateFirmwareInput(deviceId: device.id, firmwareVersion: version, jobId: jobId)
            _ = try await client.call(Ops.DevicesUpdateFirmware.self, input, expectedVersion: Int(device.version))
        }
    }

    /// Records a calibration of a sensor at the Core API's time: the reference and what the sensor measured.
    public func calibrate() async {
        guard let sensor = device.value?.sensors.first(where: { $0.id == sensorId }) else { return }
        var errors: [String: String] = [:]
        // Double also reads "nan" and "inf", which JSON cannot carry
        let reference = Double(reference.trimmingCharacters(in: .whitespaces)).flatMap { $0.isFinite ? $0 : nil }
        let measured = Double(measured.trimmingCharacters(in: .whitespaces)).flatMap { $0.isFinite ? $0 : nil }
        if reference == nil { errors["referenceValue"] = tr("Not valid") }
        if measured == nil { errors["measuredValue"] = tr("Not valid") }
        fieldErrors = errors
        guard let reference, let measured else { return }
        let at = now.ISO8601Format()
        await write(tr("Calibration recorded — earlier readings unchanged")) { [client] device, jobId throws(CoreError) in
            let input = DevicesCalibrateInput(
                deviceId: device.id,
                sensorId: sensor.id,
                metric: sensor.metric,
                unit: sensor.unit,
                referenceValue: reference,
                measuredValue: measured,
                calibratedAt: at,
                jobId: jobId
            )
            _ = try await client.call(Ops.DevicesCalibrate.self, input, expectedVersion: Int(device.version))
        }
        if refusal == nil, fieldErrors.isEmpty {
            self.reference = ""
            self.measured = ""
        }
    }

    /// Binds the device to another assigned unit with a reason; the device gets new sensor IDs (SR24).
    public func bind() async {
        guard let target = rebindTargets.first(where: { $0.id == bindUnitId }) else { return }
        let reason = bindReason.trimmingCharacters(in: .whitespacesAndNewlines)
        // the Core API counts code points (utf8.RuneCountInString), not the characters on screen
        guard (1...1000).contains(reason.unicodeScalars.count) else {
            fieldErrors = ["reason": tr("1–1000 characters")]
            return
        }
        let done = tr("History stays with the previous binding; the device gets new sensor IDs on the new unit.")
        await write(done) { [client] device, _ throws(CoreError) in
            let input = DevicesBindInput(deviceId: device.id, unitId: target.id, jobId: target.sub, reason: reason)
            _ = try await client.call(Ops.DevicesBind.self, input, expectedVersion: Int(device.version))
        }
        if refusal == nil, fieldErrors.isEmpty { bindReason = "" }
    }

    /// Adds a response note to an event (its version); the device's physical state does not change.
    public func addNote(to event: DeviceWords.EventRow) async {
        let text = noteText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard (1...1000).contains(text.unicodeScalars.count) else {
            fieldErrors = ["responseNote": tr("1–1000 characters")]
            return
        }
        await write(tr("Response note added — the device state is unchanged"), needsJob: false) { [client] device, _ throws(CoreError) in
            let input = DevicesAddResponseNoteInput(deviceId: device.id, eventId: event.id, responseNote: text)
            _ = try await client.call(Ops.DevicesAddResponseNote.self, input, expectedVersion: Int(event.version))
        }
        if refusal == nil, fieldErrors.isEmpty { noteText = "" }
    }

    /// Acknowledges an alert an event raised (its version); a reconnection never clears it (DD-T12).
    public func acknowledge(_ alert: CoreAlert) async {
        await write(tr("Acknowledged"), needsJob: false) { [client] _, _ throws(CoreError) in
            _ = try await client.call(
                Ops.AlertsAcknowledge.self,
                AlertsAcknowledgeInput(alertId: alert.id),
                expectedVersion: Int(alert.version)
            )
        }
    }

    private func write(_ done: String, needsJob: Bool = true, _ send: (DeviceDetail, String?) async throws(CoreError) -> Void) async {
        // the maintenance runs under the unit's job with device.maintain; a note and an acknowledgement need neither
        guard let device = device.value, !isSending, !needsJob || canWork else { return }
        isSending = true
        defer { isSending = false }
        note = nil
        refusal = nil
        fieldErrors = [:]
        do {
            try await send(device, jobId)
            note = done
        } catch {
            refusal = DeviceWords.refusal(error.error)
            fieldErrors = error.error.fieldErrors.mapValues(DeviceWords.field)
        }
        await load()
    }
}
