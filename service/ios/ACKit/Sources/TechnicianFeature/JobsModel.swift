import ACKit
import ACShared
import Observation

/// The jobs assigned to the signed-in technician (jobs.list: the Core API returns only theirs, technician:assigned-history),
/// the open ones first by due date, then the past ones.
@MainActor @Observable
public final class JobsModel {
    /// One job as its row shows it.
    public struct Row: Identifiable, Equatable, Sendable {
        public let id: String
        public let title: String
        public let status: String
        public let due: String
        public let isPast: Bool
    }

    public private(set) var jobs: Loadable<[Row]> = .loading
    let client: CoreClient

    public init(client: CoreClient) {
        self.client = client
    }

    public func load() async {
        jobs = await jobs.after { () async throws(CoreError) in
            let page = try await client.call(Ops.JobsList.self, Query(limit: 100, sort: QuerySort(field: .dueAt, direction: .asc)))
            let rows = page.data.items.compactMap(Self.row)
            return rows.filter { !$0.isPast } + rows.filter(\.isPast)
        }
    }

    /// An assigned job, or a past one (its history snapshot); an offer is the contractor's, never a technician's.
    static func row(_ item: JobsListResultItem) -> Row? {
        switch item {
        case let .summary(job):
            Row(
                id: job.id,
                title: Labels.jobType(job.type),
                status: Labels.job(job.displayStatus),
                due: tr("Due {time}", ["time": Format.dateTime(job.dueAt)]),
                isPast: false
            )
        case let .history(job):
            Row(
                id: job.jobId,
                title: Labels.jobType(job.type),
                status: Labels.job(job.status),
                due: Format.dateTime(job.completedAt ?? job.asOf),
                isPast: true
            )
        case .offer:
            nil
        }
    }
}
