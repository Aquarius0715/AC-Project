import ACKit
import ACShared
import Foundation
import Observation

/// One AC and its remote control (FR-C03, the web's customer unit page): the confirmed setting, the setting the customer
/// is choosing, and the sends. Only the settings the customer changed and the AC does not already have are sent, one
/// command each, each followed until the AC answers or the command expires (D04); the power is sent on its own. A
/// change is sent once the customer stops changing for a moment, without a confirmation: a fully confirmed send offers
/// its undo instead (IR810).
@MainActor @Observable
public final class UnitControlModel {
    /// What the last send came to: its line, whether the AC took it, and the commands that put it back.
    public struct Outcome: Equatable, Sendable {
        public let text: String
        public let isOk: Bool
        public var undo: [UnitAction]
    }

    /// One command of the history: its sentence, its time and whether it failed.
    public struct HistoryRow: Identifiable, Equatable, Sendable {
        public let id: String
        public let text: String
        public let when: String
        public let isBad: Bool
    }

    /// The command a send is waiting for, and its seconds to answer.
    public struct Waiting: Equatable, Sendable {
        public let commandId: String
        public let seconds: Int
    }

    /// What the model's capabilities allow (the web's controlLayout): nothing on a model that only measures.
    public struct Layout: Equatable, Sendable {
        public let control: Bool
        public let temperature: CapabilityTemperature?
        public let modes: [Mode]
        public let fans: [Fan]
    }

    public let unitId: String
    public private(set) var unit: Loadable<UnitDetail> = .loading
    public private(set) var history: [Command] = []
    public private(set) var setpoint: Double = 24
    public private(set) var mode: Mode = .cool
    public private(set) var fan: Fan = .mid
    public private(set) var isSending = false
    /// The command being followed and the seconds the AC has to answer it (IR806); the screen starts its countdown
    /// again for each command.
    public private(set) var waiting: Waiting?
    public var outcome: Outcome?
    /// Counts the customer's changes: each one starts the wait before the send again (the screen's task is keyed by it).
    public private(set) var editCount = 0
    /// True from a change until its wait is over: the changes are on their way, not left behind by a failed send.
    public private(set) var isSettling = false

    let client: CoreClient
    let me: String?
    let pause: @Sendable (Duration) async -> Void
    /// This screen's sends with their last state, shown over the stored history until the history has them.
    private var sent: [Command] = []
    private var edited: Set<Edit> = []
    private var seenReport: String?

    private enum Edit { case temperature, mode, fan }

    /// The polls of a command: every 2 s, at most 25 times (the 30 s window, the expiry grace and the scheduler's tick).
    static let pollInterval = Duration.seconds(2)
    static let polls = 25
    /// A cooling setpoint below this is worth a second look (IR808).
    static let coolHintBelow = 22.0
    /// The pause after the last change before it is sent: a few taps of + or − go as one command.
    static let settleDelay = Duration.milliseconds(1500)

    /// - Parameters:
    ///   - me: the signed-in membership, so a command sent by hand says "by you" or "by someone else" (IR804).
    ///   - pause: the wait between polls (the tests' is immediate).
    public init(
        unitId: String,
        client: CoreClient,
        me: String?,
        pause: @escaping @Sendable (Duration) async -> Void = UnitControlModel.sleep
    ) {
        self.unitId = unitId
        self.client = client
        self.me = me
        self.pause = pause
    }

    /// The wait between polls in the app: not the main actor's (the model's), so the poll loop's wait is the plain
    /// task sleep it is given as (a main-actor function converted to the nonisolated closure crashed the Swift 6.2
    /// runtime on the CI runner: "freed pointer was not the last allocation").
    nonisolated public static func sleep(_ duration: Duration) async {
        try? await Task.sleep(for: duration)
    }

    // MARK: - reading

    /// Reads the AC and its last commands; a new report of the AC starts the controls from its confirmed setting.
    public func load() async {
        unit = await unit.after { () async throws(CoreError) in
            try await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unitId)).data
        }
        if let page = try? await client.call(Ops.CommandsList.self, CommandsListInput(unitId: unitId, query: Query(limit: 20))) {
            history = page.data.items
        }
        guard let detail = unit.value else { return }
        let report = "\(detail.id)|\(detail.observedState.observedAt ?? "")"
        if report != seenReport {
            seenReport = report
            edited = []
            if let celsius = detail.observedState.celsius { setpoint = celsius }
            if let observed = detail.observedState.mode { mode = observed }
            if let observed = detail.observedState.fanLevel { fan = observed }
        }
    }

    // MARK: - what can be set

    public var layout: Layout {
        guard let capabilities = unit.value?.capabilities, capabilities.control else {
            return Layout(control: false, temperature: nil, modes: [], fans: [])
        }
        return Layout(
            control: true,
            temperature: capabilities.temperature,
            modes: capabilities.modeControl ? capabilities.modes : [],
            fans: capabilities.fanControl ? capabilities.fanLevels : []
        )
    }

    /// The restriction on the AC's setting, if one is applied or being applied.
    public var restriction: RestrictionPolicy? {
        guard case let .restricted(restricted) = unit.value?.effectiveControlPolicy else { return nil }
        return restricted.policy
    }

    /// HQ checks the AC's restriction state: remote control is paused.
    public var isBlocked: Bool {
        if case .blocked = unit.value?.controlAvailability { return true }
        return false
    }

    public var isOffline: Bool { unit.value?.connection == .offline }

    /// The lowest setpoint: the model's, raised by a cooling restriction.
    public var minimum: Double {
        let model = layout.temperature?.min ?? 16
        if case let .temperatureLimit(limit) = restriction { return max(model, limit.minimumCoolingSetpoint) }
        return model
    }

    public var maximum: Double { layout.temperature?.max ?? 30 }
    public var step: Double { layout.temperature?.step ?? 1 }

    /// Nothing is sent to an AC that is offline, paused or only measures.
    public var isDisabled: Bool { isOffline || isBlocked || !layout.control }

    /// The power stays as it is while a restriction keeps the AC off.
    public var isPowerLocked: Bool {
        if case .powerOff = restriction { return true }
        return isDisabled || isSending
    }

    public var isOn: Bool { unit.value?.effectivePowerState == .on }
    public var isPowerUnknown: Bool { unit.value?.effectivePowerState == .unknown }

    public var canLower: Bool { !isDisabled && setpoint - step >= minimum }
    public var canRaise: Bool { !isDisabled && setpoint + step <= maximum }

    // MARK: - choosing

    /// One step down; from above the range (an AC that reports more than its model's highest), to the highest.
    public func lower() { change(setpoint: min(setpoint - step, maximum)) }
    /// One step up; from below the range (a restriction just requested on a colder AC), to the lowest.
    public func raise() { change(setpoint: max(setpoint + step, minimum)) }

    private func change(setpoint value: Double) {
        guard value >= minimum, value <= maximum else { return }
        setpoint = value
        edited.insert(.temperature)
        changed()
    }

    public func choose(mode value: Mode) {
        mode = value
        edited.insert(.mode)
        changed()
    }

    public func choose(fan value: Fan) {
        fan = value
        edited.insert(.fan)
        changed()
    }

    private func changed() {
        editCount += 1
        isSettling = true
    }

    /// Changes a send did not take (it was refused, or the AC did not answer): they wait for "Try again".
    public var isLeftBehind: Bool { !changes.isEmpty && !isSettling && !isSending && !isDisabled }

    /// The settings the customer changed that the AC does not have yet, in the order they are sent, with their words
    /// ("Set temperature 26°C → 24°C").
    private var planned: [(action: UnitAction, words: String)] {
        guard let detail = unit.value, layout.control else { return [] }
        let observed = detail.observedState
        var planned: [(action: UnitAction, words: String)] = []
        if layout.temperature != nil, edited.contains(.temperature), setpoint != observed.celsius {
            let from = observed.celsius.map(Words.celsius) ?? "—"
            planned.append(
                (
                    .setTemperature(UnitActionSetTemperature(celsius: setpoint)),
                    tr("Set temperature {from}°C → {to}°C", ["from": from, "to": Words.celsius(setpoint)])
                )
            )
        }
        if !layout.modes.isEmpty, edited.contains(.mode), mode != observed.mode {
            planned.append((.setMode(UnitActionSetMode(mode: mode)), tr("Mode → {mode}", ["mode": Words.mode(mode)])))
        }
        if !layout.fans.isEmpty, edited.contains(.fan), fan != observed.fanLevel {
            planned.append((.setFan(UnitActionSetFan(fanLevel: fan)), tr("Fan → {fan}", ["fan": Words.fan(fan)])))
        }
        return planned
    }

    public var changes: [UnitAction] { planned.map(\.action) }

    /// The changes in words ("Set temperature 26°C → 24°C · Mode → Dry").
    public var changeSummary: String {
        let words = planned.map(\.words)
        return words.isEmpty ? tr("No change yet") : words.joined(separator: " · ")
    }

    /// The hint next to a very low cooling setpoint (IR808).
    public var coolingHint: String? {
        guard layout.temperature != nil, mode == .cool, setpoint < Self.coolHintBelow else { return nil }
        return tr(
            "{celsius}°C is very cold for cooling: a lower setting does not cool the room faster, it runs longer and uses more power. 24–26°C is a common comfortable range.",
            ["celsius": Words.celsius(setpoint)]
        )
    }

    // MARK: - sending

    /// Sends the chosen changes, one command after another; the first the AC does not take stops the rest.
    public func sendChanges() async {
        await send(changes, isUndo: false)
    }

    /// Sends the changes once the customer has stopped changing for a moment; a change meanwhile cancels the task
    /// (the screen starts a new one), so only the last setting is sent.
    public func sendWhenSettled() async {
        await pause(Self.settleDelay)
        // a cancelled wait is followed by the next change's, which clears the flag
        guard !Task.isCancelled else { return }
        isSettling = false
        await sendChanges()
    }

    /// The last send's outcome goes away after a while when the AC took it (its undo with it); a failure stays.
    public func dismissWhenSeen(_ shown: Outcome) async {
        guard shown.isOk else { return }
        await pause(Self.outcomeSeconds)
        guard !Task.isCancelled, outcome == shown else { return }
        outcome = nil
    }

    /// How long a confirmed send's line and its undo stay.
    static let outcomeSeconds = Duration.seconds(10)

    /// Sends the power on its own.
    public func sendPower(_ on: Bool) async {
        await send([.setPower(UnitActionSetPower(power: on))], isUndo: false)
    }

    /// Puts back what the last send changed.
    public func undo() async {
        guard let actions = outcome?.undo, !actions.isEmpty else { return }
        await send(actions, isUndo: true)
    }

    private func send(_ actions: [UnitAction], isUndo: Bool) async {
        guard !actions.isEmpty, !isSending, let detail = unit.value else { return }
        let before = detail.observedState
        isSending = true
        outcome = nil
        var isAllOk = true
        for action in actions where !(await sendOne(action, to: detail.displayName)) {
            isAllOk = false
            break
        }
        if isAllOk, !isUndo { outcome?.undo = Self.undo(actions, before: before) }
        isSending = false
        waiting = nil
        await load()
    }

    /// One command: created with the AC's current version, then followed until the AC answers; false stops the rest.
    private func sendOne(_ action: UnitAction, to name: String) async -> Bool {
        let created: Command
        do {
            let version = try await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unitId)).data.version
            let input = CommandsCreateInput(unitId: unitId, action: action, expectedUnitVersion: version)
            created = try await client.call(Ops.CommandsCreate.self, input).data
        } catch {
            outcome = Outcome(text: tr("Not sent: {reason}", ["reason": Words.refusal(error)]), isOk: false, undo: [])
            return false
        }
        waiting = Waiting(commandId: created.id, seconds: Self.answerSeconds(created))
        guard let answered = await follow(created) else {
            // the command was sent; only its answer could not be read, so it is never "not sent" (IR728)
            let text = tr(
                "{unit}: sent — its answer could not be read. Refresh to see the AC's state before sending again.",
                ["unit": name]
            )
            outcome = Outcome(text: text, isOk: false, undo: [])
            return false
        }
        let isOk = answered.status == .acknowledged
        let line = "\(name): \(Words.history(answered, me: nil))"
        let text = isOk ? line : tr("{result}. The AC keeps its confirmed setting; check it and send again.", ["result": line])
        outcome = Outcome(text: text, isOk: isOk, undo: [])
        return isOk
    }

    /// Polls the command until it is answered (acknowledged, failed, expired, cancelled) or the polls run out; nil when
    /// a poll got no answer.
    private func follow(_ first: Command) async -> Command? {
        var command = first
        track(command)
        for _ in 0..<Self.polls where !Self.isTerminal(command.status) {
            await pause(Self.pollInterval)
            guard let next = try? await client.call(Ops.CommandsGet.self, CommandsGetInput(id: command.id)).data else { return nil }
            command = next
            track(command)
        }
        return command
    }

    private func track(_ command: Command) {
        sent = [command] + sent.filter { $0.id != command.id }
    }

    static func isTerminal(_ status: CommandStatus) -> Bool {
        status == .acknowledged || status == .failed || status == .expired || status == .cancelled
    }

    /// The seconds from a command's request to its expiry, 30 when they say none (IR806).
    static func answerSeconds(_ command: Command) -> Int {
        guard let requested = Format.parse(command.requestedAt), let expires = Format.parse(command.expiresAt) else { return 30 }
        let span = Int(expires.timeIntervalSince(requested).rounded())
        return span > 0 ? span : 30
    }

    /// The commands that put back what `actions` changed: each setting back to the confirmed one it replaced, a setting
    /// with no confirmed value before or with no change left out (IR810).
    static func undo(_ actions: [UnitAction], before: ObservedState) -> [UnitAction] {
        actions.compactMap { action in
            switch action {
            case let .setPower(change):
                guard let power = before.power, power != change.power else { return nil }
                return .setPower(UnitActionSetPower(power: power))
            case let .setTemperature(change):
                guard let celsius = before.celsius, celsius != change.celsius else { return nil }
                return .setTemperature(UnitActionSetTemperature(celsius: celsius))
            case let .setMode(change):
                guard let mode = before.mode, mode != change.mode else { return nil }
                return .setMode(UnitActionSetMode(mode: mode))
            case let .setFan(change):
                guard let fan = before.fanLevel, fan != change.fanLevel else { return nil }
                return .setFan(UnitActionSetFan(fanLevel: fan))
            case .ventilate:
                return nil
            }
        }
    }

    /// The history, newest first, in words (IR216, IR804).
    public var historyRows: [HistoryRow] {
        let pending = unit.value?.pendingCommands ?? []
        var seen = Set<String>()
        return (sent + history + pending)
            .filter { seen.insert($0.id).inserted }
            .sorted { $0.requestedAt > $1.requestedAt }
            .map { command in
                HistoryRow(
                    id: command.id,
                    text: Words.history(command, me: me),
                    when: Format.dateTime(command.requestedAt),
                    isBad: command.status == .failed || command.status == .expired
                )
            }
    }
}
