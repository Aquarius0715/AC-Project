import ACKit
import ACShared
import Foundation
import Observation

/// A photo the phone is about to add: its name, its type and its bytes.
public struct NewPhoto: Sendable, Equatable {
    public let name: String
    public let mime: BlobInputMime
    public let data: Data

    public init(name: String, mime: BlobInputMime, data: Data) {
        self.name = name
        self.mime = mime
        self.data = data
    }

    var blob: BlobInput {
        BlobInput(name: name, mime: mime, size: Double(data.count), bytes: data.base64EncodedString())
    }
}

/// The work report of a job (FR-T04–T06, T09, T15, T16, DD-T04–T06, the web's workspace): the checklist, the readings,
/// the parts and refrigerant, the work text and the next action, the photos, the customer's sign-off and the submit.
/// The draft is edited on the phone and saved as a new draft version (the first save creates it); a save runs 20 s after
/// the last edit while the report is editable, and before a photo, a sign-off or the submit. Nothing is sent again by
/// itself after a refusal: the draft stays on the phone (DD-T08).
@MainActor @Observable
public final class ReportModel {
    public let job: JobDetail
    let unit: UnitDetail?
    let now: Date
    /// The job may be worked on: in progress inside its window (the job screen decides).
    let canWork: Bool

    public private(set) var report: WorkReport?
    public private(set) var isLoading = true
    public private(set) var isEditable = false
    public private(set) var draft: ReportDraft
    public private(set) var isDirty = false
    public private(set) var isSending = false
    public private(set) var parts: [PartCatalogItem] = []
    public private(set) var note: String?
    public private(set) var refusal: String?
    public private(set) var photoError: String?
    /// Whether a submit was tried: from then on the issues are shown and follow the edits.
    public private(set) var isChecked = false
    public private(set) var signOffError: String?

    let client: CoreClient
    let jobChanged: @MainActor () async -> Void
    let pause: @Sendable (Duration) async -> Void
    private var edits = 0
    private var autosave: Task<Void, Never>?

    /// The most photos a report takes, and the largest one (the Core API's limits).
    static let maxPhotos = 10
    static let maxBytes = 5 << 20
    /// The wait after the last edit before the draft saves itself.
    static let autosaveAfter = Duration.seconds(20)

    /// - Parameters:
    ///   - jobChanged: reads the job again (a submit moves it on).
    ///   - pause: the wait of the autosave (the tests' is immediate).
    public init(
        job: JobDetail,
        unit: UnitDetail?,
        now: Date,
        canWork: Bool,
        client: CoreClient,
        jobChanged: @escaping @MainActor () async -> Void,
        pause: @escaping @Sendable (Duration) async -> Void = ReportModel.sleep
    ) {
        self.job = job
        self.unit = unit
        self.now = now
        self.canWork = canWork
        self.client = client
        self.jobChanged = jobChanged
        self.pause = pause
        draft = ReportDraft(nil, components: Self.components(job, unit), isInstallation: job.type == .installation)
    }

    /// The autosave's wait in the app, off the main actor (as UnitControlModel.sleep).
    nonisolated public static func sleep(_ duration: Duration) async {
        try? await Task.sleep(for: duration)  // a cancelled wait is a later edit's: that edit's save follows
    }

    static func components(_ job: JobDetail, _ unit: UnitDetail?) -> [(group: ComponentGroup, key: ComponentKey)] {
        if job.type == .installation { return Checklist.components(scope: [.installation]) }
        let scope = unit.map { $0.serviceScope.compactMap { ComponentGroup(rawValue: $0.rawValue) } } ?? [.indoor, .outdoor, .electrical]
        return Checklist.components(scope: scope)
    }

    /// Reads the open draft, or the last report, and the parts catalog while the report is editable.
    public func load() async {
        isLoading = true
        defer { isLoading = false }
        let ref = job.draftReportRef ?? job.reportRefs.last
        var read: WorkReport?
        if let ref {
            do {
                let input = ReportsGetInput(jobId: job.id, reportId: ref.reportId, reportVersion: ref.reportVersion)
                read = try await client.call(Ops.ReportsGet.self, input).data
            } catch {
                refusal = tr("The report is not readable now ({reason}).", ["reason": ErrorText.of(error)])
            }
        }
        // the open draft is the one the job names; without it (a returned version, or none yet) the work goes on in a
        // new draft, which the first save creates (the web workspace's editableReport)
        let draftOpen = job.draftReportRef.map { $0.reportVersion == read?.version } ?? false
        isEditable = canWork && job.status == .inProgress
        report = isEditable ? (draftOpen ? read : nil) : read
        draft = ReportDraft(report, components: Self.components(job, unit), isInstallation: job.type == .installation)
        if isEditable {
            parts = (try? await client.call(Ops.PartsList.self, Query(limit: 100)).data.items) ?? []
        }
    }

    // MARK: - What the screen shows

    /// The report's photos (the signature is the sign-off's, not a photo).
    public var photos: [Attachment] {
        (report?.attachmentRefs ?? []).filter { $0.id != report?.signOff?.signatureAttachmentId }
    }

    public var isSignedOff: Bool { report?.signOff != nil }

    /// The issues that block the submit, once a submit was tried.
    public var issues: [ReportIssue] {
        isChecked ? draft.issues(photos: photos, now: now, signedOff: isSignedOff) : []
    }

    /// "draft v3", "v2" or "not started": the version the screen shows.
    public var versionText: String {
        guard let report else { return tr("not started") }
        return isEditable ? tr("draft v{version}", ["version": String(Int(report.version))]) : "v\(Int(report.version))"
    }

    /// HQ's reason for the last return, if it gave one (the rework banner).
    public var returnReason: String? { report?.reviewHistory.last?.reason }

    // MARK: - Editing

    /// One edit of the draft: it is unsaved until the next save, which runs by itself 20 s later.
    /// A change that leaves the draft as it was (a field set to its own value) is no edit: a save after the sign-off
    /// would clear it.
    public func edit(_ change: (inout ReportDraft) -> Void) {
        guard isEditable else { return }
        var next = draft
        change(&next)
        guard next != draft else { return }
        draft = next
        isDirty = true
        edits += 1
        autosave?.cancel()
        autosave = Task { [weak self, pause] in
            await pause(Self.autosaveAfter)
            // a save since then (by hand, or before a step) took the edit: nothing to save, and a save with no edit
            // would only make a version, and clear the sign-off
            guard !Task.isCancelled, self?.isDirty == true else { return }
            await self?.save()
        }
    }

    /// Saves the draft as its next version; true when the Core API took it.
    @discardableResult
    public func save() async -> Bool {
        guard isEditable, !isSending else { return false }
        autosave?.cancel()
        isSending = true
        defer { isSending = false }
        return await saved()
    }

    /// The save itself, while a step is sending.
    private func saved() async -> Bool {
        let at = edits
        let input = draft.input(jobId: job.id, reportId: report?.id, modelId: unit?.modelId)
        do throws(CoreError) {
            let version = report.map { Int($0.version) }
            report = try await client.call(Ops.JobsSaveDraft.self, input, expectedVersion: version).data
            isDirty = edits != at  // an edit while saving is still unsaved
            refusal = nil
            note = tr("Draft saved")
            return true
        } catch {
            refusal = Self.refused(error)
            return false
        }
    }

    /// A step on the saved draft: saved first when it has unsaved edits or no saved version yet.
    private func onSaved(_ step: (WorkReport) async -> Void) async {
        guard isEditable, !isSending else { return }
        isSending = true
        defer { isSending = false }
        autosave?.cancel()
        if isDirty || report == nil {
            guard await saved() else { return }
        }
        if let report { await step(report) }
    }

    static func refused(_ error: CoreError) -> String {
        "\(ErrorText.of(error)) — \(tr("The draft is kept."))"
    }

    // MARK: - Photos

    /// Adds a photo to the report (a tag says what an installation photo shows; `linkTo` links it to that finding).
    public func addPhoto(_ photo: NewPhoto, tag: AttachmentTag? = nil, linkTo key: ComponentKey? = nil) async {
        guard photo.data.count <= Self.maxBytes, photos.count < Self.maxPhotos else {
            photoError = tr(
                "“{name}” was not added — only JPEG/PNG up to 5 MiB, max {max} photos. Saved text and photos are kept.",
                ["name": photo.name, "max": String(Self.maxPhotos)]
            )
            return
        }
        photoError = nil
        await onSaved { report in
            let known = Set(report.attachmentRefs.map(\.id))
            do throws(CoreError) {
                let input = AttachmentsAddInput(jobId: job.id, reportId: report.id, file: photo.blob, tag: tag)
                _ = try await client.call(Ops.AttachmentsAdd.self, input, expectedVersion: Int(report.version))
                let read = ReportsGetInput(jobId: job.id, reportId: report.id, reportVersion: report.version + 1)
                let next = try await client.call(Ops.ReportsGet.self, read).data
                self.report = next
                draft.attachmentIds = next.attachmentRefs.filter { $0.status != .failed }.map(\.id)
                note = tr("Photo {name} added — sign-off cleared", ["name": photo.name])
                if let key, let fresh = next.attachmentRefs.first(where: { !known.contains($0.id) }) {
                    edit { $0.link(fresh.id, to: key, true) }
                }
            } catch {
                refusal = Self.refused(error)
            }
        }
    }

    /// Removes a photo from the draft (the next save leaves it out of the report).
    public func removePhoto(_ id: String) {
        edit { draft in
            draft.attachmentIds.removeAll { $0 == id }
            for index in draft.items.indices { draft.items[index].evidenceIds.removeAll { $0 == id } }
        }
    }

    // MARK: - Sign-off

    /// The customer's sign-off (FR-T15): the drawn signature (PNG), or why the customer could not sign with a site
    /// photo; bound to this draft version (a later save clears it). True when saved.
    public func signOff(signer: String, signature: Data?, absentReason: String?, sitePhoto: NewPhoto?) async -> Bool {
        let name = signer.trimmingCharacters(in: .whitespacesAndNewlines)
        let reason = absentReason?.trimmingCharacters(in: .whitespacesAndNewlines)
        signOffError = Self.signOffError(name: name, signature: signature, absentReason: reason, sitePhoto: sitePhoto)
        guard signOffError == nil else { return false }
        var done = false
        await onSaved { report in
            let input = ReportsSignOffInput(
                jobId: job.id,
                reportId: report.id,
                reportVersion: report.version,
                signerName: name,
                signature: signature.map { NewPhoto(name: "signature.png", mime: .imagePng, data: $0).blob },
                absentReason: signature == nil ? reason : nil,
                sitePhoto: signature == nil ? sitePhoto?.blob : nil
            )
            do throws(CoreError) {
                self.report = try await client.call(Ops.ReportsSignOff.self, input, expectedVersion: Int(report.version)).data
                note = tr("Sign-off saved for this report version")
                done = true
            } catch {
                refusal = Self.refused(error)
            }
        }
        return done
    }

    static func signOffError(name: String, signature: Data?, absentReason: String?, sitePhoto: NewPhoto?) -> String? {
        if name.isEmpty { return tr("The signer's name is required") }
        if signature != nil { return nil }
        guard let absentReason else { return tr("Ask the customer to sign in the box") }
        if absentReason.isEmpty { return tr("Say why the customer could not sign") }
        guard let sitePhoto, sitePhoto.data.count <= maxBytes else {
            return tr("A site photo (JPEG/PNG, ≤5 MiB) is required when the customer is not available")
        }
        return nil
    }

    // MARK: - Submit

    /// Checks the draft before the submit; true when nothing blocks it (the screen then asks to confirm).
    public func checkSubmit() -> Bool {
        isChecked = true
        return issues.isEmpty
    }

    /// Submits the saved draft (saved first): its version becomes read-only; the job moves on and is read again.
    public func submit() async {
        guard checkSubmit() else { return }
        var submitted: Double?
        await onSaved { report in
            do throws(CoreError) {
                let input = JobsSubmitInput(jobId: job.id, reportVersion: report.version)
                _ = try await client.call(Ops.JobsSubmit.self, input, expectedVersion: Int(job.version))
                submitted = report.version
            } catch {
                refusal = Self.refused(error)
            }
        }
        if let submitted {
            note = tr("Report v{version} submitted", ["version": String(Int(submitted))])
            isEditable = false
            await jobChanged()
        }
    }
}
