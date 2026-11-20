import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

@MainActor @Suite struct JobsModelTests {
    private func model(_ answers: [StubTransport.Answer]) -> (JobsModel, StubTransport) {
        let transport = StubTransport(answers)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        return (JobsModel(client: CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())), transport)
    }

    private func history() throws -> JobHistorySnapshot {
        struct Envelope: Decodable { let data: Page<JobsListResultItem> }
        let page = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.list.technician")).data
        guard case let .history(job) = try #require(page.items.first) else { throw CocoaError(.coderValueNotFound) }
        return job
    }

    @Test func pastJobsComeAfterTheOpenOnes() async throws {
        let past = try history()
        #expect(
            JobsModel.row(.history(past))
                == JobsModel.Row(
                    id: past.jobId,
                    title: Labels.jobType(past.type),
                    status: Labels.job(past.status),
                    due: Format.dateTime(past.completedAt ?? past.asOf),
                    isPast: true
                )
        )
        let answer = try #require(String(bytes: try fixture("jobs.list.technician"), encoding: .utf8))
        let (model, transport) = model([.http(200, answer)])
        await model.load()
        #expect(model.jobs.value?.allSatisfy(\.isPast) == true)
        let asked = try #require(await transport.requests.first?.url)
        #expect(query(asked.path() + "?" + (asked.query() ?? "")).values == ["limit": ["100"], "sort": ["dueAt:asc"]])
    }

    private func assigned(_ id: String, due: String) -> JobSummary {
        let slot = Slot(startAt: due, endAt: due)
        return JobSummary(
            id: id,
            version: 1,
            status: .assigned,
            createdAt: due,
            dueAt: due,
            type: .reactive,
            origin: .clientRequest,
            unitId: "u1",
            requestedSlot: slot,
            scheduledSlot: slot,
            assignmentId: "a1",
            preferredSlots: [],
            followUpOfJobId: nil,
            followUpClass: nil,
            severity: .normal,
            displayStatus: .assigned,
            assignmentAcknowledgement: nil,
            technicianMembershipId: "m1",
            accessValidFrom: nil,
            accessValidUntil: nil,
            eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
        )
    }

    @Test func openJobsComeFirstAndOffersAreNotATechniciansRows() async throws {
        let past = try history()
        // the generated Page writes its nextCursor as null, as the contract's answers do
        struct Envelope: Encodable { let data: Page<JobsListResultItem>; let meta: [String: String] }
        let slot = Slot(startAt: "2026-12-03", endAt: "2026-12-03")
        let offer = JobOfferSummary(
            status: .offered,
            jobId: "o1",
            jobVersion: 1,
            offerId: "of1",
            type: .periodic,
            siteAddress: nil,
            requiredQualifications: [],
            requestedSlot: slot,
            dueAt: "2026-12-03",
            offerExpiresAt: "2026-12-01",
            termsVersion: "t1",
            origin: .periodicPlan,
            candidateSlots: [],
            offeredAt: "2026-11-20",
            accessValidFrom: "2026-12-03",
            accessValidUntil: "2026-12-04",
            partnerSlotProposal: nil
        )
        let items: [JobsListResultItem] = [
            .history(past), .summary(assigned("j2", due: "2026-12-02")), .offer(offer), .summary(assigned("j1", due: "2026-12-01")),
        ]
        let body = try JSONEncoder().encode(
            Envelope(data: Page(items: items, nextCursor: nil, total: 4, snapshotVersion: 1), meta: ["correlationId": "c"])
        )
        let (model, _) = model([.http(200, try #require(String(bytes: body, encoding: .utf8)))])
        await model.load()
        #expect(model.jobs.value?.map(\.id) == ["j2", "j1", past.jobId])
        #expect(
            model.jobs.value?.first == JobsModel.Row(id: "j2", title: "Repair", status: "Assigned", due: "Due 2026-12-02", isPast: false)
        )
    }

    @Test func historyWithoutACompletionShowsItsSnapshotTime() throws {
        var past = try history()
        past.completedAt = nil
        #expect(JobsModel.row(.history(past))?.due == Format.dateTime(past.asOf))
    }

    @Test func failedLoadSaysWhy() async {
        let (model, _) = model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(model.jobs.value == nil)
        #expect(model.jobs.message == "The request took too long — check whether the change was saved, then try again")
    }
}

func fixture(_ name: String) throws -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures") else {
        throw CocoaError(.fileNoSuchFile)
    }
    return try Data(contentsOf: url)
}
