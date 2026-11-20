import ACKit
import ACShared
import Foundation

public typealias ComponentGroup = InspectionItemInputComponentGroup
public typealias CheckResult = InspectionItemInputResult

/// The checklist's words and tables (shared/lib/techJob.ts): the component keys of each group in canonical order
/// (maintenance.ComponentGroups of the Core API), and the labels of a group, a component, a result and a reading.
public enum Checklist {
    public static let groups: [(group: ComponentGroup, keys: [ComponentKey])] = [
        (.indoor, [.filter, .evaporatorCoil, .blowerMotor, .blowerFan, .drainPipe, .drainPan, .outlet, .louver]),
        (.outdoor, [.condenserCoil, .compressor, .fan, .blade, .refrigerantPipe]),
        (.electrical, [.thermostat, .sensor, .capacitor, .contactor, .wiring]),
        (.installation, [.mounting, .refrigerantPiping, .vacuumLeakTest, .electricalConnection, .drainage, .testRun, .customerHandover]),
    ]

    /// The components a report must cover: every key of the groups in the unit's service scope (IR100); an installation
    /// job has its own checklist (DEC-106).
    public static func components(scope: [ComponentGroup]) -> [(group: ComponentGroup, key: ComponentKey)] {
        groups.filter { scope.contains($0.group) }.flatMap { group in group.keys.map { (group.group, $0) } }
    }

    public static func group(of key: ComponentKey) -> ComponentGroup? {
        groups.first { $0.keys.contains(key) }?.group
    }

    public static func label(_ group: ComponentGroup) -> String {
        switch group {
        case .indoor: tr("Indoor")
        case .outdoor: tr("Outdoor")
        case .electrical: tr("Electrical")
        case .installation: tr("Installation")
        }
    }

    public static func label(_ key: ComponentKey) -> String {
        switch key {
        case .filter: tr("Filter")
        case .evaporatorCoil: tr("Evaporator coil")
        case .blowerMotor: tr("Blower motor")
        case .blowerFan: tr("Blower fan")
        case .drainPipe: tr("Drain pipe")
        case .drainPan: tr("Drain pan")
        case .outlet: tr("Outlet")
        case .louver: tr("Louver")
        case .condenserCoil: tr("Condenser coil")
        case .compressor: tr("Compressor")
        case .fan: tr("Fan")
        case .blade: tr("Fan blade")
        case .refrigerantPipe: tr("Refrigerant pipe")
        case .thermostat: tr("Thermostat")
        case .sensor: tr("Sensor")
        case .capacitor: tr("Capacitor")
        case .contactor: tr("Contactor")
        case .wiring: tr("Wiring insulation")
        case .mounting: tr("Mounting")
        case .refrigerantPiping: tr("Refrigerant piping")
        case .vacuumLeakTest: tr("Vacuum and leak test")
        case .electricalConnection: tr("Electrical connection")
        case .drainage: tr("Drainage")
        case .testRun: tr("Test run")
        case .customerHandover: tr("Customer handover")
        }
    }

    public static func label(_ result: CheckResult) -> String {
        switch result {
        case .normal: tr("Normal")
        case .attention: tr("Attention")
        case .notInspected: tr("Not inspected")
        case .notApplicable: tr("Not applicable")
        }
    }

    /// The readings a technician can record, each in its fixed unit (MetricUnits of the Core API).
    public static let metrics: [Metric] = [
        .temperature, .humidity, .refrigerantPressure, .vibration, .power, .compressorCycles, .airflowDrop, .co2, .pm25,
    ]

    public static func label(_ metric: Metric) -> String {
        metric == .power ? tr("Power draw") : Evidence.metric(Substring(metric.rawValue))
    }

    public static func unit(_ metric: Metric) -> UnitSymbol {
        switch metric {
        case .temperature: .c
        case .humidity, .airflowDrop: .value1
        case .co2: .ppm
        case .pm25: .gM
        case .power: .kW
        case .vibration: .mmS
        case .refrigerantPressure: .kPa
        case .compressorCycles: .cyclesH
        case .heartbeatGap: .min
        }
    }

    public static func label(_ source: PartSource) -> String {
        switch source {
        case .vanStock: tr("Van stock")
        case .hqWarehouse: tr("HQ warehouse")
        case .boughtLocally: tr("Bought locally")
        }
    }

    public static func label(_ check: RefrigerantRecordLeakCheck) -> String {
        switch check {
        case .pass: tr("Pass")
        case .fail: tr("Fail")
        case .notDone: tr("Not done")
        }
    }

    /// What a photo of an installation report shows (attachments.add `tag`, DEC-106): the report needs one of each.
    public static func label(_ tag: AttachmentTag) -> String {
        switch tag {
        case .namePlate: tr("Name plate")
        case .indoorUnit: tr("Indoor unit")
        case .outdoorUnit: tr("Outdoor unit")
        }
    }
}

/// The editable draft of a work report (techJob.ts Draft): every required component in canonical order, its result
/// empty until one is chosen (normal is never preselected, DD-T04), the readings as text so a half-typed value survives,
/// and the other sections as saved.
public struct ReportDraft: Sendable, Equatable {
    public struct Item: Sendable, Equatable {
        public let group: ComponentGroup
        public let key: ComponentKey
        public var result: CheckResult?
        public var reason: String
        public var evidenceIds: [String]
    }

    public struct Reading: Sendable, Equatable, Identifiable {
        /// This screen's own identity of the row (a saved reading keeps its measurement's `savedId`).
        public let id: UUID
        public var savedId: String?
        public var key: ComponentKey
        public var metric: Metric
        public var value: String
        public var observedAt: String
    }

    /// The equipment check of an installation: the registered model confirmed as the installed one, and its serial.
    public struct Installation: Sendable, Equatable {
        public var confirmed: Bool
        public var serial: String
    }

    public enum Next: Sendable, Equatable {
        case noFollowUp
        case followUp(date: String, note: String)
    }

    public var items: [Item]
    public var readings: [Reading]
    public var installation: Installation?
    public var parts: [Part]
    public var refrigerant: [RefrigerantRecord]
    public var workText: String
    public var next: Next
    public var attachmentIds: [String]

    /// The draft of a saved report, or an empty one.
    public init(_ report: WorkReport?, components: [(group: ComponentGroup, key: ComponentKey)], isInstallation: Bool) {
        items = components.map { component in
            let saved = report?.items.first { $0.componentKey == component.key }
            return Item(
                group: component.group,
                key: component.key,
                result: saved?.result,
                reason: saved?.reason ?? "",
                evidenceIds: saved?.evidenceIds ?? []
            )
        }
        // each reading on the component it was taken on (IR865)
        readings = (report?.measurements ?? []).map { measurement in
            Reading(
                id: UUID(),
                savedId: measurement.id,
                key: measurement.componentKey,
                metric: measurement.metric,
                value: measurement.value.map(Self.number) ?? "",
                observedAt: measurement.observedAt
            )
        }
        installation =
            isInstallation ? Installation(confirmed: report?.installation != nil, serial: report?.installation?.serial ?? "") : nil
        parts = report?.parts ?? []
        refrigerant = report?.refrigerant ?? []
        workText = report?.workText ?? ""
        switch report?.nextAction {
        case let .followUp(follow)?: next = .followUp(date: follow.date, note: follow.note)
        case .none?, nil: next = .noFollowUp
        }
        attachmentIds = (report?.attachmentRefs ?? []).filter { $0.status != .failed }.map(\.id)
    }

    /// A reading's value as the web writes it: a whole number without a fraction.
    static func number(_ value: Double) -> String {
        value.rounded() == value && abs(value) < 1e15 ? String(Int64(value)) : String(value)
    }

    static func value(_ text: String) -> Double? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let value = Double(trimmed), value.isFinite else { return nil }
        return value
    }

    /// The jobs.saveDraft body: trimmed reasons (empty is none), numeric readings in their metric's unit; only a
    /// confirmed model is sent (the Core API compares it with the unit's).
    public func input(jobId: String, reportId: String?, modelId: String?) -> WorkReportDraft {
        let check: InstallationCheck? =
            if let installation, installation.confirmed, let modelId {
                InstallationCheck(confirmedModelId: modelId, serial: installation.serial.trimmingCharacters(in: .whitespaces))
            } else {
                nil
            }
        let nextAction: NextAction =
            switch next {
            case .noFollowUp: .none(NextActionNone())
            case let .followUp(date, note):
                .followUp(NextActionFollowUp(date: date, note: note.trimmingCharacters(in: .whitespacesAndNewlines)))
            }
        return WorkReportDraft(
            jobId: jobId,
            reportId: reportId,
            installation: check,
            items: items.map { item in
                let reason = item.reason.trimmingCharacters(in: .whitespacesAndNewlines)
                return InspectionItemInput(
                    componentGroup: item.group,
                    componentKey: item.key,
                    result: item.result,
                    reason: reason.isEmpty ? nil : reason,
                    evidenceIds: item.evidenceIds
                )
            },
            measurements: readings.map {
                InspectionMeasurementInput(
                    id: $0.savedId,
                    componentKey: $0.key,
                    metric: $0.metric,
                    value: Self.value($0.value),
                    unit: Checklist.unit($0.metric),
                    observedAt: $0.observedAt
                )
            },
            parts: parts.map { part in
                var trimmed = part
                trimmed.name = part.name.trimmingCharacters(in: .whitespaces)
                return trimmed
            },
            refrigerant: refrigerant,
            workText: workText,
            nextAction: nextAction,
            attachmentIds: attachmentIds
        )
    }

    /// Links a report photo to a component's finding, or unlinks it; a photo is linked to a finding at most once.
    public mutating func link(_ photoId: String, to key: ComponentKey, _ on: Bool) {
        guard let index = items.firstIndex(where: { $0.key == key }) else { return }
        var ids = items[index].evidenceIds.filter { $0 != photoId }
        if on { ids.append(photoId) }
        items[index].evidenceIds = ids
    }
}

/// What blocks a submit (the Core API's submit rules IR100, IR126, checked first on the phone so the technician sees
/// every issue at once; the Core API stays the authority).
public struct ReportIssue: Sendable, Equatable {
    public let key: ComponentKey?
    public let group: ComponentGroup?
    public let text: String
}

extension ReportDraft {
    /// The coarsest a reason, a note and the work text may be: the Core API's limits.
    static let workText = 10...4000

    public func issues(photos: [Attachment], now: Date, signedOff: Bool) -> [ReportIssue] {
        var out: [ReportIssue] = []
        if let installation { out += Self.installationIssues(installation, photos: photos, signedOff: signedOff) }
        for item in items {
            let place = "\(Checklist.label(item.group)) › \(Checklist.label(item.key))"
            if item.result == nil {
                out.append(ReportIssue(key: item.key, group: item.group, text: tr("{where} has no result", ["where": place])))
            } else if item.result != .normal, item.reason.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                out.append(ReportIssue(key: item.key, group: item.group, text: tr("{where} needs a reason", ["where": place])))
            }
        }
        for reading in readings where !reading.value.trimmingCharacters(in: .whitespaces).isEmpty && Self.value(reading.value) == nil {
            let text = tr("{name} reading is not a number", ["name": Checklist.label(reading.key)])
            out.append(ReportIssue(key: reading.key, group: Checklist.group(of: reading.key), text: text))
        }
        if !Self.workText.contains(workText.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count) {
            out.append(ReportIssue(key: nil, group: nil, text: tr("Work performed must be 10–4000 characters")))
        }
        if case let .followUp(date, note) = next {
            let isFuture = Format.parse("\(date)T00:00:00Z").map { $0 > now } ?? false
            if !isFuture || note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                out.append(ReportIssue(key: nil, group: nil, text: tr("A follow-up needs a future date and a note")))
            }
        }
        for (index, part) in parts.enumerated() where part.quantity < 1 || part.quantity > 999 {
            let name = part.name.isEmpty ? tr("unnamed") : part.name
            out.append(
                ReportIssue(
                    key: nil,
                    group: nil,
                    text: tr("Part {n} ({name}) needs a quantity of 1–999", ["n": String(index + 1), "name": name])
                )
            )
        }
        for photo in photos where attachmentIds.contains(photo.id) && photo.status != .ready {
            let state = photo.status == .failed ? tr("failed") : tr("processing")
            out.append(
                ReportIssue(
                    key: nil,
                    group: nil,
                    text: tr("Photo {name} is {state} — remove or retry it", ["name": photo.name, "state": state])
                )
            )
        }
        return out
    }

    /// What an installation report needs besides the checklist (BR-T16): the model confirmed, the serial, a ready photo
    /// of the name plate, the indoor unit and the outdoor unit, and the customer's sign-off.
    static func installationIssues(_ installation: Installation, photos: [Attachment], signedOff: Bool) -> [ReportIssue] {
        var texts: [String] = []
        if !installation.confirmed { texts.append(tr("Confirm that the installed model is the registered one")) }
        if installation.serial.trimmingCharacters(in: .whitespaces).isEmpty {
            texts.append(tr("Enter the serial number of the installed unit"))
        }
        let added = Set(photos.filter { $0.status == .ready }.compactMap(\.tag))
        for tag in AttachmentTag.allCases where !added.contains(tag) {
            texts.append(tr("Add a photo of the {part}", ["part": Checklist.label(tag).lowercased()]))
        }
        if !signedOff { texts.append(tr("An installation needs the customer’s sign-off")) }
        return texts.map { ReportIssue(key: nil, group: nil, text: $0) }
    }

    /// The checklist's progress of one group: results chosen out of its components, and its issues.
    public struct GroupProgress: Sendable, Equatable {
        public let group: ComponentGroup
        public let done: Int
        public let total: Int
        public let issues: Int
    }

    public func progress(_ issues: [ReportIssue]) -> [GroupProgress] {
        Checklist.groups.compactMap { entry in
            let mine = items.filter { $0.group == entry.group }
            guard !mine.isEmpty else { return nil }
            return GroupProgress(
                group: entry.group,
                done: mine.filter { $0.result != nil }.count,
                total: mine.count,
                issues: issues.filter { $0.group == entry.group }.count
            )
        }
    }
}
