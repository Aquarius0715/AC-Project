import ACKit
import ACShared
import Foundation

/// The technician's alert evidence in words (shared/lib/techAlerts.ts and alertEvidence.ts, FR-T07, DD-T07): the
/// alert with the condition that raised it, its evidence and the latest reading of the rule's metric, how it resolves,
/// its history and what it relates to.
public enum AlertWords {
    /// A duration in its largest whole unit from two of them up: "60 s", "10 min", "3 h".
    static func duration(_ seconds: Double) -> String {
        let s = Int(seconds)
        if s >= 7200, s.isMultiple(of: 3600) { return tr("{n} h", ["n": String(s / 3600)]) }
        if s >= 120, s.isMultiple(of: 60) { return tr("{n} min", ["n": String(s / 60)]) }
        return tr("{n} s", ["n": String(s)])
    }

    static func symbol(_ compare: Compare) -> String {
        switch compare {
        case .gt: ">"
        case .gte: "≥"
        case .lt: "<"
        case .lte: "≤"
        }
    }

    static func value(_ metric: Metric, _ value: Double) -> String {
        "\(UnitWords.number(metric, value)) \(Checklist.unit(metric).rawValue)".trimmingCharacters(in: .whitespaces)
    }

    /// The words of a rule: the policy, its recovery and the condition that raises it (D08: recovery is below the
    /// recovery threshold for a high limit, above it for a low one).
    public struct Rule: Sendable, Equatable {
        public let policy: String
        public let recovery: String
        public let raise: String
    }

    public static func rule(_ rule: AlertRule) -> Rule {
        let isUp = rule.operator == .gt || rule.operator == .gte
        let recovery = "\(isUp ? "<" : ">") \(value(rule.metric, rule.recoveryThreshold))"
        let duration = duration(rule.durationSeconds)
        let metric = UnitWords.metric(rule.metric)
        let threshold = value(rule.metric, rule.threshold)
        return Rule(
            policy: tr(
                "Policy: {metric} {op} {threshold}, recovery {recovery}, duration {duration}",
                ["metric": metric, "op": symbol(rule.operator), "threshold": threshold, "recovery": recovery, "duration": duration]
            ),
            recovery: tr("{recovery} for {duration}", ["recovery": recovery, "duration": duration]),
            raise: "\(metric) \(symbol(rule.operator)) \(threshold)"
        )
    }

    static func kind(_ kind: CoreAlertEvidenceKind) -> String {
        switch kind {
        case .demoObservation: tr("Observed (demo)")
        case .inferred: tr("Suspected")
        case .inspection: tr("Inspection record")
        }
    }

    static func state(_ status: CoreAlertStatus) -> String {
        switch status {
        case .open: tr("Open")
        case .acknowledged: tr("Acknowledged")
        case .resolved: tr("Resolved")
        }
    }

    /// The alert the screen opens: the given one, else the worst unresolved one, else the newest.
    public static func select(_ alerts: [CoreAlert], _ alertId: String?) -> CoreAlert? {
        let open = alerts.filter { $0.status != .resolved }.sorted {
            UnitWords.rank($0.severity) != UnitWords.rank($1.severity)
                ? UnitWords.rank($0.severity) < UnitWords.rank($1.severity) : $0.detectedAt > $1.detectedAt
        }
        return alerts.first { $0.id == alertId } ?? open.first ?? alerts.max { $0.detectedAt < $1.detectedAt }
    }

    /// The unit's alerts to choose from, unresolved first, newest first.
    public static func choices(_ alerts: [CoreAlert], now: Date) -> [UnitRow] {
        alerts.sorted { a, b in
            (a.status == .resolved) != (b.status == .resolved) ? b.status == .resolved : a.detectedAt > b.detectedAt
        }
        .map { alert in
            UnitRow(
                id: alert.id,
                title: alert.rule?.name ?? UnitWords.alertTitle(alert),
                sub: "\(state(alert.status)) · \(UnitWords.relative(alert.detectedAt, now: now))",
                badge: UnitWords.severity(alert.severity)
            )
        }
    }

    /// The selected alert as the screen shows it.
    public struct View: Sendable, Equatable {
        public let title: String
        public let subtitle: String
        public let state: String?
        public let evidence: [(String, String)]
        public let resolution: String?
        public let done: String?
        public let note: String
        public let forbidden: String
        public let history: [UnitRow]
        public let related: [(String, String)]

        public static func == (a: Self, b: Self) -> Bool {
            a.title == b.title && a.subtitle == b.subtitle && a.state == b.state && a.resolution == b.resolution && a.done == b.done
                && a.note == b.note && a.history == b.history && a.evidence.map { $0.0 + $0.1 } == b.evidence.map { $0.0 + $0.1 }
                && a.related.map { $0.0 + $0.1 } == b.related.map { $0.0 + $0.1 }
        }
    }

    /// The context of an alert: the unit's other alerts, the unit, the job the screen is opened for, and whether the
    /// technician holds "Resolve alerts".
    public struct Context: Sendable {
        public let alerts: [CoreAlert]
        public let unit: UnitDetail?
        public let job: (id: String, type: MaintenanceJobType)?
        public let canResolve: Bool
    }

    // swiftlint:disable:next function_body_length - one view: each part of the web's alertView, read top to bottom
    public static func view(_ alert: CoreAlert, _ context: Context, now: Date) -> View {
        let when = { (instant: String) in UnitWords.relative(instant, now: now) }
        let words = alert.rule.map(rule)
        let title = alert.rule?.name ?? UnitWords.alertTitle(alert)
        var evidence = [
            (
                "\(kind(alert.evidenceKind)) · \(tr("Evidence at detection"))",
                "\(Evidence.text(alert.evidenceText)) · \(tr("observed {time}", ["time": when(alert.observedAt)]))"
            )
        ]
        if let rule = alert.rule, let reading = context.unit?.latestMeasurements.first(where: { $0.metric == rule.metric }) {
            let shown = reading.value.map { value(reading.metric, $0) } ?? "—"
            var sub = [tr("observed {time}", ["time": when(reading.observedAt)]), UnitWords.quality(reading.quality) ?? tr("good")]
            if reading.origin == .estimated { sub.append(tr("confidence unknown — no numeric probability shown")) }
            evidence.append((tr("Latest reading"), "\(shown) · \(sub.joined(separator: " · "))"))
        }
        var history: [UnitRow] = []
        if let resolved = alert.resolvedAt {
            history.append(
                UnitRow(
                    id: "resolved",
                    title: tr("Resolved"),
                    sub: [when(resolved), alert.resolutionReason.map(UnitWords.resolution)].compactMap { $0 }.joined(separator: " · "),
                    badge: tr("Resolved")
                )
            )
        }
        if let acknowledged = alert.acknowledgedAt {
            history.append(UnitRow(id: "acknowledged", title: tr("Acknowledged"), sub: when(acknowledged), badge: tr("Acknowledged")))
        }
        var raised = [when(alert.detectedAt), Evidence.text(alert.evidenceText)]
        if let rule = alert.rule, let words {
            raised.append(tr("{condition} for {duration}", ["condition": words.raise, "duration": duration(rule.durationSeconds)]))
        }
        history.append(UnitRow(id: "raised", title: tr("Raised"), sub: raised.joined(separator: " · "), badge: kind(alert.evidenceKind)))

        let isResolved = alert.status == .resolved
        let stateTime = (isResolved ? alert.resolvedAt : alert.acknowledgedAt) ?? alert.detectedAt
        let previous = alert.previousAlertId.map { id in
            context.alerts.first { $0.id == id }.map { "\(Format.date($0.detectedAt)) · \(state($0.status))" } ?? String(id.prefix(8))
        }
        let resolver: String
        if isResolved {
            resolver = tr("done {time}", ["time": when(alert.resolvedAt ?? alert.detectedAt)])
        } else if context.canResolve {
            resolver = words == nil ? tr("with a reason") : tr("with a reason, or by a remeasurement")
        } else {
            resolver = words == nil ? tr("no — ask a user with “Resolve alerts”") : tr("with a remeasurement only")
        }
        let unit = context.unit.map { unit in
            unit.location.pathLabels.isEmpty
                ? unit.displayName : "\(unit.displayName) · \(unit.location.pathLabels.joined(separator: " › "))"
        }
        let reason = alert.resolutionReason.map(UnitWords.resolution) ?? tr("no reason recorded")
        let done = [
            tr("Resolved {time} — {reason}.", ["time": when(alert.resolvedAt ?? alert.detectedAt), "reason": reason]),
            alert.acknowledgedAt.map { tr("Acknowledged {time}.", ["time": when($0)]) },
        ]
        let information =
            words.map {
                tr(
                    "Automatic resolution needs a remeasurement {recovery} — or a reason below with “Resolve alerts”.",
                    ["recovery": $0.recovery]
                )
            }
            ?? tr("No remeasurement resolves this alert — it has no policy. Resolve it with a reason below with “Resolve alerts”.")
        return View(
            title: "\(alert.id.prefix(8)) · \(title)",
            subtitle: words?.policy ?? tr("{kind} · no policy — it resolves only with a reason", ["kind": kind(alert.evidenceKind)]),
            state: alert.status == .open ? nil : "\(state(alert.status)) · \(when(stateTime))",
            evidence: evidence,
            resolution: isResolved ? nil : information,
            done: isResolved ? done.compactMap { $0 }.joined(separator: " ") : nil,
            note: isResolved
                ? tr(
                    "Completing the job alone never resolves an alert. If the condition returns, a new alert is created that links back to alert {id}.",
                    ["id": String(alert.id.prefix(8))]
                )
                : tr("If it recurs after it is resolved, a new alert is created that links back to this one."),
            forbidden: words.map {
                tr(
                    "Not resolved — you don’t have “Resolve alerts”. The alert resolves automatically once a remeasurement stays {recovery}, or ask a permitted user.",
                    ["recovery": $0.recovery]
                )
            } ?? tr("Not resolved — you don’t have “Resolve alerts”. Ask a permitted user to resolve it."),
            history: history,
            related: [
                (tr("Job"), context.job.map { "\($0.id.prefix(8)) · \(Labels.jobType($0.type))" } ?? "—"),
                (tr("Unit"), unit ?? String(alert.unitId.prefix(8))),
                (tr("Policy"), alert.rule?.name ?? tr("No policy")),
                (tr("Recovery rule"), words?.recovery ?? "—"),
                (tr("Previous alert"), previous ?? tr("none")),
                (tr("You can resolve"), resolver),
            ]
        )
    }

    /// The evidence a resolution may cite, newest first (alerts.evidence, IR327).
    public static func evidence(_ items: [EvidenceCandidate], now: Date) -> [UnitRow] {
        items.map { candidate in
            let time = UnitWords.relative(candidate.observedAt, now: now)
            switch candidate.kind {
            case .remeasurement:
                let metric = candidate.metric.map(UnitWords.metric) ?? ""
                let shown =
                    zip(candidate.metric, candidate.value).map { "\(UnitWords.number($0, $1)) \(candidate.unit?.rawValue ?? "")" } ?? "—"
                let origin: String =
                    switch candidate.origin {
                    case .measured?: tr("measured")
                    case .inspection?: tr("recorded on site")
                    case .estimated?: tr("estimated")
                    case nil: ""
                    }
                return UnitRow(
                    id: candidate.id,
                    title: tr(
                        "Remeasurement · {metric} {value} · {time}",
                        ["metric": metric, "value": shown.trimmingCharacters(in: .whitespaces), "time": time]
                    ),
                    sub: tr("{origin} · valid", ["origin": origin]),
                    badge: ""
                )
            case .deviceEvent:
                let sub = candidate.eventType == .restored ? tr("The device's signal is back") : candidate.eventType?.rawValue ?? ""
                return UnitRow(id: candidate.id, title: tr("Device event · {time}", ["time": time]), sub: sub, badge: "")
            case .detection:
                return UnitRow(
                    id: candidate.id,
                    title: tr("Evidence attached to this alert"),
                    sub: "\(candidate.id.prefix(8)) · \(tr("observed {time}", ["time": time]))",
                    badge: ""
                )
            }
        }
    }

    /// At most this many evidence records go with one resolution (the Core API's bound).
    static let maxEvidence = 20

    /// Why the resolution cannot be sent yet, or nil: a reason of 1–1000 characters, and evidence for an alert without a
    /// policy (IR66). The characters are counted as the Core API counts them, in code points (an emoji with a skin tone
    /// is two).
    public static func problem(reason: String, picked: [String], policyless: Bool) -> String? {
        let count = reason.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count
        if count < 1 || count > 1000 { return tr("A resolution reason is required (1–1000 characters).") }
        if policyless, picked.isEmpty { return tr("Pick at least one evidence record — this alert has no policy") }
        if picked.count > maxEvidence { return tr("Pick at most {n} evidence records", ["n": String(maxEvidence)]) }
        return nil
    }

    /// A refused acknowledgement or resolution in words; a missing "Resolve alerts" says how it can still resolve.
    public static func refusal(_ error: DomainError, forbidden: String) -> String {
        switch error.messageKey {
        case "error.forbidden": return forbidden
        case "errors.assignment_required": return tr("You need an active assignment on this unit to act on its alerts.")
        case "errors.assignment_not_started": return tr("Your work window on this unit has not started yet.")
        case "errors.assignment_ended": return tr("Your work window on this unit has ended.")
        case "error.versionConflict": return tr("The alert changed elsewhere — the latest state is shown.")
        case "error.invalidState": return tr("The alert is no longer in that state — the latest state is shown.")
        case "error.notFound": return tr("This alert is not in your assignments.")
        default: break
        }
        switch error.fieldErrors["resolutionEvidenceIds"] {
        case "errors.evidence_required"?: return tr("Pick at least one evidence record — this alert has no policy")
        case "errors.evidence_unknown"?: return tr("An evidence record is no longer a candidate of this alert — pick again from the list")
        case .some: return tr("Pick each evidence record once, at most {n}", ["n": String(maxEvidence)])
        case nil: break
        }
        if error.code == .validation { return tr("A resolution reason is required (1–1000 characters).") }
        if error.code == .unavailable || error.code == .timeout { return tr("Not sent; your reason is kept. Try again.") }
        var plain = error
        plain.fieldErrors = [:]
        return ErrorText.of(plain)
    }
}

private func zip<A, B>(_ a: A?, _ b: B?) -> (A, B)? {
    guard let a, let b else { return nil }
    return (a, b)
}
