import ACKit
import ACShared
import Observation

/// The customer's air conditioners (units.list): one row per AC with its power and connection.
@MainActor @Observable
public final class UnitsModel {
    /// One AC as its row shows it.
    public struct Row: Identifiable, Equatable, Sendable {
        public let id: String
        public let name: String
        public let power: String
        public let connection: String
    }

    public private(set) var units: Loadable<[Row]> = .loading
    let client: CoreClient

    public init(client: CoreClient) {
        self.client = client
    }

    /// Reads the ACs again; a failed read keeps the rows shown before, with the refusal's line.
    public func load() async {
        units = await units.after { () async throws(CoreError) in
            // units.list sorts by no name (the API refuses sort=name): the rows are put in order here, as a person reads them
            let page = try await client.call(Ops.UnitsList.self, Query(limit: 100))
            return page.data.items.filter { !$0.archived }.map(Self.row).sorted {
                $0.name.localizedStandardCompare($1.name) == .orderedAscending
            }
        }
    }

    static func row(_ unit: UnitSummary) -> Row {
        Row(
            id: unit.id,
            name: unit.displayName,
            power: Labels.power(unit.effectivePowerState),
            connection: Labels.connection(unit.connection)
        )
    }
}
