import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import CustomerFeature

@MainActor @Suite struct UnitsModelTests {
    private func client(_ answers: [StubTransport.Answer]) -> (CoreClient, StubTransport) {
        let transport = StubTransport(answers)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        return (CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials()), transport)
    }

    @Test func listsTheCustomersACsByName() async throws {
        let answer = try #require(String(bytes: try fixture("units.list.customer"), encoding: .utf8))
        let (client, transport) = client([.http(200, answer)])
        let model = UnitsModel(client: client)
        await model.load()
        let rows = try #require(model.units.value)
        #expect(!rows.isEmpty)
        #expect(rows.allSatisfy { !$0.name.isEmpty && !$0.power.isEmpty && !$0.connection.isEmpty })
        let asked = try #require(await transport.requests.first?.url)
        #expect(query(asked.path() + "?" + (asked.query() ?? "")).values == ["limit": ["100"], "sort": ["name:asc"]])
    }

    @Test func rowSaysTheACsPowerAndConnection() throws {
        let page = try JSONDecoder().decode(Envelope.self, from: try fixture("units.list.customer")).data
        let unit = try #require(page.items.first)
        #expect(UnitsModel.row(unit).name == unit.displayName)
        var online = unit
        online.connection = .online
        online.effectivePowerState = .on
        #expect(UnitsModel.row(online) == .init(id: unit.id, name: unit.displayName, power: "On", connection: "Online"))
    }

    @Test func failedReloadKeepsTheRows() async throws {
        let answer = try #require(String(bytes: try fixture("units.list.customer"), encoding: .utf8))
        let (client, _) = client([.http(200, answer), .failure(URLError(.notConnectedToInternet))])
        let model = UnitsModel(client: client)
        await model.load()
        let shown = model.units.value
        await model.load()
        #expect(model.units.value == shown)
        #expect(model.units.message == "No connection — check the network and try again")
    }

    private struct Envelope: Decodable {
        let data: Page<UnitSummary>
    }
}

func fixture(_ name: String) throws -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures") else {
        throw CocoaError(.fileNoSuchFile)
    }
    return try Data(contentsOf: url)
}
