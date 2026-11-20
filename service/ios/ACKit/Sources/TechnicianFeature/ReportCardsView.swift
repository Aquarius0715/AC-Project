import ACKit
import ACShared
import SwiftUI

/// The equipment check of an installation (FR-T16): the registered model confirmed, the serial and the three photos.
struct EquipmentCard<Add: View>: View {
    @Bindable var model: ReportModel
    let add: (String, ReportView.PhotoTarget, String) -> Add

    var body: some View {
        Card(tr("Equipment check")) {
            Text(verbatim: tr("The installed unit against the register")).font(.subheadline)
            let installation = model.draft.installation
            let modelName = model.unit.map { "\($0.capabilities.manufacturer) \($0.capabilities.model)" } ?? "—"
            Toggle(
                isOn: Binding(
                    get: { installation?.confirmed ?? false },
                    set: { on in model.edit { $0.installation?.confirmed = on } }
                )
            ) { Text(verbatim: tr("The installed model is {model}", ["model": modelName])) }
            .disabled(!model.isEditable || model.unit == nil)
            .accessibilityIdentifier("report.installation.confirmed")
            TextField(
                tr("Serial number"),
                text: Binding(get: { installation?.serial ?? "" }, set: { serial in model.edit { $0.installation?.serial = serial } }),
                axis: .vertical
            )
            .textFieldStyle(.roundedBorder)
            .disabled(!model.isEditable)
            .accessibilityIdentifier("report.installation.serial")
            ForEach(AttachmentTag.allCases, id: \.self) { tag in
                let isAdded = model.photos.contains { $0.tag == tag && $0.status == .ready }
                VStack(alignment: .leading) {
                    Text(verbatim: "\(Checklist.label(tag)) · \(isAdded ? tr("Added") : tr("Missing"))").font(
                        .subheadline.weight(.semibold)
                    )
                    if model.isEditable {
                        add(
                            isAdded ? tr("Replace") : tr("Add photo"),
                            ReportView.PhotoTarget(tag: tag),
                            "report.installation.\(tag.rawValue)"
                        )
                    }
                }
            }
        }
    }
}

/// One group of the checklist: each component's result, and its reason when it is not normal.
struct ChecklistCard<Add: View>: View {
    @Bindable var model: ReportModel
    let group: ComponentGroup
    let done: Int
    let total: Int
    let add: (String, ReportView.PhotoTarget, String) -> Add

    var body: some View {
        Card("\(Checklist.label(group)) · \(done) / \(total)") {
            ForEach(model.draft.items.filter { $0.group == group }, id: \.key) { item in
                row(item)
                Divider()
            }
            if model.isEditable {
                Text(
                    verbatim: tr(
                        "Initial result is always empty — normal is never preselected. Attention, not inspected and not applicable need a reason."
                    )
                )
                .font(.footnote)
            }
        }
    }

    @ViewBuilder
    private func row(_ item: ReportDraft.Item) -> some View {
        let name = Checklist.label(item.key)
        VStack(alignment: .leading, spacing: 8) {
            Text(verbatim: name).font(.headline)
            if model.isEditable {
                Picker(selection: result(item.key)) {
                    Text(verbatim: tr("— Select result")).tag(CheckResult?.none)
                    ForEach(CheckResult.allCases, id: \.self) { Text(verbatim: Checklist.label($0)).tag(CheckResult?.some($0)) }
                } label: {
                    Text(verbatim: tr("Result of {item}", ["item": name]))
                }
                .pickerStyle(.menu)
                .accessibilityIdentifier("report.result.\(item.key.rawValue)")
                if let result = item.result, result != .normal {
                    TextField(
                        tr("Reason (required for {result})", ["result": Checklist.label(result)]),
                        text: reason(item.key),
                        axis: .vertical
                    )
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("report.reason.\(item.key.rawValue)")
                }
                photos(item)
            } else {
                Text(verbatim: item.result.map(Checklist.label) ?? tr("— no result"))
                if !item.reason.isEmpty { Text(verbatim: tr("Reason: {reason}", ["reason": item.reason])).font(.subheadline) }
            }
            if let issue = model.issues.first(where: { $0.key == item.key }) {
                Text(verbatim: issue.text).font(.subheadline).accessibilityIdentifier("report.issue.\(item.key.rawValue)")
            }
        }
    }

    /// The report photos of this finding: linked ones can be unlinked, free ones linked, and one taken for it.
    @ViewBuilder
    private func photos(_ item: ReportDraft.Item) -> some View {
        let ready = model.photos.filter { $0.status == .ready && model.draft.attachmentIds.contains($0.id) }
        if item.result == .attention || !item.evidenceIds.isEmpty {
            ForEach(ready, id: \.id) { photo in
                Toggle(
                    isOn: Binding(
                        get: { item.evidenceIds.contains(photo.id) },
                        set: { on in model.edit { $0.link(photo.id, to: item.key, on) } }
                    )
                ) { Text(verbatim: photo.name) }
            }
            add(tr("Add photo"), ReportView.PhotoTarget(key: item.key), "report.photo.\(item.key.rawValue)")
        }
    }

    private func result(_ key: ComponentKey) -> Binding<CheckResult?> {
        Binding(
            get: { model.draft.items.first { $0.key == key }?.result },
            set: { value in
                model.edit { draft in if let i = draft.items.firstIndex(where: { $0.key == key }) { draft.items[i].result = value } }
            }
        )
    }

    private func reason(_ key: ComponentKey) -> Binding<String> {
        Binding(
            get: { model.draft.items.first { $0.key == key }?.reason ?? "" },
            set: { value in
                model.edit { draft in if let i = draft.items.firstIndex(where: { $0.key == key }) { draft.items[i].reason = value } }
            }
        )
    }
}

/// The readings: each a component, a metric and its value in the metric's unit.
struct ReadingsCard: View {
    @Bindable var model: ReportModel

    var body: some View {
        Card(tr("Readings")) {
            if model.draft.readings.isEmpty {
                Text(
                    verbatim: tr(
                        "No readings yet. Record each measurement with its unit and the component it belongs to — an unmeasured value is left empty, never 0."
                    )
                )
                .font(.subheadline)
            }
            ForEach(model.draft.readings) { reading in
                if model.isEditable { editable(reading) } else { Text(verbatim: line(reading)) }
                Divider()
            }
            if model.isEditable {
                Button(tr("Add reading")) {
                    let key = model.draft.items.first?.key ?? .filter
                    model.edit {
                        $0.readings.append(
                            ReportDraft.Reading(
                                id: UUID(),
                                savedId: nil,
                                key: key,
                                metric: .temperature,
                                value: "",
                                observedAt: Date().ISO8601Format()
                            )
                        )
                    }
                }
                .secondaryAction()
                .accessibilityIdentifier("report.reading.add")
            }
        }
    }

    private func line(_ reading: ReportDraft.Reading) -> String {
        let value = reading.value.isEmpty ? "—" : reading.value
        return "\(Checklist.label(reading.key)) · \(Checklist.label(reading.metric)) · \(value) \(Checklist.unit(reading.metric).rawValue)"
    }

    @ViewBuilder
    private func editable(_ reading: ReportDraft.Reading) -> some View {
        Picker(selection: binding(reading.id, \.key, default: .filter)) {
            ForEach(model.draft.items, id: \.key) { Text(verbatim: Checklist.label($0.key)).tag($0.key) }
        } label: {
            Text(verbatim: tr("Linked item"))
        }
        .pickerStyle(.menu)
        Picker(selection: binding(reading.id, \.metric, default: .temperature)) {
            ForEach(Checklist.metrics, id: \.self) { Text(verbatim: Checklist.label($0)).tag($0) }
        } label: {
            Text(verbatim: tr("Metric"))
        }
        .pickerStyle(.menu)
        HStack {
            TextField(tr("Value"), text: binding(reading.id, \.value, default: ""), axis: .vertical)
                .textFieldStyle(.roundedBorder)
                .accessibilityIdentifier("report.reading.value")
            Text(verbatim: Checklist.unit(reading.metric).rawValue)
        }
        Button(tr("Remove")) { model.edit { $0.readings.removeAll { $0.id == reading.id } } }.secondaryAction()
    }

    private func binding<Value>(_ id: UUID, _ path: WritableKeyPath<ReportDraft.Reading, Value>, default fallback: Value) -> Binding<Value>
    {
        Binding(
            get: { model.draft.readings.first { $0.id == id }?[keyPath: path] ?? fallback },
            set: { value in
                model.edit { draft in
                    if let i = draft.readings.firstIndex(where: { $0.id == id }) { draft.readings[i][keyPath: path] = value }
                }
            }
        )
    }
}

/// The parts and the refrigerant used.
struct PartsCard: View {
    @Bindable var model: ReportModel
    let open: (ReportView.Sheet) -> Void

    var body: some View {
        Card(tr("Parts & refrigerant")) {
            if model.draft.parts.isEmpty, model.draft.refrigerant.isEmpty { Text(verbatim: tr("No parts used.")) }
            ForEach(Array(model.draft.parts.enumerated()), id: \.offset) { index, part in
                HStack {
                    Text(verbatim: "\(part.name) × \(Int(part.quantity)) · \(Checklist.label(part.source))")
                    Spacer()
                    if model.isEditable { Button(tr("Remove")) { model.edit { $0.parts.remove(at: index) } }.secondaryAction() }
                }
            }
            ForEach(Array(model.draft.refrigerant.enumerated()), id: \.offset) { index, record in
                HStack {
                    Text(
                        verbatim: tr(
                            "Refrigerant ({kind}) — cylinder {id}",
                            ["kind": record.refrigerant.rawValue, "id": record.cylinderId]
                        )
                    )
                    Spacer()
                    if model.isEditable { Button(tr("Remove")) { model.edit { $0.refrigerant.remove(at: index) } }.secondaryAction() }
                }
            }
            if model.isEditable {
                HStack {
                    Button(tr("Add part")) { open(.part) }.secondaryAction().accessibilityIdentifier("report.part.add")
                    Button(tr("Refrigerant")) { open(.refrigerant) }.secondaryAction().accessibilityIdentifier("report.refrigerant.add")
                }
                Text(
                    verbatim: tr(
                        "Stock is deducted from your van when the report is accepted. Refrigerant amounts are kept per unit for leak-check and regulatory logs."
                    )
                )
                .font(.footnote)
            }
        }
    }
}

/// The work performed and the next action.
struct WorkTextCard: View {
    @Bindable var model: ReportModel

    var body: some View {
        Card(tr("Work performed & next action (10–4000 characters)")) {
            if model.isEditable {
                TextField(tr("Work performed & next action (10–4000 characters)"), text: workText, axis: .vertical)
                    .lineLimit(4...12)
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("report.workText")
                Toggle(isOn: followsUp) { Text(verbatim: tr("Follow-up visit")) }.accessibilityIdentifier("report.followUp")
                if case let .followUp(date, note) = model.draft.next {
                    DatePicker(selection: followUpDate(date), displayedComponents: .date) { Text(verbatim: tr("Follow-up date")) }
                    TextField(
                        tr("What to do (required)"),
                        text: Binding(get: { note }, set: { text in model.edit { $0.next = .followUp(date: date, note: text) } }),
                        axis: .vertical
                    )
                    .textFieldStyle(.roundedBorder)
                    .accessibilityIdentifier("report.followUp.note")
                }
            } else {
                Text(verbatim: model.draft.workText.isEmpty ? "—" : model.draft.workText)
                if case let .followUp(date, note) = model.draft.next { Text(verbatim: "\(tr("Follow-up visit")) · \(date) · \(note)") }
            }
        }
    }

    private var workText: Binding<String> {
        Binding(get: { model.draft.workText }, set: { text in model.edit { $0.workText = text } })
    }

    private var followsUp: Binding<Bool> {
        Binding(
            get: { if case .followUp = model.draft.next { true } else { false } },
            set: { on in
                let tomorrow = Date().addingTimeInterval(86_400).ISO8601Format(.iso8601.year().month().day())
                model.edit { $0.next = on ? .followUp(date: tomorrow, note: "") : .noFollowUp }
            }
        )
    }

    private func followUpDate(_ date: String) -> Binding<Date> {
        Binding(
            get: { Format.parse("\(date)T00:00:00Z") ?? Date() },
            set: { day in
                let text = day.ISO8601Format(.iso8601.year().month().day())
                model.edit { draft in if case let .followUp(_, note) = draft.next { draft.next = .followUp(date: text, note: note) } }
            }
        )
    }
}
