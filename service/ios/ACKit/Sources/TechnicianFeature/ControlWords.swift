import ACKit
import ACShared
import Foundation

/// One action the technician can choose: its key, its words and the action sent.
public struct ControlOption: Sendable, Equatable, Identifiable {
    public let id: String
    public let label: String
    public let action: UnitAction
}

/// Technician diagnostic control in words (shared/lib/techControl.ts, FR-T10, DD-T10): the unit's current state, what
/// its capability allows, its restriction, the job's command history and the test run's state. Command codes
/// ("set_mode = cool") stay codes.
public enum ControlWords {
    /// The actions the unit's capability offers (clientAutomations.ts actionOptions): power, each setpoint of its range,
    /// its modes, fan levels and ventilation levels.
    public static func options(_ capability: Capability) -> [ControlOption] {
        var out: [ControlOption] = []
        if capability.control {
            out.append(ControlOption(id: "power:on", label: tr("Power ON"), action: .setPower(UnitActionSetPower(power: true))))
            out.append(ControlOption(id: "power:off", label: tr("Power OFF"), action: .setPower(UnitActionSetPower(power: false))))
            if let range = capability.temperature {
                for celsius in stride(from: range.min, through: range.max, by: range.step > 0 ? range.step : 1) {
                    let action = UnitAction.setTemperature(UnitActionSetTemperature(celsius: celsius))
                    out.append(ControlOption(id: "temp:\(ReportDraft.number(celsius))", label: Words.action(action), action: action))
                }
            }
            if capability.modeControl {
                out += capability.modes.map { mode in
                    let action = UnitAction.setMode(UnitActionSetMode(mode: mode))
                    return ControlOption(id: "mode:\(mode.rawValue)", label: Words.action(action), action: action)
                }
            }
            if capability.fanControl {
                out += capability.fanLevels.map { fan in
                    let action = UnitAction.setFan(UnitActionSetFan(fanLevel: fan))
                    return ControlOption(id: "fan:\(fan.rawValue)", label: Words.action(action), action: action)
                }
            }
        }
        if capability.ventilation {
            out += capability.ventilationLevels.map { level in
                let action = UnitAction.ventilate(UnitActionVentilate(level: level))
                return ControlOption(id: "vent:\(level.rawValue)", label: Words.action(action), action: action)
            }
        }
        return out
    }

    /// "set_mode = cool", "set_temperature = 24 °C": the command as the device gets it.
    public static func code(_ action: CommandAction) -> String {
        switch action {
        case let .setPower(change): "set_power = \(change.power ? "ON" : "OFF")"
        case let .setTemperature(change): "set_temperature = \(ReportDraft.number(change.celsius)) °C"
        case let .setMode(change): "set_mode = \(change.mode.rawValue)"
        case let .setFan(change): "set_fan = \(change.fanLevel.rawValue)"
        case let .ventilate(change): "ventilate = \(change.level.rawValue)"
        case let .applyRestriction(restriction):
            switch restriction.policy {
            case .powerOff: "apply_restriction = power_off"
            case let .temperatureLimit(limit): "apply_restriction = min \(ReportDraft.number(limit.minimumCoolingSetpoint)) °C"
            }
        case .removeRestriction: "remove_restriction"
        }
    }

    static func command(_ action: UnitAction) -> CommandAction {
        switch action {
        case let .setPower(change): .setPower(change)
        case let .setTemperature(change): .setTemperature(change)
        case let .setMode(change): .setMode(change)
        case let .setFan(change): .setFan(change)
        case let .ventilate(change): .ventilate(change)
        }
    }

    /// A test run's action: "Power ON" in words, the other settings as their code.
    public static func word(_ action: UnitAction) -> String {
        if case let .setPower(change) = action { return change.power ? tr("Power ON") : tr("Power OFF") }
        return code(command(action))
    }

    static func clock(_ instant: String?) -> String { instant.map { Format.clock($0) } ?? "—" }

    /// The current state tiles: power, mode, setpoint, fan and the room's temperature.
    public static func tiles(_ unit: UnitDetail) -> [(String, String)] {
        let state = unit.observedState
        let room = unit.latestMeasurements.first { $0.metric == .temperature }
        let power: String =
            switch state.power {
            case nil: tr("Unknown")
            case true?: tr("On")
            case false?: tr("Off")
            }
        let roomText = room.flatMap { reading in
            reading.value.map { "\(UnitWords.number(.temperature, $0)) °C\(UnitWords.quality(reading.quality).map { " (\($0))" } ?? "")" }
        }
        return [
            (tr("Power"), power),
            (tr("Mode"), state.mode.map(Words.mode) ?? "—"),
            (tr("Setpoint"), state.celsius.map { "\(ReportDraft.number($0)) °C" } ?? "—"),
            (tr("Fan"), state.fanLevel.map(Words.fan) ?? "—"),
            (tr("Room"), roomText ?? "—"),
        ]
    }

    /// What the capability version allows, and the connection.
    public static func capability(_ unit: UnitDetail) -> String {
        let c = unit.capabilities
        var list: [String] = []
        if c.modeControl, !c.modes.isEmpty { list.append("set_mode (\(c.modes.map(\.rawValue).joined(separator: "/")))") }
        if let range = c.temperature { list.append("set_temperature \(ReportDraft.number(range.min))–\(ReportDraft.number(range.max)) °C") }
        if c.fanControl, !c.fanLevels.isEmpty { list.append("set_fan (\(c.fanLevels.map(\.rawValue).joined(separator: "/")))") }
        list.append(tr("test run 1–15 min"))
        let allows = tr(
            "Capability v{version} allows: {list}.",
            ["version": String(Int(unit.capabilityVersion)), "list": list.joined(separator: ", ")]
        )
        return "\(allows) \(tr("Connection: {state}.", ["state": Labels.connection(unit.connection)]))"
    }

    /// The active restriction in words, or nil.
    public static func restriction(_ unit: UnitDetail) -> String? {
        guard case let .restricted(restricted) = unit.effectiveControlPolicy else { return nil }
        switch restricted.policy {
        case let .temperatureLimit(limit): return tr("min {celsius} °C", ["celsius": ReportDraft.number(limit.minimumCoolingSetpoint)])
        case .powerOff: return tr("power off only")
        }
    }

    /// Why control is blocked now, or nil.
    public static func blocked(_ unit: UnitDetail) -> String? {
        guard case .blocked = unit.controlAvailability else { return nil }
        return tr("the unit must be reconciled after a restriction")
    }

    /// The job's commands, newest first as the Core API lists them: what was sent, the device's outcome and the badge;
    /// a test run's commands name their run.
    public static func history(_ commands: [Command], runs: [DiagnosticRun]) -> [UnitRow] {
        commands.map { command in
            let run = runs.first { $0.startCommandId == command.id || $0.endCommandId == command.id }
            let isStart = run?.startCommandId == command.id
            var title = code(command.action)
            if let run {
                title =
                    isStart
                    ? tr("test run start ({action}, {n} min)", ["action": word(run.startAction), "n": String(Int(run.durationMinutes))])
                    : tr("test run end ({action})", ["action": word(run.endAction)])
            }
            let outcome: String =
                switch command.status {
                case .acknowledged: tr("acknowledged {time}", ["time": clock(command.acknowledgedAt)])
                case .failed:
                    command.failureCode.map { tr("failed: {reason}", ["reason": Words.failure($0)]) } ?? tr("rejected by the device")
                case .expired: tr("no device response (expired)")
                case .cancelled: tr("cancelled")
                case .requested, .sent: tr("waiting for the device")
                }
            let isRunning = isStart && run?.state == .running
            let ends = isRunning ? run?.endAt.map { " · \(tr("ends {time}", ["time": clock($0)]))" } ?? "" : ""
            let badge: String =
                if isRunning {
                    tr("Running")
                } else {
                    switch command.status {
                    case .acknowledged: tr("Acked")
                    case .failed: tr("Rejected")
                    case .expired: tr("No response")
                    case .cancelled: tr("Cancelled")
                    case .requested, .sent: tr("Waiting")
                    }
                }
            return UnitRow(id: command.id, title: title, sub: "\(command.id.prefix(8)) · \(outcome)\(ends)", badge: badge)
        }
    }

    /// The test run's banner by its state (DD-T10: stopped only after the end acknowledgement).
    public static func banner(_ run: DiagnosticRun) -> (title: String, text: String) {
        let start = word(run.startAction)
        let end = word(run.endAction)
        switch run.state {
        case .awaitingStart:
            return (
                tr("Starting — waiting for the device"),
                tr("The start command ({action}) was sent; the run starts when the device acknowledges it.", ["action": start])
            )
        case .running:
            return (
                tr("Running — ends {end} (start acknowledged {start})", ["end": clock(run.endAt), "start": clock(run.startedAt)]),
                tr(
                    "The test run ends {n} min after it started. The end command ({action}) is sent at {time}.",
                    ["n": String(Int(run.durationMinutes)), "action": end, "time": clock(run.endAt)]
                )
            )
        case .endRequested:
            return (
                tr("End requested {time} — awaiting device response", ["time": clock(run.endAt)]),
                tr(
                    "One end command ({action}) sent. Not shown as Stopped until the device confirms; a browser timer is not proof.",
                    ["action": code(command(run.endAction))]
                )
            )
        case .completed:
            return (tr("End acknowledged — test run completed"), tr("The device confirmed {action}.", ["action": end]))
        case .startFailed:
            return (
                tr("Start not acknowledged — the test run did not start"),
                run.failureCode.map { tr("Not done: {reason} · no end command is needed.", ["reason": Words.failure($0)]) }
                    ?? tr("No end command is needed.")
            )
        case .endFailed:
            return (
                tr("No end response — the end was not confirmed"),
                run.failureCode.map {
                    tr("Not confirmed: {reason} · the unit may still be running; check it on site.", ["reason": Words.failure($0)])
                } ?? tr("The unit may still be running; check it on site.")
            )
        case .endBlocked:
            return (
                tr("End blocked — authorization ended before the end time"), tr("No end command was sent; the unit may still be running.")
            )
        }
    }

    public static func isActive(_ run: DiagnosticRun?) -> Bool {
        guard let run else { return false }
        return run.state == .awaitingStart || run.state == .running || run.state == .endRequested
    }

    /// A refused diagnostic write in words; nil for a refusal that only marks fields.
    public static func refusal(_ error: DomainError) -> String? {
        switch error.messageKey {
        case "errors.restriction_active":
            return tr("The active restriction does not allow this action (for example a setpoint below its minimum).")
        case "errors.unit_busy": return tr("The unit is busy: a firmware update or an unfinished command must finish first.")
        case "errors.reconciliation_required": return tr("The unit must be reconciled after a restriction before it can be controlled.")
        case "errors.assignment_not_started": return tr("Your work window has not started.")
        case "errors.assignment_ended": return tr("Your work window has ended.")
        default: break
        }
        switch error.code {
        case .offline: return tr("The device is not reachable (connection or power signal), so nothing was sent.")
        case .forbidden: return tr("Outside your assignment or work window, or your role has no diagnostic permission.")
        case .conflict: return tr("The unit or job changed; the page now shows the latest version.")
        case .unavailable: return tr("Not sent; your entries are kept. Try again.")
        case .validation: return error.fieldErrors.isEmpty ? ErrorText.of(error) : nil
        default: return ErrorText.of(error)
        }
    }

    /// A field error of the Core API in words.
    public static func field(_ key: String) -> String {
        switch key {
        case "error.length": tr("1–1000 characters")
        case "error.required": tr("Required")
        case "error.range": tr("1–15 minutes")
        case "error.unsupportedAction": tr("Not supported by this AC")
        case "error.invalid": tr("Not valid")
        default: key.replacingOccurrences(of: "errors.", with: "").replacingOccurrences(of: "_", with: " ")
        }
    }
}
