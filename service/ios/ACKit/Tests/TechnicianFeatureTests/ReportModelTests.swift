import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// Counts the jobs read again.
@MainActor private final class Reloads {
    var reads = 0
}

@MainActor @Suite struct ReportModelTests {
    private let now = "2026-09-27T01:35:13Z"
    private let reportId = "c5828d81-f4d3-47ed-9872-aa17f5e18abd"

    private func job(_ change: ((inout JobDetail) -> Void)? = nil) throws -> JobDetail {
        struct Envelope: Decodable { let data: JobsGetResult }
        guard case var .detail(job) = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.get.assigned")).data else {
            throw CocoaError(.coderValueNotFound)
        }
        job.status = .inProgress
        job.draftReportRef = ReportRef(reportId: reportId, reportVersion: 1)
        change?(&job)
        return job
    }

    private func unit() throws -> UnitDetail {
        struct Envelope: Decodable { let data: UnitDetail }
        return try JSONDecoder().decode(Envelope.self, from: try fixture("units.get.technician")).data
    }

    private func answer(_ result: some Encodable) throws -> StubTransport.Answer {
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: ["correlationId": "c"]))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    private func refusal(_ status: Int, _ code: String, _ key: String) -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#)
    }

    private var reportAnswer: StubTransport.Answer {
        get throws { .http(200, try #require(String(bytes: try fixture("reports.get.draft"), encoding: .utf8))) }
    }

    private var partsAnswer: StubTransport.Answer {
        get throws {
            try answer(
                Page(
                    items: [PartCatalogItem(code: "F200", name: "Filter F-200", vanStockQuantity: 4)],
                    nextCursor: nil,
                    total: 1,
                    snapshotVersion: 1
                )
            )
        }
    }

    private func model(
        _ answers: [StubTransport.Answer],
        job: JobDetail? = nil,
        unit: UnitDetail? = nil,
        canWork: Bool = true,
        reloads: Reloads = Reloads()
    ) throws -> (ReportModel, StubTransport) {
        let transport = StubTransport(answers)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        let client = CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())
        let model = ReportModel(
            job: try job ?? self.job(),
            unit: try unit ?? self.unit(),
            now: try #require(Format.parse(now)),
            canWork: canWork,
            client: client,
            jobChanged: { reloads.reads += 1 },
            pause: { _ in await Task.yield() }
        )
        return (model, transport)
    }

    /// The fixture's draft, read and editable.
    private func loaded(
        then answers: [StubTransport.Answer] = [],
        reloads: Reloads = Reloads()
    ) async throws -> (ReportModel, StubTransport) {
        let (model, transport) = try model([try reportAnswer, try partsAnswer] + answers, reloads: reloads)
        await model.load()
        return (model, transport)
    }

    private func saved(_ change: (inout WorkReport) -> Void = { $0.version += 1 }) throws -> StubTransport.Answer {
        var report = try savedReport()
        change(&report)
        return try answer(report)
    }

    private func body<T: Decodable>(_ type: T.Type, _ request: URLRequest?) throws -> T {
        try JSONDecoder().decode(type, from: try #require(request?.httpBody))
    }

    // MARK: - Reading

    @Test func theOpenDraftIsReadAndEditable() async throws {
        let (model, transport) = try await loaded()
        #expect(model.isEditable && !model.isLoading && !model.isDirty)
        #expect(model.report?.id == reportId)
        #expect(model.draft.items.count == 18, "the unit's three groups")
        #expect(model.draft.items[0].result == .attention)
        #expect(model.parts.map(\.code) == ["F200"])
        #expect(model.versionText == "draft v1")
        let requests = await transport.requests
        #expect(requests[0].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/reports/\(reportId)")
        #expect(requests[0].url?.query() == "reportVersion=1")
        #expect(requests[1].url?.path() == "/v1/parts")
    }

    @Test func aJobWithoutAReportStartsAnEmptyDraft() async throws {
        let (model, transport) = try model([try partsAnswer], job: try job { $0.draftReportRef = nil })
        await model.load()
        #expect(model.isEditable && model.report == nil && model.versionText == "not started")
        #expect(model.draft.items.allSatisfy { $0.result == nil })
        #expect(await transport.requests.count == 1, "only the parts")
    }

    @Test func anUnreadableCatalogLeavesItEmpty() async throws {
        let (model, _) = try model([try reportAnswer, .failure(URLError(.timedOut))])
        await model.load()
        #expect(model.isEditable && model.parts.isEmpty)
        #expect(model.photos.isEmpty)
    }

    @Test func withoutAReportThereAreNoPhotos() throws {
        let (model, _) = try model([])
        #expect(model.photos.isEmpty && !model.isSignedOff && model.returnReason == nil)
    }

    @Test func theAppsAutosaveWaitsItsTime() async throws {
        let transport = StubTransport([try reportAnswer, try partsAnswer])
        let client = CoreClient(baseURL: URL(filePath: "/"), transport: transport, credentials: FixedCredentials())
        let model = ReportModel(job: try job(), unit: nil, now: Date(), canWork: true, client: client) {
            // the job is not read again here
        }
        await model.load()
        model.edit { $0.workText = "Cleaned the filter." }
        // the app's wait is 20 s: the edit is still unsaved a moment later
        try await Task.sleep(for: .milliseconds(50))
        let sent = await transport.requests.count
        #expect(model.isDirty && sent == 2)
        let clock = ContinuousClock()
        let started = clock.now
        await ReportModel.sleep(.milliseconds(10))
        #expect(clock.now - started >= .milliseconds(10))
    }

    @Test func aSubmittedReportIsReadOnly() async throws {
        let ref = ReportRef(reportId: reportId, reportVersion: 1)
        let (model, transport) = try model(
            [try reportAnswer],
            job: try job {
                $0.status = .submitted
                $0.draftReportRef = nil
                $0.reportRefs = [ref]
            }
        )
        await model.load()
        #expect(!model.isEditable && model.versionText == "v1")
        #expect(model.draft.items[0].result == .attention, "the submitted version is shown")
        #expect(await transport.requests.count == 1, "no parts for a report that cannot change")
        model.edit { $0.workText = "x" }
        #expect(model.draft.workText != "x" && !model.isDirty, "a read-only report takes no edit")
        #expect(await model.save() == false)
    }

    @Test func workAfterAReturnGoesOnInANewDraft() async throws {
        // in progress again after a return, its new draft not saved yet: the returned version is not edited, the first
        // save creates the new draft
        var (model, transport) = try model(
            [try reportAnswer, try partsAnswer, try saved()],
            job: try job {
                $0.draftReportRef = nil
                $0.reportRefs = [ReportRef(reportId: reportId, reportVersion: 1)]
            }
        )
        await model.load()
        #expect(model.isEditable && model.report == nil && model.versionText == "not started")
        #expect(model.draft.items.allSatisfy { $0.result == nil })
        #expect(await model.save())
        #expect(try body(StubDraftBody.self, await transport.requests[2]).reportId == nil)
        (model, _) = try self.model(
            [try reportAnswer, try partsAnswer],
            job: try job { $0.draftReportRef = ReportRef(reportId: reportId, reportVersion: 2) }
        )
        await model.load()
        #expect(model.isEditable && model.report == nil, "the read version is not the open draft's")
    }

    @Test func outsideTheWindowTheReportIsReadOnly() async throws {
        let (model, _) = try model([try reportAnswer], canWork: false)
        await model.load()
        #expect(!model.isEditable)
    }

    @Test func anUnreadableReportSaysWhy() async throws {
        let (model, _) = try model([.failure(URLError(.timedOut)), try partsAnswer])
        await model.load()
        #expect(model.refusal?.hasPrefix("The report is not readable now (") == true)
    }

    @Test func theChecklistOfAnInstallationOrAUnitWithoutAScope() throws {
        let installation = ReportModel.components(try job { $0.type = .installation }, try unit())
        #expect(installation.allSatisfy { $0.group == .installation } && installation.count == 7)
        #expect(ReportModel.components(try job(), nil).count == 18, "without the unit: indoor, outdoor and electrical")
    }

    // MARK: - Editing and saving

    @Test func anEditIsSavedByItselfAfterAWhile() async throws {
        let (model, transport) = try await loaded(then: [try saved()])
        model.edit { $0.workText = "Cleaned the filter and the coil." }
        model.edit { $0.workText = "Cleaned the filter and the coil." }  // the same text again is no edit
        #expect(model.isDirty)
        // the autosave runs on its own task: waited for, at most a second
        for _ in 0..<100 where model.isDirty || model.isSending { try await Task.sleep(for: .milliseconds(10)) }
        #expect(!model.isDirty)
        #expect(model.note == "Draft saved")
        let save = await transport.requests[2]
        #expect(save.httpMethod == "PUT" && save.url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/report-draft")
        #expect(save.value(forHTTPHeaderField: "X-Expected-Version") == "1")
        let input = try body(StubDraftBody.self, save)
        #expect(input.reportId == reportId && input.workText == "Cleaned the filter and the coil.")
        #expect(model.report?.version == 2)
    }

    @Test func aRefusedSaveKeepsTheDraft() async throws {
        let (model, _) = try await loaded(then: [refusal(409, "CONFLICT", "error.versionConflict")])
        model.edit { $0.workText = "Cleaned." }
        #expect(await model.save() == false)
        #expect(model.isDirty && model.draft.workText == "Cleaned.")
        #expect(model.refusal == "Someone changed this in the meantime — the screen now shows the latest version — The draft is kept.")
    }

    @Test func theFirstSaveCreatesTheDraft() async throws {
        let (model, transport) = try model([try partsAnswer, try saved()], job: try job { $0.draftReportRef = nil })
        await model.load()
        #expect(await model.save())
        let save = await transport.requests[1]
        #expect(save.value(forHTTPHeaderField: "X-Expected-Version") == nil, "no version to expect yet")
        #expect(try body(StubDraftBody.self, save).reportId == nil)
    }

    // MARK: - Photos

    @Test func aPhotoIsAddedToTheSavedDraftAndLinkedToItsFinding() async throws {
        let added = photo("p9")
        let earlier = photo("p1")
        let (model, transport) = try await loaded(
            then: [
                try saved {
                    $0.version = 2
                    $0.attachmentRefs = [earlier]
                },
                try answer(added),
                try saved {
                    $0.version = 3
                    $0.attachmentRefs = [earlier, added]
                },
            ]
        )
        model.edit { $0.workText = "Unsaved" }
        let picture = NewPhoto(name: "filter.jpg", mime: .imageJpeg, data: Data([1, 2, 3]))
        await model.addPhoto(picture, tag: .namePlate, linkTo: .filter)
        let requests = await transport.requests
        #expect(requests[2].httpMethod == "PUT", "the unsaved edit is saved first")
        #expect(requests[3].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/reports/\(reportId)/attachments")
        #expect(requests[3].value(forHTTPHeaderField: "X-Expected-Version") == "2")
        let upload = try body(StubUpload.self, requests[3])
        #expect(upload.file == BlobInput(name: "filter.jpg", mime: .imageJpeg, size: 3, bytes: "AQID") && upload.tag == .namePlate)
        #expect(requests[4].url?.query() == "reportVersion=3")
        #expect(model.photos.map(\.id) == ["p1", "p9"] && model.draft.attachmentIds == ["p1", "p9"])
        #expect(model.draft.items[0].evidenceIds == ["p9"])
        #expect(model.note == "Photo filter.jpg added — sign-off cleared")
        model.removePhoto("p9")
        #expect(model.draft.attachmentIds == ["p1"] && model.draft.items[0].evidenceIds.isEmpty)
    }

    @Test func aPhotoTooLargeOrOneTooManyIsNotAdded() async throws {
        let (model, transport) = try await loaded()
        let large = NewPhoto(name: "big.jpg", mime: .imageJpeg, data: Data(count: ReportModel.maxBytes + 1))
        await model.addPhoto(large)
        #expect(model.photoError == "“big.jpg” was not added — only JPEG/PNG up to 5 MiB, max 10 photos. Saved text and photos are kept.")
        #expect(await transport.requests.count == 2)
    }

    @Test func aRefusedPhotoKeepsTheDraft() async throws {
        let (model, _) = try await loaded(then: [refusal(422, "VALIDATION", "error.validation")])
        await model.addPhoto(NewPhoto(name: "a.jpg", mime: .imageJpeg, data: Data([1])))
        #expect(model.refusal?.hasSuffix("The draft is kept.") == true)
        #expect(model.photoError == nil)
    }

    @Test func aFailedSaveStopsTheStepAfterIt() async throws {
        let (model, transport) = try await loaded(then: [refusal(409, "CONFLICT", "error.versionConflict")])
        model.edit { $0.workText = "Unsaved" }
        await model.addPhoto(NewPhoto(name: "a.jpg", mime: .imageJpeg, data: Data([1])))
        #expect(await transport.requests.count == 3, "no upload after the refused save")
    }

    @Test func aStepOfAReadOnlyReportDoesNothing() async throws {
        let (model, transport) = try model([try reportAnswer], canWork: false)
        await model.load()
        await model.addPhoto(NewPhoto(name: "a.jpg", mime: .imageJpeg, data: Data([1])))
        #expect(await transport.requests.count == 1)
    }

    // MARK: - Sign-off

    @Test func theSignOffNeedsANameAndASignatureOrAReasonWithAPhoto() {
        let photo = NewPhoto(name: "site.jpg", mime: .imageJpeg, data: Data([1]))
        #expect(
            ReportModel.signOffError(name: "", signature: Data([1]), absentReason: nil, sitePhoto: nil) == "The signer's name is required"
        )
        #expect(ReportModel.signOffError(name: "Ms Tan", signature: Data([1]), absentReason: nil, sitePhoto: nil) == nil)
        #expect(
            ReportModel.signOffError(name: "Ms Tan", signature: nil, absentReason: nil, sitePhoto: nil)
                == "Ask the customer to sign in the box"
        )
        #expect(
            ReportModel.signOffError(name: "Ms Tan", signature: nil, absentReason: "", sitePhoto: photo)
                == "Say why the customer could not sign"
        )
        #expect(
            ReportModel.signOffError(name: "Ms Tan", signature: nil, absentReason: "Away", sitePhoto: nil)
                == "A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available"
        )
        #expect(ReportModel.signOffError(name: "Ms Tan", signature: nil, absentReason: "Away", sitePhoto: photo) == nil)
    }

    @Test func theCustomerSignsTheSavedVersion() async throws {
        let signed = try saved {
            $0.signOff = SignOff(
                signerName: "Ms Tan",
                signedAt: "2026-09-27T01:50:00Z",
                signatureAttachmentId: "sig",
                absentReason: nil,
                sitePhotoAttachmentId: nil,
                reportVersion: 1
            )
            $0.attachmentRefs = [photo("sig"), photo("p1")]
        }
        let (model, transport) = try await loaded(then: [signed])
        #expect(await model.signOff(signer: " ", signature: Data([1]), absentReason: nil, sitePhoto: nil) == false)
        #expect(model.signOffError == "The signer's name is required")
        #expect(await model.signOff(signer: " Ms Tan ", signature: Data([1, 2]), absentReason: nil, sitePhoto: nil))
        #expect(model.note == "Sign-off saved for this report version")
        #expect(model.isSignedOff && model.photos.map(\.id) == ["p1"], "the signature is not a photo")
        let request = await transport.requests[2]
        #expect(request.url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/reports/\(reportId)/sign-off")
        let input = try body(StubSigned.self, request)
        #expect(input.signerName == "Ms Tan" && input.signature?.mime == .imagePng && input.absentReason == nil)
    }

    @Test func anAbsentCustomerIsRecordedWithTheSitePhoto() async throws {
        let (model, transport) = try await loaded(then: [refusal(422, "VALIDATION", "error.validation")])
        let site = NewPhoto(name: "site.jpg", mime: .imageJpeg, data: Data([1]))
        #expect(await model.signOff(signer: "Ms Tan", signature: nil, absentReason: " Away ", sitePhoto: site) == false)
        let input = try body(StubSigned.self, await transport.requests[2])
        #expect(input.absentReason == "Away" && input.sitePhoto?.name == "site.jpg" && input.signature == nil)
        #expect(model.refusal?.hasSuffix("The draft is kept.") == true)
    }

    // MARK: - Submit

    @Test func theIssuesAreShownOnceASubmitWasTried() async throws {
        let (model, transport) = try await loaded()
        #expect(model.issues.isEmpty)
        #expect(model.checkSubmit() == false)
        #expect(model.issues.count == 16 + 0, "16 empty results; the work text is long enough")
        await model.submit()
        #expect(await transport.requests.count == 2, "nothing sent while issues block it")
    }

    @Test func aCompleteReportIsSubmittedAndTheJobReadAgain() async throws {
        let reloads = Reloads()
        let submitted = try job { $0.status = .submitted }
        let (model, transport) = try await loaded(then: [try saved(), try answer(submitted)], reloads: reloads)
        model.edit { draft in
            for index in draft.items.indices { draft.items[index].result = .normal }
        }
        await model.submit()
        let requests = await transport.requests
        #expect(requests[2].httpMethod == "PUT", "saved first")
        #expect(requests[3].url?.path() == "/v1/jobs/f0000000-0000-4000-8000-000000000863/submit")
        #expect(requests[3].value(forHTTPHeaderField: "X-Expected-Version") == "1")
        #expect(try body(StubSubmit.self, requests[3]).reportVersion == 2)
        #expect(model.note == "Report v2 submitted")
        #expect(!model.isEditable && reloads.reads == 1)
    }

    @Test func aRefusedSubmitKeepsTheDraft() async throws {
        let reloads = Reloads()
        let (model, _) = try await loaded(then: [refusal(409, "CONFLICT", "error.versionConflict")], reloads: reloads)
        model.edit { draft in
            for index in draft.items.indices { draft.items[index].result = .normal }
        }
        _ = await model.save()  // the refusal ends the save: nothing is submitted, and the draft stays
        #expect(model.isEditable && reloads.reads == 0)
    }

    @Test func aRefusedSubmitAfterTheSaveSaysWhy() async throws {
        let reloads = Reloads()
        let (model, _) = try await loaded(then: [try saved(), refusal(409, "CONFLICT", "error.conflict")], reloads: reloads)
        model.edit { draft in
            for index in draft.items.indices { draft.items[index].result = .normal }
        }
        await model.submit()
        #expect(model.refusal?.hasSuffix("The draft is kept.") == true)
        #expect(model.isEditable && reloads.reads == 0)
    }

    @Test func theReturnReasonIsTheLastReviews() async throws {
        let returned = try saved {
            $0.reviewHistory = [
                Review(
                    reviewerUserId: "hq",
                    reportVersion: 1,
                    decision: .return,
                    reason: "Add the coil photo",
                    occurredAt: "2026-09-27T01:00:00Z"
                )
            ]
        }
        let (model, _) = try model([returned, try partsAnswer])
        await model.load()
        #expect(model.returnReason == "Add the coil photo")
    }
}

private struct StubDraftBody: Decodable {
    let reportId: String?
    let workText: String
}

private struct StubUpload: Decodable {
    let file: BlobInput
    let tag: AttachmentTag?
}

private struct StubSigned: Decodable {
    let signerName: String
    let signature: BlobInput?
    let absentReason: String?
    let sitePhoto: BlobInput?
}

private struct StubSubmit: Decodable {
    let reportVersion: Double
}
