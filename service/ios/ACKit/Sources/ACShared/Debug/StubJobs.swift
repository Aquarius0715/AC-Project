#if DEBUG
import ACKit
import Foundation

/// The technician's job of the UI tests (AC_UI_TEST=stub): j1 on Living room AC (u1), its work window from 30 minutes
/// ago to 3 hours from now, a new assignment to answer. Accepting, saying the time does not work, checking in and
/// pausing change it as the Core API would; a label other than u1's is refused as another unit's.
final class StubJobs {
    private var job: JobDetail
    private let reports = StubReports()
    private let at: () -> String

    init(now: Date = Date()) {
        let start = now.addingTimeInterval(-30 * 60).ISO8601Format()
        let end = now.addingTimeInterval(3 * 3600).ISO8601Format()
        let slot = Slot(startAt: start, endAt: end)
        at = { Date().ISO8601Format() }
        let assignment = Assignment(
            id: "a1",
            tenantId: "t1",
            version: 1,
            createdAt: start,
            updatedAt: start,
            jobId: "j1",
            technicianMembershipId: "m1",
            validFrom: start,
            validUntil: end,
            scheduledStart: start,
            scheduledEnd: end,
            status: .active,
            reason: nil,
            acknowledgement: .pending,
            acknowledgedAt: nil,
            cantMakeReason: nil,
            alternativeSlot: nil,
            technicianName: nil
        )
        job = JobDetail(
            id: "j1",
            tenantId: "t1",
            version: 1,
            createdAt: start,
            updatedAt: start,
            planId: nil,
            occurrenceAt: nil,
            unitId: "u1",
            alertIds: [],
            type: .reactive,
            status: .assigned,
            symptom: "Not cooling",
            contactWindow: nil,
            requestedSlot: slot,
            origin: .clientRequest,
            preferredSlots: [],
            preferenceRound: 1,
            slotProposal: nil,
            partnerSlotProposal: nil,
            scheduledSlot: slot,
            dueAt: end,
            startedAt: nil,
            completedAt: nil,
            contractorOrgId: nil,
            assignmentId: "a1",
            draftReportRef: nil,
            reportRefs: [],
            costs: [],
            followUpOfJobId: nil,
            followUpClass: nil,
            timeOnSite: nil,
            rating: nil,
            customerConfirmedAt: nil,
            warrantyClaims: [],
            assignment: assignment,
            offer: nil,
            eventCursor: 1,
            problemPhotos: []
        )
    }

    /// The answer of a job route, or nil for a route that is not one: its status and its body.
    func answer(_ method: String, _ path: String, _ body: Data?) throws -> (Int, Data)? {
        let encoder = JSONEncoder()
        let path = path.removingPercentEncoding ?? path
        if let answer = try reports.answer(method, path, body, job: &job) { return answer }
        switch (method, path) {
        case ("GET", "/v1/jobs"):
            let page = Page(items: [JobsListResultItem.summary(summary)], nextCursor: nil, total: 1, snapshotVersion: 1)
            return (200, try encoder.encode(StubJobAnswer(data: page)))
        case ("GET", "/v1/jobs/j1"):
            return (200, try encoder.encode(StubJobAnswer(data: JobsGetResult.detail(job))))
        case ("GET", "/v1/units/qr/ac-unit:u1"):
            return (200, try encoder.encode(StubJobAnswer(data: QrResolution(unitId: "u1", jobId: "j1"))))
        case ("GET", _) where path.hasPrefix("/v1/units/qr/"):
            return (404, try encoder.encode(refusal(.notFound, "error.notFound", [:])))
        case ("POST", "/v1/jobs/j1/assignment/acknowledge"):
            let input = try JSONDecoder().decode(StubAcknowledge.self, from: body ?? Data())
            job.assignment?.acknowledgement = input.decision == "accept" ? .accepted : .cantMake
            job.assignment?.cantMakeReason = input.reason
            job.assignment?.acknowledgedAt = at()
            job.assignment?.version += 1
            _ = try changed()
            return (200, try encoder.encode(StubJobAnswer(data: job.assignment)))
        case ("POST", "/v1/jobs/j1/check-in"):
            let input = try JSONDecoder().decode(StubCheckIn.self, from: body ?? Data())
            let isManual = input.method == "manual"
            guard isManual || input.qrUnitId == "u1" else {
                return (422, try encoder.encode(refusal(.validation, "error.validation", ["qrUnitId": "errors.qr_unit_mismatch"])))
            }
            job.status = .inProgress
            job.startedAt = at()
            job.timeOnSite = TimeOnSite(
                arrivedAt: at(),
                checkInMethod: isManual ? .manual : .locationQr,
                checkInReason: input.reason,
                distanceMeters: isManual ? nil : 18,
                startedAt: at(),
                pauses: [],
                finishedAt: nil,
                onSiteMinutes: nil
            )
            return try changed()
        case ("POST", "/v1/jobs/j1/pause-work"):
            let input = try JSONDecoder().decode(StubPause.self, from: body ?? Data())
            var pauses = job.timeOnSite?.pauses ?? []
            if input.paused {
                pauses.append(TimeOnSitePauses(from: at(), to: nil))
            } else if let open = pauses.firstIndex(where: { $0.to == nil }) {
                pauses[open].to = at()
            }
            job.timeOnSite?.pauses = pauses
            return try changed()
        default:
            return nil
        }
    }

    /// The job after a write: its next version, answered as the Core API answers a job write.
    private func changed() throws -> (Int, Data) {
        job.version += 1
        job.updatedAt = at()
        return (200, try JSONEncoder().encode(StubJobAnswer(data: job)))
    }

    private func refusal(_ code: ErrorCode, _ key: String, _ fields: [String: String]) -> DomainError {
        DomainError(code: code, messageKey: key, fieldErrors: fields, correlationId: "stub", retryAfterSeconds: nil)
    }

    /// The job as jobs.list lists it.
    private var summary: JobSummary {
        JobSummary(
            id: job.id,
            version: job.version,
            status: job.status,
            createdAt: job.createdAt,
            dueAt: job.dueAt,
            type: job.type,
            origin: job.origin,
            unitId: job.unitId,
            requestedSlot: job.requestedSlot,
            scheduledSlot: job.scheduledSlot,
            assignmentId: job.assignmentId,
            preferredSlots: [],
            followUpOfJobId: nil,
            followUpClass: nil,
            severity: .normal,
            displayStatus: job.status == .inProgress ? .inProgress : .assigned,
            assignmentAcknowledgement: job.assignment?.acknowledgement,
            technicianMembershipId: "m1",
            accessValidFrom: job.assignment?.validFrom,
            accessValidUntil: job.assignment?.validUntil,
            eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
        )
    }
}

private struct StubAcknowledge: Decodable {
    let decision: String
    let reason: String?
}

private struct StubCheckIn: Decodable {
    let method: String
    let qrUnitId: String?
    let reason: String?
}

private struct StubPause: Decodable {
    let paused: Bool
}

private struct StubJobAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
