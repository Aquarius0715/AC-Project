import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// The answers the device screens read, as the tests write them.
@MainActor private enum DeviceAnswers {
    static func answer(_ result: some Encodable, at: String? = nil) throws -> StubTransport.Answer {
        var meta = ["correlationId": "c"]
        meta["snapshotAt"] = at
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: meta))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    static func page<Item: Codable & Sendable & Equatable>(_ items: [Item]) throws -> StubTransport.Answer {
        try answer(Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1))
    }

    static func text(_ name: String) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try fixture(name), encoding: .utf8)))
    }

    static func refusal(_ status: Int, _ code: String, _ key: String, fields: String = "{}") -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":\#(fields),"correlationId":"c","retryAfterSeconds":null}"#)
    }

    static func session(_ permissions: [Permission] = [.deviceMaintain, .alertRead]) throws -> StubTransport.Answer {
        let session = Session(
            tenantId: "t1",
            membershipId: "m1",
            scopeVersion: 1,
            userId: "u",
            role: .technician,
            clientRole: nil,
            displayName: "Tech",
            organizationName: "HQ",
            organizationId: "o1",
            customerId: nil,
            permissions: permissions,
            generation: 1,
            viewEpoch: 1,
            issuedAt: "2026-09-27T00:00:00Z",
            expiresAt: "2026-09-28T00:00:00Z"
        )
        return try answer(session)
    }

    /// The unit summaries: Living room AC (the device's) and Bedroom AC (without one).
    static func units() throws -> StubTransport.Answer {
        struct Envelope: Decodable { let data: UnitDetail }
        let unit = try JSONDecoder().decode(Envelope.self, from: try fixture("units.get.technician")).data
        let summary = { (id: String, name: String) in
            UnitSummary(
                id: id,
                tenantId: unit.tenantId,
                version: 1,
                createdAt: unit.createdAt,
                updatedAt: unit.updatedAt,
                customerOrgId: unit.customerOrgId,
                propertyId: unit.propertyId,
                spaceId: nil,
                displayName: name,
                modelId: unit.modelId,
                installState: .installed,
                installedAt: nil,
                serviceScope: [],
                warrantyEndsAt: nil,
                alertPolicyIds: [],
                archived: false,
                capabilityVersion: 1,
                iotModelId: nil,
                iotModelVersion: nil,
                connection: .online,
                observedState: unit.observedState,
                observedRestriction: nil,
                lastSeenAt: nil,
                effectivePowerState: .unknown,
                latestMeasurements: [],
                activeAlertCount: 0
            )
        }
        return try page([summary(unit.id, "Living room AC"), summary("bedroom", "Bedroom AC")])
    }

    /// Jobs of the technician: one on each unit.
    static func jobs() throws -> StubTransport.Answer {
        let job = { (id: String, unit: String) -> JobsListResultItem in
            .summary(
                JobSummary(
                    id: id,
                    version: 1,
                    status: .assigned,
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
            )
        }
        return try page([job("j1", "e841ee95-3171-54d5-be91-1f2301bdb701"), job("j2", "bedroom")])
    }

    static func bedroom() throws -> StubTransport.Answer {
        struct Envelope: Decodable { let data: UnitDetail }
        var unit = try JSONDecoder().decode(Envelope.self, from: try fixture("units.get.technician")).data
        unit.id = "bedroom"
        unit.displayName = "Bedroom AC"
        unit.iotModel = IoTModel(
            id: "m-iot",
            tenantId: "t1",
            version: 1,
            createdAt: unit.createdAt,
            updatedAt: unit.updatedAt,
            manufacturer: "DemoIoT",
            model: "S1",
            sensors: [
                IoTModelSensors(boundaryId: nil, metric: .temperature, unit: .c, staleAfterSeconds: 60),
                IoTModelSensors(boundaryId: nil, metric: .humidity, unit: .value1, staleAfterSeconds: 60),
            ],
            firmwareCandidates: ["v1", "v2"]
        )
        return try answer(unit)
    }
}

@MainActor @Suite struct DevicesModelTests {
    private func model(_ answers: [StubTransport.Answer]) -> (DevicesModel, StubTransport) {
        let transport = StubTransport(answers)
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        return (DevicesModel(client: client), transport)
    }

    private func loaded() async throws -> (DevicesModel, StubTransport) {
        let (model, transport) = model([
            try DeviceAnswers.text("devices.list"), try DeviceAnswers.units(), try DeviceAnswers.jobs(), try DeviceAnswers.session(),
            try DeviceAnswers.bedroom(),
        ])
        await model.load()
        return (model, transport)
    }

    @Test func theDevicesAndTheUnitsANewOneCanGoOn() async throws {
        let (model, _) = try await loaded()
        #expect(model.rows.map(\.title) == ["AC-DEMO-0002"] && model.rows[0].sub.hasPrefix("Living room AC · Online"))
        #expect(model.canMaintain && model.candidates.map(\.id) == ["bedroom"] && model.unitId == "bedroom")
        #expect(model.candidates[0].label.hasPrefix("Bedroom AC · ") && model.candidates[0].sensors == [.temperature, .humidity])
        model.filter = .tamper
        #expect(model.rows.isEmpty)
    }

    @Test func aDeviceIsRegisteredOnTheUnitUnderItsJob() async throws {
        var registered = try { () throws -> Device in
            struct Envelope: Decodable { let data: Page<Device> }
            return try JSONDecoder().decode(Envelope.self, from: try fixture("devices.list")).data.items[0]
        }()
        registered.serial = "AC-NEW-1"
        let (model, transport) = try await loaded()
        await transport.queue([try DeviceAnswers.answer(registered)])
        await transport.queue([
            try DeviceAnswers.text("devices.list"), try DeviceAnswers.units(), try DeviceAnswers.jobs(), try DeviceAnswers.session(),
            try DeviceAnswers.bedroom(),
        ])
        #expect(await model.register() == false)
        #expect(model.fieldErrors == ["serial": "Required"])
        model.serial = " ac-new-1 "
        model.sensorTypes = [.humidity, .co2]
        #expect(await model.register())
        #expect(model.note == "AC-NEW-1 · Device registered — bind it to the unit once it is installed" && model.serial.isEmpty)
        let body = try JSONDecoder().decode(StubRegistration.self, from: try #require(await transport.requests[5].httpBody))
        #expect(body.serial == "ac-new-1" && body.unitId == "bedroom" && body.jobId == "j2" && body.sensorTypes == ["humidity"])
    }

    @Test func aRefusedRegistrationSaysWhy() async throws {
        let (model, transport) = try await loaded()
        await transport.queue([DeviceAnswers.refusal(409, "CONFLICT", "errors.duplicate_serial", fields: #"{"serial":"error.duplicate"}"#)])
        model.serial = "AC-DEMO-0002"
        #expect(await model.register() == false)
        #expect(model.refusal == "This serial is already registered." && model.fieldErrors == ["serial": "Already registered"])
    }

    @Test func aPlannedDeviceHoldsItsUnitAndAModelWithoutSensorsOffersNone() async throws {
        struct Envelope: Decodable { let data: Page<Device> }
        var planned = try JSONDecoder().decode(Envelope.self, from: try fixture("devices.list")).data.items[0]
        planned.unitId = nil
        planned.targetUnitId = "bedroom"
        var bound = planned
        bound.id = "other"
        bound.serial = "AC-0001"
        bound.targetUnitId = nil
        bound.unitId = "e841ee95-3171-54d5-be91-1f2301bdb701"
        let (model, _) = model([
            try DeviceAnswers.page([planned, bound]), try DeviceAnswers.units(), try DeviceAnswers.jobs(), try DeviceAnswers.session(),
        ])
        await model.load()
        #expect(model.candidates.isEmpty, "the planned device already holds Bedroom AC")
        #expect(model.rows.map(\.title) == ["AC-0001", "AC-DEMO-0002"])
        struct UnitEnvelope: Decodable { let data: UnitDetail }
        var bare = try JSONDecoder().decode(UnitEnvelope.self, from: try fixture("units.get.technician")).data
        bare.id = "bedroom"
        bare.iotModel = nil
        bare.location.pathLabels = []
        let (other, _) = self.model([
            try DeviceAnswers.page([Device]()), try DeviceAnswers.units(), try DeviceAnswers.jobs(), try DeviceAnswers.session(),
            try DeviceAnswers.answer(bare), try DeviceAnswers.answer(bare),
        ])
        await other.load()
        #expect(other.candidates.map(\.sensors) == [[], []] && other.candidates.last?.label == "Living room AC")
    }

    @Test func withoutACandidateOrWithUnreadablePartsNothingIsOffered() async throws {
        let (model, _) = model([
            try DeviceAnswers.page([Device]()), .failure(URLError(.timedOut)), .failure(URLError(.timedOut)), .failure(URLError(.timedOut)),
        ])
        await model.load()
        #expect(model.candidates.isEmpty && !model.canMaintain && model.unitId.isEmpty)
        #expect(await model.register() == false)
        let (down, _) = self.model([.failure(URLError(.timedOut))])
        await down.load()
        #expect(down.devices.message != nil && down.rows.isEmpty)
    }
}

@MainActor @Suite struct DeviceModelTests {
    private let deviceId = "88ee2ed2-79db-53b6-be15-a92f7829c53d"

    private func model(_ answers: [StubTransport.Answer]) -> (DeviceModel, StubTransport) {
        let transport = StubTransport(answers)
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        return (DeviceModel(deviceId: deviceId, client: client), transport)
    }

    private func device(tamper: DeviceTamper = .clear) throws -> DeviceDetail {
        struct Envelope: Decodable { let data: Page<Device> }
        let device = try JSONDecoder().decode(Envelope.self, from: try fixture("devices.list")).data.items[0]
        return DeviceDetail(
            id: device.id,
            tenantId: device.tenantId,
            version: 3,
            createdAt: device.createdAt,
            updatedAt: device.updatedAt,
            bindingId: device.bindingId,
            unitId: device.unitId,
            targetUnitId: nil,
            createdByMembershipId: device.createdByMembershipId,
            serial: device.serial,
            mqttNode: nil,
            connection: device.connection,
            lastSeenAt: device.lastSeenAt,
            firmwareVersion: device.firmwareVersion,
            powerSignal: device.powerSignal,
            tamper: tamper,
            iotModelId: "m-iot",
            iotModelVersion: 1,
            sensors: device.sensors,
            calibrationRefs: [],
            activeOperation: nil
        )
    }

    private func unit() throws -> StubTransport.Answer {
        struct Envelope: Decodable { let data: UnitDetail }
        var unit = try JSONDecoder().decode(Envelope.self, from: try fixture("units.get.technician")).data
        unit.iotModel = IoTModel(
            id: "m-iot",
            tenantId: "t1",
            version: 1,
            createdAt: unit.createdAt,
            updatedAt: unit.updatedAt,
            manufacturer: "DemoIoT",
            model: "S1",
            sensors: [],
            firmwareCandidates: ["v1", "v2"]
        )
        return try DeviceAnswers.answer(unit)
    }

    private func tamperEvent() throws -> StubTransport.Answer {
        try DeviceAnswers.page([try tamper()])
    }

    private func tamper() throws -> DeviceEvent {
        let at = "2026-09-27T04:00:00Z"
        let event = DeviceEvent(
            id: "de1",
            tenantId: "t1",
            version: 2,
            createdAt: at,
            updatedAt: at,
            bindingId: nil,
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            alertIds: ["al1"],
            deviceId: deviceId,
            eventType: .tamper,
            recovery: nil,
            evidenceSource: .tamperSignal,
            sequence: 1,
            occurredAt: at,
            restoredAt: nil,
            responseNotes: []
        )
        return event
    }

    /// One read: the device, the session, the jobs, the units, the unit, its operations, calibrations, events and alert.
    private func read(_ permissions: [Permission] = [.deviceMaintain, .alertRead]) throws -> [StubTransport.Answer] {
        [
            try DeviceAnswers.answer(try device(tamper: .detected), at: "2026-09-27T05:00:00Z"), try DeviceAnswers.session(permissions),
            try DeviceAnswers.jobs(),
            try DeviceAnswers.units(), try unit(), try DeviceAnswers.page([DeviceOperation]()),
            try DeviceAnswers.page([CalibrationRecord]()), try tamperEvent(),
            try DeviceAnswers.answer(alert("al1")),
        ]
    }

    private func loaded(then answers: [StubTransport.Answer] = []) async throws -> (DeviceModel, StubTransport) {
        let (model, transport) = model(try read() + answers)
        await model.load()
        return (model, transport)
    }

    @Test func theDeviceAndEverythingAboutItAreRead() async throws {
        let (model, transport) = try await loaded()
        #expect(model.canWork && model.canAcknowledge && model.jobId == "j1" && model.home == "e841ee95-3171-54d5-be91-1f2301bdb701")
        #expect(model.newerFirmware == ["v2"] && model.tiles.count == 4 && model.tiles[3].1 == "v1 · v2 available")
        #expect(model.sensorRows.count == 3 && model.sensorId == model.device.value?.sensors.first?.id)
        #expect(model.eventRows.map(\.id) == ["de1"] && model.openAlerts.map(\.id) == ["al1"])
        #expect(model.rebindTargets.map(\.id) == ["bedroom"] && model.bindUnitId == "bedroom")
        #expect(model.firmwareCard == nil && model.operationRows.isEmpty && model.calibrationRows.isEmpty)
        let paths = await transport.requests.compactMap { $0.url?.path() }
        #expect(paths.contains("/v1/devices/\(deviceId)/operations") && paths.last == "/v1/alerts/al1")
    }

    @Test func withoutDeviceMaintainItIsReadOnly() async throws {
        let (model, transport) = model(Array(try read([]).prefix(5)) + [try tamperEvent(), try DeviceAnswers.answer(alert("al1"))])
        await model.load()
        #expect(!model.canWork && !model.canAcknowledge)
        await model.check()
        #expect(await transport.requests.count == 7, "nothing sent")
    }

    @Test func eachStepSendsTheVersionItChanges() async throws {
        let (model, transport) = try await loaded()
        let done = DeviceOperation(
            id: "op1",
            tenantId: "t1",
            version: 1,
            createdAt: "2026-09-27T05:00:00Z",
            updatedAt: "2026-09-27T05:00:00Z",
            bindingId: nil,
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            deviceId: deviceId,
            kind: .check,
            status: .queued,
            targetVersion: nil,
            failureCode: nil,
            startedAt: nil,
            finishedAt: nil,
            expiresAt: "2026-09-27T05:01:00Z"
        )
        await transport.queue([try DeviceAnswers.answer(done)] + (try read()))
        await model.check()
        #expect(model.note == "Connection check requested")
        var requests = await transport.requests
        #expect(
            requests[9].url?.path() == "/v1/devices/\(deviceId)/check" && requests[9].value(forHTTPHeaderField: "X-Expected-Version") == "3"
        )

        await transport.queue([try DeviceAnswers.answer(done)] + (try read()))
        await model.update(to: "v2")
        #expect(model.note == "Firmware update to v2 requested")

        await model.calibrate()
        #expect(model.fieldErrors == ["referenceValue": "Not valid", "measuredValue": "Not valid"])
        model.reference = " 24.0 "
        model.measured = "24.4"
        await transport.queue([.http(200, "{}")] + (try read()))
        await model.calibrate()
        #expect(model.refusal != nil && model.reference == " 24.0 ", "a refused calibration keeps its values")

        await model.bind()
        #expect(model.fieldErrors == ["reason": "1–1000 characters"])
        model.bindReason = "Moved to the bedroom"
        await transport.queue(
            [DeviceAnswers.refusal(409, "CONFLICT", "error.versionConflict", fields: #"{"reason":"error.length"}"#)] + (try read())
        )
        await model.bind()
        #expect(model.refusal?.hasPrefix("Another operation is running") == true && model.bindReason == "Moved to the bedroom")
        #expect(model.fieldErrors == ["reason": "1–1000 characters"])
        requests = await transport.requests
        let bind = try JSONDecoder().decode(StubBind.self, from: try #require(requests[requests.count - 10].httpBody))
        #expect(bind.unitId == "bedroom" && bind.jobId == "j2" && bind.reason == "Moved to the bedroom")
    }

    @Test func notesAndAlertsOfTheEvents() async throws {
        let (model, transport) = try await loaded()
        let row = try #require(model.eventRows.first)
        await model.addNote(to: row)
        #expect(model.fieldErrors == ["responseNote": "1–1000 characters"])
        model.noteText = " Cover sealed "
        await transport.queue([try DeviceAnswers.answer(try tamper())] + (try read()))
        await model.addNote(to: row)
        #expect(model.note == "Response note added — the device state is unchanged" && model.noteText.isEmpty)
        var requests = await transport.requests
        #expect(requests[9].value(forHTTPHeaderField: "X-Expected-Version") == "2")
        #expect(try JSONDecoder().decode(StubNoteBody.self, from: try #require(requests[9].httpBody)).responseNote == "Cover sealed")
        await transport.queue([try DeviceAnswers.answer(alert("al1", status: .acknowledged))] + (try read()))
        await model.acknowledge(alert("al1"))
        #expect(model.note == "Acknowledged")
        requests = await transport.requests
        #expect(requests[19].url?.path() == "/v1/alerts/al1/acknowledge")
    }

    @Test func calibrationAndBindingAreSentWhenRight() async throws {
        let (model, transport) = try await loaded()
        model.reference = "24"
        model.measured = "24.4"
        let record = CalibrationRecord(
            id: "c1",
            tenantId: "t1",
            version: 1,
            createdAt: "2026-09-27T05:00:00Z",
            updatedAt: "2026-09-27T05:00:00Z",
            bindingId: nil,
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            deviceId: deviceId,
            sensorId: model.sensorId,
            metric: .temperature,
            unit: .c,
            referenceValue: 24,
            measuredValue: 24.4,
            calibratedAt: "2026-09-27T05:00:00Z",
            actorId: "m1"
        )
        await transport.queue([try DeviceAnswers.answer(record)] + (try read()))
        await model.calibrate()
        #expect(model.note == "Calibration recorded — earlier readings unchanged" && model.reference.isEmpty)
        let body = try JSONDecoder().decode(StubCalibrationBody.self, from: try #require(await transport.requests[9].httpBody))
        #expect(body.referenceValue == 24 && body.measuredValue == 24.4 && body.calibratedAt == "2026-09-27T05:00:00Z")
        model.bindReason = "Moved"
        let device = try device()
        await transport.queue(
            [
                try DeviceAnswers.answer(
                    Device(
                        id: device.id,
                        tenantId: "t1",
                        version: 4,
                        createdAt: device.createdAt,
                        updatedAt: device.updatedAt,
                        bindingId: nil,
                        unitId: "bedroom",
                        targetUnitId: nil,
                        createdByMembershipId: "m1",
                        serial: device.serial,
                        mqttNode: nil,
                        connection: .online,
                        lastSeenAt: nil,
                        firmwareVersion: "v1",
                        powerSignal: .on,
                        tamper: .clear,
                        iotModelId: nil,
                        iotModelVersion: nil,
                        sensors: []
                    )
                )
            ] + (try read())
        )
        await model.bind()
        #expect(model.note?.hasPrefix("History stays with the previous binding") == true && model.bindReason.isEmpty)
    }

    @Test func unreadableOperationsAndCalibrationsAndSeveralOpenAlerts() async throws {
        let at = "2026-09-27T04:00:00Z"
        var two = try tamper()
        two.alertIds = ["al1", "al2"]
        let record = CalibrationRecord(
            id: "c1",
            tenantId: "t1",
            version: 1,
            createdAt: at,
            updatedAt: at,
            bindingId: nil,
            unitIdAtOccurrence: "u1",
            customerOrgIdAtOccurrence: "c1",
            deviceId: deviceId,
            sensorId: "s1",
            metric: .temperature,
            unit: .c,
            referenceValue: 24,
            measuredValue: 24,
            calibratedAt: at,
            actorId: "m1"
        )
        var answers = try read()
        answers[5] = .failure(URLError(.timedOut))
        answers[6] = .failure(URLError(.timedOut))
        var (model, _) = self.model(answers)
        await model.load()
        #expect(model.operations.isEmpty && model.calibrations.isEmpty)
        answers = try read()
        answers[6] = try DeviceAnswers.page([record])
        answers[7] = try DeviceAnswers.page([two])
        answers.append(try DeviceAnswers.answer(alert("al2")))
        (model, _) = self.model(answers)
        await model.load()
        #expect(model.calibrationRows.count == 1 && model.openAlerts.map(\.id) == ["al1", "al2"])
        await model.acknowledge(alert("al1"))
    }

    @Test func aChoiceNoLongerThereSendsNothing() async throws {
        let (model, transport) = try await loaded()
        model.sensorId = "gone"
        model.bindUnitId = "gone"
        await model.calibrate()
        await model.bind()
        #expect(await transport.requests.count == 9)
    }

    @Test func anUnreadableDeviceOrUnitLeavesTheirPlacesEmpty() async throws {
        var (model, _) = model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(
            model.device.message != nil && model.tiles.isEmpty && model.sensorRows.isEmpty && model.operationRows.isEmpty
                && model.firmwareCard == nil
        )
        #expect(model.home == nil && model.jobId == nil && model.newerFirmware.isEmpty)
        await model.acknowledge(alert("al1"))
        var unbound = try device()
        unbound.unitId = nil
        unbound.sensors = []
        (model, _) = self.model([
            try DeviceAnswers.answer(unbound), .failure(URLError(.timedOut)), .failure(URLError(.timedOut)), .failure(URLError(.timedOut)),
            .failure(URLError(.timedOut)),
        ])
        await model.load()
        #expect(model.unit == nil && model.home == nil && model.events.isEmpty && model.sensorId.isEmpty && model.rebindTargets.isEmpty)
    }
}

private struct StubRegistration: Decodable {
    let serial: String
    let unitId: String?
    let jobId: String?
    let sensorTypes: [String]?
}

private struct StubBind: Decodable {
    let unitId: String
    let jobId: String?
    let reason: String
}

private struct StubNoteBody: Decodable {
    let responseNote: String
}

private struct StubCalibrationBody: Decodable {
    let referenceValue: Double
    let measuredValue: Double
    let calibratedAt: String
}
