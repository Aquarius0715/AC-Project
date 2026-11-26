import ACKit
import ACShared
import Foundation

/// The overview over the technician's jobs (FR-T01, DD-T01, the web's techOverview.ts): the tiles, the open alerts on
/// the assigned units and the reports by state.
public enum OverviewWords {
    /// One tile: its label, the number and the line under it.
    public struct Tile: Equatable, Sendable {
        public let label: String
        public let value: String
        public let sub: String
    }

    /// One row of the alerts or the reports; it opens the job.
    public struct Row: Identifiable, Equatable, Sendable {
        public let id: String
        public let jobId: String
        public let title: String
        public let sub: String
        public let badge: String
    }

    /// The tiles of the technician summary: the assigned units, the jobs not started (the first three named) and the
    /// overdue ones.
    public static func tiles(_ counts: SummaryCounts, _ jobs: [JobSummary]) -> [Tile] {
        let waiting = jobs.filter { $0.status == .assigned }.prefix(3).map { String($0.id.prefix(8)) }
        return [
            Tile(label: tr("Assigned units"), value: count(counts.total), sub: tr("in your assignments")),
            Tile(
                label: tr("Not started"),
                value: count(counts.scheduledCount),
                sub: waiting.isEmpty ? tr("No jobs waiting to start") : waiting.joined(separator: ", ")
            ),
            Tile(
                label: tr("Overdue"),
                value: count(counts.overdueCount),
                sub: counts.overdueCount > 0 ? tr("past the due date") : tr("No overdue jobs")
            ),
        ]
    }

    private static func count(_ value: Double) -> String { String(Int(value)) }

    /// The current (not completed or cancelled) job of each assigned unit, the first in the list's order.
    public static func jobOfUnit(_ jobs: [JobSummary]) -> [String: String] {
        var out: [String: String] = [:]
        for job in jobs where job.status != .completed && job.status != .cancelled && out[job.unitId] == nil {
            out[job.unitId] = job.id
        }
        return out
    }

    /// The unresolved alerts on the units with a current job, worst first, then newest; each opens its job. The Core API
    /// lists every unit the technician may see, which is wider than the assignments.
    public static func alerts(_ alerts: [CoreAlert], names: [String: String], jobs: [JobSummary]) -> [Row] {
        let jobOfUnit = jobOfUnit(jobs)
        return alerts.filter { $0.status != .resolved }
            .sorted {
                let left = UnitWords.rank($0.severity)
                let right = UnitWords.rank($1.severity)
                return left == right ? $0.detectedAt > $1.detectedAt : left < right
            }
            .compactMap { alert in
                guard let job = jobOfUnit[alert.unitId] else { return nil }
                let state = alert.status == .open ? tr("open") : tr("acknowledged")
                let sub = "\(names[alert.unitId] ?? tr("unit")) · \(state) · \(Format.dateTime(alert.detectedAt))"
                return Row(
                    id: alert.id,
                    jobId: job,
                    title: UnitWords.alertTitle(alert),
                    sub: sub,
                    badge: UnitWords.severity(alert.severity)
                )
            }
    }

    /// The reports by state: drafts in progress, returned for rework and submitted for review.
    public static func reports(_ jobs: [JobSummary], names: [String: String]) -> [Row] {
        jobs.compactMap { job in
            let state: (String, String)
            switch job.status {
            case .inProgress: state = (tr("Draft"), tr("draft in progress"))
            case .reworkRequested: state = (tr("Rework"), tr("returned — resume as a new version"))
            case .submitted: state = (tr("Submitted"), tr("awaiting quality review · read-only"))
            default: return nil
            }
            let title = "\(job.id.prefix(8)) · \(names[job.unitId] ?? tr("Unit"))"
            return Row(id: job.id, jobId: job.id, title: title, sub: state.1, badge: state.0)
        }
    }
}
