import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import CustomerFeature

/// The AC control against the UI tests' stub (an AC that takes every setting), and against scripted answers for the
/// refusals and the answers that never come.
@MainActor @Suite struct UnitControlModelTests {
    private func stubbed() async throws -> UnitControlModel {
        let configuration = try AppConfiguration(
            info: [
                "ACCoreAPIURL": "http://stub", "ACIssuerURL": "http://stub", "ACClientID": "c",
                "ACRedirectURI": "ac-customer://oauth/callback",
            ],
            role: "client"
        )
        let session = UITestStub.session(configuration: configuration)
        await session.signIn(using: UITestStub.signIn)
        let model = UnitControlModel(unitId: "u1", client: session.client, me: "m1", pause: Self.noWait)
        await model.load()
        return model
    }

    @Test func controlsStartFromTheConfirmedSetting() async throws {
        let model = try await stubbed()
        #expect(model.setpoint == 26)
        #expect(model.mode == .cool)
        #expect(model.fan == .mid)
        #expect(model.isOn)
        #expect(
            model.layout
                == UnitControlModel.Layout(
                    control: true,
                    temperature: CapabilityTemperature(min: 16, max: 30, step: 1),
                    modes: [.cool, .dry, .fan],
                    fans: [.low, .mid, .high]
                )
        )
        #expect(model.changes.isEmpty)
        #expect(model.changeSummary == "No change yet")
        #expect(!model.isDisabled)
        #expect(!model.isPowerLocked)
    }

    @Test func onlyWhatChangedIsSent() async throws {
        let model = try await stubbed()
        model.lower()
        model.raise()
        model.choose(mode: .cool)
        #expect(model.changes.isEmpty, "back to the confirmed setting is no change")
        model.lower()
        model.lower()
        model.choose(mode: .dry)
        model.choose(fan: .high)
        #expect(
            model.changes == [
                .setTemperature(UnitActionSetTemperature(celsius: 24)), .setMode(UnitActionSetMode(mode: .dry)),
                .setFan(UnitActionSetFan(fanLevel: .high)),
            ]
        )
        #expect(model.changeSummary == "Set temperature 26°C → 24°C · Mode → Dry · Fan → High")
    }

    @Test func sendIsConfirmedThenUndone() async throws {
        let model = try await stubbed()
        model.lower()
        model.lower()
        await model.sendChanges()
        #expect(model.outcome?.text == "Living room AC: Set temperature 24°C — acknowledged by device")
        #expect(model.outcome?.isOk == true)
        #expect(model.outcome?.undo == [.setTemperature(UnitActionSetTemperature(celsius: 26))])
        #expect(model.unit.value?.observedState.celsius == 24)
        #expect(model.setpoint == 24)
        #expect(model.historyRows.first?.text == "Set temperature 24°C — acknowledged by device · by you")
        await model.undo()
        #expect(model.outcome?.text == "Living room AC: Set temperature 26°C — acknowledged by device")
        #expect(model.outcome?.undo.isEmpty == true, "an undo offers no undo of its own")
        #expect(model.unit.value?.observedState.celsius == 26)
    }

    @Test func powerIsSentOnItsOwn() async throws {
        let model = try await stubbed()
        await model.sendPower(false)
        #expect(model.outcome?.text == "Living room AC: Set power OFF — acknowledged by device")
        #expect(model.outcome?.undo == [.setPower(UnitActionSetPower(power: true))])
        #expect(!model.isOn)
    }

    @Test func steppingStopsAtTheRange() async throws {
        let model = try await stubbed()
        for _ in 0..<20 { model.raise() }
        #expect(model.setpoint == 30)
        #expect(!model.canRaise)
        for _ in 0..<20 { model.lower() }
        #expect(model.setpoint == 16)
        #expect(!model.canLower)
        #expect(model.coolingHint?.hasPrefix("16°C is very cold for cooling") == true)
        model.choose(mode: .dry)
        #expect(model.coolingHint == nil, "only cooling has the hint")
    }
}
