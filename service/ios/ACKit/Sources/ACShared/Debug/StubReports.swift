#if DEBUG
import ACKit
import Foundation

/// The work report of the UI tests' job j1 (AC_UI_TEST=stub): the first save creates draft r1, each save, photo and
/// sign-off makes its next version, as the Core API does; a save clears the sign-off; the submit ends the draft.
final class StubReports {
    private(set) var report: WorkReport?
    private let at: () -> String = { Date().ISO8601Format() }

    /// The parts catalog of the technician's van.
    static let parts = [PartCatalogItem(code: "F200", name: "Filter F-200", vanStockQuantity: 4)]

    /// The answer of a report route for the job, or nil for a route that is not one.
    func answer(_ method: String, _ path: String, _ body: Data?, job: inout JobDetail) throws -> (Int, Data)? {
        let encoder = JSONEncoder()
        switch (method, path) {
        case ("GET", "/v1/parts"):
            let page = Page(items: Self.parts, nextCursor: nil, total: 1, snapshotVersion: 1)
            return (200, try encoder.encode(StubReportAnswer(data: page)))
        case ("GET", "/v1/jobs/j1/reports/r1"):
            return report.map { (200, (try? encoder.encode(StubReportAnswer(data: $0))) ?? Data()) }
        case ("PUT", "/v1/jobs/j1/report-draft"):
            let input = try JSONDecoder().decode(StubDraft.self, from: body ?? Data())
            let saved = save(input)
            job.draftReportRef = ReportRef(reportId: saved.id, reportVersion: saved.version)
            return (200, try encoder.encode(StubReportAnswer(data: saved)))
        case ("POST", "/v1/jobs/j1/reports/r1/attachments"):
            let input = try JSONDecoder().decode(StubPhoto.self, from: body ?? Data())
            guard var report else { return nil }
            let photo = Attachment(
                id: "ph\(report.attachmentRefs.count + 1)",
                tenantId: "t1",
                version: 1,
                createdAt: at(),
                updatedAt: at(),
                jobId: "j1",
                reportId: report.id,
                blobId: "b1",
                tag: input.tag,
                name: input.file.name,
                mime: input.file.mime,
                size: input.file.size,
                status: .ready
            )
            report.attachmentRefs.append(photo)
            report.signOff = nil
            report.version += 1
            self.report = report
            job.draftReportRef = ReportRef(reportId: report.id, reportVersion: report.version)
            return (200, try encoder.encode(StubReportAnswer(data: photo)))
        case ("POST", "/v1/jobs/j1/reports/r1/sign-off"):
            let input = try JSONDecoder().decode(StubSignOff.self, from: body ?? Data())
            guard var report else { return nil }
            report.signOff = SignOff(
                signerName: input.signerName,
                signedAt: at(),
                signatureAttachmentId: input.signature == nil ? nil : "sig1",
                absentReason: input.absentReason,
                sitePhotoAttachmentId: nil,
                reportVersion: report.version
            )
            self.report = report
            return (200, try encoder.encode(StubReportAnswer(data: report)))
        case ("POST", "/v1/jobs/j1/submit"):
            guard let report else { return nil }
            job.status = .submitted
            job.reportRefs.append(ReportRef(reportId: report.id, reportVersion: report.version))
            job.draftReportRef = nil
            job.version += 1
            return (200, try encoder.encode(StubReportAnswer(data: job)))
        default:
            return nil
        }
    }

    private func save(_ input: StubDraft) -> WorkReport {
        let version = (report?.version ?? 0) + 1
        let items = input.items.map { item in
            InspectionItem(
                id: "i-\(item.componentKey.rawValue)",
                componentGroup: item.componentGroup,
                componentKey: item.componentKey,
                result: item.result,
                reason: item.reason,
                evidenceIds: item.evidenceIds,
                authorId: "m1",
                observedAt: at()
            )
        }
        let measurements = input.measurements.enumerated().map { index, reading in
            ReportMeasurement(
                id: reading.id ?? "me\(index + 1)",
                tenantId: "t1",
                version: 1,
                createdAt: at(),
                updatedAt: at(),
                boundaryId: nil,
                qualityReason: nil,
                rawUnit: nil,
                unitId: "u1",
                sensorId: "s1",
                metric: reading.metric,
                value: reading.value,
                unit: reading.unit,
                observedAt: reading.observedAt,
                receivedAt: at(),
                origin: .inspection,
                quality: .valid,
                sequence: Double(index),
                eventId: "e\(index)",
                componentKey: reading.componentKey
            )
        }
        let saved = WorkReport(
            id: "r1",
            tenantId: "t1",
            version: Double(version),
            createdAt: report?.createdAt ?? at(),
            updatedAt: at(),
            reviewAvailability: .false(ReviewAvailabilityFalse(reason: .notSubmitted)),
            jobId: "j1",
            authorId: "m1",
            installation: nil,
            items: items,
            measurements: measurements,
            parts: input.parts,
            refrigerant: input.refrigerant,
            signOff: nil,
            workText: input.workText,
            nextAction: input.nextAction,
            attachmentRefs: (report?.attachmentRefs ?? []).filter { input.attachmentIds.contains($0.id) },
            submittedAt: nil,
            acceptedAt: nil,
            reviewHistory: []
        )
        report = saved
        return saved
    }
}

/// jobs.saveDraft's body as the route carries it (the job is the path's).
private struct StubDraft: Decodable {
    let items: [InspectionItemInput]
    let measurements: [InspectionMeasurementInput]
    let parts: [Part]
    let refrigerant: [RefrigerantRecord]
    let workText: String
    let nextAction: NextAction?
    let attachmentIds: [String]
}

private struct StubPhoto: Decodable {
    let file: BlobInput
    let tag: AttachmentTag?
}

private struct StubSignOff: Decodable {
    let signerName: String
    let signature: BlobInput?
    let absentReason: String?
}

private struct StubReportAnswer<Result: Encodable>: Encodable {
    let data: Result
    let meta = ["correlationId": "stub"]
}
#endif
