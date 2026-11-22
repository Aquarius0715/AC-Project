import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import CustomerFeature

/// The AC control on scripted answers: the real unit of the dev stack (Fixtures/units.get.customer.json) changed per
/// case, and the refusals and answers that never come.
@MainActor @Suite struct UnitControlFailureTests {
    /// An answer of the Core API, written by the tests (it is only encoded).
    private struct Envelope<Value: Encodable>: Encodable {
        let data: Value
        let meta = ["correlationId": "c"]
    }

    private let day = "2026-11-20T00:00:00Z"

    private func unit(_ change: (inout UnitDetail) -> Void = Self.unchanged) throws -> UnitDetail {
        struct Answer: Decodable { let data: UnitDetail }
        var detail = try JSONDecoder().decode(Answer.self, from: try fixture("units.get.customer")).data
        detail.connection = .online
        change(&detail)
        return detail
    }

    private func answer(_ value: some Encodable) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try JSONEncoder().encode(Envelope(data: value)), encoding: .utf8)))
    }

    private func command(
        _ status: CommandStatus,
        failure: CommandFailureCode? = nil,
        source: CommandSource = .ui,
        id: String = "c1"
    ) -> Command {
        Command(
            id: id,
            tenantId: "t1",
            version: 1,
            createdAt: day,
            updatedAt: day,
            unitId: "u1",
            actorMembershipId: "m1",
            action: .setTemperature(UnitActionSetTemperature(celsius: 24)),
            diagnosticRunId: nil,
            jobId: nil,
            reason: nil,
            status: status,
            delivery: .sent,
            requestedAt: day,
            sentAt: day,
            acknowledgedAt: nil,
            expiresAt: "2026-11-20T00:00:45Z",
            failureCode: failure,
            correlationId: id,
            source: source
        )
    }

    private var noCommands: StubTransport.Answer {
        get throws { try answer(Page<Command>(items: [], nextCursor: nil, total: 0, snapshotVersion: 1)) }
    }

    private func model(_ answers: [StubTransport.Answer]) -> UnitControlModel {
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let client = CoreClient(baseURL: base, transport: StubTransport(answers), credentials: FixedCredentials())
        return UnitControlModel(unitId: "u1", client: client, me: "m1", pause: Self.noWait)
    }

    /// The model with the unit loaded and its temperature lowered by one step.
    private func lowered(_ detail: UnitDetail, then answers: [StubTransport.Answer]) async throws -> UnitControlModel {
        let model = model([try answer(detail), try noCommands, try answer(detail)] + answers)
        await model.load()
        model.lower()
        return model
    }

    @Test func refusedSendSaysWhyAndKeepsTheSetting() async throws {
        let busy = #"{"code":"CONFLICT","messageKey":"errors.unit_busy","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#
        let detail = try unit()
        let model = try await lowered(detail, then: [.http(409, busy), try answer(detail), try noCommands])
        await model.sendChanges()
        #expect(model.outcome == .init(text: "Not sent: another command is still waiting for this AC", isOk: false, undo: []))
        #expect(!model.isSending)
    }

    @Test(arguments: [
        ("error.versionConflict", "this AC changed — reload and try again"),
        ("control.restriction_active", "a service restriction limits this setting"),
        ("control.reconciliation_required", "HQ is checking this AC"),
        ("error.forbidden", "You don't have permission for this action"),
    ])
    func refusalsInWords(key: String, words: String) {
        let error = CoreError(
            status: 409,
            error: DomainError(code: .forbidden, messageKey: key, fieldErrors: [:], correlationId: "", retryAfterSeconds: nil)
        )
        #expect(Words.refusal(error) == words)
    }

    @Test func answerThatCannotBeReadIsSentNotUnsent() async throws {
        let detail = try unit()
        let created = command(.sent)
        let model = try await lowered(
            detail,
            then: [try answer(created), .failure(URLError(.notConnectedToInternet)), try answer(detail), try noCommands]
        )
        await model.sendChanges()
        #expect(
            model.outcome?.text
                == "Meeting room AC: sent — its answer could not be read. Refresh to see the AC's state before sending again."
        )
        #expect(model.outcome?.isOk == false)
    }

    @Test func refusedByTheACKeepsItsConfirmedSetting() async throws {
        let detail = try unit()
        let failed = command(.failed, failure: .deviceRejected)
        let model = try await lowered(detail, then: [try answer(command(.sent)), try answer(failed), try answer(detail), try noCommands])
        await model.sendChanges()
        #expect(
            model.outcome?.text
                == "Meeting room AC: Set temperature 24°C — failed: the AC refused it. The AC keeps its confirmed setting; check it and send again."
        )
        #expect(model.outcome?.undo.isEmpty == true)
        #expect(model.historyRows.first?.isBad == true)
        #expect(model.historyRows.first?.text == "Set temperature 24°C — failed: the AC refused it · by you")
    }

    @Test func commandThatIsNeverAnsweredStopsAfterItsPolls() async throws {
        let detail = try unit()
        let sent = command(.sent)
        let polls = try (0..<UnitControlModel.polls).map { _ in try answer(sent) }
        let model = try await lowered(detail, then: [try answer(sent)] + polls + [try answer(detail), try noCommands])
        await model.sendChanges()
        #expect(
            model.outcome?.text
                == "Meeting room AC: Set temperature 24°C — waiting for device. The AC keeps its confirmed setting; check it and send again."
        )
    }

    @Test func coolingRestrictionRaisesTheLowestSetpoint() async throws {
        let detail = try unit {
            $0.effectiveControlPolicy = .restricted(
                EffectiveControlPolicyRestricted(
                    phase: .applied,
                    policy: .temperatureLimit(RestrictionPolicyTemperatureLimit(minimumCoolingSetpoint: 25))
                )
            )
        }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(model.minimum == 25)
        #expect(!model.canLower)
        model.lower()
        #expect(model.setpoint == 25, "a setpoint below the restriction is never chosen")
        #expect(!model.isPowerLocked)
    }

    /// A restriction just requested finds the AC below its lowest setpoint: "+" goes to that lowest setpoint at once (it
    /// did nothing while the setpoint stayed below it, though the button was enabled).
    @Test func raisingFromBelowTheRestrictionGoesToItsLowest() async throws {
        let detail = try unit {
            $0.observedState.celsius = 20
            $0.effectiveControlPolicy = .restricted(
                EffectiveControlPolicyRestricted(
                    phase: .requested,
                    policy: .temperatureLimit(RestrictionPolicyTemperatureLimit(minimumCoolingSetpoint: 25))
                )
            )
        }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(model.setpoint == 20)
        #expect(model.canRaise)
        #expect(!model.canLower)
        model.raise()
        #expect(model.setpoint == 25)
        #expect(model.changes == [.setTemperature(UnitActionSetTemperature(celsius: 25))])
        model.raise()
        #expect(model.setpoint == 26, "from the lowest setpoint on, one step at a time")
    }

    /// An AC that reports a setpoint above its model's highest: "−" goes to that highest setpoint.
    @Test func loweringFromAboveTheRangeGoesToItsHighest() async throws {
        let detail = try unit { $0.observedState.celsius = 33 }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(!model.canRaise)
        model.lower()
        #expect(model.setpoint == model.maximum)
        #expect(model.changes == [.setTemperature(UnitActionSetTemperature(celsius: model.maximum))])
    }

    /// Each command of a send is waited for on its own: the wait names the command being followed, so the screen's
    /// countdown starts again for the next one (the web keys its wait banner by the command id).
    @Test func eachCommandOfASendHasItsOwnWait() async throws {
        let detail = try unit()
        let recorder = WaitRecorder()
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let answers = [
            try answer(detail), try noCommands,
            try answer(detail), try answer(command(.sent, id: "c1")), try answer(command(.acknowledged, id: "c1")),
            try answer(detail), try answer(command(.sent, id: "c2")), try answer(command(.acknowledged, id: "c2")),
            try answer(detail), try noCommands,
        ]
        let client = CoreClient(baseURL: base, transport: StubTransport(answers), credentials: FixedCredentials())
        let model = UnitControlModel(unitId: "u1", client: client, me: "m1") { _ in await recorder.note() }
        recorder.model = model
        await model.load()
        #expect(model.waiting == nil)
        model.lower()
        model.choose(mode: .dry)
        await model.sendChanges()
        #expect(recorder.seen == ["c1", "c2"])
        #expect(model.waiting == nil, "no wait is shown once the send is over")
    }

    @Test func powerOffRestrictionLocksThePower() async throws {
        let detail = try unit {
            $0.effectiveControlPolicy = .restricted(
                EffectiveControlPolicyRestricted(phase: .applied, policy: .powerOff(RestrictionPolicyPowerOff()))
            )
        }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(model.isPowerLocked)
        #expect(model.minimum == 16)
    }

    @Test func pausedOfflineOrMeasuringACsTakeNothing() async throws {
        let blocked = try unit { $0.controlAvailability = .blocked(ControlAvailabilityBlocked()) }
        let offline = try unit { $0.connection = .offline }
        let measuring = try unit { $0.capabilities.control = false }
        for detail in [blocked, offline, measuring] {
            let model = model([try answer(detail), try noCommands])
            await model.load()
            #expect(model.isDisabled)
            #expect(model.isPowerLocked)
            #expect(!model.canLower && !model.canRaise)
        }
        let model = model([try answer(measuring), try noCommands])
        await model.load()
        #expect(model.layout == .init(control: false, temperature: nil, modes: [], fans: []))
        model.choose(mode: .dry)
        #expect(model.changes.isEmpty)
        await model.sendChanges()
        #expect(model.outcome == nil, "nothing is sent")
    }

    @Test func modelWithoutModeOrFanControlSendsNeither() async throws {
        let detail = try unit {
            $0.capabilities.modeControl = false
            $0.capabilities.fanControl = false
            $0.capabilities.temperature = nil
        }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(model.layout.modes.isEmpty && model.layout.fans.isEmpty)
        #expect(model.maximum == 30 && model.step == 1)
        model.choose(mode: .dry)
        model.choose(fan: .high)
        model.raise()
        #expect(model.changes.isEmpty)
        #expect(model.coolingHint == nil)
    }

    @Test func failedReadSaysWhy() async throws {
        let model = model([.failure(URLError(.timedOut)), .failure(URLError(.timedOut))])
        await model.load()
        #expect(model.unit.value == nil)
        #expect(model.unit.message == "The request took too long — check whether the change was saved, then try again")
        await model.sendPower(true)
        #expect(model.outcome == nil, "no AC, nothing sent")
    }

    @Test func historyMergesThePendingAndThisScreensSends() async throws {
        let older = command(.acknowledged, source: .automation, id: "c0")
        let pending = command(.sent, source: .restriction, id: "c2")
        var detail = try unit()
        detail.pendingCommands = [pending, older]
        let page = Page(items: [older], nextCursor: nil, total: 1, snapshotVersion: 1)
        let model = model([try answer(detail), try answer(page)])
        await model.load()
        #expect(model.historyRows.map(\.id) == ["c0", "c2"])
        #expect(
            model.historyRows.map(\.text) == [
                "Set temperature 24°C — acknowledged by device · by an automation",
                "Set temperature 24°C — waiting for device · by a restriction",
            ]
        )
    }

    @Test func undoPutsBackOnlyWhatHadAConfirmedValue() {
        let before = ObservedState(power: nil, celsius: 25, mode: nil, fanLevel: .mid, observedAt: nil)
        let actions: [UnitAction] = [
            .setPower(UnitActionSetPower(power: true)), .setTemperature(UnitActionSetTemperature(celsius: 25)),
            .setMode(UnitActionSetMode(mode: .dry)), .setFan(UnitActionSetFan(fanLevel: .high)),
            .ventilate(UnitActionVentilate(level: .low)), .setTemperature(UnitActionSetTemperature(celsius: 22)),
        ]
        #expect(
            UnitControlModel.undo(actions, before: before) == [
                .setFan(UnitActionSetFan(fanLevel: .mid)), .setTemperature(UnitActionSetTemperature(celsius: 25)),
            ]
        )
    }

    @Test func secondsToAnswerComeFromTheCommand() {
        #expect(UnitControlModel.answerSeconds(command(.sent)) == 45)
        var unreadable = command(.sent)
        unreadable.expiresAt = "soon"
        #expect(UnitControlModel.answerSeconds(unreadable) == 30)
        var backwards = command(.sent)
        backwards.expiresAt = "2026-11-19T00:00:00Z"
        #expect(UnitControlModel.answerSeconds(backwards) == 30)
    }

    @Test func undoWithoutAnOutcomeSendsNothing() async throws {
        let model = model([try answer(try unit()), try noCommands])
        await model.load()
        await model.undo()
        #expect(model.outcome == nil)
    }
}

extension UnitControlFailureTests {
    @Test func acThatReportedNoSettingStartsFromTheDefaults() async throws {
        let detail = try unit { $0.observedState = ObservedState(power: nil, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil) }
        let model = model([try answer(detail), try noCommands])
        #expect(model.historyRows.isEmpty, "nothing before the unit is read")
        await model.load()
        #expect(model.setpoint == 24 && model.mode == .cool && model.fan == .mid)
        model.raise()
        #expect(model.changeSummary == "Set temperature —°C → 25°C")
        #expect(UnitControlModel.undo(model.changes, before: detail.observedState).isEmpty, "no confirmed value to put back")
    }

    @Test func undoLeavesOutAFanThatDidNotChange() {
        let before = ObservedState(power: true, celsius: 25, mode: .cool, fanLevel: .mid, observedAt: nil)
        #expect(UnitControlModel.undo([.setFan(UnitActionSetFan(fanLevel: .mid))], before: before).isEmpty)
    }

    @Test func appWaitsBetweenPolls() async {
        let clock = ContinuousClock()
        let started = clock.now
        await UnitControlModel.sleep(.milliseconds(10))
        #expect(clock.now - started >= .milliseconds(10))
    }
}

extension UnitControlFailureTests {
    /// The app's own wait between polls (no stand-in): one poll, two seconds.
    @Test func appsModelWaitsBeforeItPolls() async throws {
        let detail = try unit()
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let answers = [
            try answer(detail), try noCommands, try answer(detail), try answer(command(.sent)), try answer(command(.acknowledged)),
            try answer(detail), try noCommands,
        ]
        let model = UnitControlModel(
            unitId: "u1",
            client: CoreClient(baseURL: base, transport: StubTransport(answers), credentials: FixedCredentials()),
            me: nil
        )
        await model.load()
        model.lower()
        let clock = ContinuousClock()
        let started = clock.now
        await model.sendChanges()
        #expect(clock.now - started >= UnitControlModel.pollInterval)
        #expect(model.outcome?.isOk == true)
    }
}

extension UnitControlFailureTests {
    @Test func powerTheACHasNotReportedIsUnknown() async throws {
        let detail = try unit { $0.effectivePowerState = .unknown }
        let model = model([try answer(detail), try noCommands])
        await model.load()
        #expect(model.isPowerUnknown)
        #expect(!model.isOn)
    }

    @Test func undoPutsTheModeBack() {
        let before = ObservedState(power: true, celsius: 25, mode: .cool, fanLevel: .mid, observedAt: nil)
        #expect(
            UnitControlModel.undo([.setMode(UnitActionSetMode(mode: .dry))], before: before) == [.setMode(UnitActionSetMode(mode: .cool))]
        )
    }
}

extension UnitControlFailureTests {
    /// The unit of the fixture as it is.
    nonisolated static func unchanged(_: inout UnitDetail) {
        // the fixture's unit, online (unit(_:) sets it)
    }
}

/// Notes, at each wait between polls, which command the model says it is waiting for.
@MainActor private final class WaitRecorder {
    weak var model: UnitControlModel?
    private(set) var seen: [String] = []

    func note() {
        seen.append(model?.waiting?.commandId ?? "none")
    }
}
