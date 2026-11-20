#if DEBUG
import ACKit
import Foundation

/// The device of the UI tests' AC u1 (AC_UI_TEST=stub): d1, online, a temperature sensor, firmware 1.0 of the model
/// m-iot (2.0 on offer), and one tamper event with an open alert. A connection check, a calibration, a firmware update
/// and a response note answer as the Core API would.
final class StubDevices {
    private var device: DeviceDetail
    private var operations: [DeviceOperation] = []
    private var calibrations: [CalibrationRecord] = []
    private var event: DeviceEvent
    private let at: () -> String = { Date().ISO8601Format() }

    static let model = IoTModel(
        id: "m-iot",
        tenantId: "t1",
        version: 1,
        createdAt: StubUnitData.at(86_400),
        updatedAt: StubUnitData.at(86_400),
        manufacturer: "DemoIoT",
        model: "S1",
        sensors: [IoTModelSensors(boundaryId: nil, metric: .temperature, unit: .c, staleAfterSeconds: 60)],
        firmwareCandidates: ["1.0", "2.0"]
    )

    init() {
        let created = StubUnitData.at(86_400)
        let sensor = Sensor(
            boundaryId: nil,
            id: "s1",
            deviceId: "d1",
            metric: .temperature,
            unit: .c,
            staleAfterSeconds: 60,
            calibratedAt: nil
        )
        device = DeviceDetail(
            id: "d1",
            tenantId: "t1",
            version: 1,
            createdAt: created,
            updatedAt: created,
            bindingId: "b1",
            unitId: "u1",
            targetUnitId: nil,
            createdByMembershipId: "m1",
            serial: "ACD-0001",
            mqttNode: nil,
            connection: .online,
            lastSeenAt: StubUnitData.at(30),
            firmwareVersion: "1.0",
            powerSignal: .on,
            tamper: .detected,
            iotModelId: "m-iot",
            iotModelVersion: 1,
            sensors: [sensor],
            calibrationRefs: [],
            activeOperation: nil
        )
        event = DeviceEvent(
            id: "de1",
            tenantId: "t1",
            version: 1,
            createdAt: StubUnitData.at(600),
            updatedAt: StubUnitData.at(600),
            bindingId: "b1",
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            alertIds: ["al1"],
            deviceId: "d1",
            eventType: .tamper,
            recovery: nil,
            evidenceSource: .tamperSignal,
            sequence: 1,
            occurredAt: StubUnitData.at(600),
            restoredAt: nil,
            responseNotes: []
        )
    }

    var summary: Device {
        Device(
            id: device.id,
            tenantId: "t1",
            version: device.version,
            createdAt: device.createdAt,
            updatedAt: device.updatedAt,
            bindingId: device.bindingId,
            unitId: device.unitId,
            targetUnitId: nil,
            createdByMembershipId: "m1",
            serial: device.serial,
            mqttNode: nil,
            connection: device.connection,
            lastSeenAt: device.lastSeenAt,
            firmwareVersion: device.firmwareVersion,
            powerSignal: device.powerSignal,
            tamper: device.tamper,
            iotModelId: device.iotModelId,
            iotModelVersion: device.iotModelVersion,
            sensors: device.sensors
        )
    }

    func answer(_ method: String, _ path: String, _ body: Data?) throws -> (Int, Data)? {
        let encoder = JSONEncoder()
        switch (method, path) {
        case ("GET", "/v1/devices"): return (200, try encoder.encode(StubDeviceAnswer(data: page([summary]))))
        case ("GET", "/v1/devices/d1"): return (200, try encoder.encode(StubDeviceAnswer(data: device)))
        case ("GET", "/v1/devices/d1/operations"): return (200, try encoder.encode(StubDeviceAnswer(data: page(operations))))
        case ("GET", "/v1/devices/d1/calibrations"): return (200, try encoder.encode(StubDeviceAnswer(data: page(calibrations))))
        case ("GET", "/v1/devices/d1/events"): return (200, try encoder.encode(StubDeviceAnswer(data: page([event]))))
        case ("POST", "/v1/devices/d1/check"):
            return (200, try encoder.encode(StubDeviceAnswer(data: operation(.check, target: nil))))
        case ("POST", "/v1/devices/d1/update-firmware"):
            let input = try JSONDecoder().decode(StubFirmware.self, from: body ?? Data())
            let done = operation(.firmware, target: input.firmwareVersion)
            device.firmwareVersion = input.firmwareVersion
            return (200, try encoder.encode(StubDeviceAnswer(data: done)))
        case ("POST", "/v1/devices/d1/calibrations"):
            let input = try JSONDecoder().decode(StubCalibration.self, from: body ?? Data())
            let record = CalibrationRecord(
                id: "cal\(calibrations.count + 1)",
                tenantId: "t1",
                version: 1,
                createdAt: at(),
                updatedAt: at(),
                bindingId: "b1",
                unitIdAtOccurrence: "u1",
                customerOrgIdAtOccurrence: "c1",
                deviceId: "d1",
                sensorId: input.sensorId,
                metric: .temperature,
                unit: .c,
                referenceValue: input.referenceValue,
                measuredValue: input.measuredValue,
                calibratedAt: at(),
                actorId: "m1"
            )
            calibrations.insert(record, at: 0)
            device.version += 1
            return (200, try encoder.encode(StubDeviceAnswer(data: record)))
        case ("POST", "/v1/devices/d1/events/de1/response-note"):
            let input = try JSONDecoder().decode(StubNote.self, from: body ?? Data())
            event.responseNotes.append(DeviceEventResponseNotes(actorId: "m1", message: input.responseNote, at: at()))
            event.version += 1
            return (200, try encoder.encode(StubDeviceAnswer(data: event)))
        default:
            return nil
        }
    }

    private func page<T: Codable & Sendable & Equatable>(_ items: [T]) -> Page<T> {
        Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1)
    }

    /// An operation the device finished at once.
    private func operation(_ kind: DeviceOperationKind, target: String?) -> DeviceOperation {
        let done = DeviceOperation(
            id: "op\(operations.count + 1)",
            tenantId: "t1",
            version: 1,
            createdAt: at(),
            updatedAt: at(),
            bindingId: "b1",
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            deviceId: "d1",
            kind: kind,
            status: .succeeded,
            targetVersion: target,
            failureCode: nil,
            startedAt: at(),
            finishedAt: at(),
            expiresAt: at()
        )
        operations.insert(done, at: 0)
        device.version += 1
        return done
    }
}

private struct StubFirmware: Decodable {
    let firmwareVersion: String
}

private struct StubCalibration: Decodable {
    let sensorId: String
    let referenceValue: Double
    let measuredValue: Double
}

private struct StubNote: Decodable {
    let responseNote: String
}

private struct StubDeviceAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
