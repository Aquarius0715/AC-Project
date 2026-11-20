import ACKit
import ACShared
import Foundation
import Observation

/// The jobs assigned to the signed-in technician (FR-T01, jobs.list: the Core API returns only theirs,
/// technician:assigned-history), the open ones first by due date, then the past ones; each row says what the technician
/// can do now (the web's overview, techOverview.ts techRow).
@MainActor @Observable
public final class JobsModel {
    /// One job as its row shows it.
    public struct Row: Identifiable, Equatable, Sendable {
        public enum Tone: Sendable { case info, ok, warn, critical, muted }
        public let id: String
        /// The unit's name (or the job type of a past job).
        public let title: String
        public let badge: String
        public let tone: Tone
        public let line: String
        public let isPast: Bool
        /// A new assignment the technician has not answered yet.
        public let needsAnswer: Bool
    }

    public private(set) var jobs: Loadable<[Row]> = .loading
    let client: CoreClient

    public init(client: CoreClient) {
        self.client = client
    }

    public var needingAnswer: [Row] { (jobs.value ?? []).filter(\.needsAnswer) }

    /// The new assignments to answer, over the list (the web overview's banner).
    public var answerLine: String? {
        let rows = needingAnswer
        guard let first = rows.first else { return nil }
        if rows.count == 1 {
            let job = tr("New assignment to accept — {job}", ["job": "\(first.id.prefix(8)) · \(first.title)"])
            return "\(job). \(tr("The visit time was agreed with the client; accept it or say you can’t make it."))"
        }
        let count = tr("{n} new assignments to accept", ["n": String(rows.count)])
        return "\(count) — \(tr("The visit times were agreed with the client."))"
    }

    public func load() async {
        jobs = await jobs.after { () async throws(CoreError) in
            let answer = try await client.call(Ops.JobsList.self, Query(limit: 100, sort: QuerySort(field: .dueAt, direction: .asc)))
            let now = answer.meta.snapshotAt.flatMap(Format.parse) ?? Date()
            // the names of the assigned units; without them a row says "Unit"
            let units = try? await client.call(Ops.UnitsList.self, Query(limit: 100)).data.items
            let names = Dictionary((units ?? []).map { ($0.id, $0.displayName) }) { first, _ in first }
            let rows = answer.data.items.compactMap { Self.row($0, now: now, names: names) }
            return rows.filter { !$0.isPast } + rows.filter(\.isPast)
        }
    }

    static func row(_ item: JobsListResultItem, now: Date, names: [String: String]) -> Row? {
        switch item {
        case let .summary(job): open(job, now: now, names: names)
        case let .history(job): past(job)
        case .offer: nil  // an offer is the contractor's, never a technician's
        }
    }

    /// A past job: its history snapshot. One whose window ended with nobody having finished it says who to ask (IR642).
    static func past(_ job: JobHistorySnapshot) -> Row {
        let unfinished = job.completedAt == nil && (job.status == .assigned || job.status == .inProgress)
        return Row(
            id: job.jobId,
            title: Labels.jobType(job.type),
            badge: unfinished ? tr("Work window ended") : Labels.job(job.status),
            tone: unfinished ? .critical : .muted,
            line: unfinished ? tr("ask your contractor or HQ to extend or reassign") : Format.dateTime(job.completedAt ?? job.asOf),
            isPast: true,
            needsAnswer: false
        )
    }

    /// An assigned job: what the technician can do now, from its status, the acknowledgement and the work window.
    static func open(_ job: JobSummary, now: Date, names: [String: String]) -> Row {
        let slot = job.scheduledSlot
        let start = slot.flatMap { Format.parse($0.startAt) }
        let end = slot.flatMap { Format.parse($0.endAt) }
        let isWorking = job.status == .assigned || job.status == .inProgress
        let isBefore = start.map { $0 > now } ?? false
        let isEnded = isWorking && (end.map { $0 <= now } ?? false)
        let standing = state(job, opens: isBefore ? slot?.startAt : nil, isEnded: isEnded)
        var alert = ""
        if job.severity == .critical { alert = tr("critical alert on the unit") }
        if job.severity == .warning { alert = tr("warning on the unit") }
        var ending = ""
        if isWorking, !isBefore, !isEnded, let end {
            let left = Int((end.timeIntervalSince(now) / 60).rounded(.up))
            if left <= JobWords.warnMinutes { ending = tr("window ends in {n} min", ["n": String(left)]) }
        }
        let window = slot.map { tr("window {slot}", ["slot": Format.span($0.startAt, $0.endAt)]) } ?? ""
        return Row(
            id: job.id,
            title: names[job.unitId] ?? tr("Unit"),
            badge: standing.badge,
            tone: standing.tone,
            line: [Labels.jobType(job.type), window, alert, standing.note, ending].filter { !$0.isEmpty }.joined(separator: " · "),
            isPast: false,
            needsAnswer: job.status == .assigned && job.assignmentAcknowledgement == .pending
        )
    }

    /// What a row says of its job: the badge, its tone and the note of what to do.
    private struct Standing {
        let badge: String
        let tone: Row.Tone
        let note: String

        init(_ badge: String, _ tone: Row.Tone, _ note: String) {
            self.badge = badge
            self.tone = tone
            self.note = note
        }
    }

    /// - Parameter opens: the window's start when it has not begun yet.
    private static func state(_ job: JobSummary, opens: String?, isEnded: Bool) -> Standing {
        let answer = job.status == .assigned ? job.assignmentAcknowledgement : nil
        if answer == .pending { return Standing(tr("Not accepted"), .warn, tr("new assignment — review and accept it")) }
        if answer == .cantMake {
            return Standing(tr("Can’t make it"), .warn, tr("you said you can’t make it — waiting for your contractor or HQ"))
        }
        if isEnded { return Standing(tr("Work window ended"), .critical, tr("ask your contractor or HQ to extend or reassign")) }
        switch job.status {
        case .assigned:
            let note = opens.map { tr("opens {time} (read-only until then)", ["time": Format.dateTime($0)]) } ?? tr("check in to start")
            return Standing(tr("Work not started"), .info, note)
        case .inProgress: return Standing(tr("In progress"), .warn, tr("report draft open"))
        case .onHold: return Standing(tr("On hold"), .warn, tr("held by HQ — actions are blocked"))
        case .reworkRequested: return Standing(tr("Rework requested"), .warn, tr("returned — resume in a new draft version"))
        case .submitted: return Standing(tr("Submitted"), .info, tr("awaiting quality review (read-only)"))
        case .completed: return Standing(tr("Completed"), .ok, tr("accepted in the quality review"))
        case .requested, .offered, .accepted, .cancelled: return Standing(Labels.job(job.status), .muted, "")
        }
    }
}
