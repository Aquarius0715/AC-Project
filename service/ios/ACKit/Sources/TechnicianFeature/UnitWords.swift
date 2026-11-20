import ACKit
import ACShared
import Foundation

/// The period of a unit's series (DD-T03): 1 h and 24 h roll back from now; 7 days are the Kuala Lumpur calendar days up
/// to today.
public enum SeriesPeriod: String, Sendable, CaseIterable {
    case hour = "1h"
    case day = "24h"
    case week = "7d"

    public func range(now: Date) -> (from: Date, to: Date) {
        switch self {
        case .hour: return (now.addingTimeInterval(-3600), now)
        case .day: return (now.addingTimeInterval(-86_400), now)
        case .week:
            // 00:00 in Kuala Lumpur (UTC+8 all year) of the day six days before today
            let kualaLumpur = 8 * 3600.0
            let today = ((now.timeIntervalSince1970 + kualaLumpur) / 86_400).rounded(.down) * 86_400 - kualaLumpur
            return (Date(timeIntervalSince1970: today - 6 * 86_400), now)
        }
    }

    /// The averages a day or a week comes as (bucketMinutes, IR605); an hour comes as its readings.
    var bucketMinutes: Double? {
        switch self {
        case .hour: nil
        case .day: 5
        case .week: 60
        }
    }

    /// The longest step between two points of a line before it is a gap the line does not cross.
    var longestStep: TimeInterval {
        switch self {
        case .hour: 120
        case .day: 900
        case .week: 3 * 3600
        }
    }

    public var label: String {
        switch self {
        case .hour: tr("1h")
        case .day: tr("24h")
        case .week: tr("7d")
        }
    }
}

/// One point of a chart line: the time, the value, and which stretch of the line it belongs to (a gap starts the next).
public struct SeriesPoint: Sendable, Equatable, Identifiable {
    public let at: Date
    public let value: Double
    public let segment: Int
    public var id: Date { at }
}

/// A metric's line over the period and what the screen says of its readings (techUnit.ts seriesChart).
public struct SeriesLine: Sendable, Equatable {
    public let metric: Metric
    public let name: String
    public let unit: String
    public let points: [SeriesPoint]
    public let count: String?
    public let cut: String?
    public let gap: String?
}

/// A metric's readings of the period: its points, how many the period holds, and whether they are all of them (the
/// period's averages) or the latest only (an hour's readings, IR598).
public struct SeriesReadings: Sendable {
    public let points: [(at: Date, value: Double)]
    public let total: Int
    public let complete: Bool

    public init(points: [(at: Date, value: Double)], total: Int, complete: Bool) {
        self.points = points
        self.total = total
        self.complete = complete
    }
}

/// A tile of the latest values: its label, value and when it was observed.
public struct UnitTile: Sendable, Equatable {
    public let label: String
    public let value: String
    public let sub: String
    public let isWarning: Bool
}

/// A line of a list: its title, its line under it and its badge.
public struct UnitRow: Sendable, Equatable, Identifiable {
    public let id: String
    public let title: String
    public let sub: String
    public let badge: String
}

/// The technician's unit screen in words (shared/lib/techUnit.ts, FR-T02, FR-T03, DD-T02, DD-T03).
public enum UnitWords {
    static func metric(_ metric: Metric) -> String { Evidence.metric(Substring(metric.rawValue)) }

    /// A reading as the screens write it (IR44): temperature and humidity with one decimal, ppm and µg/m³ whole, rounded
    /// half away from zero; other metrics as received.
    static func number(_ metric: Metric, _ value: Double) -> String {
        let digits: Int? =
            switch metric {
            case .temperature, .humidity: 1
            case .co2, .pm25: 0
            default: nil
            }
        guard let digits else { return ReportDraft.number(value) }
        let scale = pow(10, Double(digits))
        let rounded = (abs(value) * scale).rounded(.toNearestOrAwayFromZero) / scale * (value < 0 ? -1 : 1)
        return String(format: "%.\(digits)f", rounded)
    }

    static func quality(_ quality: Quality) -> String? {
        switch quality {
        case .valid: nil
        case .stale: tr("stale")
        case .suspect: tr("suspect")
        case .missing: tr("missing")
        }
    }

    /// The unit register (DD-T02): what it lacks says "Not registered", never a similar model's value.
    public static func register(_ unit: UnitDetail) -> [(String, String)] {
        let none = tr("Not registered")
        let maker = unit.capabilities.manufacturer
        let model = unit.capabilities.model
        let scope = unit.serviceScope.compactMap { ComponentGroup(rawValue: $0.rawValue).map { Checklist.label($0) } }
        var rows = [
            (tr("Location"), unit.location.pathLabels.isEmpty ? none : unit.location.pathLabels.joined(separator: " › ")),
            (tr("Manufacturer / model"), maker.isEmpty || model.isEmpty ? none : "\(maker) / \(model)"),
            (tr("Installed"), unit.installedAt.map { Format.date($0) } ?? none),
            (tr("Capability version"), String(Int(unit.capabilityVersion))),
            (tr("Maintenance scope"), scope.isEmpty ? none : scope.joined(separator: " · ")),
        ]
        if let access = unit.location.accessInstructions { rows.append((tr("Access"), access)) }
        return rows
    }

    /// The components of the service scope by group, in the canonical order (IR100).
    public static func components(_ unit: UnitDetail) -> [(String, String)] {
        Checklist.groups.compactMap { entry in
            let keys = entry.keys.filter { unit.components.contains($0) }
            guard !keys.isEmpty else { return nil }
            let title = tr("{group} · {n} components", ["group": Checklist.label(entry.group), "n": String(keys.count)])
            return (title, keys.map { Checklist.label($0) }.joined(separator: ", "))
        }
    }

    static func relative(_ instant: String, now: Date) -> String {
        guard let at = Format.parse(instant) else { return instant }
        let formatter = RelativeDateTimeFormatter()
        formatter.locale = Locale(identifier: Localizer.language.rawValue)
        formatter.dateTimeStyle = .named
        formatter.formattingContext = .middleOfSentence
        return formatter.localizedString(for: at, relativeTo: now)
    }

    /// The latest values (DD-T03): temperature, power, operation and connection, each with its time — last known, never
    /// live, while the unit is not online; a reading of another quality than valid keeps it beside the value (IR213).
    public static func tiles(_ unit: UnitDetail, now: Date) -> [UnitTile] {
        let isDown = unit.connection != .online
        let seen: (String?) -> String = { instant in
            guard let instant else { return "—" }
            let time = relative(instant, now: now)
            return isDown ? tr("last known · observed {time}", ["time": time]) : tr("observed {time}", ["time": time])
        }
        let reading: (Metric) -> UnitTile = { metric in
            let name = Self.metric(metric)
            guard let measurement = unit.latestMeasurements.first(where: { $0.metric == metric }) else {
                return UnitTile(label: name, value: "—", sub: tr("no reading"), isWarning: false)
            }
            guard let value = measurement.value else {
                return UnitTile(label: name, value: "—", sub: seen(measurement.observedAt), isWarning: false)
            }
            let note = quality(measurement.quality).map { " (\($0))" } ?? ""
            let text = "\(number(metric, value)) \(measurement.unit.rawValue)\(note)"
            return UnitTile(label: name, value: text, sub: seen(measurement.observedAt), isWarning: !note.isEmpty)
        }
        let state = unit.observedState
        let operation: String =
            switch (state.power, state.mode) {
            case (nil, _): "—"
            case (false?, _): tr("Off")
            case (true?, .cool?): tr("Cooling")
            case (true?, .dry?): tr("Drying")
            case (true?, .fan?): tr("Fan only")
            case (true?, _): tr("On")
            }
        let lastSeen = unit.lastSeenAt.map { tr("last seen {time}", ["time": relative($0, now: now)]) } ?? tr("never seen")
        return [
            reading(.temperature), reading(.power),
            UnitTile(label: tr("Operation"), value: operation, sub: seen(state.observedAt), isWarning: false),
            UnitTile(label: tr("Connection"), value: Labels.connection(unit.connection), sub: lastSeen, isWarning: isDown),
        ]
    }

    /// The banner while the connection is down: the values shown are the last known.
    public static func stopped(_ unit: UnitDetail, now: Date) -> String? {
        guard unit.connection != .online else { return nil }
        guard let seen = unit.lastSeenAt else {
            return tr("No data has reached the server from this unit yet — nothing here is real-time.")
        }
        return tr(
            "Updates stopped — communication lost {time}. Showing last known values with their times; nothing here is real-time.",
            ["time": relative(seen, now: now)]
        )
    }

    /// The chart's heading: the metrics, and the period rolling or in Kuala Lumpur calendar days.
    static func title(_ names: [String], period: SeriesPeriod) -> String {
        let metric = names.joined(separator: " · ")
        switch period {
        case .week: return tr("{metric} · last 7 days (Kuala Lumpur calendar days)", ["metric": metric])
        case .hour: return tr("{metric} · last {period} (rolling)", ["metric": metric, "period": tr("1 h")])
        case .day: return tr("{metric} · last {period} (rolling)", ["metric": metric, "period": tr("24 h")])
        }
    }

    /// A metric's line: its valid points in time order, a new stretch after a gap longer than the period's step; the
    /// readings it counts, or that the earlier ones of the hour are not loaded (IR598), and its first gap.
    public static func line(_ metric: Metric, unit: String, period: SeriesPeriod, _ readings: SeriesReadings) -> SeriesLine {
        let points = readings.points
        let total = readings.total
        let complete = readings.complete
        var segment = 0
        var gap: String?
        var out: [SeriesPoint] = []
        for point in points.sorted(by: { $0.at < $1.at }) {
            if let last = out.last, point.at.timeIntervalSince(last.at) > period.longestStep {
                segment += 1
                if gap == nil {
                    let span = Format.span(last.at.ISO8601Format(), point.at.ISO8601Format())
                    gap = tr("no data {span} (gap not connected)", ["span": span])
                }
            }
            out.append(SeriesPoint(at: point.at, value: point.value, segment: segment))
        }
        let isCut = !complete && total > points.count
        let counted = complete ? total : points.count
        let count = counted == 1 ? tr("1 measurement in this period") : tr("{n} measurements in this period", ["n": String(counted)])
        let cut = tr(
            "Showing the latest {n} of {total} readings — earlier readings in this period are not loaded.",
            ["n": String(points.count), "total": String(total)]
        )
        return SeriesLine(
            metric: metric,
            name: Self.metric(metric),
            unit: unit,
            points: out,
            count: isCut ? nil : count,
            cut: isCut ? cut : nil,
            gap: gap
        )
    }

    // MARK: - Alerts and events

    /// An alert's title: its suspected cause, else its type (alerts.ts alertTitle, IR260).
    public static func alertTitle(_ alert: CoreAlert) -> String {
        switch alert.causeCode {
        case .windowOpen: return tr("Possible open window")
        case .insulationLoss: return tr("Poor insulation suspected")
        default: break
        }
        switch alert.type {
        case .maintenance: return tr("Filter cleaning reminder")
        case .quality: return tr("Air quality alert")
        case .tamper: return tr("Device tamper")
        case .reconciliationRequired: return tr("Restriction check needed")
        case .restrictionBypass: return tr("Restriction may be bypassed")
        case .sensor: return tr("Sensor alert")
        }
    }

    /// The order of the severities, worst first.
    static func rank(_ severity: Severity) -> Int {
        switch severity {
        case .critical: 0
        case .warning: 1
        case .normal: 2
        }
    }

    static func severity(_ severity: Severity) -> String {
        switch severity {
        case .critical: tr("Critical")
        case .warning: tr("Warning")
        case .normal: tr("Info")
        }
    }

    /// An alert's resolution reason in the display language; a person's own reason stays as written.
    static func resolution(_ stored: String) -> String {
        if stored == "filter cleaned" { return tr("filter cleaned") }
        if stored == "recovery resolved" { return tr("recovery resolved") }
        guard let m = stored.wholeMatch(of: /Recovered: (\S+) (\S+) ([<>]) (\S+) for (\d+) s \(D08\)/) else { return stored }
        return tr(
            "Recovered: {metric} {value} {dir} {threshold} for {seconds} s",
            ["metric": Evidence.metric(m.1), "value": String(m.2), "dir": String(m.3), "threshold": String(m.4), "seconds": String(m.5)]
        )
    }

    /// The unit's unresolved alerts, worst and newest first, with their evidence and who acknowledged them.
    public static func openAlerts(_ alerts: [CoreAlert], now: Date) -> [UnitRow] {
        return alerts.filter { $0.status != .resolved }
            .sorted { rank($0.severity) != rank($1.severity) ? rank($0.severity) < rank($1.severity) : $0.detectedAt > $1.detectedAt }
            .map { alert in
                let acknowledged =
                    alert.acknowledgedAt.map { tr("Acknowledged {time}.", ["time": relative($0, now: now)]) }
                    ?? tr("Acknowledged by nobody yet.")
                let sub =
                    "\(Evidence.text(alert.evidenceText)) · \(relative(alert.detectedAt, now: now)) · \(acknowledged) \(tr("Completing the job does not resolve it."))"
                return UnitRow(id: alert.id, title: alertTitle(alert), sub: sub, badge: severity(alert.severity))
            }
    }

    /// The events of the period, newest first: alerts raised, acknowledged or resolved, and the device's connection,
    /// power and tamper events.
    public static func events(_ alerts: [CoreAlert], _ events: [DeviceEvent], period: SeriesPeriod, now: Date) -> [UnitRow] {
        let range = period.range(now: now)
        let inside: (String?) -> String? = { instant in
            guard let instant, let at = Format.parse(instant), at >= range.from, at <= range.to else { return nil }
            return instant
        }
        var out: [(at: String, row: UnitRow)] = []
        for alert in alerts {
            let title = alertTitle(alert)
            if let at = inside(alert.detectedAt) {
                let row = UnitRow(
                    id: "\(alert.id):raised",
                    title: tr("Alert raised — {title}", ["title": title]),
                    sub: Evidence.text(alert.evidenceText),
                    badge: severity(alert.severity)
                )
                out.append((at, row))
            }
            if let at = inside(alert.acknowledgedAt) {
                let row = UnitRow(
                    id: "\(alert.id):acknowledged",
                    title: tr("Alert acknowledged — {title}", ["title": title]),
                    sub: "",
                    badge: tr("Acknowledged")
                )
                out.append((at, row))
            }
            if let at = inside(alert.resolvedAt) {
                let row = UnitRow(
                    id: "\(alert.id):resolved",
                    title: tr("Alert resolved — {title}", ["title": title]),
                    sub: alert.resolutionReason.map(resolution) ?? "",
                    badge: tr("Resolved")
                )
                out.append((at, row))
            }
        }
        for event in events {
            guard let at = inside(event.occurredAt) else { continue }
            out.append((at, eventRow(event)))
        }
        return out.sorted { $0.at != $1.at ? $0.at > $1.at : $0.row.id < $1.row.id }
            .map {
                UnitRow(
                    id: $0.row.id,
                    title: $0.row.title,
                    sub: $0.row.sub.isEmpty ? relative($0.at, now: now) : "\($0.row.sub) · \(relative($0.at, now: now))",
                    badge: $0.row.badge
                )
            }
    }

    static func eventRow(_ event: DeviceEvent) -> UnitRow {
        let row = { (title: String, sub: String, badge: String) in UnitRow(id: event.id, title: title, sub: sub, badge: badge) }
        switch (event.eventType, event.recovery) {
        case (.restored, .power?): return row(tr("Power signal restored"), tr("the demo power signal is back"), tr("Power"))
        case (.restored, .tamper?):
            return row(tr("Tamper cleared"), tr("the cover is closed — the alert stays until it is handled"), tr("Tamper"))
        case (.restored, _): return row(tr("Data resumed"), tr("connection restored — the gap stays unconnected"), tr("Connectivity"))
        case (.communicationLost, _):
            return row(tr("Updates stopped"), tr("missed heartbeats — the values are frozen and not shown as live"), tr("Connectivity"))
        case (.powerLost, _): return row(tr("Power signal lost"), tr("the demo power signal dropped"), tr("Power"))
        case (.tamper, _): return row(tr("Cover opened"), tr("the device cover was opened while powered"), tr("Tamper"))
        case (.operationFailed, _): return row(tr("Device operation failed"), tr("a device operation did not complete"), tr("Device"))
        }
    }

    // MARK: - Jobs

    /// The technician's jobs on the unit: the current ones with their visit, newest first, then the ended ones.
    public static func jobs(_ items: [JobsListResultItem]) -> [UnitRow] {
        let current = items.compactMap { item -> JobSummary? in if case let .summary(job) = item { job } else { nil } }
            .sorted { ($0.scheduledSlot?.startAt ?? $0.dueAt) > ($1.scheduledSlot?.startAt ?? $1.dueAt) }
            .map { job in
                UnitRow(
                    id: job.id,
                    title: "\(job.id.prefix(8)) · \(Labels.jobType(job.type))",
                    sub: job.scheduledSlot.map { Format.span($0.startAt, $0.endAt) } ?? tr("not scheduled yet"),
                    badge: Labels.job(job.status)
                )
            }
        let ended = items.compactMap { item -> JobHistorySnapshot? in if case let .history(job) = item { job } else { nil } }
            .sorted { ($0.completedAt ?? "") > ($1.completedAt ?? "") }
            .map { job in
                UnitRow(
                    id: job.jobId,
                    title: "\(job.jobId.prefix(8)) · \(Labels.jobType(job.type))",
                    sub: job.completedAt.map { tr("completed {date}", ["date": Format.date($0)]) } ?? tr("your assignment ended"),
                    badge: Labels.job(job.status)
                )
            }
        return current + ended
    }

    /// The job diagnostic control works for (DD-T10, IR94): the given one when it is one of the technician's active jobs
    /// on the unit, else the one in progress, then assigned, then returned for rework.
    public static func controlJob(_ items: [JobsListResultItem], preferring jobId: String?) -> String? {
        let rank: (JobStatus) -> Int? = { status in
            switch status {
            case .inProgress: 0
            case .assigned: 1
            case .reworkRequested: 2
            default: nil
            }
        }
        let active = items.compactMap { item -> (job: JobSummary, rank: Int)? in
            guard case let .summary(job) = item, let order = rank(job.status) else { return nil }
            return (job, order)
        }
        .sorted { $0.rank < $1.rank }
        .map(\.job)
        return (active.first { $0.id == jobId } ?? active.first)?.id
    }
}
