import ACShared
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

@Suite struct JobWordsTests {
    private let start = "2026-12-01T01:00:00Z"
    private let end = "2026-12-01T04:00:00Z"

    private func assignment(start: String? = nil, end: String? = nil) -> Assignment {
        Assignment(
            id: "a1",
            tenantId: "t1",
            version: 1,
            createdAt: start ?? self.start,
            updatedAt: start ?? self.start,
            jobId: "j1",
            technicianMembershipId: "m1",
            validFrom: start ?? self.start,
            validUntil: end ?? self.end,
            scheduledStart: start ?? self.start,
            scheduledEnd: end ?? self.end,
            status: .active,
            reason: nil,
            acknowledgement: .accepted,
            acknowledgedAt: nil,
            cantMakeReason: nil,
            alternativeSlot: nil,
            technicianName: nil
        )
    }

    private func at(_ instant: String) throws -> Date { try #require(Format.parse(instant)) }

    @Test func theWindowIsBeforeInEndingOrAfter() throws {
        let range = Format.span(start, end)
        #expect(JobWords.window(nil, now: Date()) == WorkWindow(phase: .noWindow, text: "No work window", notice: nil))
        #expect(
            JobWords.window(assignment(start: "soon"), now: Date())
                == WorkWindow(phase: .noWindow, text: "No work window", notice: nil)
        )
        let before = JobWords.window(assignment(), now: try at("2026-12-01T00:00:00Z"))
        #expect(before == WorkWindow(phase: .before, text: "Starts \(Format.dateTime(start)) · \(range)", notice: nil))
        let after = JobWords.window(assignment(), now: try at(end))
        #expect(after == WorkWindow(phase: .ended, text: "Ended \(Format.dateTime(end)) · \(range)", notice: nil))
        let text = "\(range) · warning at \(Format.clock("2026-12-01T03:45:00Z")) · discard at \(Format.clock(end))"
        #expect(JobWords.window(assignment(), now: try at(start)) == WorkWindow(phase: .open, text: text, notice: nil))
        let ending = JobWords.window(assignment(), now: try at("2026-12-01T03:50:30Z"))
        #expect(ending.phase == .ending)
        #expect(ending.notice == "Work window ends in 10 min — unsaved input will be discarded at \(Format.clock(end)).")
    }

    @Test func anAcceptedAssignmentSaysWhenItCanStart() {
        let open = WorkWindow(phase: .open, text: "", notice: nil)
        #expect(
            JobWords.accepted(WorkWindow(phase: .before, text: "", notice: nil), opens: "9:00")
                == "Read-only until the window opens at 9:00 — Start is disabled before then."
        )
        #expect(
            JobWords.accepted(WorkWindow(phase: .ended, text: "", notice: nil), opens: "")
                == "The work window has ended — ask your coordinator to reschedule."
        )
        #expect(
            JobWords.accepted(open, opens: "") == "On site: Start job opens Check in (location + unit QR); checking in starts the work."
        )
    }

    @Test func theOriginInWords() {
        #expect(JobWords.origin(.periodicPlan) == "periodic plan")
        #expect(JobWords.origin(.hqRequest) == "HQ request")
        #expect(JobWords.origin(.clientRequest) == "client request")
    }

    @Test func timeOnSiteBeforeTheCheckIn() throws {
        #expect(JobWords.timeRows(nil, now: Date()).map(\.1) == ["not checked in"])
        let none = TimeOnSite(
            arrivedAt: nil,
            checkInMethod: nil,
            checkInReason: nil,
            distanceMeters: nil,
            startedAt: nil,
            pauses: [],
            finishedAt: nil,
            onSiteMinutes: nil
        )
        #expect(JobWords.timeRows(none, now: Date()).map(\.0) == ["Arrived"])
    }

    @Test func timeOnSiteWhilePaused() throws {
        let time = TimeOnSite(
            arrivedAt: "2026-12-01T01:00:00Z",
            checkInMethod: .locationQr,
            checkInReason: nil,
            distanceMeters: 18.4,
            startedAt: "2026-12-01T01:00:00Z",
            pauses: [TimeOnSitePauses(from: "2026-12-01T01:30:00Z", to: nil)],
            finishedAt: nil,
            onSiteMinutes: nil
        )
        let rows = JobWords.timeRows(time, now: try at("2026-12-01T02:40:00Z"))
        #expect(rows.map(\.0) == ["Arrived", "Location", "Started", "Paused", "Finished", "On-site time"])
        #expect(
            rows.map(\.1) == [
                "\(Format.clock("2026-12-01T01:00:00Z")) · checked in", "18 m from site · QR matched",
                Format.clock("2026-12-01T01:00:00Z"), "since \(Format.clock("2026-12-01T01:30:00Z"))", "— (on submit)", "30 min",
            ]
        )
    }

    @Test func timeOnSiteOnceFinished() throws {
        let time = TimeOnSite(
            arrivedAt: nil,
            checkInMethod: .manual,
            checkInReason: "No GPS",
            distanceMeters: nil,
            startedAt: "2026-12-01T01:00:00Z",
            pauses: [
                TimeOnSitePauses(from: "2026-12-01T01:10:00Z", to: "2026-12-01T01:20:00Z"),
                TimeOnSitePauses(from: "bad", to: "2026-12-01T01:25:00Z"),
            ],
            finishedAt: "2026-12-01T03:00:00Z",
            onSiteMinutes: nil
        )
        let rows = JobWords.timeRows(time, now: try at("2026-12-01T05:00:00Z"))
        // a pause without a readable start counts nothing
        #expect(
            rows.map(\.1).dropFirst() == [
                "manual — No GPS", Format.clock("2026-12-01T01:00:00Z"), "2 pauses · 10 min", Format.clock("2026-12-01T03:00:00Z"),
                "1 h 50 min",
            ]
        )
        #expect(rows.first?.1 == "\(Format.clock("2026-12-01T01:00:00Z")) · manual check-in")
    }

    @Test func theCountedMinutesWhenTheCoreAPIGivesThem() {
        let time = TimeOnSite(
            arrivedAt: "2026-12-01T01:00:00Z",
            checkInMethod: .locationQr,
            checkInReason: nil,
            distanceMeters: nil,
            startedAt: nil,
            pauses: [TimeOnSitePauses(from: "2026-12-01T01:10:00Z", to: "2026-12-01T01:15:00Z")],
            finishedAt: nil,
            onSiteMinutes: 75
        )
        #expect(JobWords.timeRows(time, now: Date()).map(\.1).dropFirst() == ["—", "—", "1 pause · 5 min", "— (on submit)", "1 h 15 min"])
        var manual = time
        manual.checkInMethod = .manual
        #expect(JobWords.place(manual) == "manual — no reason")
    }

    @Test func aPastJobSaysWhatIsLeft() {
        var snapshot = JobHistorySnapshot(
            asOf: "2026-12-01T00:00:00Z",
            jobId: "41de64d5-73e5-52a7",
            type: .preventive,
            status: .completed,
            contractorOrgId: nil,
            completedAt: "2026-11-30T00:00:00Z",
            ownDecisionEvents: [],
            redactedReportSummary: .true(ReportHistorySummaryTrue(acceptance: .accepted))
        )
        let done = JobWords.history(snapshot)
        #expect(done.title == "41de64d5 · \(Labels.jobType(.preventive))")
        #expect(done.subtitle == "Completed — your assignment ended with the job")
        #expect(
            done.rows
                == [
                    JobWords.History.Row(label: "Status", value: "Completed \(Format.date("2026-11-30T00:00:00Z"))"),
                    JobWords.History.Row(label: "Your report", value: "Accepted in the quality review"),
                    JobWords.History.Row(label: "Assignment ended", value: Format.dateTime("2026-12-01T00:00:00Z")),
                ]
        )
        #expect(done.note.hasPrefix("Your time is free for other jobs."))
        snapshot.completedAt = nil
        #expect(JobWords.history(snapshot).rows.first?.value == "Completed")

        snapshot.status = .cancelled
        snapshot.redactedReportSummary = .true(ReportHistorySummaryTrue(acceptance: .notAccepted))
        let cancelled = JobWords.history(snapshot)
        #expect(cancelled.subtitle == "Your assignment on this job has ended")
        #expect(
            cancelled.rows.map(\.value).prefix(2) == [
                "\(Labels.job(JobStatus.cancelled)) when your assignment ended", "Submitted — not accepted while you were assigned",
            ]
        )
        #expect(cancelled.note == "The job was cancelled, so the job details and the unit are no longer shown to you.")

        snapshot.status = .inProgress
        snapshot.redactedReportSummary = .false(ReportHistorySummaryFalse())
        let ended = JobWords.history(snapshot)
        #expect(ended.rows[1].value == "No report from your assignment")
        #expect(ended.note.hasPrefix("Your work window ended, or HQ or your coordinator changed the assignment"))
    }
}
