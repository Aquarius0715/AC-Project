import ACKit
import ACShared
import Foundation

/// Where the technician is in the assignment's work window (shared/lib/techJob.ts windowState): before it (read-only),
/// in it, in its last 15 minutes, or after it.
public enum WindowPhase: Sendable, Equatable {
    case noWindow, before, open, ending, ended
}

/// The work window in words: its phase, its line, and the notice of its last minutes.
public struct WorkWindow: Sendable, Equatable {
    public let phase: WindowPhase
    public let text: String
    public let notice: String?
}

/// The job workspace's texts as the web's technician job page words them (techJob.ts, workspace-view.tsx).
public enum JobWords {
    /// The minutes before the window's end when the work window warns.
    static let warnMinutes = 15

    public static func window(_ assignment: Assignment?, now: Date) -> WorkWindow {
        guard let assignment, let start = Format.parse(assignment.scheduledStart), let end = Format.parse(assignment.scheduledEnd)
        else {
            return WorkWindow(phase: .noWindow, text: tr("No work window"), notice: nil)
        }
        let range = Format.span(assignment.scheduledStart, assignment.scheduledEnd)
        if now < start {
            let text = tr("Starts {time} · {range}", ["time": Format.dateTime(assignment.scheduledStart), "range": range])
            return WorkWindow(phase: .before, text: text, notice: nil)
        }
        if now >= end {
            let text = tr("Ended {time} · {range}", ["time": Format.dateTime(assignment.scheduledEnd), "range": range])
            return WorkWindow(phase: .ended, text: text, notice: nil)
        }
        let warn = end.addingTimeInterval(TimeInterval(-warnMinutes * 60)).ISO8601Format()
        let text = tr(
            "{range} · warning at {warn} · discard at {end}",
            ["range": range, "warn": Format.clock(warn), "end": Format.clock(assignment.scheduledEnd)]
        )
        let left = Int((end.timeIntervalSince(now) / 60).rounded(.up))
        guard left <= warnMinutes else { return WorkWindow(phase: .open, text: text, notice: nil) }
        let notice = tr(
            "Work window ends in {n} min — unsaved input will be discarded at {time}.",
            ["n": String(left), "time": Format.clock(assignment.scheduledEnd)]
        )
        return WorkWindow(phase: .ending, text: text, notice: notice)
    }

    /// The accepted assignment's line: before the window, after it, or on site.
    static func accepted(_ window: WorkWindow, opens: String) -> String {
        switch window.phase {
        case .before: tr("Read-only until the window opens at {time} — Start is disabled before then.", ["time": opens])
        case .ended: tr("The work window has ended — ask your coordinator to reschedule.")
        case .noWindow, .open, .ending: tr("On site: Start job opens Check in (location + unit QR); checking in starts the work.")
        }
    }

    /// How the job came: a periodic plan, HQ, or the client.
    static func origin(_ origin: JobOrigin) -> String {
        switch origin {
        case .periodicPlan: tr("periodic plan")
        case .hqRequest: tr("HQ request")
        case .clientRequest: tr("client request")
        }
    }

    /// The time on site (FR-T13, T14): arrival and how, the place, start, pauses, finish and the counted minutes.
    public static func timeRows(_ time: TimeOnSite?, now: Date) -> [(String, String)] {
        guard let time, let arrived = time.arrivedAt ?? time.startedAt, let from = Format.parse(arrived) else {
            return [(tr("Arrived"), tr("not checked in"))]
        }
        let open = time.pauses.first { $0.to == nil }
        let paused = time.pauses.reduce(0.0) { sum, pause in
            let start = Format.parse(pause.from) ?? now
            return sum + max(0, (pause.to.flatMap(Format.parse) ?? now).timeIntervalSince(start))
        }
        let until = time.finishedAt.flatMap(Format.parse) ?? now
        let minutes = time.onSiteMinutes.map { Int($0) } ?? max(0, Int((until.timeIntervalSince(from) - paused) / 60))
        let how = time.checkInMethod == .manual ? tr("manual check-in") : tr("checked in")
        return [
            (tr("Arrived"), "\(Format.clock(arrived)) · \(how)"),
            (tr("Location"), place(time)),
            (tr("Started"), time.startedAt.map { Format.clock($0) } ?? "—"),
            (tr("Paused"), open.map { tr("since {time}", ["time": Format.clock($0.from)]) } ?? pauses(time.pauses.count, paused)),
            (tr("Finished"), time.finishedAt.map { Format.clock($0) } ?? tr("— (on submit)")),
            (tr("On-site time"), duration(minutes)),
        ]
    }

    /// How the technician checked in: manually with the reason, or by QR at a distance from the site.
    static func place(_ time: TimeOnSite) -> String {
        if time.checkInMethod == .manual {
            return tr("manual — {reason}", ["reason": time.checkInReason ?? tr("no reason")])
        }
        guard let meters = time.distanceMeters else { return "—" }
        return tr("{m} m from site · QR matched", ["m": String(Int(meters.rounded()))])
    }

    /// The ended pauses: how many and how long in all.
    static func pauses(_ count: Int, _ seconds: Double) -> String {
        guard count > 0 else { return "—" }
        let times = count == 1 ? tr("1 pause") : tr("{n} pauses", ["n": String(count)])
        return "\(times) · \(duration(Int((seconds / 60).rounded())))"
    }

    static func duration(_ minutes: Int) -> String {
        minutes >= 60
            ? tr("{h} h {m} min", ["h": String(minutes / 60), "m": String(minutes % 60)]) : tr("{n} min", ["n": String(minutes)])
    }

    /// A job whose viewing window ended (IR49, IR124): completed with the job, or the assignment ended or changed.
    public struct History: Sendable, Equatable {
        public let title: String
        public let subtitle: String
        public let rows: [Row]
        public let note: String

        public struct Row: Sendable, Equatable {
            public let label: String
            public let value: String
        }
    }

    public static func history(_ snapshot: JobHistorySnapshot) -> History {
        let isDone = snapshot.status == .completed
        let completed = snapshot.completedAt.map { tr("Completed {time}", ["time": Format.date($0)]) } ?? tr("Completed")
        let status = isDone ? completed : tr("{status} when your assignment ended", ["status": Labels.job(snapshot.status)])
        let report: String =
            switch snapshot.redactedReportSummary {
            case .false: tr("No report from your assignment")
            case let .true(summary) where summary.acceptance == .accepted: tr("Accepted in the quality review")
            case .true: tr("Submitted — not accepted while you were assigned")
            }
        let note: String =
            switch snapshot.status {
            case .completed:
                tr("Your time is free for other jobs. The report, the unit and its devices stay with HQ and the customer.")
            case .cancelled: tr("The job was cancelled, so the job details and the unit are no longer shown to you.")
            default:
                tr(
                    "Your work window ended, or HQ or your coordinator changed the assignment, so the job details and the unit are no longer shown to you."
                )
            }
        return History(
            title: "\(snapshot.jobId.prefix(8)) · \(Labels.jobType(snapshot.type))",
            subtitle: isDone ? tr("Completed — your assignment ended with the job") : tr("Your assignment on this job has ended"),
            rows: [
                History.Row(label: tr("Status"), value: status),
                History.Row(label: tr("Your report"), value: report),
                History.Row(label: tr("Assignment ended"), value: Format.dateTime(snapshot.asOf)),
            ],
            note: note
        )
    }
}
