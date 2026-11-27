import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// A phone that answers the check-in's position with what the test says.
private struct FixedPosition: PositionSource {
    let answer: PositionAnswer

    func locate() async -> PositionAnswer { answer }
}

@MainActor @Suite struct JobDetailModelTests {
    /// The Core API's time of the fixture's read: inside its work window (01:05–04:35).
    private let now = "2026-09-27T01:35:13Z"

    private func model(
        _ answers: [StubTransport.Answer],
        position: PositionAnswer = .unavailable
    ) -> (JobDetailModel, StubTransport) {
        let transport = StubTransport(answers)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let client = CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())
        let model = JobDetailModel(jobId: "f0000000-0000-4000-8000-000000000863", client: client, position: FixedPosition(answer: position))
        return (model, transport)
    }

    /// The live job of the fixture (an assigned job before its acknowledgement), as the test changes it.
    private func detail(_ change: ((inout JobDetail) -> Void)? = nil) throws -> JobDetail {
        struct Envelope: Decodable { let data: JobsGetResult }
        guard case var .detail(job) = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.get.assigned")).data else {
            throw CocoaError(.coderValueNotFound)
        }
        change?(&job)
        return job
    }

    private func answer(_ result: some Encodable, at: String? = nil) throws -> StubTransport.Answer {
        var meta = ["correlationId": "c"]
        meta["snapshotAt"] = at
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: meta))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    private func job(_ change: ((inout JobDetail) -> Void)? = nil) throws -> StubTransport.Answer {
        try answer(JobsGetResult.detail(try detail(change)), at: now)
    }

    private var unit: StubTransport.Answer {
        get throws { .http(200, try #require(String(bytes: try fixture("units.get.technician"), encoding: .utf8))) }
    }

    private func refusal(_ status: Int, _ code: String, _ key: String, fields: String = "{}") -> StubTransport.Answer {
        .http(
            status,
            #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":\#(fields),"correlationId":"c","retryAfterSeconds":null}"#
        )
    }

    /// A job read with its unit, as the screen opens.
    private func loaded(
        _ change: ((inout JobDetail) -> Void)? = nil,
        then answers: [StubTransport.Answer] = []
    ) async throws
        -> (JobDetailModel, StubTransport)
    {
        let (model, transport) = model([try job(change), try unit] + answers)
        await model.load()
        return (model, transport)
    }

    private func body(_ request: URLRequest?) throws -> [String: String] {
        let object = try JSONSerialization.jsonObject(with: try #require(request?.httpBody))
        let fields = try #require(object as? [String: Any])
        return fields.mapValues { String(describing: $0) }
    }

    // MARK: - Reading the job

    @Test func theJobAndItsUnitAreRead() async throws {
        let (model, transport) = try await loaded()
        let work = try #require(model.work)
        #expect(work.unit?.displayName == "Living room AC")
        #expect(work.now == Format.parse(now))
        #expect(model.title == "f0000000 · Living room AC — Assigned")
        let requests = await transport.requests
        #expect(
            requests.map { $0.url?.path() } == [
                "/v1/jobs/f0000000-0000-4000-8000-000000000863", "/v1/units/e841ee95-3171-54d5-be91-1f2301bdb701",
            ]
        )
        #expect(requests.last?.url?.query() == "jobId=f0000000-0000-4000-8000-000000000863")
        let assignment = try #require(work.job.assignment)
        #expect(
            model.facts.map(\.1) == [
                Format.span(assignment.scheduledStart, assignment.scheduledEnd), "Home A › 1F › Living room",
                "Ring the demo bell (fictional)",
                "Living room AC · \(work.unit?.capabilities.manufacturer ?? "") \(work.unit?.capabilities.model ?? "")",
                "Repair · client request", "“Not cooling (iOS live check)”", "none",
            ]
        )
        #expect(model.window?.phase == .open)
        #expect(model.timeRows.map(\.1) == ["not checked in"])
        #expect(!model.isInWork && !model.isPaused && !model.canPause)
    }

    @Test func anUnreadableUnitAndListLeaveTheUnitUnnamed() async throws {
        let listed = #"{"data":{"items":[{"id":"u1","displayName":"Lobby AC"}],"nextCursor":null},"meta":{"correlationId":"c"}}"#
        let (model, transport) = model([try job(), refusal(403, "FORBIDDEN", "error.forbidden"), .http(200, listed)])
        await model.load()
        // a list item the contract does not know reads as unavailable: no name, the screen says "Unit"
        #expect(model.work?.unit == nil)
        #expect(await transport.requests.last?.url?.path() == "/v1/units")
        #expect(model.facts[1].1 == "opens at the start of your window")
        #expect(model.facts[2].1 == "—")
    }

    @Test func aUnitNamedByTheListOnly() async throws {
        let item = UnitSummary(
            id: "u1",
            tenantId: "t1",
            version: 1,
            createdAt: now,
            updatedAt: now,
            customerOrgId: "c1",
            propertyId: "p1",
            spaceId: nil,
            displayName: "Lobby AC",
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
        let page = Page(items: [item], nextCursor: nil, total: 1, snapshotVersion: 1)
        let (model, _) = model([try job(), refusal(403, "FORBIDDEN", "error.forbidden"), try answer(page)])
        await model.load()
        #expect(model.work?.unitName == "Lobby AC")
        #expect(model.facts[3].1 == "Lobby AC")
        #expect(model.title.hasSuffix("· Lobby AC — Assigned"))
    }

    @Test func aJobWithoutAnyUnitNameShowsItsShortId() async throws {
        let (model, _) = model([try answer(JobsGetResult.detail(try detail())), refusal(403, "FORBIDDEN", "error.forbidden")])
        await model.load()
        #expect(model.facts[3].1 == "e841ee95")
        #expect(model.title == "f0000000 · Unit — Assigned")
        #expect(model.work.map { abs($0.now.timeIntervalSinceNow) < 60 } == true, "no time from the Core API: the phone's")
    }

    @Test func aJobThatIsNotTheTechniciansIsNotFound() async throws {
        for answer in [
            refusal(404, "NOT_FOUND", "error.notFound"), refusal(403, "FORBIDDEN", "error.forbidden"),
            refusal(422, "VALIDATION", "error.validation", fields: #"{"jobId":"error.invalid"}"#),
        ] {
            let (model, _) = model([answer])
            await model.load()
            #expect(model.content.value == .notFound)
            #expect(model.title == "Job")
        }
    }

    @Test func anOfferIsTheContractorsAndAPastJobItsHistory() async throws {
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
        let history = String(bytes: try fixture("jobs.get.history"), encoding: .utf8) ?? ""
        let (model, _) = model([try answer(JobsGetResult.offer(offer)), .http(200, history)])
        await model.load()
        #expect(model.content.value == .notFound)
        await model.load()
        guard case let .history(snapshot)? = model.content.value else {
            Issue.record("the history is shown")
            return
        }
        #expect(snapshot.jobId == "41de64d5-73e5-52a7-bdd1-7c6c2cf13cf6")
        #expect(model.work == nil && model.banners.isEmpty && model.facts.isEmpty && model.timeRows.isEmpty && model.window == nil)
    }

    @Test func anUnreadableJobSaysWhy() async {
        let (model, _) = model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(model.content.message == "The request took too long — check whether the change was saved, then try again")
    }

    // MARK: - What the status asks

    @Test func aNewAssignmentAsksForAnAnswer() async throws {
        let (model, _) = try await loaded()
        let banner = try #require(model.banners.first)
        #expect(banner.action == .acknowledge)
        #expect(banner.tone == .info)
        #expect(banner.text.hasPrefix("New assignment for "))
    }

    @Test func anAssignedJobWithoutItsAssignmentAsksForTheAnswer() async throws {
        let (model, _) = try await loaded { $0.assignment = nil }
        #expect(model.banners.first?.action == .acknowledge)
        #expect(model.banners.first?.text.hasPrefix("New assignment for . ") == true)
    }

    @Test func aTimeTheTechnicianCannotMakeWaitsForTheCoordinator() async throws {
        var (model, _) = try await loaded {
            $0.assignment?.acknowledgement = .cantMake
            $0.assignment?.cantMakeReason = "Clinic"
        }
        #expect(
            model.banners.map(\.text) == [
                "You said you can’t make this time (“Clinic”). Your coordinator reassigns the job or asks the client for another time."
            ]
        )
        (model, _) = try await loaded { $0.assignment?.acknowledgement = .cantMake }
        #expect(model.banners.first?.text.hasPrefix("You said you can’t make this time. ") == true)
        #expect(model.banners.first?.action == nil)
    }

    @Test func anAcceptedAssignmentStartsInsideItsWindow() async throws {
        var (model, _) = try await loaded { $0.assignment?.acknowledgement = .accepted }
        #expect(model.banners.first?.action == .checkIn(enabled: true))
        #expect(model.banners.first?.tone == .ok)
        (model, _) = try await loaded {
            $0.assignment?.acknowledgement = .accepted
            $0.assignment?.scheduledStart = "2026-09-27T02:00:00Z"
        }
        #expect(model.banners.first?.action == .checkIn(enabled: false))
        #expect(model.banners.first?.tone == .info)
        #expect(
            model.banners.first?.text
                == "Read-only until the window opens at \(Format.dateTime("2026-09-27T02:00:00Z")) — Start is disabled before then."
        )
        (model, _) = try await loaded {
            $0.assignment?.acknowledgement = .accepted
            $0.assignment?.scheduledEnd = "2026-09-27T01:40:00Z"
        }
        #expect(model.banners.map(\.action) == [.checkIn(enabled: true), nil], "the last minutes add their notice")
        #expect(model.banners.last?.text.hasPrefix("Work window ends in 5 min") == true)
    }

    @Test func eachStatusSaysWhatItAsks() async throws {
        var (model, _) = try await loaded { $0.status = .onHold }
        #expect(model.banners.first?.text.hasPrefix("On hold by HQ") == true)
        (model, _) = try await loaded {
            $0.status = .submitted
            $0.reportRefs = [ReportRef(reportId: "r1", reportVersion: 2)]
        }
        #expect(model.banners.first?.text == "Submitted v2 — read-only, awaiting quality review.")
        (model, _) = try await loaded { $0.status = .submitted }
        #expect(model.banners.first?.text == "Submitted v — read-only, awaiting quality review.")
        (model, _) = try await loaded { $0.status = .completed }
        #expect(model.banners.first?.text == "Accepted — the job is completed and the report is visible to the customer.")
        (model, _) = try await loaded { $0.status = .reworkRequested }
        #expect(model.banners.first?.action == .resumeRework(enabled: true))
        (model, _) = try await loaded {
            $0.status = .reworkRequested
            $0.assignment?.scheduledEnd = "2026-09-27T01:00:00Z"
        }
        #expect(model.banners.first?.action == .resumeRework(enabled: false))
        (model, _) = try await loaded { $0.status = .requested }
        #expect(model.banners.isEmpty)
    }

    @Test func workAfterItsWindowCanNoLongerBeEdited() async throws {
        let (model, _) = try await loaded {
            $0.status = .inProgress
            $0.assignment?.scheduledEnd = "2026-09-27T01:30:00Z"
            $0.timeOnSite = TimeOnSite(
                arrivedAt: "2026-09-27T01:10:00Z",
                checkInMethod: .locationQr,
                checkInReason: nil,
                distanceMeters: 20,
                startedAt: "2026-09-27T01:10:00Z",
                pauses: [TimeOnSitePauses(from: "2026-09-27T01:20:00Z", to: nil)],
                finishedAt: nil,
                onSiteMinutes: nil
            )
        }
        #expect(model.banners.map(\.tone) == [.critical])
        #expect(model.isInWork && model.isPaused && !model.canPause)
        #expect(model.timeRows.count == 6)
    }

    @Test func workInsideItsWindowMayBePaused() async throws {
        var (model, _) = try await loaded { $0.status = .inProgress }
        #expect(model.canPause)
        (model, _) = try await loaded {
            $0.status = .inProgress
            $0.assignment = nil
        }
        #expect(model.window?.phase == .noWindow)
        #expect(model.canPause, "no window to end the work")
        #expect(model.facts.first?.1 == "—")
        #expect(model.banners.isEmpty)
    }

    @Test func theLinkedAlertsAreCounted() async throws {
        var (model, _) = try await loaded {
            $0.alertIds = ["a1"]
            $0.symptom = ""
        }
        #expect(model.facts.last?.1 == "1 alert")
        #expect(model.facts[5].1 == "—")
        (model, _) = try await loaded { $0.alertIds = ["a1", "a2"] }
        #expect(model.facts.last?.1 == "2 alerts")
    }

    // MARK: - The steps

    @Test func acceptingSendsTheJobsVersionAndReadsItAgain() async throws {
        let (model, transport) = try await loaded(
            then: [#"{"data":{},"meta":{"correlationId":"c"}}"#].map { .http(200, $0) } + [
                try job { $0.assignment?.acknowledgement = .accepted }, try unit,
            ]
        )
        await model.accept()
        // the write's answer is no Assignment of the contract: refused as unavailable, and the job is read again
        #expect(model.refusal == "The service is unavailable — try again shortly")
        let write = await transport.requests[2]
        #expect(write.httpMethod == "POST")
        #expect(write.url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/assignment/acknowledge")
        #expect(write.value(forHTTPHeaderField: "X-Expected-Version") == "1")
        #expect(try body(write) == ["decision": "accept"])
        #expect(model.banners.first?.action == .checkIn(enabled: true))
        #expect(!model.isSending)
    }

    @Test func anAcceptedAssignmentSaysSo() async throws {
        let assignment = try #require(try detail().assignment)
        let (model, _) = try await loaded(then: [try answer(assignment), try job(), try unit])
        await model.accept()
        #expect(model.note == "Assignment accepted")
        #expect(model.refusal == nil)
    }

    @Test func aStepWithoutAJobSendsNothing() async {
        let (model, transport) = model([])
        await model.accept()
        #expect(await transport.requests.isEmpty)
        #expect(model.note == nil)
    }

    @Test func cantMakeNeedsAReasonAndMaySayAnotherTime() async throws {
        let assignment = try #require(try detail().assignment)
        let (model, transport) = try await loaded(then: [try answer(assignment), try job(), try unit])
        #expect(await model.sendCantMake() == false)
        #expect(model.cantMakeError == "Tell your coordinator why (1–1000 characters)")
        #expect(await transport.requests.count == 2, "nothing sent")
        model.cantMake.reason = "  Clinic "
        model.cantMake.date = Format.parse("2026-09-28T02:00:00Z")
        model.cantMake.window = "08:00-10:00"
        let slot = try #require(model.cantMake.slot())
        #expect(await model.sendCantMake())
        #expect(model.note == "Sent to your coordinator")
        #expect(model.cantMake == CantMakeForm(), "the form starts empty again")
        let sent = try #require(await transport.requests[2].httpBody)
        let input = try JSONDecoder().decode(StubAcknowledgement.self, from: sent)
        #expect(input == StubAcknowledgement(decision: "cant_make", reason: "Clinic", alternativeSlot: slot))
    }

    @Test func aRefusedCantMakeKeepsTheForm() async throws {
        let (model, _) = try await loaded(then: [refusal(409, "CONFLICT", "error.versionConflict"), try job(), try unit])
        model.cantMake.reason = "Clinic"
        #expect(await model.sendCantMake() == false)
        #expect(model.cantMake.reason == "Clinic")
        #expect(model.refusal == "Someone changed this in the meantime — the screen now shows the latest version")
    }

    @Test func thePhoneLocatesItself() async throws {
        let fix = CheckInPosition(latitude: 3.1, longitude: 101.6, accuracyMeters: 8)
        var (model, _) = model([], position: .fix(fix))
        await model.locate()
        #expect(model.checkIn.position == fix && model.checkInErrors.isEmpty && !model.isLocating)
        (model, _) = self.model([], position: .refused)
        await model.locate()
        #expect(model.checkInErrors[.position] == "This device cannot share its location — check in manually with a reason")
        (model, _) = self.model([], position: .unavailable)
        await model.locate()
        #expect(
            model.checkInErrors[.position]
                == "The location could not be read — allow location for this app, or check in manually with a reason"
        )
    }

    @Test func aCheckInTheFormRefusesSendsNothing() async throws {
        let (model, transport) = try await loaded()
        #expect(await model.submitCheckIn() == false)
        #expect(model.checkInErrors.keys.sorted { $0.rawValue < $1.rawValue } == [.position, .qr])
        #expect(await transport.requests.count == 2)
    }

    @Test func aCheckInResolvesTheLabelThenStartsTheWork() async throws {
        let qr = #"{"data":{"unitId":"e841ee95-3171-54d5-be91-1f2301bdb701","jobId":null},"meta":{"correlationId":"c"}}"#
        let started = try detail { $0.status = .inProgress }
        let (model, transport) = try await loaded(then: [
            .http(200, qr), try answer(started), try job { $0.status = .inProgress }, try unit,
        ])
        model.checkIn.position = CheckInPosition(latitude: 3.139, longitude: 101.6869, accuracyMeters: 12)
        model.checkIn.qr = " ac-unit:e841ee95-3171-54d5-be91-1f2301bdb701 "
        #expect(await model.submitCheckIn())
        #expect(model.note == "Checked in — the job has started")
        #expect(model.checkIn == CheckInForm())
        let requests = await transport.requests
        #expect(requests[2].url?.path(percentEncoded: false) == "/v1/units/qr/ac-unit:e841ee95-3171-54d5-be91-1f2301bdb701")
        #expect(requests[3].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/check-in")
        let input = try JSONDecoder().decode(StubCheckInBody.self, from: try #require(requests[3].httpBody))
        #expect(input.method == "location_qr" && input.qrUnitId == "e841ee95-3171-54d5-be91-1f2301bdb701" && input.reason == nil)
        #expect(input.position?.accuracyMeters == 12)
        #expect(model.isInWork)
    }

    @Test func aLabelTheCoreAPIDoesNotKnowIsAnotherUnits() async throws {
        let (model, transport) = try await loaded(then: [refusal(404, "NOT_FOUND", "error.notFound"), try job(), try unit])
        model.checkIn.position = CheckInPosition(latitude: 3.139, longitude: 101.6869, accuracyMeters: 12)
        model.checkIn.qr = "ac-unit:other"
        #expect(await model.submitCheckIn() == false)
        #expect(
            model.checkInErrors == [
                .qr: "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason"
            ]
        )
        #expect(model.refusal == nil, "the field says it")
        #expect(await transport.requests.count == 5, "no check-in sent; the job read again")
    }

    @Test func aRefusedFixIsShownUnderTheLocation() async throws {
        let tooFar = refusal(422, "VALIDATION", "error.validation", fields: #"{"position":"errors.too_far"}"#)
        let (model, transport) = try await loaded(then: [tooFar, try job(), try unit])
        model.checkIn.method = .manual
        model.checkIn.reason = " Basement "
        model.checkIn.position = CheckInPosition(latitude: 3, longitude: 101, accuracyMeters: 30)
        #expect(await model.submitCheckIn() == false)
        #expect(model.checkInErrors.keys.first == .position)
        let input = try JSONDecoder().decode(StubCheckInBody.self, from: try #require(await transport.requests[2].httpBody))
        #expect(input.method == "manual" && input.reason == "Basement" && input.qrUnitId == nil)
    }

    @Test func anotherRefusalOfTheCheckInIsTheScreens() async throws {
        let (model, _) = try await loaded(then: [refusal(409, "CONFLICT", "error.versionConflict"), try job(), try unit])
        model.checkIn.method = .manual
        model.checkIn.reason = "Basement"
        #expect(await model.submitCheckIn() == false)
        #expect(model.checkInErrors.isEmpty)
        #expect(model.refusal == "Someone changed this in the meantime — the screen now shows the latest version")
        #expect(model.checkIn.reason == "Basement", "the form is kept")
    }

    @Test func theWorkIsPausedAndResumed() async throws {
        let inWork = try detail { $0.status = .inProgress }
        let (model, transport) = try await loaded(
            { $0.status = .inProgress },
            then: [try answer(inWork), try job { $0.status = .inProgress }, try unit, try answer(inWork), try job(), try unit]
        )
        await model.setPaused(true)
        #expect(model.note == "Work paused")
        #expect(try body(await transport.requests[2]) == ["paused": "1"])
        #expect(await transport.requests[2].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/pause-work")
        await model.setPaused(false)
        #expect(model.note == "Work resumed")
    }

    @Test func aReturnedReportSaysHQsReason() async throws {
        var report = try savedReport()
        report.reviewHistory = [
            Review(reviewerUserId: "hq", reportVersion: 1, decision: .return, reason: "Add the coil photo", occurredAt: now)
        ]
        let ref = ReportRef(reportId: report.id, reportVersion: 1)
        let (model, transport) = model([
            try job {
                $0.status = .reworkRequested
                $0.reportRefs = [ref]
            },
            try unit, try answer(report),
        ])
        await model.load()
        #expect(
            model.banners.first?.text
                == "Returned for rework — “Add the coil photo”. Resume to continue as a new draft version; the returned version stays as it was."
        )
        #expect(await transport.requests[2].url?.query() == "reportVersion=1", "the job, its unit, then the returned report")
    }

    @Test func theReportOpensOnceTheWorkHasStarted() async throws {
        var (model, _) = try await loaded()
        #expect(!model.hasReport && model.report() == nil, "before the check-in")
        (model, _) = try await loaded({ $0.status = .inProgress }, then: [try job { $0.status = .inProgress }, try unit])
        let report = try #require(model.report())
        #expect(model.hasReport && report.job.id == model.jobId)
        await report.jobChanged()
        #expect(model.work?.job.status == .inProgress, "the job is read again")
        (model, _) = try await loaded {
            $0.status = .submitted
            $0.reportRefs = [ReportRef(reportId: "r1", reportVersion: 1)]
        }
        #expect(model.hasReport, "a submitted report is read")
    }

    @Test func eachStateHasOnePrimaryStepAndTheRestInItsMenu() async throws {
        let paused = TimeOnSite(
            arrivedAt: "2026-09-27T01:10:00Z",
            checkInMethod: .locationQr,
            checkInReason: nil,
            distanceMeters: 20,
            startedAt: "2026-09-27T01:10:00Z",
            pauses: [TimeOnSitePauses(from: "2026-09-27T01:20:00Z", to: nil)],
            finishedAt: nil,
            onSiteMinutes: nil
        )
        var (model, _) = try await loaded()
        #expect(model.primaryStep == .accept && model.moreSteps == [.cantMake])
        (model, _) = try await loaded { $0.assignment?.acknowledgement = .accepted }
        #expect(model.primaryStep == .checkIn(enabled: true) && model.moreSteps.isEmpty)
        (model, _) = try await loaded { $0.status = .reworkRequested }
        #expect(model.primaryStep == .resumeRework(enabled: true))
        (model, _) = try await loaded { $0.status = .inProgress }
        #expect(model.primaryStep == .report && model.moreSteps == [.pause], "the report first, the pause in the menu")
        (model, _) = try await loaded {
            $0.status = .inProgress
            $0.timeOnSite = paused
        }
        #expect(model.primaryStep == .resume && model.moreSteps == [.report], "paused: resuming first")
        (model, _) = try await loaded {
            $0.status = .inProgress
            $0.timeOnSite = paused
            $0.assignment?.scheduledEnd = "2026-09-27T01:30:00Z"
        }
        #expect(model.primaryStep == .report && model.moreSteps.isEmpty, "after the window: only the report to read")
        (model, _) = try await loaded {
            $0.status = .inProgress
            $0.assignment?.scheduledEnd = "2026-09-27T01:30:00Z"
        }
        #expect(model.primaryStep == .report && model.moreSteps.isEmpty)
        (model, _) = try await loaded {
            $0.status = .submitted
            $0.reportRefs = [ReportRef(reportId: "r1", reportVersion: 1)]
        }
        #expect(model.primaryStep == .report)
        (model, _) = try await loaded { $0.assignment?.acknowledgement = .cantMake }
        #expect(model.primaryStep == nil, "waiting for the coordinator: nothing to do")
        let (unread, _) = self.model([])
        #expect(unread.primaryStep == nil && unread.moreSteps.isEmpty, "nothing before the job is read")
    }

    @Test func returnedWorkIsResumed() async throws {
        let rework = try detail { $0.status = .reworkRequested }
        let (model, transport) = try await loaded({ $0.status = .reworkRequested }, then: [try answer(rework), try job(), try unit])
        await model.resumeRework()
        #expect(model.note == "Rework resumed — continue in the new draft")
        #expect(await transport.requests[2].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/resume-rework")
    }
}

private struct StubAcknowledgement: Decodable, Equatable {
    let decision: String
    let reason: String?
    let alternativeSlot: Slot?
}

private struct StubCheckInBody: Decodable {
    let method: String
    let position: CheckInPosition?
    let qrUnitId: String?
    let reason: String?
}
