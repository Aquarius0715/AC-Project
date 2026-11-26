import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// The live device of the dev stack (AC-DEMO-0002 on Living room AC, three sensors, firmware v1).
private func liveDevice() throws -> Device {
    struct Envelope: Decodable { let data: Page<Device> }
    return try JSONDecoder().decode(Envelope.self, from: try fixture("devices.list")).data.items[0]
}

private func detail(_ device: Device, active: DeviceOperation? = nil) -> DeviceDetail {
    DeviceDetail(
        id: device.id,
        tenantId: device.tenantId,
        version: device.version,
        createdAt: device.createdAt,
        updatedAt: device.updatedAt,
        bindingId: device.bindingId,
        unitId: device.unitId,
        targetUnitId: device.targetUnitId,
        createdByMembershipId: device.createdByMembershipId,
        serial: device.serial,
        mqttNode: device.mqttNode,
        connection: device.connection,
        lastSeenAt: device.lastSeenAt,
        firmwareVersion: device.firmwareVersion,
        powerSignal: device.powerSignal,
        tamper: device.tamper,
        iotModelId: device.iotModelId,
        iotModelVersion: device.iotModelVersion,
        sensors: device.sensors,
        calibrationRefs: [],
        activeOperation: active
    )
}

private func operation(
    _ kind: DeviceOperationKind,
    _ status: DeviceOperationStatus,
    target: String? = nil,
    failure: ErrorCode? = nil
) -> DeviceOperation {
    DeviceOperation(
        id: "op-\(kind.rawValue)-\(status.rawValue)",
        tenantId: "t1",
        version: 1,
        createdAt: "2026-09-27T04:00:00Z",
        updatedAt: "2026-09-27T04:00:00Z",
        bindingId: nil,
        unitIdAtOccurrence: "u1",
        customerOrgIdAtOccurrence: "c1",
        deviceId: "d1",
        kind: kind,
        status: status,
        targetVersion: target,
        failureCode: failure,
        startedAt: nil,
        finishedAt: status == .succeeded ? "2026-09-27T04:01:00Z" : nil,
        expiresAt: "2026-09-27T04:01:00Z"
    )
}

private func event(
    _ id: String,
    _ type: EvidenceCandidateEventType,
    at: String = "2026-09-27T04:00:00Z",
    restored: String? = nil,
    alerts: [String] = [],
    recovery: DeviceRecovery? = nil
) -> DeviceEvent {
    DeviceEvent(
        id: id,
        tenantId: "t1",
        version: 1,
        createdAt: at,
        updatedAt: at,
        bindingId: nil,
        unitIdAtOccurrence: "u1",
        customerOrgIdAtOccurrence: "c1",
        alertIds: alerts,
        deviceId: "d1",
        eventType: type,
        recovery: recovery,
        evidenceSource: .heartbeat,
        sequence: 1,
        occurredAt: at,
        restoredAt: restored,
        responseNotes: []
    )
}

private func job(_ id: String, unit: String, status: JobStatus) -> JobSummary {
    JobSummary(
        id: id,
        version: 1,
        status: status,
        createdAt: "2026-09-27T00:00:00Z",
        dueAt: "2026-09-28T00:00:00Z",
        type: .reactive,
        origin: .clientRequest,
        unitId: unit,
        requestedSlot: Slot(startAt: "2026-09-27T01:00:00Z", endAt: "2026-09-27T04:00:00Z"),
        scheduledSlot: nil,
        assignmentId: nil,
        preferredSlots: [],
        followUpOfJobId: nil,
        followUpClass: nil,
        severity: .normal,
        displayStatus: .assigned,
        assignmentAcknowledgement: .accepted,
        technicianMembershipId: "m1",
        accessValidFrom: nil,
        accessValidUntil: nil,
        eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
    )
}

@Suite struct DeviceWordsTests {
    private let now = Format.parse("2026-09-27T05:00:00Z") ?? Date()

    @Test func theJobsADeviceWriteRunsUnder() {
        let jobs = [
            job("a", unit: "u1", status: .assigned), job("p", unit: "u1", status: .inProgress), job("c", unit: "u1", status: .completed),
            job("o", unit: "u2", status: .inProgress),
        ]
        #expect(DeviceWords.jobs(for: "u1", jobs).map(\.id) == ["p", "a"])
        #expect(DeviceWords.jobs(for: nil, jobs).isEmpty)
    }

    @Test func howLongAgo() {
        let ago = { (seconds: TimeInterval) in DeviceWords.ago(now.addingTimeInterval(-seconds).ISO8601Format(), now: now) }
        #expect([ago(30), ago(600), ago(10_800), ago(345_600)] == ["30 s ago", "10 min ago", "3 h ago", "4 d ago"])
        #expect(DeviceWords.ago(nil, now: now) == "never seen")
    }

    @Test func aRowOfTheList() throws {
        var device = try liveDevice()
        let names = ["e841ee95-3171-54d5-be91-1f2301bdb701": "Living room AC"]
        #expect(DeviceWords.row(device, names: names, now: now).sub == "Living room AC · Online · v1 · Sensors (3) · from the IoT model")
        device.connection = .offline
        device.tamper = .detected
        device.sensors = [device.sensors[0]]
        #expect(DeviceWords.row(device, names: [:], now: now).sub.hasPrefix("unit · Offline · "))
        #expect(DeviceWords.row(device, names: [:], now: now).sub.hasSuffix("Sensors (1) · from the IoT model · Tamper"))
        device.unitId = nil
        device.targetUnitId = "t"
        #expect(DeviceWords.row(device, names: ["t": "Bedroom AC"], now: now).sub.hasPrefix("Not bound yet — planned for Bedroom AC"))
        #expect(DeviceWords.row(device, names: [:], now: now).sub.hasPrefix("Not bound yet — planned for unit"))
        device.targetUnitId = nil
        device.lastSeenAt = nil
        #expect(DeviceWords.row(device, names: [:], now: now).sub.hasPrefix("Unbound · Offline · v1"))
        #expect(DeviceFilter.allCases.map(\.label) == ["All", "Online", "Offline", "Tamper"])
        #expect(DeviceFilter.allCases.map { $0.keeps(device) } == [true, false, true, true])
    }

    @Test func theOpenFaultOfEachAxis() {
        let faults = DeviceWords.openFaults([
            event("e1", .communicationLost, at: "2026-09-27T04:00:00Z"),
            event("e2", .communicationLost, at: "2026-09-27T03:00:00Z"),
            event("e3", .powerLost, at: "2026-09-27T04:10:00Z", restored: "2026-09-27T04:20:00Z"),
            event("e4", .tamper, at: "2026-09-27T04:30:00Z"),
            event("e5", .restored, at: "2026-09-27T04:40:00Z"),
            event("e6", .operationFailed, at: "2026-09-27T04:41:00Z"),
        ])
        #expect(faults == ["connection": "2026-09-27T04:00:00Z", "tamper": "2026-09-27T04:30:00Z"])
    }

    @Test func newerFirmwareOnly() {
        #expect(DeviceWords.newer("v2", ["v1", "v10", "v3", "v3"]) == ["v3", "v10"])
        #expect(DeviceWords.newer("v10", ["v1"]).isEmpty)
    }

    @Test func theTiles() throws {
        var device = detail(try liveDevice())
        var tiles = DeviceWords.tiles(device, newest: "v2", faults: [:], now: now).map(\.1)
        #expect(tiles[0].hasPrefix("Online · last seen ") && tiles[1] == "ON · from the device's dedicated power signal")
        #expect(tiles[2] == "Clear · no removal detected" && tiles[3] == "v1 · v2 available")
        device.connection = .offline
        device.powerSignal = .off
        device.tamper = .detected
        let faults = ["connection": "2026-09-27T04:00:00Z", "power": "2026-09-27T04:10:00Z", "tamper": "2026-09-27T04:20:00Z"]
        tiles = DeviceWords.tiles(device, newest: nil, faults: faults, now: now).map(\.1)
        #expect(
            tiles[0].hasPrefix("Offline · lost ") && tiles[1].hasPrefix("Lost · power_signal ")
                && tiles[2].hasPrefix("Detected · cover opened ")
        )
        #expect(tiles[3] == "v1 · latest")
        device.connection = .connecting
        device.powerSignal = .unknown
        tiles = DeviceWords.tiles(device, newest: nil, faults: [:], now: now).map(\.1)
        #expect(tiles[0] == "Connecting · check running — waiting for the device" && tiles[1].hasPrefix("Unknown · "))
        #expect(tiles[2] == "Detected · cover opened — see device events")
        device.powerSignal = .off
        #expect(DeviceWords.tiles(device, newest: nil, faults: [:], now: now)[1].1 == "Lost · from the device's dedicated power signal")
    }

    @Test func theFirmwareCard() throws {
        let device = detail(try liveDevice())
        #expect(DeviceWords.firmwareCard(device, []) == nil)
        #expect(DeviceWords.firmwareCard(device, [operation(.check, .succeeded)]) == nil)
        let running = DeviceWords.firmwareCard(detail(try liveDevice(), active: operation(.firmware, .running, target: "v2")), [])
        #expect(
            running?.title == "Firmware update v1 → v2 · Running"
                && running?.text.contains("fails if the device does not confirm by ") == true
        )
        let queued = DeviceWords.firmwareCard(device, [operation(.firmware, .queued)])
        #expect(queued?.title == "Firmware update v1 → — · Queued")
        let failed = DeviceWords.firmwareCard(device, [operation(.firmware, .failed, target: "v2", failure: .offline)])
        #expect(
            failed?.title == "Firmware update to v2 failed"
                && failed?.text == "the device was not online at the start · the old version v1 is kept"
        )
        let done = DeviceWords.firmwareCard(device, [operation(.firmware, .succeeded, target: "v2")])
        #expect(done?.title == "Firmware v2 installed" && done?.text.hasPrefix("finished ") == true)
        var unfinished = operation(.firmware, .succeeded, target: "v2")
        unfinished.finishedAt = nil
        #expect(DeviceWords.firmwareCard(device, [unfinished])?.text == "finished — · history appended")
        let other = detail(try liveDevice(), active: operation(.check, .running))
        #expect(DeviceWords.firmwareCard(other, []) == nil)
    }

    @Test func eachOperationInWords() {
        let rows = DeviceWords.operations(
            [
                operation(.firmware, .running, target: "v2"), operation(.firmware, .succeeded, target: "v2"), operation(.check, .succeeded),
                operation(.check, .running), operation(.check, .queued), operation(.calibrate, .failed, failure: .timeout),
            ],
            current: "v1"
        )
        #expect(
            rows.map(\.title) == [
                "Firmware update · Running", "Firmware update · Succeeded", "Connection check · Succeeded", "Connection check · Running",
                "Connection check · Queued", "Calibration · Failed",
            ]
        )
        #expect(
            rows.map { $0.sub.components(separatedBy: " · ").last ?? "" } == [
                "v1 → v2", "v2 installed", "heartbeat confirmed", "waiting for the heartbeat", "starts within a second",
                "no confirmation within 60 s",
            ]
        )
        let failures: [ErrorCode?] = [.conflict, .unavailable, .forbidden, nil]
        #expect(
            failures.map(DeviceWords.failure) == [
                "a command or another operation was running", "the device reported a failure", "FORBIDDEN", "no result",
            ]
        )
        var noTarget = operation(.firmware, .running)
        noTarget.startedAt = "2026-09-27T04:00:05Z"
        #expect(DeviceWords.operations([noTarget], current: "v1")[0].sub.hasSuffix("v1 → —"))
        var doneNoTarget = operation(.firmware, .succeeded)
        doneNoTarget.targetVersion = nil
        #expect(DeviceWords.operations([doneNoTarget], current: "v1")[0].sub.hasSuffix("— installed"))
    }

    @Test func calibrationsAndSensors() throws {
        let record = CalibrationRecord(
            id: "c1",
            tenantId: "t1",
            version: 1,
            createdAt: "2026-09-27T04:00:00Z",
            updatedAt: "2026-09-27T04:00:00Z",
            bindingId: nil,
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            deviceId: "d1",
            sensorId: "s1",
            metric: .temperature,
            unit: .c,
            referenceValue: 24,
            measuredValue: 24.4,
            calibratedAt: "2026-09-27T04:00:00Z",
            actorId: "m1"
        )
        #expect(DeviceWords.calibration(record).hasPrefix("Temperature · ref 24.0 °C / measured 24.4 °C · offset −0.4 °C · "))
        #expect(DeviceWords.signed(0.4) == "0.4")
        var device = detail(try liveDevice())
        #expect(DeviceWords.sensors(device).count == 3 && DeviceWords.sensors(device)[0].1.hasSuffix("Never"))
        device.sensors[0].calibratedAt = "2026-09-27T04:00:00Z"
        #expect(DeviceWords.sensors(device)[0].1.hasSuffix(Format.dateTime("2026-09-27T04:00:00Z")))
    }

    @Test func theEventsWithTheirAlerts() {
        let open = alert("a1")
        let acknowledged = alert("a2", status: .acknowledged)
        let resolved = alert("a3", status: .resolved)
        let source = event("lost", .communicationLost, at: "2026-09-27T03:00:00Z", alerts: ["a1"])
        let rows = DeviceWords.events(
            [
                source, event("back", .restored, recovery: .connection(DeviceRecoveryConnection(sourceEventId: "lost"))),
                event("p", .restored, recovery: .power(DeviceRecoveryPower(sourceEventId: "gone"))),
                event("t", .restored, recovery: .tamper(DeviceRecoveryTamper(sourceEventId: "gone"))),
                event("r", .powerLost, restored: "2026-09-27T04:30:00Z"), event("n", .tamper), event("ack", .tamper, alerts: ["a2"]),
                event("res", .tamper, alerts: ["a3", "a4"]), event("f", .operationFailed), event("plain", .restored),
            ],
            alerts: ["a1": open, "a2": acknowledged, "a3": resolved],
            now: now
        )
        #expect(rows.map(\.state)[0] == "open · unacknowledged" && rows[0].detail.hasSuffix(" · heartbeat · 1 alert"))
        #expect(rows[1].state.hasPrefix("recovers Communication lost ") && rows[1].detail.hasPrefix("Connection restored"))
        #expect(rows[2].state == "recovers an earlier fault" && rows[2].detail.hasPrefix("Power signal restored"))
        #expect(rows[3].detail.hasPrefix("Tamper cleared — cover closed"))
        #expect(rows[4].state.hasPrefix("restored ") && rows[5].state == "open" && rows[6].state == "open · acknowledged")
        #expect(rows[7].state == "open · resolved" && rows[7].detail.hasSuffix("2 alerts"))
        #expect(
            rows[8].title.hasPrefix("Operation failed · ") && rows[9].state == "recovers an earlier fault"
                && rows[9].detail.hasPrefix("Connection restored")
        )
        var noted = source
        noted.responseNotes = [DeviceEventResponseNotes(actorId: "m1", message: "Checked", at: "2026-09-27T04:00:00Z")]
        #expect(DeviceWords.events([noted], alerts: [:], now: now)[0].notes.first?.hasSuffix(" — Checked") == true)
        #expect(
            [DeviceWords.label(.tamper), DeviceWords.cause(event("x", .tamper)), DeviceWords.cause(event("x", .powerLost))] == [
                "Tamper", "Cover opened while powered", "Dedicated demo power signal lost",
            ]
        )
    }

    @Test func refusalsAndFields() {
        let refused = { (code: ErrorCode, key: String, fields: [String: String]) in
            DeviceWords.refusal(DomainError(code: code, messageKey: key, fieldErrors: fields, correlationId: "c", retryAfterSeconds: nil))
        }
        #expect(refused(.conflict, "errors.duplicate_serial", [:]) == "This serial is already registered.")
        #expect(refused(.conflict, "error.conflict", ["serial": "error.duplicate"]) == "This serial is already registered.")
        #expect(refused(.offline, "error.offline", [:])?.hasPrefix("The device is not reachable") == true)
        #expect(refused(.conflict, "error.versionConflict", [:])?.hasPrefix("Another operation is running") == true)
        #expect(refused(.forbidden, "error.forbidden", [:])?.hasPrefix("Outside your assignment") == true)
        #expect(refused(.validation, "error.validation", ["serial": "error.required"]) == nil)
        #expect(refused(.validation, "error.validation", [:]) != nil && refused(.unavailable, "error.unavailable", [:]) != nil)
        let keys = ["error.required", "error.length", "error.invalid", "error.duplicate", "errors.serial_format"]
        #expect(keys.map(DeviceWords.field) == ["Required", "1–1000 characters", "Not valid", "Already registered", "serial format"])
    }
}
