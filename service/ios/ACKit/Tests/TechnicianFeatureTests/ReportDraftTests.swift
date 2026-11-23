import ACShared
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// The live draft of the fixture (a filter with attention, an evaporator coil normal, one reading, one part).
func savedReport() throws -> WorkReport {
    struct Envelope: Decodable { let data: WorkReport }
    return try JSONDecoder().decode(Envelope.self, from: try fixture("reports.get.draft")).data
}

func photo(_ id: String, status: AttachmentStatus = .ready, tag: AttachmentTag? = nil) -> ACKit.Attachment {
    ACKit.Attachment(
        id: id,
        tenantId: "t1",
        version: 1,
        createdAt: "2026-09-27T01:00:00Z",
        updatedAt: "2026-09-27T01:00:00Z",
        jobId: "j1",
        reportId: "r1",
        blobId: "b1",
        tag: tag,
        name: "\(id).jpg",
        mime: .imageJpeg,
        size: 10,
        status: status
    )
}

@Suite struct ChecklistTests {
    @Test func theComponentsOfAScopeInCanonicalOrder() {
        let indoor = Checklist.components(scope: [.indoor])
        #expect(indoor.map(\.key) == [.filter, .evaporatorCoil, .blowerMotor, .blowerFan, .drainPipe, .drainPan, .outlet, .louver])
        #expect(
            Checklist.components(scope: [.electrical, .outdoor]).map(\.group) == Array(repeating: .outdoor, count: 5)
                + Array(repeating: .electrical, count: 5)
        )
        #expect(Checklist.group(of: .compressor) == .outdoor)
        #expect(Checklist.group(of: .testRun) == .installation)
    }

    @Test func everyWordHasItsText() {
        #expect(ComponentKey.allCases.allSatisfy { !Checklist.label($0).isEmpty })
        #expect(Checklist.label(ComponentKey.blade) == "Fan blade")
        #expect(Checklist.label(ComponentKey.wiring) == "Wiring insulation")
        #expect(ComponentGroup.allCases.map { Checklist.label($0) } == ["Installation", "Indoor", "Outdoor", "Electrical"])
        #expect(CheckResult.allCases.map { Checklist.label($0) } == ["Normal", "Attention", "Not inspected", "Not applicable"])
        #expect(Checklist.label(Metric.power) == "Power draw")
        #expect(Checklist.label(Metric.refrigerantPressure) == "Refrigerant pressure")
        #expect(PartSource.allCases.map { Checklist.label($0) } == ["Van stock", "HQ warehouse", "Bought locally"])
        #expect(RefrigerantRecordLeakCheck.allCases.map { Checklist.label($0) } == ["Pass", "Fail", "Not done"])
        #expect(AttachmentTag.allCases.map { Checklist.label($0) } == ["Name plate", "Indoor unit", "Outdoor unit"])
    }

    @Test func eachMetricHasItsUnit() {
        let units = Metric.allCases.map { Checklist.unit($0).rawValue }
        #expect(units == ["°C", "%", "ppm", "µg/m³", "kW", "mm/s", "kPa", "cycles/h", "%", "min"])
    }
}

@Suite struct ReportDraftTests {
    private let indoor = Checklist.components(scope: [.indoor])

    @Test func anEmptyDraftHasEveryComponentWithoutAResult() {
        let draft = ReportDraft(nil, components: indoor, isInstallation: false)
        #expect(draft.items.count == 8 && draft.items.allSatisfy { $0.result == nil && $0.reason.isEmpty })
        #expect(draft.readings.isEmpty && draft.parts.isEmpty && draft.workText.isEmpty && draft.next == .noFollowUp)
        #expect(draft.installation == nil)
        #expect(
            ReportDraft(nil, components: indoor, isInstallation: true).installation
                == ReportDraft.Installation(confirmed: false, serial: "")
        )
    }

    @Test func aSavedReportIsEditedAsItWasSaved() throws {
        var report = try savedReport()
        report.attachmentRefs = [photo("p1"), photo("p2", status: .failed)]
        report.nextAction = .followUp(NextActionFollowUp(date: "2026-10-01", note: "Replace the coil"))
        report.installation = InstallationCheck(confirmedModelId: "m1", serial: "SN-1")
        var unmeasured = try #require(report.measurements.first)
        unmeasured.value = nil
        report.measurements.append(unmeasured)
        let draft = ReportDraft(report, components: indoor, isInstallation: true)
        #expect(draft.items[0].result == .attention && draft.items[0].reason == "Clogged")
        #expect(draft.items[1].result == .normal && draft.items[2].result == nil)
        #expect(draft.readings.map(\.key) == [.evaporatorCoil, .evaporatorCoil], "a reading stays on its component")
        #expect(draft.readings.map(\.value) == ["12.5", ""], "an unmeasured value stays empty, never 0")
        #expect(draft.parts.first?.name == "Filter F-200")
        #expect(draft.attachmentIds == ["p1"], "a failed photo is not the draft's")
        #expect(draft.next == .followUp(date: "2026-10-01", note: "Replace the coil"))
        #expect(draft.installation == ReportDraft.Installation(confirmed: true, serial: "SN-1"))
    }

    @Test func numbersAreWrittenAsTheWebWritesThem() {
        #expect(ReportDraft.number(12) == "12")
        #expect(ReportDraft.number(12.5) == "12.5")
        #expect(ReportDraft.number(1e16) == String(1e16))
        #expect(ReportDraft.value(" 3.5 ") == 3.5)
        #expect(ReportDraft.value("") == nil && ReportDraft.value("abc") == nil && ReportDraft.value("inf") == nil)
    }

    @Test func theSaveBodyTrimsAndConvertsTheDraft() throws {
        var draft = ReportDraft(try savedReport(), components: indoor, isInstallation: true)
        draft.items[1].reason = "   "
        draft.readings.append(
            ReportDraft.Reading(id: UUID(), savedId: nil, key: .filter, metric: .humidity, value: "", observedAt: "2026-09-27T01:00:00Z")
        )
        draft.parts[0].name = "  Filter F-200 "
        draft.next = .followUp(date: "2026-10-01", note: " Replace ")
        draft.installation = ReportDraft.Installation(confirmed: true, serial: " SN-9 ")
        let input = draft.input(jobId: "j1", reportId: "r1", modelId: "m1")
        #expect(input.jobId == "j1" && input.reportId == "r1")
        #expect(input.installation == InstallationCheck(confirmedModelId: "m1", serial: "SN-9"))
        #expect(input.items[0].reason == "Clogged" && input.items[1].reason == nil)
        #expect(input.measurements.map(\.value) == [12.5, nil])
        #expect(input.measurements.map(\.unit) == [.c, .value1])
        #expect(input.measurements.first?.id != nil && input.measurements.last?.id == nil)
        #expect(input.parts.first?.name == "Filter F-200")
        #expect(input.nextAction == .followUp(NextActionFollowUp(date: "2026-10-01", note: "Replace")))
        draft.installation?.confirmed = false
        draft.next = .noFollowUp
        let unconfirmed = draft.input(jobId: "j1", reportId: nil, modelId: "m1")
        #expect(unconfirmed.installation == nil, "only a confirmed model is sent")
        #expect(unconfirmed.nextAction == .none(NextActionNone()))
        #expect(draft.input(jobId: "j1", reportId: nil, modelId: nil).installation == nil)
    }

    @Test func aPhotoIsLinkedToAFindingOnce() {
        var draft = ReportDraft(nil, components: indoor, isInstallation: false)
        draft.link("p1", to: .filter, true)
        draft.link("p1", to: .filter, true)
        #expect(draft.items[0].evidenceIds == ["p1"])
        draft.link("p1", to: .filter, false)
        #expect(draft.items[0].evidenceIds.isEmpty)
        draft.link("p1", to: .compressor, true)
        #expect(draft.items.allSatisfy { $0.evidenceIds.isEmpty }, "a component the draft does not have")
    }

    @Test func everyIssueThatBlocksTheSubmitIsListed() throws {
        let now = try #require(Format.parse("2026-09-27T02:00:00Z"))
        var draft = ReportDraft(nil, components: Checklist.components(scope: [.indoor]), isInstallation: false)
        #expect(draft.issues(photos: [], now: now, signedOff: false).count == 9, "8 results and the work text")
        for index in draft.items.indices { draft.items[index].result = .normal }
        draft.items[0].result = .notInspected
        draft.workText = "Cleaned the filter and the drain."
        let issues = draft.issues(photos: [], now: now, signedOff: false)
        #expect(issues == [ReportIssue(key: .filter, group: .indoor, text: "Indoor › Filter needs a reason")])
        draft.items[0].reason = "Locked cabinet"
        #expect(draft.issues(photos: [], now: now, signedOff: false).isEmpty)

        draft.readings = [ReportDraft.Reading(id: UUID(), savedId: nil, key: .drainPan, metric: .temperature, value: "hot", observedAt: "")]
        draft.next = .followUp(date: "2026-09-26", note: "Check")
        draft.parts = [
            Part(
                name: "",
                quantity: 0,
                catalogCode: nil,
                source: .vanStock,
                lotSerial: nil,
                replacesComponentKey: nil,
                oldPartDisposal: nil,
                receiptAttachmentId: nil
            )
        ]
        draft.attachmentIds = ["p1", "p2"]
        draft.workText = "short"
        let texts = draft.issues(
            photos: [photo("p1", status: .processing), photo("p2", status: .failed), photo("p3", status: .failed)],
            now: now,
            signedOff: true
        )
        .map(\.text)
        #expect(
            texts == [
                "Drain pan reading is not a number", "Work performed must be 10–4000 characters",
                "A follow-up needs a future date and a note",
                "Part 1 (unnamed) needs a quantity of 1–999", "Photo p1.jpg is processing — remove or retry it",
                "Photo p2.jpg is failed — remove or retry it",
            ]
        )
        draft.next = .followUp(date: "2026-10-01", note: " ")
        #expect(draft.issues(photos: [], now: now, signedOff: true).map(\.text).contains("A follow-up needs a future date and a note"))
        draft.next = .followUp(date: "someday", note: "Check")
        #expect(draft.issues(photos: [], now: now, signedOff: true).map(\.text).contains("A follow-up needs a future date and a note"))
    }

    /// The Core API's limits on the texts, counted in code points after trimming: one over is listed, the limit is not.
    @Test func aTextOverTheCoreAPIsLimitIsListed() throws {
        let now = try #require(Format.parse("2026-09-27T02:00:00Z"))
        let accent = "e\u{301}"  // one character, two code points
        var draft = ReportDraft(nil, components: Checklist.components(scope: [.installation]), isInstallation: true)
        for index in draft.items.indices { draft.items[index].result = .normal }
        draft.items[0].result = .attention
        draft.items[1].result = .attention
        draft.items[0].reason = " " + String(repeating: "a", count: 1000) + " "
        draft.items[1].reason = String(repeating: accent, count: 501)
        draft.workText = "Installed and tested the unit."
        draft.next = .followUp(date: "2026-10-01", note: String(repeating: accent, count: 501))
        draft.installation = ReportDraft.Installation(confirmed: true, serial: String(repeating: "S", count: 81))
        let photos = AttachmentTag.allCases.map { photo($0.rawValue, tag: $0) }
        let texts = draft.issues(photos: photos, now: now, signedOff: true).map(\.text)
        #expect(
            texts == [
                "Serial number: up to 80 characters", "Installation › Refrigerant piping: up to 1000 characters",
                "What to do (required): up to 1000 characters",
            ]
        )
        let piping = try #require(draft.issues(photos: photos, now: now, signedOff: true).dropFirst().first)
        #expect(piping.key == .refrigerantPiping && piping.group == .installation)
        draft.items[1].reason = String(repeating: "a", count: 1000)
        draft.next = .followUp(date: "2026-10-01", note: String(repeating: "a", count: 1000))
        draft.installation?.serial = " " + String(repeating: "S", count: 80) + " "
        #expect(draft.issues(photos: photos, now: now, signedOff: true).isEmpty)
    }

    @Test func anInstallationNeedsItsEquipmentPhotosAndSignOff() throws {
        let now = try #require(Format.parse("2026-09-27T02:00:00Z"))
        var draft = ReportDraft(nil, components: Checklist.components(scope: [.installation]), isInstallation: true)
        let texts = draft.issues(photos: [photo("p1", status: .processing, tag: .namePlate)], now: now, signedOff: false).map(\.text)
        #expect(
            Array(texts.prefix(6)) == [
                "Confirm that the installed model is the registered one", "Enter the serial number of the installed unit",
                "Add a photo of the name plate", "Add a photo of the indoor unit", "Add a photo of the outdoor unit",
                "An installation needs the customer’s sign-off",
            ]
        )
        draft.installation = ReportDraft.Installation(confirmed: true, serial: "SN-1")
        let photos = AttachmentTag.allCases.map { photo($0.rawValue, tag: $0) }
        let rest = draft.issues(photos: photos, now: now, signedOff: true)
        #expect(rest.allSatisfy { $0.group == .installation || $0.key == nil })
        #expect(!rest.map(\.text).contains { $0.hasPrefix("Add a photo") })
    }

    @Test func theProgressOfEachGroup() {
        var draft = ReportDraft(nil, components: Checklist.components(scope: [.indoor, .electrical]), isInstallation: false)
        draft.items[0].result = .normal
        let issue = ReportIssue(key: .sensor, group: .electrical, text: "")
        #expect(
            draft.progress([issue])
                == [
                    ReportDraft.GroupProgress(group: .indoor, done: 1, total: 8, issues: 0),
                    ReportDraft.GroupProgress(group: .electrical, done: 0, total: 5, issues: 1),
                ]
        )
    }
}

@Suite struct ReportFormsTests {
    private let catalog = [
        PartCatalogItem(code: "F200", name: "Filter F-200", vanStockQuantity: 4),
        PartCatalogItem(code: "X1", name: "Coil", vanStockQuantity: nil),
    ]

    @Test func aPartFromTheCatalogOrNamed() throws {
        var form = PartForm()
        #expect(form.error(catalog: catalog) == nil, "not tried yet")
        #expect(form.part(catalog: catalog) == nil)
        form.isChecked = true
        #expect(form.error(catalog: catalog) == "Choose a catalog part or name it")
        form.code = "F200"
        form.quantity = 2
        form.replaces = .filter
        let part = try #require(form.part(catalog: catalog))
        #expect(part.name == "Filter F-200" && part.catalogCode == "F200" && part.quantity == 2)
        #expect(part.oldPartDisposal == .disposedOnSite)
        form.code = nil
        form.name = " Gasket "
        form.replaces = nil
        form.source = .boughtLocally
        let named = try #require(form.part(catalog: catalog))
        #expect(named.name == "Gasket" && named.catalogCode == nil && named.oldPartDisposal == nil && named.source == .boughtLocally)
        form.quantity = 1000
        #expect(form.error(catalog: catalog) == "Quantity must be 1–999")
        form.quantity = 1
        form.name = " " + String(repeating: "e\u{301}", count: 61)
        #expect(form.error(catalog: catalog) == "Part name: up to 120 characters", "122 code points")
        #expect(form.part(catalog: catalog) == nil)
        form.name = String(repeating: "a", count: 120)
        #expect(form.error(catalog: catalog) == nil)
    }

    @Test func aCatalogPartSaysItsVanStock() {
        #expect(PartForm.label(catalog[0]) == "Filter F-200 · F200 · 4 in van")
        #expect(PartForm.label(catalog[1]) == "Coil · X1")
    }

    @Test func aRefrigerantRecordNeedsItsCylinderAndKilograms() {
        var form = RefrigerantForm()
        #expect(form.error == "The cylinder ID is required" && form.record == nil)
        form.cylinder = " C-7 "
        form.recovered = ""
        form.charged = "0.4"
        #expect(form.error == "Kilograms, 0 or more", "an empty field is refused, not read as 0 kg")
        form.recovered = "-1"
        #expect(form.error == "Kilograms, 0 or more")
        form.recovered = "0"
        form.kind = .r410a
        form.leakCheck = .pass
        #expect(
            form.record
                == RefrigerantRecord(
                    refrigerant: .r410a,
                    cylinderId: "C-7",
                    recoveredKg: 0,
                    chargedKg: 0.4,
                    leakCheck: .pass,
                    leakCheckMethod: nil
                )
        )
        form.cylinder = String(repeating: "C", count: 65)
        #expect(form.error == "Cylinder ID: up to 64 characters" && form.record == nil)
        form.cylinder = " " + String(repeating: "C", count: 64) + " "
        #expect(form.error == nil)
    }
}
