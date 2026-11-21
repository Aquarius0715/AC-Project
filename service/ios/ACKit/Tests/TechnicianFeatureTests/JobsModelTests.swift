import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

@MainActor @Suite struct JobsModelTests {
    /// The Core API's time of the reads: 1 Dec 2026, 02:00.
    private let now = "2026-12-01T02:00:00Z"

    private func model(_ answers: [StubTransport.Answer]) -> (JobsModel, StubTransport) {
        let transport = StubTransport(answers)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        return (JobsModel(client: CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())), transport)
    }

    private func at(_ instant: String) throws -> Date { try #require(Format.parse(instant)) }

    private func history() throws -> JobHistorySnapshot {
        struct Envelope: Decodable { let data: Page<JobsListResultItem> }
        let page = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.list.technician")).data
        guard case let .history(job) = try #require(page.items.first) else { throw CocoaError(.coderValueNotFound) }
        return job
    }

    private func answer(_ result: some Encodable) throws -> StubTransport.Answer {
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: ["correlationId": "c", "snapshotAt": now]))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    /// An assigned job, its window 01:00–04:00 on 1 Dec unless the test says otherwise.
    private func assigned(
        _ id: String,
        status: JobStatus = .assigned,
        answer: AssignmentAcknowledgement? = .accepted,
        slot: Slot? = Slot(startAt: "2026-12-01T01:00:00Z", endAt: "2026-12-01T04:00:00Z"),
        severity: Severity = .normal
    ) -> JobSummary {
        JobSummary(
            id: id,
            version: 1,
            status: status,
            createdAt: now,
            dueAt: "2026-12-02T00:00:00Z",
            type: .reactive,
            origin: .clientRequest,
            unitId: "u1",
            requestedSlot: Slot(startAt: now, endAt: now),
            scheduledSlot: slot,
            assignmentId: "a1",
            preferredSlots: [],
            followUpOfJobId: nil,
            followUpClass: nil,
            severity: severity,
            displayStatus: .assigned,
            assignmentAcknowledgement: answer,
            technicianMembershipId: "m1",
            accessValidFrom: nil,
            accessValidUntil: nil,
            eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
        )
    }

    private func unit(_ id: String, _ name: String) -> UnitSummary {
        UnitSummary(
            id: id,
            tenantId: "t1",
            version: 1,
            createdAt: now,
            updatedAt: now,
            customerOrgId: "c1",
            propertyId: "p1",
            spaceId: nil,
            displayName: name,
            modelId: "m1",
            installState: .installed,
            installedAt: nil,
            serviceScope: [],
            warrantyEndsAt: nil,
            alertPolicyIds: [],
            archived: false,
            capabilityVersion: 1,
            iotModelId: nil,
            iotModelVersion: nil,
            connection: .online,
            observedState: ObservedState(power: nil, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil),
            observedRestriction: nil,
            lastSeenAt: nil,
            effectivePowerState: .unknown,
            latestMeasurements: [],
            activeAlertCount: 0
        )
    }

    private func row(_ job: JobSummary, at time: String? = nil) throws -> JobsModel.Row {
        JobsModel.open(job, now: try at(time ?? now), names: ["u1": "Living room AC"])
    }

    private var window: String { "window \(Format.span("2026-12-01T01:00:00Z", "2026-12-01T04:00:00Z"))" }

    @Test func theOpenJobsComeFirstWithTheirUnitsAndTheNewOnesAreAnnounced() async throws {
        let past = try history()
        let slot = Slot(startAt: now, endAt: now)
        let offer = JobOfferSummary(
            status: .offered,
            jobId: "o1",
            jobVersion: 1,
            offerId: "of1",
            type: .periodic,
            siteAddress: nil,
            requiredQualifications: [],
            requestedSlot: slot,
            dueAt: now,
            offerExpiresAt: now,
            termsVersion: "t1",
            origin: .periodicPlan,
            candidateSlots: [],
            offeredAt: now,
            accessValidFrom: now,
            accessValidUntil: now,
            partnerSlotProposal: nil
        )
        let items: [JobsListResultItem] = [
            .history(past), .summary(assigned("j2", answer: .pending)), .offer(offer), .summary(assigned("j1")),
        ]
        let jobs = Page(items: items, nextCursor: nil, total: 4, snapshotVersion: 1)
        let units = Page(items: [unit("u1", "Living room AC"), unit("u1", "Duplicate")], nextCursor: nil, total: 2, snapshotVersion: 1)
        let (model, transport) = model([try answer(jobs), try answer(units)])
        await model.load()
        #expect(model.jobs.value?.map(\.id) == ["j2", "j1", past.jobId], "the offer is the contractor's")
        #expect(model.jobs.value?.first?.title == "Living room AC")
        #expect(model.needingAnswer.map(\.id) == ["j2"])
        #expect(
            model.answerLine
                == "New assignment to accept — j2 · Living room AC. The visit time was agreed with the client; accept it or say you can’t make it."
        )
        let asked = await transport.requests.compactMap(\.url)
        #expect(query(asked[0].path() + "?" + (asked[0].query() ?? "")).values == ["limit": ["100"], "sort": ["dueAt:asc"]])
        #expect(asked[1].path() == "/v1/units")
    }

    @Test func severalNewAssignmentsAreCounted() async throws {
        let jobs = Page(
            items: [JobsListResultItem.summary(assigned("j1", answer: .pending)), .summary(assigned("j2", answer: .pending))],
            nextCursor: nil,
            total: 2,
            snapshotVersion: 1
        )
        let (model, _) = model([try answer(jobs), .failure(URLError(.timedOut))])
        await model.load()
        #expect(model.answerLine == "2 new assignments to accept — The visit times were agreed with the client.")
        #expect(model.jobs.value?.first?.title == "Unit", "without the units' names")
    }

    @Test func nothingToAnswer() async {
        let (model, _) = model([])
        #expect(model.answerLine == nil)
    }

    @Test func eachRowSaysWhatTheTechnicianCanDoNow() throws {
        let pending = try row(assigned("j1", answer: .pending))
        #expect(pending.badge == "Not accepted" && pending.tone == .warn && pending.needsAnswer)
        #expect(pending.line == "Repair · \(window) · new assignment — review and accept it")
        let cantMake = try row(assigned("j1", answer: .cantMake))
        #expect(cantMake.badge == "Can’t make it" && !cantMake.needsAnswer)
        #expect(cantMake.line.hasSuffix("you said you can’t make it — waiting for your contractor or HQ"))
        let open = try row(assigned("j1"))
        #expect(open.badge == "Work not started" && open.tone == .info && open.line.hasSuffix("check in to start"))
        let before = try row(assigned("j1"), at: "2026-12-01T00:00:00Z")
        #expect(before.line.hasSuffix("opens \(Format.dateTime("2026-12-01T01:00:00Z")) (read-only until then)"))
        let ended = try row(assigned("j1", status: .inProgress), at: "2026-12-01T04:00:00Z")
        #expect(ended.badge == "Work window ended" && ended.tone == .critical)
        let ending = try row(assigned("j1", status: .inProgress), at: "2026-12-01T03:50:00Z")
        #expect(ending.badge == "In progress" && ending.line.hasSuffix("report draft open · window ends in 10 min"))
    }

    @Test func eachStatusHasItsBadge() throws {
        struct Expected {
            let status: JobStatus
            let badge: String
            let tone: JobsModel.Row.Tone
            let note: String
        }
        let rows = [
            Expected(status: .onHold, badge: "On hold", tone: .warn, note: "held by HQ — actions are blocked"),
            Expected(status: .reworkRequested, badge: "Rework requested", tone: .warn, note: "returned — resume in a new draft version"),
            Expected(status: .submitted, badge: "Submitted", tone: .info, note: "awaiting quality review (read-only)"),
            Expected(status: .completed, badge: "Completed", tone: .ok, note: "accepted in the quality review"),
            Expected(status: .cancelled, badge: Labels.job(JobStatus.cancelled), tone: .muted, note: ""),
        ]
        for expected in rows {
            let status = expected.status
            let note = expected.note
            let row = try row(assigned("j1", status: status, answer: .pending, slot: nil))
            #expect(row.badge == expected.badge && row.tone == expected.tone, "\(status)")
            #expect(row.line == ["Repair", note].filter { !$0.isEmpty }.joined(separator: " · "), "\(status): no window, no answer asked")
        }
    }

    @Test func aJobWithoutAWindowHasNotBegunOrEnded() async throws {
        let row = try row(assigned("j1", slot: nil))
        #expect(row.line == "Repair · check in to start")
        // without the Core API's time the phone's: the list is read all the same
        let jobs = #"{"data":{"items":[],"nextCursor":null,"total":0,"snapshotVersion":1},"meta":{"correlationId":"c"}}"#
        let (model, _) = model([.http(200, jobs), .failure(URLError(.timedOut))])
        await model.load()
        #expect(model.jobs.value == [])
    }

    @Test func theUnitsAlertIsNamed() throws {
        #expect(try row(assigned("j1", severity: .critical)).line.contains("critical alert on the unit"))
        #expect(try row(assigned("j1", severity: .warning)).line.contains("warning on the unit"))
    }

    @Test func aPastJobShowsItsSnapshot() throws {
        var past = try history()
        let unfinished = JobsModel.past(past)
        #expect(unfinished.badge == "Work window ended" && unfinished.tone == .critical)
        #expect(unfinished.line == "ask your contractor or HQ to extend or reassign")
        past.status = .completed
        past.completedAt = "2026-11-30T00:00:00Z"
        let done = JobsModel.past(past)
        #expect(done.badge == Labels.job(JobStatus.completed) && done.line == Format.dateTime("2026-11-30T00:00:00Z") && done.isPast)
        past.completedAt = nil
        #expect(JobsModel.past(past).line == Format.dateTime(past.asOf))
        #expect(JobsModel.row(.history(past), now: Date(), names: [:])?.isPast == true)
    }

    private func counts(scheduled: Double, overdue: Double) -> SummaryCounts {
        SummaryCounts(
            total: 3,
            online: 0,
            offline: 0,
            unknown: 0,
            powerOn: 0,
            powerOff: 0,
            powerUnknown: 0,
            alertCount: 0,
            offerCount: 0,
            activeCount: 0,
            reviewCount: 0,
            scheduledCount: scheduled,
            inProgressCount: 0,
            overdueCount: overdue,
            assignedCount: 3
        )
    }

    @Test func theOverviewHasItsTilesAlertsAndReports() async throws {
        var other = assigned("j3", status: .submitted)
        other.unitId = "u2"
        var done = assigned("j4", status: .completed)
        done.unitId = "u3"
        let items: [JobsListResultItem] = [
            .summary(assigned("j1")), .summary(assigned("j2", status: .inProgress)), .summary(other), .summary(done),
        ]
        let jobs = Page(items: items, nextCursor: nil, total: 4, snapshotVersion: 1)
        let units = Page(items: [unit("u1", "Living room AC")], nextCursor: nil, total: 1, snapshotVersion: 1)
        var onDone = alert("a-done", severity: .critical)
        onDone.unitId = "u3"
        var onOther = alert("a-other", severity: .normal, status: .acknowledged)
        onOther.unitId = "u2"
        let open = Page(
            items: [alert("a-old", detected: "2026-09-27T03:00:00Z"), alert("a-new"), alert("a-crit", severity: .critical), onDone],
            nextCursor: nil,
            total: 4,
            snapshotVersion: 1
        )
        let acknowledged = Page(items: [onOther, alert("a-res", status: .resolved)], nextCursor: nil, total: 2, snapshotVersion: 1)
        let summary = Summary(kind: .technician, counts: counts(scheduled: 1, overdue: 0), asOf: now)
        let (model, transport) = model([
            try answer(jobs), try answer(units), try answer(summary), try answer(open), try answer(acknowledged),
        ])
        await model.load()
        #expect(model.tiles.map(\.value) == ["3", "1", "0"])
        #expect(model.tiles.map(\.sub) == ["in your assignments", "j1", "No overdue jobs"])
        #expect(
            model.alerts.map(\.id) == ["a-crit", "a-new", "a-old", "a-other"],
            "worst first, newest first, only units with a current job"
        )
        #expect(
            model.alerts[0].jobId == "j1" && model.alerts[0].sub.hasPrefix("Living room AC · open · ")
                && model.alerts[3].sub.hasPrefix("unit · acknowledged · ")
        )
        #expect(
            model.reports.map(\.badge) == ["Draft", "Submitted"] && model.reports[0].title == "j2 · Living room AC"
                && model.reports[1].title == "j3 · Unit"
        )
        #expect(model.reports.map(\.sub) == ["draft in progress", "awaiting quality review · read-only"])
        let asked = await transport.requests.compactMap(\.url)
        #expect(asked[2].path() == "/v1/summaries/technician")
        #expect(query(asked[3].path() + "?" + (asked[3].query() ?? "")).values["status"] == ["open"])
    }

    @Test func theTilesNameTheOverdueAndAReworkIsAReport() async throws {
        let tiles = OverviewWords.tiles(counts(scheduled: 0, overdue: 2), [])
        #expect(tiles.map(\.sub) == ["in your assignments", "No jobs waiting to start", "past the due date"])
        let rework = OverviewWords.reports([assigned("j1", status: .reworkRequested)], names: [:])
        #expect(rework.map(\.badge) == ["Rework"] && rework[0].sub == "returned — resume as a new version")
    }

    @Test func failedLoadSaysWhy() async {
        let (model, _) = model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(model.jobs.value == nil)
        #expect(model.jobs.message == "The request took too long — check whether the change was saved, then try again")
    }

    @Test func theRealListIsRead() async throws {
        let answer = try #require(String(bytes: try fixture("jobs.list.technician"), encoding: .utf8))
        let (model, _) = model([
            .http(200, answer),
            .http(200, #"{"data":{"items":[],"nextCursor":null,"total":0,"snapshotVersion":1},"meta":{"correlationId":"c"}}"#),
        ])
        await model.load()
        #expect(model.jobs.value?.allSatisfy(\.isPast) == true)
    }
}

func fixture(_ name: String) throws -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures") else {
        throw CocoaError(.fileNoSuchFile)
    }
    return try Data(contentsOf: url)
}

/// An answer of the Core API as the tests write it.
struct AnswerEnvelope<Result: Encodable>: Encodable {
    let data: Result
    let meta: [String: String]
}
