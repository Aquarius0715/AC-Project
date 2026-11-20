import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

/// The live unit as the technician reads it (humidity, power and temperature; indoor, outdoor and electrical).
func technicianUnit() throws -> UnitDetail {
    struct Envelope: Decodable { let data: UnitDetail }
    return try JSONDecoder().decode(Envelope.self, from: try fixture("units.get.technician")).data
}

func alert(
    _ id: String,
    severity: Severity = .warning,
    status: CoreAlertStatus = .open,
    detected: String = "2026-09-27T04:00:00Z",
    type: CoreAlertType = .sensor,
    cause: CoreAlertCauseCode = .unknown
) -> CoreAlert {
    CoreAlert(
        id: id,
        tenantId: "t1",
        version: 1,
        createdAt: detected,
        updatedAt: detected,
        unitId: "u1",
        policyId: nil,
        type: type,
        severity: severity,
        status: status,
        causeCode: cause,
        evidenceKind: .demoObservation,
        evidenceText: "Demo: high temperature observed (fictional)",
        observedAt: detected,
        evidenceIds: [],
        detectedAt: detected,
        acknowledgedAt: nil,
        resolvedAt: nil,
        resolutionReason: nil,
        previousAlertId: nil,
        deliveryFailures: [],
        rule: nil
    )
}

@Suite struct UnitWordsTests {
    private let now = Format.parse("2026-09-27T05:00:00Z") ?? Date()

    @Test func thePeriods() throws {
        #expect(SeriesPeriod.hour.range(now: now).from == now.addingTimeInterval(-3600))
        #expect(SeriesPeriod.day.range(now: now).from == now.addingTimeInterval(-86_400))
        // 7 days: from 00:00 in Kuala Lumpur six days before today (05:00 UTC is 13:00 there)
        #expect(SeriesPeriod.week.range(now: now).from == Format.parse("2026-09-20T16:00:00Z"))
        #expect(SeriesPeriod.allCases.map(\.bucketMinutes) == [nil, 5, 60])
        #expect(SeriesPeriod.allCases.map(\.label) == ["1h", "24h", "7d"])
    }

    @Test func numbersAsTheScreensWriteThem() {
        #expect(UnitWords.number(.temperature, 25.66) == "25.7")
        #expect(UnitWords.number(.temperature, -2.25) == "-2.3", "half away from zero")
        #expect(UnitWords.number(.co2, 612.5) == "613")
        #expect(UnitWords.number(.power, 1.01) == "1.01", "as received")
        #expect([Quality.valid, .stale, .suspect, .missing].map(UnitWords.quality) == [nil, "stale", "suspect", "missing"])
    }

    @Test func theRegisterSaysWhatIsMissing() throws {
        var unit = try technicianUnit()
        #expect(
            UnitWords.register(unit).map(\.0) == [
                "Location", "Manufacturer / model", "Installed", "Capability version", "Maintenance scope", "Access",
            ]
        )
        #expect(UnitWords.register(unit)[1].1 == "DemoAir / SPL-100")
        unit.location = UnitDetailLocation(pathLabels: [], address: nil, accessInstructions: nil)
        unit.capabilities.model = ""
        unit.installedAt = nil
        unit.serviceScope = []
        let rows = UnitWords.register(unit)
        #expect(rows.count == 5 && rows.filter { $0.1 == "Not registered" }.count == 4)
    }

    @Test func theInstalledDayAndAPointsIdentity() throws {
        var unit = try technicianUnit()
        unit.installedAt = "2026-01-15T00:00:00Z"
        #expect(UnitWords.register(unit)[2].1 == Format.date("2026-01-15T00:00:00Z"))
        let point = SeriesPoint(at: now, value: 1, segment: 0)
        #expect(point.id == now)
    }

    @Test func eventsAtTheSameTimeKeepTheirOrder() {
        var both = alert("a1", detected: "2026-09-27T04:00:00Z")
        both.acknowledgedAt = "2026-09-27T04:00:00Z"
        #expect(UnitWords.events([both], [], period: .day, now: now).map(\.id) == ["a1:acknowledged", "a1:raised"])
        var resolved = alert("a2", detected: "2026-09-27T04:00:00Z")
        resolved.resolvedAt = "2026-09-27T04:30:00Z"
        #expect(UnitWords.events([resolved], [], period: .day, now: now).first?.sub.contains("·") == false, "no reason: the time alone")
    }

    @Test func jobsWithoutAVisitOrACompletionAreOrderedAllTheSame() throws {
        struct Envelope: Decodable { let data: Page<JobsListResultItem> }
        let past = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.list.technician")).data.items
        guard case var .history(open) = past[0] else { throw CocoaError(.coderValueNotFound) }
        open.completedAt = nil
        var other = open
        other.jobId = "other"
        #expect(UnitWords.jobs([.history(open), .history(other)]).count == 2)
        #expect(UnitWords.jobs([.history(other), .history(open)]).allSatisfy { $0.sub == "your assignment ended" })
    }

    @Test func theComponentsByGroup() throws {
        var unit = try technicianUnit()
        unit.components = [.filter, .compressor, .blade]
        #expect(UnitWords.components(unit).map(\.0) == ["Indoor · 1 components", "Outdoor · 2 components"])
        #expect(UnitWords.components(unit).last?.1 == "Compressor, Fan blade")
    }

    @Test func theLatestValuesWithTheirTimes() throws {
        var unit = try technicianUnit()
        unit.connection = .online
        let tiles = UnitWords.tiles(unit, now: now)
        #expect(tiles.map(\.label) == ["Temperature", "Power", "Operation", "Connection"])
        #expect(tiles[0].value == "25.6 °C" && tiles[0].sub.hasPrefix("observed "))
        #expect(UnitWords.stopped(unit, now: now) == nil)
        unit.connection = .offline
        unit.latestMeasurements[2].quality = .stale
        unit.latestMeasurements[1].value = nil
        unit.observedState = ObservedState(power: true, celsius: nil, mode: .dry, fanLevel: nil, observedAt: nil)
        let down = UnitWords.tiles(unit, now: now)
        #expect(down[0].value == "25.6 °C (stale)" && down[0].isWarning && down[0].sub.hasPrefix("last known · observed "))
        #expect(down[1].value == "—")
        #expect(down[2].value == "Drying" && down[2].sub == "—")
        #expect(down[3].isWarning && down[3].sub.hasPrefix("last seen "))
        #expect(UnitWords.stopped(unit, now: now)?.hasPrefix("Updates stopped — communication lost ") == true)
        unit.lastSeenAt = nil
        unit.latestMeasurements = []
        #expect(UnitWords.tiles(unit, now: now)[0].sub == "no reading")
        #expect(UnitWords.tiles(unit, now: now)[3].sub == "never seen")
        #expect(UnitWords.stopped(unit, now: now) == "No data has reached the server from this unit yet — nothing here is real-time.")
    }

    @Test func eachOperationInWords() throws {
        var unit = try technicianUnit()
        let operation = { (state: ObservedState) -> String in
            unit.observedState = state
            return UnitWords.tiles(unit, now: now)[2].value
        }
        let states = [
            ObservedState(power: nil, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil),
            ObservedState(power: false, celsius: nil, mode: .cool, fanLevel: nil, observedAt: nil),
            ObservedState(power: true, celsius: nil, mode: .cool, fanLevel: nil, observedAt: nil),
            ObservedState(power: true, celsius: nil, mode: .fan, fanLevel: nil, observedAt: nil),
            ObservedState(power: true, celsius: nil, mode: nil, fanLevel: nil, observedAt: nil),
        ]
        #expect(states.map(operation) == ["—", "Off", "Cooling", "Fan only", "On"])
    }

    @Test func theChartTitles() {
        #expect(UnitWords.title(["Temperature (°C)"], period: .hour) == "Temperature (°C) · last 1 h (rolling)")
        #expect(UnitWords.title(["A", "B"], period: .day) == "A · B · last 24 h (rolling)")
        #expect(UnitWords.title(["A"], period: .week) == "A · last 7 days (Kuala Lumpur calendar days)")
    }

    @Test func aLineBreaksAtAGapAndCountsItsReadings() {
        let at = { (minutes: Double) in now.addingTimeInterval(minutes * 60) }
        let points = [(at: at(-10), value: 1.0), (at: at(-60), value: 2.0), (at: at(-5), value: 3.0)]
        let line = UnitWords.line(.temperature, unit: "°C", period: .day, SeriesReadings(points: points, total: 300, complete: true))
        #expect(line.points.map(\.value) == [2, 1, 3], "in time order")
        #expect(line.points.map(\.segment) == [0, 1, 1], "the 50 minutes are a gap")
        #expect(line.gap?.hasPrefix("no data ") == true && line.count == "300 measurements in this period" && line.cut == nil)
        let one = UnitWords.line(.temperature, unit: "°C", period: .hour, SeriesReadings(points: [points[0]], total: 1, complete: false))
        #expect(one.count == "1 measurement in this period" && one.gap == nil)
        let cut = UnitWords.line(.temperature, unit: "°C", period: .hour, SeriesReadings(points: points, total: 500, complete: false))
        #expect(cut.count == nil && cut.cut == "Showing the latest 3 of 500 readings — earlier readings in this period are not loaded.")
        #expect(SeriesPeriod.allCases.map(\.longestStep) == [120, 900, 10_800])
    }

    @Test func alertTitlesByCauseThenType() {
        #expect(UnitWords.alertTitle(alert("a", cause: .windowOpen)) == "Possible open window")
        #expect(UnitWords.alertTitle(alert("a", cause: .insulationLoss)) == "Poor insulation suspected")
        let titles = CoreAlertType.allCases.map { UnitWords.alertTitle(alert("a", type: $0)) }
        #expect(
            titles == [
                "Sensor alert", "Air quality alert", "Filter cleaning reminder", "Device tamper", "Restriction check needed",
                "Restriction may be bypassed",
            ]
        )
        #expect(Severity.allCases.map(UnitWords.severity).sorted() == ["Critical", "Info", "Warning"])
    }

    @Test func resolutionReasonsInWords() {
        #expect(UnitWords.resolution("filter cleaned") == "filter cleaned")
        #expect(UnitWords.resolution("recovery resolved") == "recovery resolved")
        #expect(UnitWords.resolution("Recovered: temperature 26.1 < 30 for 60 s (D08)") == "Recovered: Temperature 26.1 < 30 for 60 s")
        #expect(UnitWords.resolution("Cleaned it myself") == "Cleaned it myself")
    }

    @Test func theOpenAlertsWorstAndNewestFirst() {
        var acknowledged = alert("a2", severity: .critical, detected: "2026-09-27T03:00:00Z")
        acknowledged.acknowledgedAt = "2026-09-27T03:30:00Z"
        let rows = UnitWords.openAlerts(
            [
                alert("a1"), acknowledged, alert("a3", status: .resolved), alert("a4", detected: "2026-09-27T04:30:00Z"),
                alert("a5", severity: .normal),
            ],
            now: now
        )
        #expect(rows.map(\.id) == ["a2", "a4", "a1", "a5"])
        #expect(
            rows[0].badge == "Critical" && rows[0].sub.contains("Acknowledged ")
                && rows[0].sub.hasSuffix("Completing the job does not resolve it.")
        )
        #expect(rows[1].sub.contains("Acknowledged by nobody yet."))
    }

    @Test func thePeriodsEventsNewestFirst() throws {
        struct Envelope: Decodable { let data: Page<DeviceEvent> }
        var resolved = alert("a1", detected: "2026-09-27T04:00:00Z")
        resolved.acknowledgedAt = "2026-09-27T04:10:00Z"
        resolved.resolvedAt = "2026-09-27T04:20:00Z"
        resolved.resolutionReason = "filter cleaned"
        let old = alert("a0", detected: "2026-09-20T04:00:00Z")
        let event = try JSONDecoder().decode(Envelope.self, from: Data(Self.event("communication_lost", at: "2026-09-27T04:30:00Z").utf8))
            .data.items[0]
        let rows = UnitWords.events([resolved, old], [event], period: .day, now: now)
        #expect(rows.map(\.id) == ["ev1", "a1:resolved", "a1:acknowledged", "a1:raised"])
        #expect(rows[0].title == "Updates stopped" && rows[0].badge == "Connectivity")
        #expect(rows[1].title == "Alert resolved — Sensor alert" && rows[1].sub.hasPrefix("filter cleaned · "))
        #expect(rows[2].sub.isEmpty == false, "the time alone")
    }

    static func event(_ type: String, at: String, recovery: String = "null") -> String {
        let item =
            #"{"id":"ev1","tenantId":"t1","version":1,"createdAt":"\#(at)","updatedAt":"\#(at)","bindingId":null,"#
            + #""unitIdAtOccurrence":"u1","#
            + #""customerOrgIdAtOccurrence":"c1","alertIds":[],"deviceId":"d1","eventType":"\#(type)","recovery":\#(recovery),"#
            + #""evidenceSource":"heartbeat","sequence":1,"occurredAt":"\#(at)","restoredAt":null,"responseNotes":[]}"#
        return #"{"data":{"items":["# + item + #"],"nextCursor":null,"total":1,"snapshotVersion":1},"meta":{"correlationId":"c"}}"#
    }

    @Test func eachDeviceEventInWords() throws {
        struct Envelope: Decodable { let data: Page<DeviceEvent> }
        let words = { (type: String, recovery: String) throws -> String in
            let page = try JSONDecoder().decode(
                Envelope.self,
                from: Data(Self.event(type, at: "2026-09-27T04:00:00Z", recovery: recovery).utf8)
            )
            return UnitWords.eventRow(page.data.items[0]).title
        }
        #expect(try words("power_lost", "null") == "Power signal lost")
        #expect(try words("tamper", "null") == "Cover opened")
        #expect(try words("operation_failed", "null") == "Device operation failed")
        #expect(try words("restored", "null") == "Data resumed")
        #expect(try words("restored", #"{"axis":"power","sourceEventId":"e0","value":"on"}"#) == "Power signal restored")
        #expect(try words("restored", #"{"axis":"tamper","sourceEventId":"e0","value":"clear"}"#) == "Tamper cleared")
    }

    @Test func theTechniciansJobsOnTheUnit() throws {
        struct Envelope: Decodable { let data: Page<JobsListResultItem> }
        let history = try JSONDecoder().decode(Envelope.self, from: try fixture("jobs.list.technician")).data.items
        var later = history[0]
        if case var .history(job) = later {
            job.completedAt = "2026-09-25T00:00:00Z"
            job.jobId = "later"
            later = .history(job)
        }
        let assigned = JobsListResultItem.summary(
            JobSummary(
                id: "j1",
                version: 1,
                status: .assigned,
                createdAt: "2026-09-27T00:00:00Z",
                dueAt: "2026-09-28T00:00:00Z",
                type: .reactive,
                origin: .clientRequest,
                unitId: "u1",
                requestedSlot: Slot(startAt: "2026-09-27T01:00:00Z", endAt: "2026-09-27T02:00:00Z"),
                scheduledSlot: nil,
                assignmentId: nil,
                preferredSlots: [],
                followUpOfJobId: nil,
                followUpClass: nil,
                severity: .normal,
                displayStatus: .assigned,
                assignmentAcknowledgement: .accepted,
                technicianMembershipId: "m1",
                accessValidFrom: nil,
                accessValidUntil: nil,
                eventSummary: JobEventSummary(count: 0, customerVisible: 0, notes: 0, latest: nil)
            )
        )
        var working = assigned
        if case var .summary(job) = working {
            job.id = "j2"
            job.status = .inProgress
            job.scheduledSlot = Slot(startAt: "2026-09-27T03:00:00Z", endAt: "2026-09-27T04:00:00Z")
            working = .summary(job)
        }
        var unscheduled = assigned
        if case var .summary(job) = unscheduled {
            job.id = "j0"
            job.dueAt = "2026-09-29T00:00:00Z"
            unscheduled = .summary(job)
        }
        // without a visit a job is ordered by its due date
        #expect(UnitWords.jobs([assigned, unscheduled]).map(\.id) == ["j0", "j1"])
        #expect(UnitWords.jobs([unscheduled, assigned]).map(\.id) == ["j0", "j1"])
        let rows = UnitWords.jobs([history[0], assigned, later, working])
        #expect(rows.map(\.id) == ["j1", "j2", "later", "41de64d5-73e5-52a7-bdd1-7c6c2cf13cf6"])
        #expect(rows[0].sub == "not scheduled yet" && rows[2].sub.hasPrefix("completed ") && rows[3].sub == "your assignment ended")
        #expect(UnitWords.controlJob([assigned, working], preferring: nil) == "j2", "the one in progress first")
        #expect(UnitWords.controlJob([assigned, working], preferring: "j1") == "j1")
        #expect(UnitWords.controlJob(history, preferring: nil) == nil)
    }
}

@MainActor @Suite struct TechUnitModelTests {
    private func model(_ answers: [StubTransport.Answer], jobId: String? = "j1") -> (TechUnitModel, StubTransport) {
        let transport = StubTransport(answers)
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        return (TechUnitModel(unitId: "e841ee95-3171-54d5-be91-1f2301bdb701", jobId: jobId, client: client), transport)
    }

    private func text(_ name: String) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try fixture(name), encoding: .utf8)))
    }

    private func page<Item: Codable & Sendable & Equatable>(_ items: [Item]) throws -> StubTransport.Answer {
        let body = try JSONEncoder().encode(
            AnswerEnvelope(
                data: Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1),
                meta: ["correlationId": "c"]
            )
        )
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    private func refusal(_ status: Int, _ code: String, _ key: String, fields: String = "{}") -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":\#(fields),"correlationId":"c","retryAfterSeconds":null}"#)
    }

    /// The unit, its three alert reads, its jobs, the day's temperature, its device list and the device's events.
    private func loaded() async throws -> (TechUnitModel, StubTransport) {
        let (model, transport) = model([
            try text("units.get.technician"), try page([alert("a1")]), try page([CoreAlert]()), try page([CoreAlert]()),
            try text("jobs.list.technician"), try text("telemetry.series.day"), try text("devices.list"), try text("devices.events"),
        ])
        await model.load()
        return (model, transport)
    }

    @Test func theUnitAndItsMonitoringAreRead() async throws {
        let (model, transport) = try await loaded()
        #expect(model.unit?.displayName == "Living room AC")
        #expect(model.metrics == [.temperature])
        // 255 of the 288 buckets hold readings; the empty ones are a gap
        #expect(
            model.lines.first?.points.count == 255 && model.lines.first?.gap != nil
                && model.lines.first?.count == "1249 measurements in this period"
        )
        #expect(model.openAlerts.map(\.id) == ["a1"] && model.history.count == 1)
        #expect(model.tiles.count == 4 && model.register.count == 6 && model.components.count == 3)
        #expect(model.chartTitle == "Temperature (°C) · last 24 h (rolling)")
        #expect(model.measured == [.humidity, .power, .temperature] && model.controlJob == nil && model.periodEvents.isEmpty)
        let requests = await transport.requests.compactMap(\.url)
        #expect(requests[0].query() == "jobId=j1")
        #expect(query(requests[1].path() + "?" + (requests[1].query() ?? "")).values["status"] == ["open"])
        #expect(query(requests[3].path() + "?" + (requests[3].query() ?? "")).values["sort"] == ["updatedAt:desc"])
        #expect(query(requests[5].path() + "?" + (requests[5].query() ?? "")).values["bucketMinutes"] == ["5"])
        #expect(requests[7].path() == "/v1/devices/88ee2ed2-79db-53b6-be15-a92f7829c53d/events")
    }

    @Test func anHourIsReadAsItsLatestReadingsPageByPage() async throws {
        let (model, transport) = try await loaded()
        let first =
            String(bytes: try fixture("telemetry.series.hour"), encoding: .utf8)?.replacingOccurrences(
                of: #""nextCursor": null"#,
                with: #""nextCursor": "p2""#
            ) ?? ""
        let more = Array(repeating: StubTransport.Answer.http(200, first), count: TechUnitModel.hourPages)
        await transport.queue(more + [try text("devices.list"), try page([DeviceEvent]())])
        await model.show(.hour)
        #expect(model.period == .hour)
        let line = try #require(model.lines.first)
        #expect(line.points.count == 60 * TechUnitModel.hourPages, "the pages, while each says there is a next")
        let requests = await transport.requests.compactMap(\.url)
        #expect(query(requests[8].path() + "?" + (requests[8].query() ?? "")).values["limit"] == ["100"])
        #expect(query(requests[9].path() + "?" + (requests[9].query() ?? "")).values["cursor"] == ["p2"])
    }

    @Test func anUnreadableSeriesIsAnEmptyLine() async throws {
        let (model, transport) = try await loaded()
        await transport.queue([.failure(URLError(.timedOut)), .failure(URLError(.timedOut))])
        await model.show(.hour)
        #expect(model.lines.first?.points.isEmpty == true)
        await transport.queue([.failure(URLError(.timedOut)), .failure(URLError(.timedOut))])
        await model.show(.week)
        #expect(model.lines.first?.points.isEmpty == true && model.events.isEmpty)
        let page =
            String(bytes: try fixture("telemetry.series.hour"), encoding: .utf8)?.replacingOccurrences(
                of: #""nextCursor": null"#,
                with: #""nextCursor": "p2""#
            ) ?? ""
        await transport.queue([.http(200, page), .failure(URLError(.timedOut)), try text("devices.list"), .failure(URLError(.timedOut))])
        await model.show(.hour)
        #expect(model.lines.first?.points.count == 60, "the pages read before the refusal")
    }

    @Test func metricsJoinTheChartUpToTwoUnits() async throws {
        let (model, transport) = try await loaded()
        #expect(model.canAdd(.humidity) && model.canAdd(.temperature))
        await transport.queue([
            try text("telemetry.series.day"), try text("telemetry.series.day"), try text("devices.list"), try page([DeviceEvent]()),
        ])
        await model.toggle(.humidity)
        #expect(model.metrics == [.temperature, .humidity])
        #expect(!model.canAdd(.power), "kW would be a third unit")
        await model.toggle(.power)
        #expect(model.metrics == [.temperature, .humidity])
        await transport.queue([try text("telemetry.series.day"), try text("devices.list"), try page([DeviceEvent]())])
        await model.toggle(.humidity)
        #expect(model.metrics == [.temperature])
        await model.toggle(.temperature)
        #expect(model.metrics == [.temperature], "the last metric stays")
    }

    @Test func beforeTheWindowItSaysWhenItOpens() async throws {
        let notStarted = refusal(403, "FORBIDDEN", "errors.assignment_not_started")
        var (model, _) = model([notStarted, try text("jobs.get.assigned")])
        await model.load()
        guard case let .notStarted(text)? = model.content.value else {
            Issue.record("not started")
            return
        }
        #expect(text.hasPrefix("You can see this AC from "))
        (model, _) = self.model([notStarted], jobId: nil)
        await model.load()
        #expect(model.content.value == .notStarted("Available from the work start time of your assigned job."))
        #expect(model.tiles.isEmpty && model.register.isEmpty && model.components.isEmpty && model.measured.isEmpty && model.stopped == nil)
        await model.toggle(.humidity)
        #expect(model.canAdd(.humidity))
    }

    @Test func aUnitOutsideTheAssignmentsIsNotFound() async throws {
        for answer in [
            refusal(404, "NOT_FOUND", "error.notFound"), refusal(403, "FORBIDDEN", "error.forbidden"),
            refusal(422, "VALIDATION", "error.validation", fields: #"{"id":"error.invalid"}"#),
        ] {
            let (model, _) = model([answer])
            await model.load()
            #expect(model.content.value == .notFound)
        }
        let (model, _) = model([.failure(URLError(.timedOut))])
        await model.load()
        #expect(model.content.message != nil)
    }

    @Test func unreadableAlertsAndJobsLeaveTheirListsEmpty() async throws {
        let (model, _) = model([try text("units.get.technician"), .failure(URLError(.timedOut)), .failure(URLError(.timedOut))])
        await model.load()
        #expect(model.alerts.isEmpty && model.jobs.isEmpty && model.unit != nil)
    }

    @Test func aUnitWithoutTheCoreAPIsTimeOrReadingsIsShownAllTheSame() async throws {
        var unit = try technicianUnit()
        unit.latestMeasurements = []
        unit.connection = .offline
        let body = try JSONEncoder().encode(AnswerEnvelope(data: unit, meta: ["correlationId": "c"]))
        let noBuckets =
            #"{"data":{"items":[],"nextCursor":null,"total":0,"snapshotVersion":1,"allergenObservation":null,"buckets":null},"#
            + #""meta":{"correlationId":"c"}}"#
        let (model, _) = model([
            .http(200, try #require(String(bytes: body, encoding: .utf8))), try page([CoreAlert]()), try page([CoreAlert]()),
            try page([CoreAlert]()),
            try page([JobsListResultItem]()), .http(200, noBuckets), try page([Device]()),
        ])
        await model.load()
        #expect(abs(model.now.timeIntervalSinceNow) < 60, "the phone's time")
        #expect(model.lines.first?.unit == "°C", "temperature's own unit")
        #expect(model.lines.first?.points.isEmpty == true)
        #expect(model.stopped != nil)
    }

    @Test func aUnitWithoutReadingsStartsOnTemperatureAndOneWithoutItOnItsFirst() throws {
        var unit = try technicianUnit()
        #expect(TechUnitModel.firstMetric(unit) == .temperature)
        unit.latestMeasurements.removeLast()
        #expect(TechUnitModel.firstMetric(unit) == .humidity)
        unit.latestMeasurements = []
        #expect(TechUnitModel.firstMetric(unit) == .temperature)
    }
}
