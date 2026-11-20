import ACKit
import ACShared
import Foundation
import Observation

/// The technician's unit screen (FR-T02, FR-T03, DD-T02, DD-T03, the web's technician unit page): the register and the
/// components, the latest values with their times, the chosen metrics over a period, the other metrics, the period's
/// events, the open alerts and the technician's jobs on the unit. Before the work window starts it says when it opens
/// (IR76); a unit outside the technician's assignments is not found (IR169).
@MainActor @Observable
public final class TechUnitModel {
    public enum Content: Sendable, Equatable {
        case unit(UnitDetail)
        /// The work window has not started: when it does, if the job says.
        case notStarted(String)
        case notFound
    }

    public let unitId: String
    /// The job the screen is opened for (its window, and the job diagnostics work for).
    public let jobId: String?
    public private(set) var content: Loadable<Content> = .loading
    public private(set) var period = SeriesPeriod.day
    /// The metrics of the chart: two units at most (a left and a right axis).
    public private(set) var metrics: [Metric] = []
    public private(set) var lines: [SeriesLine] = []
    public private(set) var alerts: [CoreAlert] = []
    public private(set) var events: [DeviceEvent] = []
    public private(set) var jobs: [JobsListResultItem] = []
    public private(set) var now = Date()

    let client: CoreClient

    public init(unitId: String, jobId: String?, client: CoreClient) {
        self.unitId = unitId
        self.jobId = jobId
        self.client = client
    }

    public var unit: UnitDetail? {
        if case let .unit(unit)? = content.value { unit } else { nil }
    }

    public func load() async {
        content = await content.after { () async throws(CoreError) in try await read() }
        guard let unit else { return }
        if metrics.isEmpty { metrics = [Self.firstMetric(unit)] }
        alerts = (try? await unitAlerts()) ?? []
        jobs = (try? await client.call(Ops.JobsList.self, Query(limit: 20, filters: QueryFilters(unitId: unitId))).data.items) ?? []
        await loadSeries()
    }

    /// The unit's alerts: every unresolved one, then the latest resolved ones (IR712). The list sorts the most severe
    /// first across the statuses, so one read of all of them could lose unresolved ones behind resolved ones.
    private func unitAlerts() async throws(CoreError) -> [CoreAlert] {
        var alerts: [CoreAlert] = []
        for status in ["open", "acknowledged"] {
            let query = Query(limit: 100, filters: QueryFilters(unitId: unitId, status: status))
            alerts += try await client.call(Ops.AlertsList.self, query).data.items
        }
        let resolved = Query(
            limit: 20,
            sort: QuerySort(field: .updatedAt, direction: .desc),
            filters: QueryFilters(unitId: unitId, status: "resolved")
        )
        return alerts + (try await client.call(Ops.AlertsList.self, resolved).data.items)
    }

    private func read() async throws(CoreError) -> Content {
        do {
            let answer = try await client.call(Ops.UnitsGet.self, UnitsGetInput(id: unitId, jobId: jobId))
            now = answer.meta.snapshotAt.flatMap(Format.parse) ?? Date()
            return .unit(answer.data)
        } catch {
            if error.error.messageKey == "errors.assignment_not_started" { return .notStarted(await opening()) }
            let code = error.error.code
            guard code == .notFound || code == .forbidden || error.error.fieldErrors["id"] != nil else { throw error }
            return .notFound
        }
    }

    /// When the AC can be seen: the start of the job's work window, or the general sentence.
    private func opening() async -> String {
        guard let jobId,
            case let .detail(job)? = try? await client.call(Ops.JobsGet.self, JobsGetInput(jobId: jobId)).data,
            let start = job.assignment?.scheduledStart
        else { return tr("Available from the work start time of your assigned job.") }
        return tr("You can see this AC from {time}, when your work window starts.", ["time": Format.dateTime(start)])
    }

    /// The metric the chart starts with: temperature when the unit measures it, else its first measured one.
    static func firstMetric(_ unit: UnitDetail) -> Metric {
        let measured = unit.latestMeasurements.map(\.metric)
        return measured.contains(.temperature) ? .temperature : measured.first ?? .temperature
    }

    // MARK: - The chart

    public func show(_ period: SeriesPeriod) async {
        self.period = period
        await loadSeries()
    }

    /// Adds a metric to the chart or takes it away; a third unit is not added (the chart has two axes), and the last
    /// metric stays.
    public func toggle(_ metric: Metric) async {
        guard let unit else { return }
        if metrics.contains(metric) {
            guard metrics.count > 1 else { return }
            metrics.removeAll { $0 == metric }
        } else {
            let unitOf: (Metric) -> UnitSymbol? = { m in unit.latestMeasurements.first { $0.metric == m }?.unit }
            let units = Set((metrics + [metric]).compactMap(unitOf))
            guard units.count <= 2 else { return }
            metrics.append(metric)
        }
        await loadSeries()
    }

    /// Whether a metric can join the chart (its unit is one of the two axes, or there is a free one).
    public func canAdd(_ metric: Metric) -> Bool {
        guard let unit, !metrics.contains(metric) else { return true }
        let unitOf: (Metric) -> UnitSymbol? = { m in unit.latestMeasurements.first { $0.metric == m }?.unit }
        return Set((metrics + [metric]).compactMap(unitOf)).count <= 2
    }

    private func loadSeries() async {
        guard let unit else { return }
        let range = period.range(now: now)
        var lines: [SeriesLine] = []
        for metric in metrics {
            let symbol = unit.latestMeasurements.first { $0.metric == metric }?.unit.rawValue ?? Checklist.unit(metric).rawValue
            lines.append(await line(metric, unit: symbol, from: range.from, to: range.to))
        }
        self.lines = lines
        events = await deviceEvents(from: range.from, to: range.to)
    }

    /// A metric over the period: a day or a week as its 5- or 60-minute averages (IR605), an hour as its latest
    /// readings, up to 4 pages of 100 (IR598); an unreadable series is an empty line.
    private func line(_ metric: Metric, unit: String, from: Date, to: Date) async -> SeriesLine {
        let period = period
        let empty = UnitWords.line(metric, unit: unit, period: period, SeriesReadings(points: [], total: 0, complete: true))
        let range = (from: from.ISO8601Format(), to: to.addingTimeInterval(1).ISO8601Format())
        if let minutes = period.bucketMinutes {
            let input = TelemetrySeriesInput(
                from: range.from,
                to: range.to,
                unitIds: [unitId],
                metric: metric,
                bucketMinutes: minutes,
                query: Query()
            )
            guard let series = try? await client.call(Ops.TelemetrySeries.self, input).data else { return empty }
            let points = (series.buckets ?? []).compactMap { bucket in
                // swiftlint:disable:next empty_count - a bucket's count is its number of readings (a Double), not a collection
                bucket.count > 0 ? zip(Format.parse(bucket.start), bucket.value).map { (at: $0, value: $1) } : nil
            }
            return UnitWords.line(
                metric,
                unit: unit,
                period: period,
                SeriesReadings(points: points, total: Int(series.total), complete: true)
            )
        }
        var readings: [CoreMeasurement] = []
        var total = 0
        var cursor: String?
        for page in 0..<Self.hourPages {
            let query = Query(cursor: cursor, limit: 100, sort: QuerySort(field: .observedAt, direction: .desc))
            let input = TelemetrySeriesInput(from: range.from, to: range.to, unitIds: [unitId], metric: metric, query: query)
            guard let series = try? await client.call(Ops.TelemetrySeries.self, input).data else {
                return page == 0 ? empty : line(metric, unit, readings, total)
            }
            readings += series.items
            if page == 0 { total = Int(series.total) }
            cursor = series.nextCursor
            if cursor == nil { break }
        }
        return line(metric, unit, readings, total)
    }

    /// The most pages of an hour's readings the chart reads.
    static let hourPages = 4

    private func line(_ metric: Metric, _ unit: String, _ readings: [CoreMeasurement], _ total: Int) -> SeriesLine {
        // stale, suspect and missing readings are not plotted
        let points = readings.compactMap { reading in
            reading.quality == .valid ? zip(Format.parse(reading.observedAt), reading.value).map { (at: $0, value: $1) } : nil
        }
        return UnitWords.line(
            metric,
            unit: unit,
            period: period,
            SeriesReadings(points: points, total: max(total, readings.count), complete: false)
        )
    }

    /// The events of the unit's devices in the period (connection, power, tamper).
    private func deviceEvents(from: Date, to: Date) async -> [DeviceEvent] {
        let devices =
            (try? await client.call(Ops.DevicesList.self, Query(limit: 5, filters: QueryFilters(unitId: unitId))).data.items) ?? []
        var events: [DeviceEvent] = []
        for device in devices {
            let query = Query(limit: 50, filters: QueryFilters(from: from.ISO8601Format(), to: to.addingTimeInterval(1).ISO8601Format()))
            events += (try? await client.call(Ops.DevicesEvents.self, DevicesEventsInput(id: device.id, query: query)).data.items) ?? []
        }
        return events
    }

    // MARK: - What the screen shows

    public var tiles: [UnitTile] { unit.map { UnitWords.tiles($0, now: now) } ?? [] }
    public var stopped: String? { unit.flatMap { UnitWords.stopped($0, now: now) } }
    public var register: [(String, String)] { unit.map(UnitWords.register) ?? [] }
    public var components: [(String, String)] { unit.map(UnitWords.components) ?? [] }
    public var openAlerts: [UnitRow] { UnitWords.openAlerts(alerts, now: now) }
    public var periodEvents: [UnitRow] { UnitWords.events(alerts, events, period: period, now: now) }
    public var history: [UnitRow] { UnitWords.jobs(jobs) }
    public var controlJob: String? { UnitWords.controlJob(jobs, preferring: jobId) }
    public var chartTitle: String { UnitWords.title(lines.map { "\($0.name) (\($0.unit))" }, period: period) }

    /// The metrics the unit measures, for the chart's choice.
    public var measured: [Metric] { unit?.latestMeasurements.map(\.metric) ?? [] }
}

/// Both values, or nil (a point needs its time and its value).
private func zip<A, B>(_ a: A?, _ b: B?) -> (A, B)? {
    guard let a, let b else { return nil }
    return (a, b)
}
