import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

private func command(
    _ id: String,
    _ action: CommandAction = .setMode(UnitActionSetMode(mode: .cool)),
    status: CommandStatus = .acknowledged,
    failure: CommandFailureCode? = nil
) -> Command {
    Command(
        id: id,
        tenantId: "t1",
        version: 1,
        createdAt: "2026-09-27T04:00:00Z",
        updatedAt: "2026-09-27T04:00:00Z",
        unitId: "u1",
        actorMembershipId: "m1",
        action: action,
        diagnosticRunId: nil,
        jobId: "j1",
        reason: "Check",
        status: status,
        delivery: .sent,
        requestedAt: "2026-09-27T04:00:00Z",
        sentAt: nil,
        acknowledgedAt: status == .acknowledged ? "2026-09-27T04:00:05Z" : nil,
        expiresAt: "2026-09-27T04:00:30Z",
        failureCode: failure,
        correlationId: id,
        source: .diagnostic
    )
}

private func run(_ state: DiagnosticRunState, failure: CommandFailureCode? = nil) -> DiagnosticRun {
    DiagnosticRun(
        id: "r1",
        tenantId: "t1",
        version: 1,
        createdAt: "2026-09-27T04:00:00Z",
        updatedAt: "2026-09-27T04:00:00Z",
        jobId: "j1",
        unitId: "u1",
        actorMembershipId: "m1",
        startAction: .setPower(UnitActionSetPower(power: true)),
        endAction: .setMode(UnitActionSetMode(mode: .fan)),
        durationMinutes: 5,
        reason: "Cool down",
        state: state,
        startCommandId: "start",
        endCommandId: "end",
        startedAt: "2026-09-27T04:00:05Z",
        endAt: "2026-09-27T04:05:05Z",
        failureCode: failure
    )
}

@Suite struct ControlWordsTests {
    @Test func theOptionsOfACapability() throws {
        var unit = try technicianUnit()
        unit.capabilities.temperature = CapabilityTemperature(min: 24, max: 25, step: 0.5)
        unit.capabilities.modes = [.cool]
        unit.capabilities.fanLevels = [.low]
        unit.capabilities.ventilation = true
        unit.capabilities.ventilationLevels = [.high]
        let options = ControlWords.options(unit.capabilities)
        #expect(options.map(\.id) == ["power:on", "power:off", "temp:24", "temp:24.5", "temp:25", "mode:cool", "fan:low", "vent:high"])
        unit.capabilities.temperature = CapabilityTemperature(min: 24, max: 25, step: 0)
        unit.capabilities.modeControl = false
        unit.capabilities.fanControl = false
        #expect(ControlWords.options(unit.capabilities).map(\.id) == ["power:on", "power:off", "temp:24", "temp:25", "vent:high"])
        unit.capabilities.control = false
        unit.capabilities.ventilation = false
        #expect(ControlWords.options(unit.capabilities).isEmpty, "a model that only measures")
    }

    @Test func commandsAsTheDeviceGetsThem() {
        let codes = [
            CommandAction.setPower(UnitActionSetPower(power: true)), .setPower(UnitActionSetPower(power: false)),
            .setTemperature(UnitActionSetTemperature(celsius: 24)), .setMode(UnitActionSetMode(mode: .dry)),
            .setFan(UnitActionSetFan(fanLevel: .high)),
            .ventilate(UnitActionVentilate(level: .low)),
            .applyRestriction(
                CommandActionApplyRestriction(restrictionId: "x", rulesVersion: "1", policy: .powerOff(RestrictionPolicyPowerOff()))
            ),
            .applyRestriction(
                CommandActionApplyRestriction(
                    restrictionId: "x",
                    rulesVersion: "1",
                    policy: .temperatureLimit(RestrictionPolicyTemperatureLimit(minimumCoolingSetpoint: 26))
                )
            ),
            .removeRestriction(CommandActionRemoveRestriction(restrictionId: "x", rulesVersion: "1")),
        ].map(ControlWords.code)
        #expect(
            codes == [
                "set_power = ON", "set_power = OFF", "set_temperature = 24 °C", "set_mode = dry", "set_fan = high", "ventilate = low",
                "apply_restriction = power_off", "apply_restriction = min 26 °C", "remove_restriction",
            ]
        )
        #expect(ControlWords.word(.setPower(UnitActionSetPower(power: true))) == "Power ON")
        #expect(ControlWords.word(.setPower(UnitActionSetPower(power: false))) == "Power OFF")
        #expect(ControlWords.word(.setFan(UnitActionSetFan(fanLevel: .mid))) == "set_fan = mid")
        #expect(ControlWords.word(.ventilate(UnitActionVentilate(level: .mid))) == "ventilate = mid")
        #expect(ControlWords.word(.setTemperature(UnitActionSetTemperature(celsius: 22.5))) == "set_temperature = 22.5 °C")
    }

    @Test func theCurrentState() throws {
        var unit = try technicianUnit()
        unit.observedState = ObservedState(power: true, celsius: 24, mode: .cool, fanLevel: .mid, observedAt: nil)
        #expect(ControlWords.tiles(unit).map(\.1) == ["On", "Cool", "24 °C", "Mid", "25.6 °C"])
        unit.observedState = ObservedState(power: false, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil)
        unit.latestMeasurements[2].quality = .stale
        #expect(ControlWords.tiles(unit).map(\.1) == ["Off", "—", "—", "—", "25.6 °C (stale)"])
        unit.observedState.power = nil
        unit.latestMeasurements = []
        #expect(ControlWords.tiles(unit).map(\.1) == ["Unknown", "—", "—", "—", "—"])
    }

    @Test func whatTheCapabilityAllowsAndTheRestriction() throws {
        var unit = try technicianUnit()
        #expect(
            ControlWords.capability(unit).hasPrefix("Capability v1 allows: ")
                && ControlWords.capability(unit).hasSuffix("Connection: Online.")
        )
        unit.capabilities.modes = []
        unit.capabilities.fanLevels = []
        unit.capabilities.temperature = nil
        #expect(ControlWords.capability(unit).hasPrefix("Capability v1 allows: test run 1–15 min."))
        #expect(ControlWords.restriction(unit) == nil && ControlWords.blocked(unit) == nil)
        unit.effectiveControlPolicy = .restricted(
            EffectiveControlPolicyRestricted(
                phase: .applied,
                policy: .temperatureLimit(RestrictionPolicyTemperatureLimit(minimumCoolingSetpoint: 26))
            )
        )
        #expect(ControlWords.restriction(unit) == "min 26 °C")
        unit.effectiveControlPolicy = .restricted(
            EffectiveControlPolicyRestricted(phase: .applied, policy: .powerOff(RestrictionPolicyPowerOff()))
        )
        #expect(ControlWords.restriction(unit) == "power off only")
        unit.controlAvailability = .blocked(ControlAvailabilityBlocked())
        #expect(ControlWords.blocked(unit) == "the unit must be reconciled after a restriction")
    }

    @Test func theJobsCommandHistory() {
        var running = run(.running)
        running.startCommandId = "c1"
        running.endCommandId = "c2"
        let rows = ControlWords.history(
            [
                command("c1", status: .sent), command("c2", status: .requested), command("c3"),
                command("c4", status: .failed, failure: .offline),
                command("c5", status: .failed), command("c6", status: .expired), command("c7", status: .cancelled),
            ],
            runs: [running]
        )
        #expect(rows[0].title == "test run start (Power ON, 5 min)" && rows[0].badge == "Running" && rows[0].sub.contains(" · ends "))
        #expect(rows[1].title == "test run end (set_mode = fan)" && rows[1].badge == "Waiting")
        #expect(rows[2].title == "set_mode = cool" && rows[2].badge == "Acked" && rows[2].sub.contains("acknowledged "))
        #expect(
            rows[3].sub.hasSuffix("failed: the AC was offline") && rows[4].sub.hasSuffix("rejected by the device")
                && rows[4].badge == "Rejected"
        )
        #expect(rows[5].badge == "No response" && rows[6].badge == "Cancelled" && rows[6].sub.hasSuffix("cancelled"))
        var noEnd = running
        noEnd.endAt = nil
        #expect(ControlWords.history([command("c1", status: .sent)], runs: [noEnd])[0].sub.contains("ends") == false)
    }

    @Test func eachRunStateHasItsBanner() {
        let titles = DiagnosticRunState.allCases.map { ControlWords.banner(run($0)).title }
        #expect(
            titles == [
                "Running — ends \(Format.clock("2026-09-27T04:05:05Z")) (start acknowledged \(Format.clock("2026-09-27T04:00:05Z")))",
                "Starting — waiting for the device", "End requested \(Format.clock("2026-09-27T04:05:05Z")) — awaiting device response",
                "End acknowledged — test run completed", "Start not acknowledged — the test run did not start",
                "No end response — the end was not confirmed", "End blocked — authorization ended before the end time",
            ]
        )
        #expect(ControlWords.banner(run(.completed)).text == "The device confirmed set_mode = fan.")
        #expect(ControlWords.banner(run(.startFailed)).text == "No end command is needed.")
        #expect(
            ControlWords.banner(run(.startFailed, failure: .offline)).text == "Not done: the AC was offline · no end command is needed."
        )
        #expect(ControlWords.banner(run(.endFailed)).text == "The unit may still be running; check it on site.")
        #expect(ControlWords.banner(run(.endFailed, failure: .timeout)).text.hasPrefix("Not confirmed: no answer from the AC in time"))
        var unstarted = run(.awaitingStart)
        unstarted.startedAt = nil
        unstarted.endAt = nil
        #expect(ControlWords.banner(unstarted).text.contains("(Power ON)"))
        #expect(DiagnosticRunState.allCases.filter { ControlWords.isActive(run($0)) } == [.running, .awaitingStart, .endRequested])
        #expect(!ControlWords.isActive(nil))
    }

    @Test func eachRefusalInWords() {
        let refused = { (code: ErrorCode, key: String, fields: [String: String]) in
            ControlWords.refusal(DomainError(code: code, messageKey: key, fieldErrors: fields, correlationId: "c", retryAfterSeconds: nil))
        }
        #expect(refused(.conflict, "errors.restriction_active", [:])?.hasPrefix("The active restriction") == true)
        #expect(refused(.conflict, "errors.unit_busy", [:])?.hasPrefix("The unit is busy") == true)
        #expect(refused(.conflict, "errors.reconciliation_required", [:])?.hasPrefix("The unit must be reconciled") == true)
        #expect(refused(.forbidden, "errors.assignment_not_started", [:]) == "Your work window has not started.")
        #expect(refused(.forbidden, "errors.assignment_ended", [:]) == "Your work window has ended.")
        #expect(refused(.offline, "error.offline", [:])?.hasPrefix("The device is not reachable") == true)
        #expect(refused(.forbidden, "error.forbidden", [:])?.hasPrefix("Outside your assignment") == true)
        #expect(refused(.conflict, "error.versionConflict", [:]) == "The unit or job changed; the page now shows the latest version.")
        #expect(refused(.unavailable, "error.unavailable", [:]) == "Not sent; your entries are kept. Try again.")
        #expect(refused(.validation, "error.validation", ["reason": "error.length"]) == nil, "the field says it")
        #expect(refused(.validation, "error.validation", [:]) != nil)
        #expect(refused(.rateLimited, "error.rateLimited", [:]) == "Too many changes in a short time — try again shortly")
        let fields = ["error.length", "error.required", "error.range", "error.unsupportedAction", "error.invalid", "errors.too_long"]
        #expect(
            fields.map(ControlWords.field) == [
                "1–1000 characters", "Required", "1–15 minutes", "Not supported by this AC", "Not valid", "too long",
            ]
        )
    }
}

@MainActor @Suite struct ControlModelTests {
    private func model(
        _ answers: [StubTransport.Answer],
        jobId: String? = "f0000000-0000-4000-8000-000000000863"
    ) -> (ControlModel, StubTransport) {
        let transport = StubTransport(answers)
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        return (
            ControlModel(unitId: "e841ee95-3171-54d5-be91-1f2301bdb701", jobId: jobId, client: client) { _ in await Task.yield() },
            transport
        )
    }

    private func answer(_ result: some Encodable, at: String? = nil) throws -> StubTransport.Answer {
        var meta = ["correlationId": "c"]
        meta["snapshotAt"] = at
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: meta))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    private func page<Item: Codable & Sendable & Equatable>(_ items: [Item]) throws -> StubTransport.Answer {
        try answer(Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1))
    }

    private func text(_ name: String) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try fixture(name), encoding: .utf8)))
    }

    private func refusal(_ status: Int, _ code: String, _ key: String, fields: String = "{}") -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":\#(fields),"correlationId":"c","retryAfterSeconds":null}"#)
    }

    /// The technician's session at a time inside the fixture job's window (01:05–04:35), with or without control.diagnose.
    private func session(_ diagnose: Bool = true, at: String = "2026-09-27T02:00:00Z") throws -> StubTransport.Answer {
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
            permissions: diagnose ? [.controlDiagnose] : [],
            generation: 1,
            viewEpoch: 1,
            issuedAt: at,
            expiresAt: at
        )
        return try answer(session, at: at)
    }

    /// One read: the unit, the session, the job, its runs and its commands.
    private func read(
        runs: [DiagnosticRun] = [],
        commands: [Command] = [],
        at: String = "2026-09-27T02:00:00Z"
    ) throws -> [StubTransport.Answer] {
        [try text("units.get.technician"), try session(at: at), try text("jobs.get.assigned"), try page(runs), try page(commands)]
    }

    @Test func theUnitAndTheJobsDiagnosticsAreRead() async throws {
        let (model, transport) = model(try read(runs: [run(.completed)], commands: [command("c1")]))
        await model.load()
        #expect(model.canDiagnose && model.job != nil && model.isInWindow && model.unavailable == nil)
        #expect(
            model.options.first?.id == "power:on" && model.actionId == "power:on" && model.startId == "power:on"
                && model.endId == "power:off"
        )
        #expect(model.tiles.count == 5 && model.capability != nil && model.restriction == nil && model.blocked == nil)
        #expect(model.history.map(\.id) == ["c1"] && model.banner?.title == "End acknowledged — test run completed" && !model.isActive)
        let requests = await transport.requests.compactMap(\.url)
        #expect(
            requests.map { $0.path() } == [
                "/v1/units/e841ee95-3171-54d5-be91-1f2301bdb701", "/v1/session", "/v1/jobs/f0000000-0000-4000-8000-000000000863",
                "/v1/diagnostic-runs", "/v1/commands",
            ]
        )
    }

    @Test func withoutThePermissionOrAJobOrTheWindowNothingIsSent() async throws {
        var (model, _) = model([try text("units.get.technician"), try session(false)])
        await model.load()
        #expect(model.unavailable == "Diagnostic control needs “Run test runs” on an assigned job. Ask HQ if you should have it.")
        (model, _) = self.model([try text("units.get.technician"), try session()], jobId: nil)
        await model.load()
        #expect(model.unavailable?.hasPrefix("Diagnostic commands and test runs belong to a job") == true)
        await model.send()
        (model, _) = self.model(try read(at: "2026-09-27T06:00:00Z"))
        await model.load()
        #expect(!model.isInWindow && model.unavailable?.hasPrefix("Outside your work window ") == true)
    }

    @Test func aBlockedUnitWaits() async throws {
        var unit = try technicianUnit()
        unit.controlAvailability = .blocked(ControlAvailabilityBlocked())
        let (model, _) = model([
            try answer(unit), try session(), try text("jobs.get.assigned"), try page([DiagnosticRun]()), try page([Command]()),
        ])
        await model.load()
        #expect(
            model.unavailable
                == "Control is blocked right now (the unit must be reconciled after a restriction) — commands and test runs wait until it clears."
        )
    }

    @Test func beforeTheWindowAndOutsideTheAssignments() async throws {
        let notStarted = refusal(403, "FORBIDDEN", "errors.assignment_not_started")
        var (model, _) = model([notStarted, try text("jobs.get.assigned")])
        await model.load()
        #expect(
            model.content.value.map { if case let .notStarted(text) = $0 { text.hasPrefix("Diagnostic control opens at ") } else { false } }
                == true
        )
        (model, _) = self.model([notStarted], jobId: nil)
        await model.load()
        #expect(model.content.value == .notStarted("Available from the work start time of your assigned job."))
        #expect(model.unavailable == nil && model.tiles.isEmpty && model.options.isEmpty && model.capability == nil)
        (model, _) = self.model([refusal(404, "NOT_FOUND", "error.notFound")])
        await model.load()
        #expect(model.content.value == .notFound)
        (model, _) = self.model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(model.content.message != nil)
    }

    @Test func aDiagnosticCommandNeedsItsReasonAndIsFollowedUntilTheDeviceAnswers() async throws {
        let sent = command("c9", status: .sent)
        let (model, transport) = model(
            try read() + [try answer(sent)] + (try read(commands: [sent])) + (try read(commands: [command("c9")]))
        )
        await model.load()
        await model.send()
        #expect(model.fieldErrors["reason"] == "Reason is required (1–1000 characters)")
        model.actionId = "mode:cool"
        model.reason = " Check the compressor "
        await model.send()
        #expect(model.note == "c9 sent — waiting for the device" && model.reason.isEmpty)
        #expect(!model.isActive && model.history.first?.badge == "Acked", "followed until acknowledged")
        let create = await transport.requests[5]
        let body = try JSONDecoder().decode(StubCreate.self, from: try #require(create.httpBody))
        #expect(
            body.reason == "Check the compressor" && body.jobId == "f0000000-0000-4000-8000-000000000863" && body.expectedUnitVersion == 1
        )
    }

    @Test func aTestRunIsCheckedThenStarted() async throws {
        let (model, transport) = model(try read() + [try answer(run(.awaitingStart))] + (try read(runs: [run(.completed)])))
        await model.load()
        model.minutes = 16
        await model.startRun()
        #expect(
            model.fieldErrors == [
                "runReason": "Reason is required (1–1000 characters)", "durationMinutes": "A test run lasts 1–15 minutes",
            ]
        )
        model.minutes = 5
        model.runReason = "Cooling for five minutes"
        await model.startRun()
        #expect(model.note == "Test run requested — waiting for the start acknowledgement" && model.runReason.isEmpty)
        let body = try JSONDecoder().decode(StubRun.self, from: try #require(await transport.requests[5].httpBody))
        #expect(body.durationMinutes == 5 && body.expectedJobVersion == 1)
    }

    @Test func aRefusedWriteKeepsTheEntries() async throws {
        let refused = refusal(422, "VALIDATION", "error.validation", fields: #"{"reason":"error.length"}"#)
        let (model, _) = model(try read() + [refused] + (try read()))
        await model.load()
        model.reason = "Check"
        await model.send()
        #expect(model.refusal == nil && model.fieldErrors == ["reason": "1–1000 characters"] && model.reason == "Check")
    }

    @Test func theFollowingStopsAfterItsPolls() async throws {
        let waiting = command("c9", status: .sent)
        let reads = (0...ControlModel.polls).flatMap { _ in (try? read(commands: [waiting])) ?? [] }
        let (model, _) = model(reads)
        await model.load()
        #expect(model.isActive)
        await model.follow()
        #expect(model.isActive, "still waiting: the screen stops reading after its polls")
    }

    @Test func anOptionNoLongerOfferedOrNoneAtAll() async throws {
        let (model, _) = model(try read())
        await model.load()
        model.actionId = "gone"
        model.startId = "gone"
        await model.send()
        await model.startRun()
        #expect(model.note == nil)
        var unit = try technicianUnit()
        unit.capabilities.control = false
        let (bare, _) = self.model([try answer(unit), try session(false)])
        await bare.load()
        #expect(bare.actionId.isEmpty && bare.endId.isEmpty)
    }

    @Test func unreadablePartsLeaveTheirPlacesEmpty() async throws {
        // the session, then the runs and the commands, cannot be read
        var (model, _) = model([try text("units.get.technician"), .failure(URLError(.timedOut))])
        await model.load()
        #expect(!model.canDiagnose)
        (model, _) = self.model([
            try text("units.get.technician"), try session(), try text("jobs.get.assigned"), .failure(URLError(.timedOut)),
            .failure(URLError(.timedOut)),
        ])
        await model.load()
        #expect(model.runs.isEmpty && model.commands.isEmpty && model.job != nil)
        #expect(ControlWords.clock(nil) == "—")
    }

    @Test func aJobWithoutItsAssignmentIsOutsideAnyWindow() async throws {
        struct Envelope: Decodable { let data: JobsGetResult }
        guard case var .detail(job) = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.get.assigned")).data else {
            throw CocoaError(.coderValueNotFound)
        }
        job.assignment = nil
        let (model, _) = model([
            try text("units.get.technician"), try session(), try answer(JobsGetResult.detail(job)), try page([DiagnosticRun]()),
            try page([Command]()),
        ])
        await model.load()
        #expect(model.unavailable == "Outside your work window no assignment — diagnostic commands can’t be sent now.")
        let plain = ControlModel(
            unitId: "u1",
            jobId: nil,
            client: CoreClient(baseURL: URL(filePath: "/"), transport: StubTransport([]), credentials: FixedCredentials())
        )
        #expect(plain.unit == nil)
        await plain.pause(.milliseconds(1))
    }

    @Test func theAppsWaitIsAPlainSleep() async {
        let clock = ContinuousClock()
        let started = clock.now
        await UnitControlSleep.sleep(.milliseconds(10))
        #expect(clock.now - started >= .milliseconds(10))
    }
}

private struct StubCreate: Decodable {
    let reason: String?
    let jobId: String?
    let expectedUnitVersion: Double
}

private struct StubRun: Decodable {
    let durationMinutes: Double
    let expectedJobVersion: Double
}
