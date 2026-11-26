import ACKit
import ACShared
import Foundation
import Observation

/// The technician's alert evidence of one unit (FR-T07, DD-T07, the web's technician alerts page): the unit's alerts,
/// the selected one with its evidence and history, and its acknowledgement and resolution with the alert's version.
/// A resolution gives a reason and may cite the evidence candidates of alerts.evidence (IR327); an alert without a
/// policy needs at least one. Resolving needs "Resolve alerts" (alert.resolve).
@MainActor @Observable
public final class AlertsModel {
    public enum Content: Sendable, Equatable {
        case alerts([CoreAlert])
        /// The alerts cannot be read: why, in words.
        case unavailable(title: String, text: String)
    }

    public let unitId: String
    public let jobId: String?
    public private(set) var content: Loadable<Content> = .loading
    public private(set) var selectedId: String?
    public private(set) var unit: UnitDetail?
    public private(set) var job: JobDetail?
    public private(set) var canResolve = false
    /// The evidence the selected alert's resolution may cite; nil when there is nothing to cite or it is unreadable.
    public private(set) var candidates: [EvidenceCandidate]?
    public private(set) var isSending = false
    public private(set) var note: String?
    public private(set) var refusal: String?
    public var reason = ""
    public private(set) var picked: [String] = []
    public private(set) var isChecked = false
    public private(set) var now = Date()

    let client: CoreClient

    public init(unitId: String, jobId: String?, alertId: String?, client: CoreClient) {
        self.unitId = unitId
        self.jobId = jobId
        selectedId = alertId
        self.client = client
    }

    public var alerts: [CoreAlert] {
        if case let .alerts(alerts)? = content.value { alerts } else { [] }
    }

    public var selected: CoreAlert? { AlertWords.select(alerts, selectedId) }

    public func load() async {
        content = await content.after { () async throws(CoreError) in try await read() }
        guard case .alerts? = content.value else { return }
        unit = try? await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unitId, jobId: jobId)).data
        if let jobId, case let .detail(job)? = try? await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId)).data {
            self.job = job
        }
        let session = try? await client.call(Ops.SessionGet.self, Empty())
        canResolve = session?.data.permissions.contains(.alertResolve) ?? false
        if let at = session?.meta.snapshotAt.flatMap(Format.parse) { now = at }
        await loadCandidates()
    }

    private func read() async throws(CoreError) -> Content {
        do {
            return .alerts(try await TechUnitModel.alerts(of: unitId, client: client))
        } catch  where error.status > 0 && error.status < 500 {
            if error.error.messageKey == "errors.assignment_not_started" {
                return .unavailable(title: tr("Not started yet"), text: await opening())
            }
            let text: String =
                switch error.error.messageKey {
                case "errors.assignment_ended": tr("Your work window on this unit has ended.")
                case "errors.assignment_required": tr("You need an active assignment on this unit to see its alerts.")
                case "error.notFound": tr("This AC is not in your assignments.")
                default: error.error.code.rawValue
                }
            return .unavailable(title: tr("Alerts aren’t available"), text: text)
        }
    }

    private func opening() async -> String {
        guard let jobId,
            case let .detail(job)? = try? await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId)).data,
            let start = job.assignment?.scheduledStart
        else { return tr("Available from the work start time of your assigned job.") }
        return tr("You can see this AC from {time}, when your work window starts.", ["time": Format.dateTime(start)])
    }

    /// Counts the candidate reads: only the latest one's answer is shown (another alert chosen while a read is out would
    /// otherwise list the earlier alert's evidence under the later one).
    private var candidateRead = 0

    /// The candidates of an unresolved alert, for a technician who may resolve it.
    private func loadCandidates() async {
        candidateRead += 1
        let read = candidateRead
        guard let alert = selected, alert.status != .resolved, canResolve else {
            candidates = nil
            return
        }
        let input = AlertsEvidenceInput(alertId: alert.id, query: Query(limit: 100))
        let items = try? await client.call(Ops.AlertsEvidence.self, input).data.items
        guard read == candidateRead else { return }
        candidates = items
    }

    /// Shows another alert of the unit; its resolution starts empty. Not while a step is sent: its note or refusal
    /// would show under the other alert.
    public func select(_ alertId: String) async {
        guard !isSending else { return }
        selectedId = alertId
        reason = ""
        picked = []
        isChecked = false
        refusal = nil
        note = nil
        candidates = nil
        await loadCandidates()
    }

    // MARK: - What the screen shows

    public var view: AlertWords.View? {
        guard let alert = selected else { return nil }
        let context = AlertWords.Context(
            alerts: alerts,
            unit: unit,
            job: job.map { (id: $0.id, type: $0.type) },
            canResolve: canResolve
        )
        return AlertWords.view(alert, context, now: now)
    }

    public var choices: [UnitRow] { AlertWords.choices(alerts, now: now) }

    public var evidenceRows: [UnitRow] { AlertWords.evidence(candidates ?? [], now: now) }

    /// Why the resolution cannot be sent, once it was tried.
    public var problem: String? {
        guard isChecked, let alert = selected else { return nil }
        return AlertWords.problem(reason: reason, picked: picked, policyless: alert.policyId == nil)
    }

    public func pick(_ id: String, _ on: Bool) {
        picked.removeAll { $0 == id }
        if on { picked.append(id) }
    }

    // MARK: - The steps

    /// Acknowledges the selected alert: the technician has seen it; it stays open until it resolves.
    public func acknowledge() async {
        await write(done: tr("Acknowledged")) { [client] alert throws(CoreError) in
            _ = try await client.call(
                Ops.AlertsAcknowledge.self,
                AlertsAcknowledgeInput(alertId: alert.id),
                expectedVersion: Int(alert.version)
            )
        }
    }

    /// Resolves the selected alert with the reason and the cited evidence.
    public func resolve() async {
        isChecked = true
        guard problem == nil else { return }
        let reason = reason.trimmingCharacters(in: .whitespacesAndNewlines)
        let picked = picked
        await write(done: tr("Resolved")) { [client] alert throws(CoreError) in
            let input = AlertsResolveInput(alertId: alert.id, resolutionReason: reason, resolutionEvidenceIds: picked)
            _ = try await client.call(Ops.AlertsResolve.self, input, expectedVersion: Int(alert.version))
        }
    }

    private func write(done: String, _ send: (CoreAlert) async throws(CoreError) -> Void) async {
        guard let alert = selected, let view, !isSending else { return }
        isSending = true
        defer { isSending = false }
        note = nil
        refusal = nil
        do {
            try await send(alert)
            note = done
            reason = ""
            picked = []
            isChecked = false
        } catch {
            refusal = AlertWords.refusal(error.error, forbidden: view.forbidden)
        }
        await load()
    }
}
