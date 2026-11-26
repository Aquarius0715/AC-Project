import ACShared
import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

private let rule = AlertRule(
    name: "High room temperature",
    metric: .temperature,
    operator: .gte,
    threshold: 30,
    recoveryThreshold: 28,
    durationSeconds: 60
)

private func candidate(
    _ kind: EvidenceCandidateKind,
    origin: EvidenceCandidateOrigin? = .measured,
    event: EvidenceCandidateEventType? = nil
) -> EvidenceCandidate {
    EvidenceCandidate(
        id: "c-\(kind.rawValue)-\(origin?.rawValue ?? "none")",
        kind: kind,
        observedAt: "2026-09-27T04:50:00Z",
        metric: kind == .remeasurement ? .temperature : nil,
        value: kind == .remeasurement ? 24.5 : nil,
        unit: kind == .remeasurement ? .c : nil,
        origin: origin,
        quality: .valid,
        eventType: event
    )
}

@Suite struct AlertWordsTests {
    private let now = Format.parse("2026-09-27T05:00:00Z") ?? Date()

    @Test func durationsInTheirLargestWholeUnit() {
        #expect([60, 120, 150, 7200, 5400].map { AlertWords.duration(Double($0)) } == ["60 s", "2 min", "150 s", "2 h", "90 min"])
        #expect(Compare.allCases.map(AlertWords.symbol).sorted() == ["<", ">", "≤", "≥"])
    }

    @Test func aRuleInWords() {
        let words = AlertWords.rule(rule)
        #expect(words.policy == "Policy: Temperature ≥ 30.0 °C, recovery < 28.0 °C, duration 60 s")
        #expect(words.recovery == "< 28.0 °C for 60 s")
        #expect(words.raise == "Temperature ≥ 30.0 °C")
        var low = rule
        low.operator = .lt
        #expect(AlertWords.rule(low).recovery.hasPrefix("> "), "a low limit recovers above its threshold")
    }

    @Test func theAlertTheScreenOpens() {
        let resolved = alert("old", status: .resolved, detected: "2026-09-27T04:59:00Z")
        let warning = alert("w")
        let critical = alert("c", severity: .critical, detected: "2026-09-27T03:00:00Z")
        #expect(AlertWords.select([resolved, warning, critical], "w")?.id == "w")
        #expect(AlertWords.select([resolved, warning, critical], nil)?.id == "c", "the worst unresolved")
        #expect(AlertWords.select([warning, alert("w2", detected: "2026-09-27T04:30:00Z")], nil)?.id == "w2", "then the newest")
        #expect(AlertWords.select([alert("r1", status: .resolved), resolved], nil)?.id == "old", "else the newest")
        #expect(AlertWords.select([], nil) == nil)
        let choices = AlertWords.choices([resolved, warning, critical], now: now)
        #expect(choices.map(\.id) == ["w", "c", "old"])
        #expect(choices[2].sub.hasPrefix("Resolved · "))
    }

    @Test func anOpenAlertWithAPolicyAndItsLatestReading() throws {
        var open = alert("a1")
        open.rule = rule
        open.policyId = "p1"
        open.previousAlertId = "a0"
        var unit = try technicianUnit()
        unit.latestMeasurements[2].origin = .estimated
        let context = AlertWords.Context(
            alerts: [open, alert("a0", status: .resolved)],
            unit: unit,
            job: (id: "f0000000-1", type: .reactive),
            canResolve: true
        )
        let view = AlertWords.view(open, context, now: now)
        #expect(view.title == "a1 · High room temperature")
        #expect(view.subtitle.hasPrefix("Policy: Temperature ≥ 30.0 °C") && view.state == nil)
        #expect(view.evidence.count == 2 && view.evidence[1].1.hasSuffix("confidence unknown — no numeric probability shown"))
        #expect(view.resolution?.hasPrefix("Automatic resolution needs a remeasurement < 28.0 °C for 60 s") == true && view.done == nil)
        #expect(view.history.map(\.id) == ["raised"] && view.history[0].sub.hasSuffix("Temperature ≥ 30.0 °C for 60 s"))
        #expect(
            view.related.map(\.1) == [
                "f0000000 · Repair", "Living room AC · Home A › 1F › Living room", "High room temperature", "< 28.0 °C for 60 s",
                view.related[4].1, "with a reason, or by a remeasurement",
            ]
        )
        #expect(view.related[4].1.hasSuffix(" · Resolved"))
        #expect(view.forbidden.contains("once a remeasurement stays < 28.0 °C for 60 s"))
        #expect(view == AlertWords.view(open, context, now: now))
    }

    @Test func aResolvedAlertWithoutAPolicy() {
        var done = alert("a1", status: .resolved)
        done.acknowledgedAt = "2026-09-27T04:10:00Z"
        done.resolvedAt = "2026-09-27T04:20:00Z"
        done.resolutionReason = "filter cleaned"
        done.previousAlertId = "gone-alert-id"
        let context = AlertWords.Context(alerts: [done], unit: nil, job: nil, canResolve: false)
        let view = AlertWords.view(done, context, now: now)
        #expect(view.subtitle == "Observed (demo) · no policy — it resolves only with a reason")
        #expect(view.state?.hasPrefix("Resolved · ") == true && view.resolution == nil)
        #expect(
            view.done?.hasPrefix("Resolved ") == true && view.done?.contains("filter cleaned") == true
                && view.done?.contains("Acknowledged ") == true
        )
        #expect(view.history.map(\.id) == ["resolved", "acknowledged", "raised"])
        #expect(view.related.map(\.1) == ["—", "u1", "No policy", "—", "gone-ale", view.related[5].1])
        #expect(view.related[5].1.hasPrefix("done "))
        #expect(view.note.contains("links back to alert a1"))
        #expect(view.forbidden == "Not resolved — you don’t have “Resolve alerts”. Ask a permitted user to resolve it.")
        var unexplained = done
        unexplained.resolutionReason = nil
        unexplained.acknowledgedAt = nil
        #expect(AlertWords.view(unexplained, context, now: now).done?.contains("no reason recorded") == true)
    }

    @Test func whoCanResolveWhat() {
        var policy = alert("a1", status: .acknowledged)
        policy.acknowledgedAt = "2026-09-27T04:10:00Z"
        policy.rule = rule
        let without = AlertWords.Context(alerts: [], unit: nil, job: nil, canResolve: false)
        #expect(AlertWords.view(policy, without, now: now).related[5].1 == "with a remeasurement only")
        #expect(AlertWords.view(policy, without, now: now).state?.hasPrefix("Acknowledged · ") == true)
        #expect(AlertWords.view(alert("a2"), without, now: now).related[5].1 == "no — ask a user with “Resolve alerts”")
        let with = AlertWords.Context(alerts: [], unit: nil, job: nil, canResolve: true)
        #expect(AlertWords.view(alert("a2"), with, now: now).related[5].1 == "with a reason")
        #expect(AlertWords.view(alert("a2"), with, now: now).related[4].1 == "none")
        #expect(AlertWords.view(alert("a2"), with, now: now).resolution?.hasPrefix("No remeasurement resolves this alert") == true)
        #expect(AlertWords.state(.open) == "Open")
        #expect([AlertWords.kind(.inferred), AlertWords.kind(.inspection)] == ["Suspected", "Inspection record"])
    }

    @Test func aLatestReadingWithoutAValue() throws {
        var open = alert("a1")
        open.rule = rule
        var unit = try technicianUnit()
        unit.latestMeasurements[2].value = nil
        unit.latestMeasurements[2].quality = .valid
        let view = AlertWords.view(open, AlertWords.Context(alerts: [], unit: unit, job: nil, canResolve: true), now: now)
        #expect(view.evidence[1].1.hasPrefix("— · observed ") && view.evidence[1].1.hasSuffix("good"))
    }

    @Test func theEvidenceCandidatesInWords() {
        let rows = AlertWords.evidence(
            [
                candidate(.remeasurement), candidate(.remeasurement, origin: .inspection), candidate(.remeasurement, origin: .estimated),
                candidate(.remeasurement, origin: nil), candidate(.deviceEvent, event: .restored), candidate(.deviceEvent, event: .tamper),
                candidate(.detection),
            ],
            now: now
        )
        #expect(rows[0].title.hasPrefix("Remeasurement · Temperature 24.5 °C · ") && rows[0].sub == "measured · valid")
        #expect(rows.map(\.sub).dropFirst().prefix(3) == ["recorded on site · valid", "estimated · valid", " · valid"])
        #expect(rows[4].sub == "The device's signal is back" && rows[5].sub == "tamper")
        #expect(rows[6].title == "Evidence attached to this alert")
        var bare = candidate(.remeasurement)
        bare.value = nil
        #expect(AlertWords.evidence([bare], now: now)[0].title.contains(" — · "))
        var unnamed = candidate(.deviceEvent)
        unnamed.eventType = nil
        #expect(AlertWords.evidence([unnamed], now: now)[0].sub.isEmpty)
        var noMetric = candidate(.remeasurement)
        noMetric.metric = nil
        #expect(AlertWords.evidence([noMetric], now: now)[0].title.hasPrefix("Remeasurement ·  — · "))
    }

    @Test func anAlertWithoutTheTimeOfItsState() {
        let context = AlertWords.Context(alerts: [], unit: nil, job: nil, canResolve: true)
        let resolved = AlertWords.view(alert("a1", status: .resolved), context, now: now)
        #expect(resolved.state?.hasPrefix("Resolved · ") == true && resolved.done?.hasPrefix("Resolved ") == true)
        #expect(resolved.related[5].1.hasPrefix("done "), "the detection's time stands in")
        let acknowledged = AlertWords.view(alert("a2", status: .acknowledged), context, now: now)
        #expect(acknowledged.state?.hasPrefix("Acknowledged · ") == true)
        var unitless = candidate(.remeasurement)
        unitless.unit = nil
        #expect(AlertWords.evidence([unitless], now: now)[0].title.hasPrefix("Remeasurement · Temperature 24.5 · "))
    }

    @Test func theResolutionsChecks() {
        #expect(AlertWords.problem(reason: " ", picked: [], policyless: false) == "A resolution reason is required (1–1000 characters).")
        #expect(
            AlertWords.problem(reason: "Fixed", picked: [], policyless: true)
                == "Pick at least one evidence record — this alert has no policy"
        )
        #expect(
            AlertWords.problem(reason: "Fixed", picked: (0...20).map(String.init), policyless: false) == "Pick at most 20 evidence records"
        )
        #expect(AlertWords.problem(reason: "Fixed", picked: [], policyless: false) == nil)
        // the Core API counts code points: 1000 is the most, and a thumbs-up with a skin tone is two of them
        #expect(AlertWords.problem(reason: String(repeating: "a", count: 1000), picked: [], policyless: false) == nil)
        #expect(
            AlertWords.problem(reason: String(repeating: "a", count: 1001), picked: [], policyless: false)
                == "A resolution reason is required (1–1000 characters)."
        )
        #expect(
            AlertWords.problem(reason: String(repeating: "👍🏽", count: 501), picked: [], policyless: false)
                == "A resolution reason is required (1–1000 characters).",
            "501 characters on screen, 1002 code points"
        )
    }

    @Test func eachRefusalInWords() {
        let refused = { (code: ErrorCode, key: String, fields: [String: String]) in
            AlertWords.refusal(
                DomainError(code: code, messageKey: key, fieldErrors: fields, correlationId: "c", retryAfterSeconds: nil),
                forbidden: "F"
            )
        }
        #expect(refused(.forbidden, "error.forbidden", [:]) == "F")
        #expect(refused(.forbidden, "errors.assignment_required", [:]).hasPrefix("You need an active assignment"))
        #expect(refused(.forbidden, "errors.assignment_not_started", [:]).hasSuffix("has not started yet."))
        #expect(refused(.forbidden, "errors.assignment_ended", [:]).hasSuffix("has ended."))
        #expect(refused(.conflict, "error.versionConflict", [:]).hasPrefix("The alert changed elsewhere"))
        #expect(refused(.conflict, "error.invalidState", [:]).hasPrefix("The alert is no longer in that state"))
        #expect(refused(.notFound, "error.notFound", [:]) == "This alert is not in your assignments.")
        #expect(
            refused(.validation, "error.validation", ["resolutionEvidenceIds": "errors.evidence_required"]).hasPrefix("Pick at least one")
        )
        #expect(
            refused(.validation, "error.validation", ["resolutionEvidenceIds": "errors.evidence_unknown"]).hasPrefix(
                "An evidence record is no longer"
            )
        )
        #expect(
            refused(.validation, "error.validation", ["resolutionEvidenceIds": "error.invalid"])
                == "Pick each evidence record once, at most 20"
        )
        #expect(
            refused(.validation, "error.validation", ["resolutionReason": "error.required"])
                == "A resolution reason is required (1–1000 characters)."
        )
        #expect(refused(.unavailable, "error.unavailable", [:]) == "Not sent; your reason is kept. Try again.")
        #expect(refused(.timeout, "error.timeout", [:]) == "Not sent; your reason is kept. Try again.")
        #expect(refused(.rateLimited, "error.rateLimited", [:]) == "Too many changes in a short time — try again shortly")
    }
}

@MainActor @Suite struct AlertsModelTests {
    private func model(_ answers: [StubTransport.Answer], jobId: String? = "j1", alertId: String? = nil) -> (AlertsModel, StubTransport) {
        let transport = StubTransport(answers)
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        return (AlertsModel(unitId: "u1", jobId: jobId, alertId: alertId, client: client), transport)
    }

    private func answer(_ result: some Encodable, at: String? = nil) throws -> StubTransport.Answer {
        var meta = ["correlationId": "c"]
        meta["snapshotAt"] = at
        let body = try JSONEncoder().encode(AnswerEnvelope(data: result, meta: meta))
        return .http(200, try #require(String(bytes: body, encoding: .utf8)))
    }

    private func page<Item: Codable & Sendable & Equatable>(_ items: [Item]) throws -> StubTransport.Answer {
        try answer(Page(items: items, nextCursor: nil, total: Double(items.count), snapshotVersion: 1))
    }

    private func text(_ name: String) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try fixture(name), encoding: .utf8)))
    }

    private func session(canResolve: Bool) throws -> StubTransport.Answer {
        let session = Session(
            tenantId: "t1",
            membershipId: "m1",
            scopeVersion: 1,
            userId: "u",
            role: .technician,
            clientRole: nil,
            displayName: "Tech",
            organizationName: "HQ",
            organizationId: "o1",
            customerId: nil,
            permissions: canResolve ? [.alertResolve] : [],
            generation: 1,
            viewEpoch: 1,
            issuedAt: "2026-09-27T00:00:00Z",
            expiresAt: "2026-09-28T00:00:00Z"
        )
        return try answer(session, at: "2026-09-27T05:00:00Z")
    }

    private func refusal(_ status: Int, _ code: String, _ key: String, fields: String = "{}") -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"\#(key)","fieldErrors":\#(fields),"correlationId":"c","retryAfterSeconds":null}"#)
    }

    /// The unit's alert a1 open (its three reads), the unit, the job, the session and the alert's evidence.
    private func reads(_ alerts: [CoreAlert] = [alert("a1")], canResolve: Bool = true) throws -> [StubTransport.Answer] {
        let open = alerts.filter { $0.status == .open }
        let acknowledged = alerts.filter { $0.status == .acknowledged }
        let resolved = alerts.filter { $0.status == .resolved }
        var answers = [
            try page(open), try page(acknowledged), try page(resolved), try text("units.get.technician"), try text("jobs.get.assigned"),
            try session(canResolve: canResolve),
        ]
        let selected = AlertWords.select(alerts, nil)
        if canResolve, selected?.status != .resolved, selected != nil { answers.append(try page([candidate(.remeasurement)])) }
        return answers
    }

    @Test func theAlertsAreReadWithTheirEvidence() async throws {
        let (model, transport) = model(try reads())
        await model.load()
        #expect(model.alerts.map(\.id) == ["a1"] && model.selected?.id == "a1")
        #expect(model.unit != nil && model.job != nil && model.canResolve)
        #expect(model.candidates?.count == 1 && model.evidenceRows.count == 1)
        #expect(model.view?.related.first?.1 == "f0000000 · Repair")
        #expect(model.choices.count == 1 && model.now == Format.parse("2026-09-27T05:00:00Z"))
        let requests = await transport.requests.compactMap(\.url)
        #expect(requests.last?.path() == "/v1/alerts/a1/evidence")
    }

    @Test func withoutResolveAlertsThereIsNothingToCite() async throws {
        let (model, transport) = model(try reads(canResolve: false))
        await model.load()
        #expect(!model.canResolve && model.candidates == nil)
        #expect(model.evidenceRows.isEmpty)
        #expect(await transport.requests.count == 6)
    }

    @Test func anUnreadableSessionOrJobLeavesThemOut() async throws {
        let (model, _) = model(
            [try page([alert("a1")]), try page([CoreAlert]()), try page([CoreAlert]()), .failure(URLError(.timedOut))],
            jobId: nil
        )
        await model.load()
        #expect(model.unit == nil && model.job == nil && !model.canResolve)
    }

    @Test func alertsThatCannotBeReadSayWhy() async throws {
        let cases: [(StubTransport.Answer, String)] = [
            (refusal(403, "FORBIDDEN", "errors.assignment_ended"), "Your work window on this unit has ended."),
            (refusal(403, "FORBIDDEN", "errors.assignment_required"), "You need an active assignment on this unit to see its alerts."),
            (refusal(404, "NOT_FOUND", "error.notFound"), "This AC is not in your assignments."),
            (refusal(409, "CONFLICT", "error.other"), "CONFLICT"),
        ]
        for (answer, text) in cases {
            let (model, _) = model([answer])
            await model.load()
            #expect(model.content.value == .unavailable(title: "Alerts aren’t available", text: text))
        }
        let (down, _) = model([refusal(503, "UNAVAILABLE", "error.unavailable")])
        await down.load()
        #expect(down.content.message != nil, "a server failure is the screen's refusal")
        #expect(down.alerts.isEmpty && down.view == nil)
    }

    @Test func beforeTheWindowItSaysWhenItOpens() async throws {
        let notStarted = refusal(403, "FORBIDDEN", "errors.assignment_not_started")
        var (model, _) = model([notStarted, try text("jobs.get.assigned")])
        await model.load()
        guard case let .unavailable(title, text)? = model.content.value else {
            Issue.record("unavailable")
            return
        }
        #expect(title == "Not started yet" && text.hasPrefix("You can see this AC from "))
        (model, _) = self.model([notStarted], jobId: nil)
        await model.load()
        #expect(
            model.content.value == .unavailable(title: "Not started yet", text: "Available from the work start time of your assigned job.")
        )
    }

    @Test func anotherAlertIsSelectedAndItsFormStartsEmpty() async throws {
        let second = alert("a2", status: .resolved)
        let (model, _) = model(try reads([alert("a1"), second]))
        await model.load()
        model.reason = "typed"
        model.pick("x", true)
        await model.select("a2")
        #expect(model.selected?.id == "a2" && model.reason.isEmpty && model.picked.isEmpty && model.candidates == nil)
    }

    @Test func anotherAlertChosenWhileTheFirstOnesEvidenceIsReadShowsOnlyItsOwn() async throws {
        let gate = Gate(after: 7)
        let transport = StubTransport(
            try reads([alert("a1"), alert("a2"), alert("a3")])
                + [try page([candidate(.detection)]), try page([candidate(.deviceEvent, event: .restored)])],
            gate: gate
        )
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        let model = AlertsModel(unitId: "u1", jobId: "j1", alertId: nil, client: client)
        await model.load()
        let first = Task { await model.select("a2") }
        await gate.until()
        #expect(model.candidates == nil, "the earlier alert's evidence is not offered for this one")
        // a3 overtakes a2: its evidence arrives first
        await gate.allow(1)
        await model.select("a3")
        #expect(model.candidates?.map(\.id) == ["c-detection-measured"])
        await gate.open()
        await first.value
        #expect(model.selected?.id == "a3")
        #expect(model.candidates?.map(\.id) == ["c-detection-measured"], "a2's late answer is dropped")
    }

    @Test func anAlertIsAcknowledgedWithItsVersion() async throws {
        var acknowledged = alert("a1", status: .acknowledged)
        acknowledged.acknowledgedAt = "2026-09-27T04:59:00Z"
        let (model, transport) = model(try reads() + [try answer(acknowledged)] + (try reads([acknowledged])))
        await model.load()
        await model.acknowledge()
        #expect(model.note == "Acknowledged" && model.refusal == nil)
        #expect(model.selected?.status == .acknowledged)
        let write = await transport.requests[7]
        #expect(write.url?.path() == "/v1/alerts/a1/acknowledge" && write.value(forHTTPHeaderField: "X-Expected-Version") == "1")
    }

    @Test func anotherAlertCannotBeChosenWhileAStepIsSent() async throws {
        var acknowledged = alert("a1", status: .acknowledged)
        acknowledged.acknowledgedAt = "2026-09-27T04:59:00Z"
        let other = alert("a2", status: .resolved)
        let gate = Gate(after: 7)
        let transport = StubTransport(
            try reads([alert("a1"), other]) + [try answer(acknowledged)] + (try reads([acknowledged, other])),
            gate: gate
        )
        let client = CoreClient(
            baseURL: URL(string: "http://core.test") ?? URL(filePath: "/"),
            transport: transport,
            credentials: FixedCredentials()
        )
        let model = AlertsModel(unitId: "u1", jobId: "j1", alertId: nil, client: client)
        await model.load()
        let sent = Task { await model.acknowledge() }
        await gate.until()
        #expect(model.isSending)
        await model.select("a2")
        #expect(model.selected?.id == "a1", "the alert being sent stays shown")
        await gate.open()
        await sent.value
        // the note belongs to the alert it was sent for
        #expect(model.selected?.id == "a1" && model.selected?.status == .acknowledged && model.note == "Acknowledged")
        await model.select("a2")
        #expect(model.selected?.id == "a2" && model.note == nil, "once sent, another alert is chosen as before")
    }

    @Test func aResolutionIsCheckedThenSentWithItsEvidence() async throws {
        var resolved = alert("a1", status: .resolved)
        resolved.resolvedAt = "2026-09-27T04:59:00Z"
        let (model, transport) = model(try reads() + [try answer(resolved)] + (try reads([resolved])))
        await model.load()
        #expect(model.problem == nil, "not tried yet")
        await model.resolve()
        #expect(model.problem == "A resolution reason is required (1–1000 characters).")
        model.reason = " Cooled down "
        await model.resolve()
        #expect(model.problem == "Pick at least one evidence record — this alert has no policy")
        model.pick("c1", true)
        model.pick("c1", true)
        model.pick("c2", true)
        model.pick("c2", false)
        #expect(model.picked == ["c1"])
        await model.resolve()
        #expect(model.note == "Resolved" && model.reason.isEmpty && model.picked.isEmpty && model.problem == nil)
        let write = try #require(await transport.requests[7].httpBody)
        let body = try JSONDecoder().decode(StubResolution.self, from: write)
        #expect(body.resolutionReason == "Cooled down" && body.resolutionEvidenceIds == ["c1"])
    }

    @Test func aRefusedStepSaysWhyAndKeepsTheReason() async throws {
        let (model, _) = model(try reads() + [refusal(403, "FORBIDDEN", "error.forbidden")] + (try reads()))
        await model.load()
        model.reason = "Cooled down"
        model.pick("c1", true)
        await model.resolve()
        #expect(model.refusal?.hasPrefix("Not resolved — you don’t have “Resolve alerts”.") == true)
        #expect(model.reason == "Cooled down" && model.picked == ["c1"])
    }

    @Test func aStepWithoutAnAlertSendsNothing() async throws {
        let (model, transport) = model([])
        await model.acknowledge()
        #expect(await transport.requests.isEmpty)
    }
}

private struct StubResolution: Decodable {
    let resolutionReason: String
    let resolutionEvidenceIds: [String]
}
