import ACKit
import ACShared
import Foundation
import Observation

/// One assigned job (FR-T04, T08, T13, T14, DD-T13, the web's technician job page): the job and its unit, the work
/// window, and the steps before the report: accepting the assignment or saying the time does not work, checking in at
/// the site, pausing and resuming the work, resuming a returned report. Every write carries the job's version; after
/// one the job is read again, so the screen shows the state the Core API keeps.
@MainActor @Observable
public final class JobDetailModel {
    /// What the screen shows: the job while it is the technician's, its history once the viewing window ended (IR124),
    /// or nothing (another technician's job, or an offer, which is a contractor's).
    public enum Content: Sendable, Equatable {
        case work(Work)
        case history(JobHistorySnapshot)
        case notFound
    }

    /// The job, its unit when the window lets the technician read it, and the Core API's time of the read.
    public struct Work: Sendable, Equatable {
        public let job: JobDetail
        public let unit: UnitDetail?
        public let unitName: String?
        public let now: Date
        /// HQ's reason for returning the last report, while the job is returned for rework.
        public var returnReason: String?
    }

    /// A line over the job: what the status asks of the technician, with the step it offers.
    public struct Banner: Sendable, Equatable {
        public enum Tone: Sendable { case info, ok, warn, critical }
        public enum Action: Sendable, Equatable { case acknowledge, checkIn(enabled: Bool), resumeRework(enabled: Bool) }
        public let tone: Tone
        public let text: String
        public let action: Action?
    }

    public let jobId: String
    public private(set) var content: Loadable<Content> = .loading
    public private(set) var isSending = false
    public private(set) var isLocating = false
    /// What the last step came to: done, or why it was refused.
    public private(set) var note: String?
    public private(set) var refusal: String?
    public var checkIn = CheckInForm()
    public private(set) var checkInErrors: [CheckInField: String] = [:]
    public var cantMake = CantMakeForm()
    public private(set) var cantMakeError: String?

    let client: CoreClient
    let position: any PositionSource

    public init(jobId: String, client: CoreClient, position: any PositionSource) {
        self.jobId = jobId
        self.client = client
        self.position = position
    }

    public var work: Work? {
        if case let .work(work)? = content.value { work } else { nil }
    }

    public func load() async {
        content = await content.after { () async throws(CoreError) in try await read() }
    }

    private func read() async throws(CoreError) -> Content {
        let answer: CoreAnswer<JobsGetResult>
        do {
            answer = try await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId))
        } catch {
            // another technician's job, or one the Core API does not know: not found, as the web says
            let code = error.error.code
            guard code == .notFound || code == .forbidden || error.error.fieldErrors["jobId"] != nil else { throw error }
            return .notFound
        }
        switch answer.data {
        case let .detail(job):
            let now = answer.meta.snapshotAt.flatMap(Format.parse) ?? Date()
            let unit = try? await client.call(Ops.UnitsGet.self, UnitsGetInput(id: job.unitId, jobId: job.id)).data
            // before the work window units.get refuses, but units.list still names the unit (IR570)
            var name = unit?.displayName
            if name == nil { name = await listedName(job.unitId) }
            return .work(Work(job: job, unit: unit, unitName: name, now: now, returnReason: await returnReason(job)))
        case let .history(snapshot):
            return .history(snapshot)
        case .offer:
            return .notFound
        }
    }

    /// The reason HQ gave when it returned the last report (an unreadable report gives none).
    private func returnReason(_ job: JobDetail) async -> String? {
        guard job.status == .reworkRequested, let ref = job.reportRefs.last else { return nil }
        let input = ReportsGetInput(jobId: job.id, reportId: ref.reportId, reportVersion: ref.reportVersion)
        return try? await client.call(Ops.ReportsGet.self, input).data.reviewHistory.last?.reason
    }

    private func listedName(_ unitId: String) async -> String? {
        let query = Query(limit: 1, filters: QueryFilters(unitIds: [unitId]))
        return try? await client.call(Ops.UnitsList.self, query).data.items.first?.displayName
    }

    // MARK: - The steps

    /// Accepts the assignment: the coordinator and HQ learn the technician will attend.
    public func accept() async {
        _ = await write(tr("Assignment accepted")) { [client] job throws(CoreError) in
            let input = JobsAcknowledgeAssignmentInput(jobId: job.id, decision: .accept)
            _ = try await client.call(Ops.JobsAcknowledgeAssignment.self, input, expectedVersion: Int(job.version))
        }
    }

    /// Tells the coordinator the time does not work, with the reason and an optional other time; true when sent.
    public func sendCantMake() async -> Bool {
        cantMakeError = cantMake.error
        guard cantMakeError == nil else { return false }
        let form = cantMake
        let sent = await write(tr("Sent to your coordinator")) { [client] job throws(CoreError) in
            let input = JobsAcknowledgeAssignmentInput(
                jobId: job.id,
                decision: .cantMake,
                reason: form.reason.trimmingCharacters(in: .whitespacesAndNewlines),
                alternativeSlot: form.slot()
            )
            _ = try await client.call(Ops.JobsAcknowledgeAssignment.self, input, expectedVersion: Int(job.version))
        }
        if sent { cantMake = CantMakeForm() }
        return sent
    }

    /// A fresh fix of the phone for the check-in.
    public func locate() async {
        isLocating = true
        defer { isLocating = false }
        checkInErrors[.position] = nil
        switch await position.locate() {
        case let .fix(fix):
            checkIn.position = fix
        case .refused:
            checkInErrors[.position] = tr("This device cannot share its location — check in manually with a reason")
        case .unavailable:
            checkInErrors[.position] = tr(
                "The location could not be read — allow location for this app, or check in manually with a reason"
            )
        }
    }

    /// Checks in and starts the work; true when the Core API took it. A refusal of a field (too far, another unit's
    /// label) is shown under that field. The label is resolved first: one the Core API does not find among the
    /// technician's units is refused as the check-in refuses another unit's label (IR486).
    public func submitCheckIn() async -> Bool {
        checkInErrors = checkIn.errors
        guard checkInErrors.isEmpty else { return false }
        let form = checkIn
        var fields: [CheckInField: String] = [:]
        let done = await write(tr("Checked in — the job has started")) { [client] job throws(CoreError) in
            do throws(CoreError) {
                var qrUnitId: String?
                if form.method == .locationQr {
                    let code = form.qr.trimmingCharacters(in: .whitespacesAndNewlines)
                    do throws(CoreError) {
                        qrUnitId = try await client.call(Ops.UnitsResolveQr.self, UnitsResolveQrInput(code: code)).data.unitId
                    } catch  where error.error.code == .notFound {
                        throw CoreError(
                            status: 422,
                            error: DomainError(
                                code: .validation,
                                messageKey: "error.validation",
                                fieldErrors: ["qrUnitId": "errors.qr_unit_mismatch"],
                                correlationId: error.error.correlationId,
                                retryAfterSeconds: nil
                            )
                        )
                    }
                }
                let input = JobsCheckInInput(
                    jobId: job.id,
                    method: form.method,
                    position: form.position,
                    qrUnitId: qrUnitId,
                    reason: form.method == .manual ? form.reason.trimmingCharacters(in: .whitespacesAndNewlines) : nil
                )
                _ = try await client.call(Ops.JobsCheckIn.self, input, expectedVersion: Int(job.version))
            } catch {
                fields = CheckInForm.fields(of: error.error)
                throw error
            }
        }
        if done {
            checkIn = CheckInForm()
        } else if !fields.isEmpty {
            checkInErrors = fields
            refusal = nil
        }
        return done
    }

    /// Pauses or resumes the time on site.
    public func setPaused(_ paused: Bool) async {
        _ = await write(paused ? tr("Work paused") : tr("Work resumed")) { [client] job throws(CoreError) in
            let input = JobsPauseWorkInput(jobId: job.id, paused: paused)
            _ = try await client.call(Ops.JobsPauseWork.self, input, expectedVersion: Int(job.version))
        }
    }

    /// Resumes a report HQ returned: the work goes on in a new draft version.
    public func resumeRework() async {
        _ = await write(tr("Rework resumed — continue in the new draft")) { [client] job throws(CoreError) in
            let input = JobsResumeReworkInput(jobId: job.id)
            _ = try await client.call(Ops.JobsResumeRework.self, input, expectedVersion: Int(job.version))
        }
    }

    /// One write on the job as last read; the job is read again after it, also after a refusal (a conflict then shows
    /// the latest state).
    private func write(_ done: String, _ send: (JobDetail) async throws(CoreError) -> Void) async -> Bool {
        guard let job = work?.job, !isSending else { return false }
        isSending = true
        note = nil
        refusal = nil
        defer { isSending = false }
        let sent: Bool
        do {
            try await send(job)
            note = done
            sent = true
        } catch {
            refusal = ErrorText.of(error)
            sent = false
        }
        await load()
        return sent
    }
}

/// What the screen shows of the job as last read.
extension JobDetailModel {
    /// "f0000000 · Living room AC — Assigned": the job's short id, its unit and its status.
    public var title: String {
        guard let work else { return tr("Job") }
        return "\(work.job.id.prefix(8)) · \(work.unitName ?? tr("Unit")) — \(Labels.job(work.job.status))"
    }

    public var window: WorkWindow? {
        work.map { JobWords.window($0.job.assignment, now: $0.now) }
    }

    public var isInWork: Bool { work?.job.status == .inProgress }

    public var isPaused: Bool { work?.job.timeOnSite?.pauses.contains { $0.to == nil } ?? false }

    /// The time on site may be paused and resumed while the job is in progress inside its window.
    public var canPause: Bool {
        guard isInWork, let phase = window?.phase else { return false }
        return phase != .ended && phase != .before
    }

    /// What the status asks of the technician (the web's banners, top to bottom).
    public var banners: [Banner] {
        guard let work, let window else { return [] }
        let job = work.job
        let assignment = job.assignment
        var banners: [Banner] = []
        switch job.status {
        case .assigned:
            switch assignment?.acknowledgement ?? .pending {
            case .pending:
                let visit = assignment.map { Format.span($0.scheduledStart, $0.scheduledEnd) } ?? ""
                let text = tr(
                    "New assignment for {window}. Accepting tells your coordinator and HQ you will attend; the client then sees your name.",
                    ["window": visit]
                )
                banners.append(Banner(tone: .info, text: text, action: .acknowledge))
            case .cantMake:
                let text =
                    if let reason = assignment?.cantMakeReason {
                        tr(
                            "You said you can’t make this time (“{reason}”). Your coordinator reassigns the job or asks the client for another time.",
                            ["reason": reason]
                        )
                    } else {
                        tr("You said you can’t make this time. Your coordinator reassigns the job or asks the client for another time.")
                    }
                banners.append(Banner(tone: .warn, text: text, action: nil))
            case .accepted:
                // accepted is the answer of an assignment, so there is one
                if let assignment {
                    let canStart = window.phase == .open || window.phase == .ending
                    banners.append(
                        Banner(
                            tone: window.phase == .before ? .info : .ok,
                            text: JobWords.accepted(window, opens: Format.dateTime(assignment.scheduledStart)),
                            action: .checkIn(enabled: canStart)
                        )
                    )
                }
            }
        case .onHold:
            let text = tr(
                "On hold by HQ — saving and submitting are blocked. Your saved draft is kept; continue when HQ resumes the job."
            )
            banners.append(Banner(tone: .warn, text: text, action: nil))
        case .submitted:
            let version = job.reportRefs.last.map { String(Int($0.reportVersion)) } ?? ""
            let text = tr("Submitted v{version} — read-only, awaiting quality review.", ["version": version])
            banners.append(Banner(tone: .ok, text: text, action: nil))
        case .completed:
            let text = tr("Accepted — the job is completed and the report is visible to the customer.")
            banners.append(Banner(tone: .ok, text: text, action: nil))
        case .reworkRequested:
            let text =
                if let reason = work.returnReason {
                    tr(
                        "Returned for rework — “{reason}”. Resume to continue as a new draft version; the returned version stays as it was.",
                        ["reason": reason]
                    )
                } else {
                    tr("Returned for rework. Resume to continue as a new draft version; the returned version stays as it was.")
                }
            banners.append(Banner(tone: .warn, text: text, action: .resumeRework(enabled: window.phase != .ended)))
        case .requested, .offered, .accepted, .inProgress, .cancelled:
            break
        }
        if job.status == .inProgress, window.phase == .ended {
            let text = tr(
                "The work window ended — unsaved input was discarded and the job can no longer be edited. Ask your coordinator."
            )
            banners.append(Banner(tone: .critical, text: text, action: nil))
        }
        if let notice = window.notice { banners.append(Banner(tone: .warn, text: notice, action: nil)) }
        return banners
    }

    /// The job and its unit, as the web's Job & unit card lists them.
    public var facts: [(String, String)] {
        guard let work else { return [] }
        let job = work.job
        let unit = work.unit
        let site = unit.map { $0.location.pathLabels.joined(separator: " › ") } ?? tr("opens at the start of your window")
        let model = unit.map { "\($0.displayName) · \($0.capabilities.manufacturer) \($0.capabilities.model)" }
        let alerts =
            switch job.alertIds.count {
            case 0: tr("none")
            case 1: tr("1 alert")
            default: tr("{n} alerts", ["n": String(job.alertIds.count)])
            }
        return [
            (tr("Window"), job.assignment.map { Format.span($0.scheduledStart, $0.scheduledEnd) } ?? "—"),
            (tr("Site"), site),
            (tr("Access"), unit?.location.accessInstructions ?? "—"),
            (tr("Unit"), model ?? work.unitName ?? String(job.unitId.prefix(8))),
            (tr("Job"), "\(Labels.jobType(job.type)) · \(JobWords.origin(job.origin))"),
            (tr("Request"), job.symptom.isEmpty ? "—" : "“\(job.symptom)”"),
            (tr("Linked alerts"), alerts),
        ]
    }

    /// The work report opens once the work has started: while in progress, and afterwards read-only.
    public var hasReport: Bool {
        guard let job = work?.job else { return false }
        return job.status == .inProgress || job.draftReportRef != nil || !job.reportRefs.isEmpty
    }

    /// The work report of the job as last read.
    public func report() -> ReportModel? {
        guard let work, hasReport else { return nil }
        return ReportModel(job: work.job, unit: work.unit, now: work.now, canWork: canPause, client: client) { [weak self] in
            await self?.load()
        }
    }

    public var timeRows: [(String, String)] {
        guard let work else { return [] }
        return JobWords.timeRows(work.job.timeOnSite, now: work.now)
    }
}
