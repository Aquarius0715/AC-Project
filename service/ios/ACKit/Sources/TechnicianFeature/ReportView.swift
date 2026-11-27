import ACKit
import ACShared
import PhotosUI
import SwiftUI

/// The work report of a job (the web workspace's checklist, readings, parts, work text, photos, sign-off and submit).
struct ReportView: View {
    @State private var model: ReportModel
    @State private var sheet: Sheet?
    @State private var isConfirmingSubmit = false
    @State private var shown: ShownPhoto?

    /// A report photo opened full size.
    struct ShownPhoto: Identifiable {
        let id: String
        let name: String
        let data: Data
    }
    @State private var picked: PhotosPickerItem?
    @State private var pickedFor: PhotoTarget?
    @State private var isTakingPhoto = false

    enum Sheet: String, Identifiable {
        case part, refrigerant, signOff
        var id: String { rawValue }
    }

    /// What a photo being added is for: the report, an installation part (its tag), or a finding.
    struct PhotoTarget: Equatable {
        var tag: AttachmentTag?
        var key: ComponentKey?
    }

    init(model: ReportModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            lines
            if model.draft.installation != nil { EquipmentCard(model: model, add: add) }
            ForEach(model.draft.progress(model.issues), id: \.group) { entry in
                ChecklistCard(model: model, group: entry.group, done: entry.done, total: entry.total, add: add)
            }
            ReadingsCard(model: model)
            PartsCard(model: model) { sheet = $0 }
            WorkTextCard(model: model)
            photosCard
            versionsCard
            signOffCard
            if model.isEditable { submitCard }
        }
        .cardScreen()
        .navigationTitle(Text(verbatim: tr("Work report — {version}", ["version": model.versionText])))
        .inlineTitle()
        .hideTabBar()
        .task { await model.load() }
        // a refusal is felt as well as read; no success haptic here: the draft saves itself every 20 s
        .sensoryFeedback(.error, trigger: model.refusal) { _, new in new != nil }
        .sheet(item: $shown) { photo in
            NavigationStack {
                PhotoImage(data: photo.data, name: photo.name)
                    .accessibilityIdentifier("report.photo.view")
                    .navigationTitle(Text(verbatim: photo.name))
                    .inlineTitle()
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button(tr("Close")) { shown = nil }.accessibilityIdentifier("report.photo.close")
                        }
                    }
            }
        }
        .sheet(item: $sheet) { sheet in
            switch sheet {
            case .part: PartSheet(model: model) { self.sheet = nil }
            case .refrigerant: RefrigerantSheet(model: model) { self.sheet = nil }
            case .signOff: SignOffSheet(model: model) { self.sheet = nil }
            }
        }
        .confirmationDialog(
            Text(verbatim: tr("Submit report v{version}?", ["version": String(Int(model.report?.version ?? 1))])),
            isPresented: $isConfirmingSubmit,
            titleVisibility: .visible
        ) {
            Button(tr("Submit")) { Task { await model.submit() } }.accessibilityIdentifier("report.submit.confirm")
            Button(tr("Cancel"), role: .cancel) { isConfirmingSubmit = false }
        } message: {
            Text(verbatim: tr("Submitted versions become read-only; the quality review by your company or HQ follows."))
        }
        .onChange(of: picked) { _, item in
            guard let item else { return }
            let target = pickedFor ?? PhotoTarget()
            picked = nil
            Task {
                guard let data = try? await item.loadTransferable(type: Data.self),  // an unreadable pick adds nothing
                    let photo = PhotoEncoding.jpeg(data, name: "photo-\(model.photos.count + 1)")
                else { return }
                await model.addPhoto(photo, tag: target.tag, linkTo: target.key)
            }
        }
        #if os(iOS)
        .fullScreenCover(isPresented: $isTakingPhoto) {
            CameraPicker { image in
                isTakingPhoto = false
                let target = pickedFor ?? PhotoTarget()
                guard let image, let photo = PhotoEncoding.jpeg(image, name: "photo-\(model.photos.count + 1)") else { return }
                Task { await model.addPhoto(photo, tag: target.tag, linkTo: target.key) }
            }
            .ignoresSafeArea()
        }
        #endif
    }

    /// The buttons that add a photo for a target: the library, and the camera where the phone has one.
    @ViewBuilder
    private func add(_ title: String, _ target: PhotoTarget, id: String) -> some View {
        HStack {
            PhotosPicker(
                selection: Binding(
                    get: { picked },
                    set: {
                        pickedFor = target; picked = $0
                    }
                ),
                matching: .images
            ) {
                Label(title, symbol: .photo)
            }
            .secondaryAction()
            .disabled(model.isSending)
            .accessibilityIdentifier(id)
            if CameraPicker.isAvailable {
                Button {
                    pickedFor = target
                    isTakingPhoto = true
                } label: {
                    Label(tr("Take photo"), symbol: .camera)
                }
                .secondaryAction()
                .disabled(model.isSending)
            }
        }
    }

    @ViewBuilder private var lines: some View {
        if let note = model.note {
            Card {
                StatusLabel(note, tone: .ok).accessibilityIdentifier("report.note")
            }
        }
        if let refusal = model.refusal {
            Card {
                StatusLabel(refusal, tone: .critical)
                    .accessibilityIdentifier("report.refusal")
            }
        }
        if let error = model.photoError {
            Card { Text(verbatim: error).accessibilityIdentifier("report.photoError") }
        }
        if model.isLoading { ProgressView() }
        if model.isDirty { Text(verbatim: tr("Unsaved changes")).font(.factLabel) }
    }

    @ViewBuilder private var photosCard: some View {
        Card(tr("Photos (JPEG/PNG, ≤5 MiB, ≤{max})", ["max": String(ReportModel.maxPhotos)])) {
            let photos = model.photos.filter { model.draft.attachmentIds.contains($0.id) }
            if photos.isEmpty { Text(verbatim: tr("No photos.")) }
            ForEach(photos, id: \.id) { photo in
                HStack {
                    Button {
                        Task {
                            if let data = await model.photoData(photo.id, isImage: PhotoImage.isImage) {
                                shown = ShownPhoto(id: photo.id, name: photo.name, data: data)
                            }
                        }
                    } label: {
                        Label(photo.name, symbol: .photo)
                    }
                    .linkAction()
                    .accessibilityIdentifier("report.photo.open.\(photo.id)")
                    Spacer()
                    if model.isEditable {
                        Button(tr("Remove"), role: .destructive) { model.removePhoto(photo.id) }
                            .destructiveAction()
                            .accessibilityIdentifier("report.photo.remove.\(photo.id)")
                    }
                }
            }
            if model.isEditable, model.photos.count < ReportModel.maxPhotos {
                add(tr("Add photos"), PhotoTarget(), id: "report.photo.add")
            }
        }
    }

    @ViewBuilder private var versionsCard: some View {
        let rows = model.versionRows
        if !rows.isEmpty {
            Card(tr("Versions & autosave")) {
                ForEach(rows, id: \.title) { row in
                    VStack(alignment: .leading, spacing: Spacing.xxs) {
                        Text(verbatim: "\(row.title) · \(row.badge)").font(.headline)
                        Text(verbatim: row.sub).font(.subheadline)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("report.version")
                }
            }
        }
    }

    @ViewBuilder private var signOffCard: some View {
        Card(tr("Customer sign-off")) {
            if let signOff = model.report?.signOff {
                Text(verbatim: tr("Signed · {name} · {time}", ["name": signOff.signerName, "time": Format.dateTime(signOff.signedAt)]))
                    .accessibilityIdentifier("report.signed")
                if let reason = signOff.absentReason { Text(verbatim: tr("Customer not available") + " — " + reason) }
            } else {
                Text(verbatim: tr("Not signed")).accessibilityIdentifier("report.signed")
                Text(verbatim: tr("No customer sign-off yet — the report can still be submitted; the reviewer sees “not signed”."))
                    .font(.footnote)
            }
            if model.isEditable {
                Button(tr("Customer sign-off")) { sheet = .signOff }
                    .secondaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("report.signOff")
                Text(verbatim: tr("The sign-off is bound to this report version; saving the draft again clears it.")).font(.footnote)
            }
        }
    }

    @ViewBuilder private var submitCard: some View {
        Card {
            let issues = model.issues
            if !issues.isEmpty {
                Text(
                    verbatim: issues.count == 1
                        ? tr("1 issue blocks submit — it is listed above Submit report.")
                        : tr("{n} issues block submit — they are listed above Submit report.", ["n": String(issues.count)])
                )
                .font(.factLabel)
                .accessibilityIdentifier("report.issues")
                ForEach(Array(issues.enumerated()), id: \.offset) { _, issue in
                    StatusLabel.fieldError(issue.text)
                }
            }
            HStack {
                Button(tr("Save draft")) { Task { await model.save() } }
                    .secondaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("report.save")
                Button(tr("Submit report")) { if model.checkSubmit() { isConfirmingSubmit = true } }
                    .primaryAction()
                    .disabled(model.isSending)
                    .accessibilityIdentifier("report.submit")
            }
            Text(
                verbatim: tr(
                    "Submitted versions become read-only. A report returned for rework continues as a new draft version. Saving again clears the sign-off."
                )
            )
            .font(.footnote)
        }
    }
}
