import ACKit
import Foundation

/// The words of the AC's settings, commands and readings, as the web apps word them (shared/lib/units.ts,
/// commandFailure.ts, the customer unit page): one switch per value, so every text is a literal the generator finds.
public enum Words {
    /// 24 → "24", 24.5 → "24.5": a setpoint as the web writes it.
    public static func celsius(_ value: Double) -> String {
        value.rounded() == value && abs(value) < 1e15 ? String(Int64(value)) : String(value)
    }

    public static func mode(_ mode: Mode) -> String {
        switch mode {
        case .cool: tr("Cool")
        case .dry: tr("Dry")
        case .fan: tr("Fan")
        }
    }

    public static func fan(_ fan: Fan) -> String {
        switch fan {
        case .low: tr("Low")
        case .mid: tr("Mid")
        case .high: tr("High")
        }
    }

    /// A command as a sentence ("Set temperature 24°C").
    public static func action(_ action: UnitAction) -> String {
        switch action {
        case let .setPower(change): tr(change.power ? "Set power ON" : "Set power OFF")
        case let .setTemperature(change): tr("Set temperature {celsius}°C", ["celsius": celsius(change.celsius)])
        case let .setMode(change): tr("Set mode {mode}", ["mode": mode(change.mode).uppercased()])
        case let .setFan(change): tr("Set fan {level}", ["level": fan(change.fanLevel).uppercased()])
        case let .ventilate(change): tr("Set ventilation {level}", ["level": fan(change.level).uppercased()])
        }
    }

    /// A command of the history, which may also be a restriction's.
    public static func command(_ action: CommandAction) -> String {
        switch action {
        case let .setPower(change): self.action(UnitAction.setPower(change))
        case let .setTemperature(change): self.action(UnitAction.setTemperature(change))
        case let .setMode(change): self.action(UnitAction.setMode(change))
        case let .setFan(change): self.action(UnitAction.setFan(change))
        case let .ventilate(change): self.action(UnitAction.ventilate(change))
        case let .applyRestriction(restriction): tr("Apply restriction: {policy}", ["policy": policy(restriction.policy)])
        case .removeRestriction: tr("Remove restriction")
        }
    }

    public static func policy(_ policy: RestrictionPolicy) -> String {
        switch policy {
        case .powerOff: tr("Power off")
        case let .temperatureLimit(limit):
            tr("Temperature limit — cooling setpoint ≥ {value} °C", ["value": celsius(limit.minimumCoolingSetpoint)])
        }
    }

    public static func status(_ status: CommandStatus) -> String {
        switch status {
        case .requested: tr("requested")
        case .sent: tr("waiting for device")
        case .acknowledged: tr("acknowledged by device")
        case .failed: tr("failed")
        case .expired: tr("no device response (expired)")
        case .cancelled: tr("cancelled")
        }
    }

    /// Why a command to an AC failed (IR799): the stored code is never shown as it is.
    public static func failure(_ code: CommandFailureCode) -> String {
        switch code {
        case .deviceRejected: tr("the AC refused it")
        case .timeout: tr("no answer from the AC in time")
        case .offline: tr("the AC was offline")
        case .unavailable: tr("the AC reported a fault")
        case .conflict: tr("another command was still running")
        }
    }

    /// One history line: the sentence, its state, why it failed and who sent it ("by you", "by someone else", IR804:
    /// several people use one account; no name is shown).
    public static func history(_ command: Command, me: String?) -> String {
        var text = "\(self.command(command.action)) — \(status(command.status))"
        if command.status == .failed, let code = command.failureCode { text += ": \(failure(code))" }
        switch command.source {
        case .automation: text += tr(" · by an automation")
        case .restriction: text += tr(" · by a restriction")
        case .diagnostic: text += tr(" · technician test run")
        case .ui, .group:
            if let me { text += command.actorMembershipId == me ? tr(" · by you") : tr(" · by someone else") }
        }
        return text
    }

    /// Why a send was refused, in the customer unit page's words where it has them, else the refusal's line.
    public static func refusal(_ error: CoreError) -> String {
        switch error.error.messageKey {
        case "errors.unit_busy": tr("another command is still waiting for this AC")
        case "error.versionConflict": tr("this AC changed — reload and try again")
        case "control.restriction_active": tr("a service restriction limits this setting")
        case "control.reconciliation_required": tr("HQ is checking this AC")
        default: ErrorText.of(error)
        }
    }

    /// A reading ("28.0 °C"): temperature and humidity with one decimal, ppm and µg/m³ whole (IR44); a value that is not
    /// valid keeps its quality beside it, never shown as a current value (D07). Nil without a value.
    public static func reading(_ measurement: CoreMeasurement) -> String? {
        guard let value = measurement.value else { return nil }
        let digits: Int? =
            switch measurement.metric {
            case .temperature, .humidity: 1
            case .co2, .pm25: 0
            default: nil
            }
        let number =
            digits.map { value.formatted(.number.precision(.fractionLength($0)).locale(Locale(identifier: "en"))) } ?? celsius(value)
        let quality: String? =
            switch measurement.quality {
            case .valid: nil
            case .missing: tr("missing")
            case .stale: tr("stale")
            case .suspect: tr("suspect")
            }
        return quality.map { "\(number) \(measurement.unit.rawValue) (\($0))" } ?? "\(number) \(measurement.unit.rawValue)"
    }
}
