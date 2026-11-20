import ACTestSupport
import Foundation
import Testing

@testable import ACKit

/// The generated types read real answers of the Core API (Fixtures: the dev stack's answers to the iOS clients' tokens,
/// IR860), and write them back to the same JSON: a contract the generator maps wrongly, or an API that drifted from the
/// contract, fails here.
@Suite struct ContractTests {
    private struct Envelope<Result: Codable>: Codable {
        let data: Result
    }

    private func roundTrip<Result: Codable & Equatable>(_: Result.Type, _ name: String) throws -> Result {
        let data = try fixture(name)
        let decoded = try JSONDecoder().decode(Envelope<Result>.self, from: data).data
        let again = try JSONDecoder().decode(Result.self, from: JSONEncoder().encode(decoded))
        #expect(again == decoded)
        // the encoded result is the same JSON as the answer's data (no field lost, no null turned into a missing key)
        let original = try JSONDecoder().decode([String: JSONValue].self, from: data)["data"]
        #expect(try JSONValue.of(decoded) == original)
        return decoded
    }

    @Test func customerSession() throws {
        let session = try roundTrip(Ops.SessionGet.Result.self, "session.get.customer")
        #expect(session.role == .client)
    }

    @Test func customerPreferences() throws {
        _ = try roundTrip(Ops.PreferencesGet.Result.self, "preferences.get.customer")
    }

    @Test func unitsOfACustomerAndOfATechnician() throws {
        #expect(try !roundTrip(Ops.UnitsList.Result.self, "units.list.customer").items.isEmpty)
        #expect(try !roundTrip(Ops.UnitsList.Result.self, "units.list.technician").items.isEmpty)
    }

    @Test func unitDetail() throws {
        let unit = try roundTrip(Ops.UnitsGet.Result.self, "units.get.customer")
        #expect(!unit.id.isEmpty)
    }

    @Test func properties() throws {
        _ = try roundTrip(Ops.PropertiesList.Result.self, "properties.list.customer")
    }

    @Test func commandsAndNotifications() throws {
        _ = try roundTrip(Ops.CommandsList.Result.self, "commands.list.customer")
        _ = try roundTrip(Ops.NotificationsList.Result.self, "notifications.list.customer")
    }

    @Test func technicianJobs() throws {
        let jobs = try roundTrip(Ops.JobsList.Result.self, "jobs.list.technician")
        #expect(!jobs.items.isEmpty)
        let job = try roundTrip(Ops.JobsGet.Result.self, "jobs.get.technician.history")
        guard case .history = job else {
            Issue.record("a past job reads as its history snapshot")
            return
        }
    }

    @Test func wrongConstantIsRefused() {
        let body = Data(#"{"kind":"set_mode","power":true}"#.utf8)
        #expect(throws: DecodingError.self) { try JSONDecoder().decode(UnitActionSetPower.self, from: body) }
        #expect(throws: DecodingError.self) { try JSONDecoder().decode(UnitAction.self, from: Data(#"{"kind":"fly"}"#.utf8)) }
    }

    @Test func actionsEncodeWithTheirTag() throws {
        let action = UnitAction.setTemperature(UnitActionSetTemperature(celsius: 24))
        #expect(try JSONValue.of(action) == .object(["kind": .string("set_temperature"), "celsius": .number(24)]))
        #expect(try JSONDecoder().decode(UnitAction.self, from: JSONEncoder().encode(action)) == action)
    }

    @Test func jsonValueReadsEveryKind() throws {
        let text = #"{"a":null,"b":true,"c":1.5,"d":"x","e":[1,"y"],"f":{"g":false}}"#
        let value = try JSONDecoder().decode(JSONValue.self, from: Data(text.utf8))
        #expect(
            value
                == .object([
                    "a": .null, "b": .bool(true), "c": .number(1.5), "d": .string("x"), "e": .array([.number(1), .string("y")]),
                    "f": .object(["g": .bool(false)]),
                ])
        )
        #expect(try JSONDecoder().decode(JSONValue.self, from: JSONEncoder().encode(value)) == value)
    }
}
