import ACKit
import ACShared
import Foundation

/// A device filter of the list.
public enum DeviceFilter: String, Sendable, CaseIterable {
    case all, online, offline, tamper

    public var label: String {
        switch self {
        case .all: tr("All")
        case .online: tr("Online")
        case .offline: tr("Offline")
        case .tamper: tr("Tamper")
        }
    }

    func keeps(_ device: Device) -> Bool {
        switch self {
        case .all: true
        case .online: device.connection == .online
        case .offline: device.connection != .online
        case .tamper: device.tamper == .detected
        }
    }
}

/// Technician IoT devices in words (shared/lib/techDevices.ts, FR-T11, FR-T12, DD-T11, DD-T12): the list rows, the job a
/// device write runs under (IR94), the detail tiles, the firmware card, the operations, calibrations and sensors, and the
/// device events with their alerts and response notes. Event types, evidence sources and versions stay codes.
public enum DeviceWords {
    /// The jobs a device write on the unit can run under (IR94): in progress first, then assigned, then rework.
    public static func jobs(for unitId: String?, _ jobs: [JobSummary]) -> [JobSummary] {
        guard let unitId else { return [] }
        let rank: (JobStatus) -> Int? = { status in
            switch status {
            case .inProgress: 0
            case .assigned: 1
            case .reworkRequested: 2
            default: nil
            }
        }
        return jobs.compactMap { job in rank(job.status).flatMap { job.unitId == unitId ? (job, $0) : nil } }
            .sorted { $0.1 < $1.1 }
            .map(\.0)
    }

    /// "12 s ago", "5 min ago", "3 h ago", "4 d ago".
    static func ago(_ instant: String?, now: Date) -> String {
        guard let instant, let at = Format.parse(instant) else { return tr("never seen") }
        let s = max(0, Int(now.timeIntervalSince(at).rounded()))
        if s < 120 { return tr("{n} s ago", ["n": String(s)]) }
        if s < 7200 { return tr("{n} min ago", ["n": String(Int((Double(s) / 60).rounded()))]) }
        if s < 172_800 { return tr("{n} h ago", ["n": String(Int((Double(s) / 3600).rounded()))]) }
        return tr("{n} d ago", ["n": String(Int((Double(s) / 86_400).rounded()))])
    }

    /// One device of the list: serial, the unit it serves, its connection, firmware and sensor count.
    public static func row(_ device: Device, names: [String: String], now: Date) -> UnitRow {
        let unit: String =
            if let unitId = device.unitId {
                names[unitId] ?? tr("unit")
            } else if let target = device.targetUnitId {
                tr("Not bound yet — planned for {unit}", ["unit": names[target] ?? tr("unit")])
            } else {
                tr("Unbound")
            }
        var connection = Labels.connection(device.connection)
        if device.connection != .online, device.lastSeenAt != nil { connection += " · \(ago(device.lastSeenAt, now: now))" }
        let sensors =
            device.sensors.count == 1
            ? tr("Sensors ({n}) · from the IoT model", ["n": "1"])
            : tr("Sensors ({n}) · from the IoT model", ["n": String(device.sensors.count)])
        var parts = [unit, connection, device.firmwareVersion, sensors]
        if device.tamper == .detected { parts.append(tr("Tamper")) }
        return UnitRow(
            id: device.id,
            title: device.serial,
            sub: parts.joined(separator: " · "),
            badge: Labels.connection(device.connection)
        )
    }

    /// When the open fault of each axis was detected: the newest fault of the axis while it has not been restored.
    public static func openFaults(_ events: [DeviceEvent]) -> [String: String] {
        var out: [String: String] = [:]
        var seen: Set<String> = []
        for event in events.sorted(by: { $0.occurredAt > $1.occurredAt }) {
            let axis: String? =
                switch event.eventType {
                case .communicationLost: "connection"
                case .powerLost: "power"
                case .tamper: "tamper"
                case .restored, .operationFailed: nil
                }
            guard let axis, !seen.contains(axis) else { continue }
            seen.insert(axis)
            if event.restoredAt == nil { out[axis] = event.occurredAt }
        }
        return out
    }

    /// The firmware candidates above the installed version, oldest first (v2 < v10): a technician only updates forward.
    public static func newer(_ current: String, _ candidates: [String]) -> [String] {
        Array(Set(candidates)).filter { $0.compare(current, options: .numeric) == .orderedDescending }
            .sorted { $0.compare($1, options: .numeric) == .orderedAscending }
    }

    /// The four tiles: connection, power signal, tamper and firmware with their evidence lines.
    public static func tiles(_ device: DeviceDetail, newest: String?, faults: [String: String], now: Date) -> [(String, String)] {
        let lost = faults["connection"].flatMap { device.connection != .online ? tr("lost {time}", ["time": Format.dateTime($0)]) : nil }
        let connection =
            device.connection == .connecting
            ? tr("check running — waiting for the device") : lost ?? tr("last seen {time}", ["time": ago(device.lastSeenAt, now: now)])
        let power: String =
            switch device.powerSignal {
            case .on: tr("ON")
            case .off: tr("Lost")
            case .unknown: tr("Unknown")
            }
        let powerSub =
            device.powerSignal == .off ? faults["power"].map { "power_signal \(Format.dateTime($0))" } : nil
        let isTampered = device.tamper == .detected
        let tamperSub =
            isTampered
            ? faults["tamper"].map { tr("cover opened {time}", ["time": Format.dateTime($0)]) } ?? tr("cover opened — see device events")
            : tr("no removal detected")
        return [
            (tr("Connection"), "\(Labels.connection(device.connection)) · \(connection)"),
            (tr("Power signal"), "\(power) · \(powerSub ?? tr("from the device's dedicated power signal"))"),
            (tr("Tamper"), "\(isTampered ? tr("Detected") : tr("tamper::Clear")) · \(tamperSub)"),
            (tr("Firmware"), "\(device.firmwareVersion) · \(newest.map { tr("{version} available", ["version": $0]) } ?? tr("latest"))"),
        ]
    }

    /// Why a device operation failed.
    static func failure(_ code: ErrorCode?) -> String {
        switch code {
        case .timeout?: tr("no confirmation within 60 s")
        case .offline?: tr("the device was not online at the start")
        case .conflict?: tr("a command or another operation was running")
        case .unavailable?: tr("the device reported a failure")
        case let other?: other.rawValue
        case nil: tr("no result")
        }
    }

    static func kind(_ kind: DeviceOperationKind) -> String {
        switch kind {
        case .check: tr("Connection check")
        case .calibrate: tr("Calibration")
        case .firmware: tr("Firmware update")
        }
    }

    static func status(_ status: DeviceOperationStatus) -> String {
        switch status {
        case .queued: tr("Queued")
        case .running: tr("Running")
        case .succeeded: tr("Succeeded")
        case .failed: tr("Failed")
        }
    }

    /// The firmware card: the active update, else the last finished one; nil without one.
    public static func firmwareCard(_ device: DeviceDetail, _ operations: [DeviceOperation]) -> (title: String, text: String)? {
        let active = device.activeOperation?.kind == .firmware ? device.activeOperation : nil
        guard let last = active ?? operations.first(where: { $0.kind == .firmware }) else { return nil }
        let target = last.targetVersion ?? "—"
        switch last.status {
        case .queued, .running:
            let started = tr("Started {time} · control commands locked", ["time": Format.clock(last.startedAt ?? last.createdAt)])
            let deadline = tr("fails if the device does not confirm by {time}", ["time": Format.clock(last.expiresAt)])
            return (
                "\(tr("Firmware update {from} → {to}", ["from": device.firmwareVersion, "to": target])) · \(status(last.status))",
                "\(started) · \(deadline)"
            )
        case .failed:
            return (
                tr("Firmware update to {version} failed", ["version": target]),
                "\(failure(last.failureCode)) · \(tr("the old version {version} is kept", ["version": device.firmwareVersion]))"
            )
        case .succeeded:
            return (
                tr("Firmware {version} installed", ["version": target]),
                tr("finished {time} · history appended", ["time": last.finishedAt.map { Format.dateTime($0) } ?? "—"])
            )
        }
    }

    /// One operation of the history: what it did, its state and when.
    public static func operations(_ operations: [DeviceOperation], current: String) -> [UnitRow] {
        operations.map { operation in
            let isOpen = operation.status == .queued || operation.status == .running
            let detail: String
            if operation.status == .failed {
                detail = failure(operation.failureCode)
            } else if operation.kind == .firmware {
                let target = operation.targetVersion ?? "—"
                detail = isOpen ? "\(current) → \(target)" : tr("{version} installed", ["version": target])
            } else if operation.status == .succeeded {
                detail = tr("heartbeat confirmed")
            } else {
                detail = operation.status == .running ? tr("waiting for the heartbeat") : tr("starts within a second")
            }
            return UnitRow(
                id: operation.id,
                title: "\(kind(operation.kind)) · \(status(operation.status))",
                sub: "\(Format.dateTime(operation.startedAt ?? operation.createdAt)) · \(detail)",
                badge: status(operation.status)
            )
        }
    }

    static func signed(_ value: Double) -> String {
        value < 0 ? "−\(String(format: "%.1f", -value))" : String(format: "%.1f", value)
    }

    /// One calibration: reference, measured and the offset to apply (reference − measured).
    public static func calibration(_ record: CalibrationRecord) -> String {
        let unit = record.unit.rawValue
        return tr(
            "{metric} · ref {reference} / measured {measured} · offset {offset} · {time} — appended; earlier readings unchanged.",
            [
                "metric": UnitWords.metric(record.metric),
                "reference": "\(String(format: "%.1f", record.referenceValue)) \(unit)",
                "measured": "\(String(format: "%.1f", record.measuredValue)) \(unit)",
                "offset": "\(signed(record.referenceValue - record.measuredValue)) \(unit)",
                "time": Format.dateTime(record.calibratedAt),
            ]
        )
    }

    /// The device's sensors: metric, unit, stale limit and the last calibration.
    public static func sensors(_ device: DeviceDetail) -> [(String, String)] {
        device.sensors.map { sensor in
            (
                "\(UnitWords.metric(sensor.metric)) (\(sensor.unit.rawValue))",
                "\(tr("{n} s", ["n": String(Int(sensor.staleAfterSeconds))])) · \(sensor.calibratedAt.map { Format.dateTime($0) } ?? tr("Never"))"
            )
        }
    }

    static func label(_ type: EvidenceCandidateEventType) -> String {
        switch type {
        case .communicationLost: tr("Communication lost")
        case .powerLost: tr("Power lost")
        case .tamper: tr("Tamper")
        case .restored: tr("Restored")
        case .operationFailed: tr("Operation failed")
        }
    }

    /// What a device event means: the fault, or for a recovery the axis that came back.
    static func cause(_ event: DeviceEvent) -> String {
        switch event.recovery {
        case .connection?: return tr("Connection restored")
        case .power?: return tr("Power signal restored")
        case .tamper?: return tr("Tamper cleared — cover closed (the alert stays until handled)")
        case nil: break
        }
        switch event.eventType {
        case .tamper: return tr("Cover opened while powered")
        case .powerLost: return tr("Dedicated demo power signal lost")
        case .communicationLost: return tr("Missed heartbeats (not proof of power loss)")
        case .restored: return tr("Connection restored")
        case .operationFailed: return tr("Device operation failed")
        }
    }

    /// One event of the device: detection, evidence, its alerts, and where it stands (restored, a recovery, or open with
    /// its alerts' state); its response notes below.
    public struct EventRow: Sendable, Equatable, Identifiable {
        public let id: String
        public let version: Double
        public let title: String
        public let detail: String
        public let state: String
        public let notes: [String]
    }

    public static func events(_ events: [DeviceEvent], alerts: [String: CoreAlert], now: Date) -> [EventRow] {
        var byId: [String: DeviceEvent] = [:]
        for event in events where byId[event.id] == nil { byId[event.id] = event }
        return events.map { event in
            let mine = event.alertIds.compactMap { alerts[$0] }
            let count = event.alertIds.count
            let alertCount = count == 0 ? "" : " · \(count == 1 ? tr("1 alert") : tr("{n} alerts", ["n": String(count)]))"
            let state: String
            if let restored = event.restoredAt {
                state = tr("restored {time}", ["time": UnitWords.relative(restored, now: now)])
            } else if event.eventType == .restored {
                let source = event.recovery.flatMap { recovery -> DeviceEvent? in
                    switch recovery {
                    case let .connection(back): byId[back.sourceEventId]
                    case let .power(back): byId[back.sourceEventId]
                    case let .tamper(back): byId[back.sourceEventId]
                    }
                }
                state =
                    source.map {
                        tr("recovers {event} {time}", ["event": label($0.eventType), "time": UnitWords.relative($0.occurredAt, now: now)])
                    } ?? tr("recovers an earlier fault")
            } else if mine.isEmpty {
                state = tr("open")
            } else if mine.contains(where: { $0.status == .open }) {
                state = tr("open · unacknowledged")
            } else {
                state = mine.allSatisfy { $0.status == .resolved } ? tr("open · resolved") : tr("open · acknowledged")
            }
            return EventRow(
                id: event.id,
                version: event.version,
                title: "\(label(event.eventType)) · \(UnitWords.relative(event.occurredAt, now: now))",
                detail: "\(cause(event)) · \(event.evidenceSource.rawValue)\(alertCount)",
                state: state,
                notes: event.responseNotes.map { "\(UnitWords.relative($0.at, now: now)) — \($0.message)" }
            )
        }
    }

    /// A refused device write in words; nil for a refusal that only marks fields.
    public static func refusal(_ error: DomainError) -> String? {
        if error.messageKey == "errors.duplicate_serial" || (error.code == .conflict && error.fieldErrors["serial"] != nil) {
            return tr("This serial is already registered.")
        }
        switch error.code {
        case .offline: return tr("The device is not reachable, so nothing was started (an update needs an online device).")
        case .conflict: return tr("Another operation is running or the device changed; the page shows the latest state.")
        case .forbidden: return tr("Outside your assignment or work window, or your role has no device permission.")
        case .validation: return error.fieldErrors.isEmpty ? ErrorText.of(error) : nil
        default: return ErrorText.of(error)
        }
    }

    /// A field error of the Core API in words.
    public static func field(_ key: String) -> String {
        switch key {
        case "error.required": tr("Required")
        case "error.length": tr("1–1000 characters")
        case "error.invalid": tr("Not valid")
        case "error.duplicate": tr("Already registered")
        default:
            key.replacingOccurrences(of: "errors.", with: "").replacingOccurrences(of: "error.", with: "").replacingOccurrences(
                of: "_",
                with: " "
            )
        }
    }
}
