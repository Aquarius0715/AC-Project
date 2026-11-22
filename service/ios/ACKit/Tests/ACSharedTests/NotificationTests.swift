import ACTestSupport
import Foundation
import Testing

@testable import ACKit
@testable import ACShared

/// The inbox as the web words it (notifications.ts, evidence.ts) and the notifications model.
@Suite struct NotificationTextTests {
    private let day = "2026-11-20T00:00:00Z"

    func notice(
        _ template: CoreNotificationTemplateKey,
        type: NotificationType = .completion,
        status: String = "completion",
        reason: String? = nil,
        message: String? = nil,
        amount: Double? = nil,
        currency: Currency? = nil,
        channel: Channel = .inApp,
        readAt: String? = nil
    ) -> CoreNotification {
        CoreNotification(
            id: "n1",
            tenantId: "t1",
            version: 3,
            createdAt: day,
            updatedAt: day,
            sourceAlertId: nil,
            type: type,
            recipientMembershipId: "m1",
            scopeVersionAtCreation: 1,
            target: Target(kind: .unit, id: "u1"),
            templateKey: template,
            params: CoreNotificationParams(
                targetName: "Bedroom AC",
                at: day,
                status: status,
                reason: reason,
                amountMinor: amount,
                currency: currency,
                method: nil,
                message: message
            ),
            channel: channel,
            deliveryState: .queued,
            severity: .normal,
            occurredAt: day,
            readAt: readAt,
            targetOpen: true
        )
    }

    @Test func everyTemplateHasItsTitle() {
        let titles = CoreNotificationTemplateKey.allCases.map { NotificationText.title(notice($0)) }
        #expect(Set(titles).count == CoreNotificationTemplateKey.allCases.count)
        #expect(titles.allSatisfy { $0.contains("Bedroom AC") })
        #expect(NotificationText.title(notice(.alert, type: .cleaningDue)) == "Filter cleaning due on Bedroom AC")
        #expect(NotificationText.title(notice(.completion)) == "Work completed — Bedroom AC")
    }

    @Test func detailSaysTheMessageAndTheStatusInWords() {
        #expect(NotificationText.detail(notice(.completion)).isEmpty, "a status filled in from the type is left out")
        #expect(NotificationText.detail(notice(.jobUpdate, type: .jobUpdate, status: "on_hold")) == "On hold")
        #expect(NotificationText.detail(notice(.jobUpdate, type: .jobUpdate, status: "something_new")) == "something_new")
        #expect(
            NotificationText.detail(notice(.jobUpdate, type: .jobUpdate, status: "cancelled", reason: "technician_access_ended"))
                == "Technician’s access ended — to be reassigned · Cancelled"
        )
        #expect(
            NotificationText.detail(notice(.jobUpdate, type: .jobUpdate, status: "jobUpdate", reason: "own words")).hasPrefix("own words")
        )
        #expect(
            NotificationText.detail(
                notice(.alert, type: .cleaningDue, status: "cleaning_due", message: "Cleaning due. Not a fault.", channel: .email)
            )
                == "Cleaning due. Not a fault. · email"
        )
        #expect(NotificationText.detail(notice(.completion, channel: .whatsapp)) == "WhatsApp")
    }

    @Test func remindersSayTheirDayOrAmount() {
        #expect(NotificationText.detail(notice(.certificateExpiry)).hasPrefix("Expires "))
        #expect(NotificationText.detail(notice(.warrantyExpiring)).hasSuffix(" · no maintenance contract"))
        #expect(NotificationText.detail(notice(.paymentReminder, amount: 211_000, currency: .myr)) == "Amount due 2,110.00 MYR")
        #expect(NotificationText.detail(notice(.paymentReminder)).isEmpty)
    }

    @Test func everyStatusAndReasonIsWorded() {
        let codes = [
            "open", "requested", "time_proposed", "offered", "accepted", "declined", "assigned", "in_progress", "submitted", "returned",
            "rework_requested", "completed", "cancelled", "on_hold", "scheduled", "applied", "release_requested", "released", "received",
            "answered", "confirmed", "failed", "escalated", "expired", "reschedule_requested", "time_change_proposed", "time_change_kept",
            "cant_make", "restriction_bypass",
        ]
        let words = codes.map(NotificationText.statusWords)
        #expect(Set(words).count == codes.count)
        #expect(zip(codes, words).allSatisfy { $0 != $1 })
        #expect(NotificationText.reasonWords("technician_qualification_ended") == "Technician’s qualification ended — to be reassigned")
        #expect(NotificationText.reasonWords("typed by a person") == "typed by a person")
    }

    @Test func firmwareNoticeIsDatedAgain() {
        let text = NotificationText.message(
            "Firmware update from 5 Dec 2026, 02:00–04:00 local time. The AC may restart for about 2 minutes."
        )
        #expect(text.hasPrefix("Firmware update from "))
        #expect(text.contains("2026") && text.contains("02:00–04:00"))
        let twoDigits = NotificationText.message(
            "Firmware update from 15 Dec 2026, 02:00–04:00 local time. The AC may restart for about 2 minutes."
        )
        #expect(twoDigits.contains("15"))
        #expect(
            NotificationText.message("Firmware update from 5 Xyz 2026, 02:00–04:00 local time. The AC may restart for about 2 minutes.")
                .contains("Xyz")
        )
    }
}

@Suite struct EvidenceTests {
    @Test func textsTheSourcesWriteAreWorded() {
        let fixed = [
            "Cleaning due. Not a fault.", "Device still reports an ended restriction; reconcile it from HQ", "Demo insulation",
            "Demo observation",
            "Demo: cooling load rose while the outdoor temperature was stable.",
            "Demo: cooling load rose; no cause evidence is available (fictional).", "Demo: high temperature observed (fictional)",
            "Demo: inspection recorded worn insulation around the indoor unit piping (fictional).",
            "Demo: synthetic allergen observation without unit (fictional).",
            "Demo: synthetic allergen observation; not a health assessment.",
            "Demo: temperature at or above 30°C for 60 seconds (fictional)",
        ]
        for text in fixed { #expect(Evidence.text(text) == text) }
        #expect(Evidence.text("Cleaning due (120 h of run time since the last cleaning). Not a fault.").contains("120 h"))
        #expect(Evidence.text("Cleaning due (80 h of run time, no cleaning recorded). Not a fault.").contains("80 h"))
        #expect(Evidence.text("Cleaning due (90 days since the last cleaning, run time unknown). Not a fault.").contains("90 days"))
        #expect(
            Evidence.text("Device SN-1 reported its cover opened while powered (tamper_signal)")
                == "Device SN-1 reported its cover opened while powered"
        )
        #expect(Evidence.text("a device's own words") == "a device's own words")
    }

    @Test func bypassAlertsNameTheRestrictionInWords() {
        let tail = " restriction: the restriction may no longer hold"
        #expect(Evidence.text("Device SN-1 stopped reporting while the unit is under a power_off" + tail).contains("(Power off)"))
        #expect(Evidence.text("Device SN-1 lost power while the unit is under a temperature_limit" + tail).contains("(Temperature limit)"))
        #expect(Evidence.text("Device SN-1 reported its cover opened while the unit is under a odd_kind" + tail).contains("(odd_kind)"))
    }

    @Test func demoEventsNameTheMetricAndComparison() {
        #expect(Evidence.text("Demo event: co2 gte threshold") == "Demo event: CO₂ at or above its alert threshold")
        #expect(Evidence.text("Demo event: humidity lt threshold") == "Demo event: Humidity below its alert threshold")
        #expect(Evidence.text("Demo event: mystery gt threshold") == "Demo event: mystery above its alert threshold")
        #expect(Evidence.text("Demo event: power lte threshold").contains("at or below"))
        #expect(Evidence.text("Demo event: power eq threshold").contains(" eq "))
        let metrics = [
            "temperature", "humidity", "co2", "pm25", "power", "vibration", "refrigerant_pressure", "compressor_cycles", "airflow_drop",
            "heartbeat_gap",
        ]
        let names = metrics.map { Evidence.metric(Substring($0)) }
        #expect(Set(names).count == metrics.count && zip(metrics, names).allSatisfy { $0 != $1 })
    }
}

@MainActor @Suite struct NotificationsModelTests {
    private struct Envelope<Value: Encodable>: Encodable {
        let data: Value
        let meta = ["correlationId": "c"]
    }

    private func answer(_ value: some Encodable) throws -> StubTransport.Answer {
        .http(200, try #require(String(bytes: try JSONEncoder().encode(Envelope(data: value)), encoding: .utf8)))
    }

    private func page(_ readFirst: Bool = false) throws -> StubTransport.Answer {
        let words = NotificationTextTests()
        var unread = words.notice(.completion)
        unread.id = "n1"
        var read = words.notice(.alert, type: .cleaningDue, status: "cleaning_due", readAt: "2026-11-20T01:00:00Z")
        read.id = "n2"
        if readFirst { unread.readAt = "2026-11-20T02:00:00Z" }
        return try answer(Page(items: [unread, read], nextCursor: nil, total: 2, snapshotVersion: 1))
    }

    private func refusal(_ code: String, _ status: Int) -> StubTransport.Answer {
        .http(status, #"{"code":"\#(code)","messageKey":"error.x","fieldErrors":{},"correlationId":"c","retryAfterSeconds":null}"#)
    }

    private func model(_ answers: [StubTransport.Answer], delay: Duration = .zero) -> (NotificationsModel, StubTransport) {
        let transport = StubTransport(answers, delay: delay)
        let base = URL(string: "http://core.test") ?? URL(filePath: "/")
        return (NotificationsModel(client: CoreClient(baseURL: base, transport: transport, credentials: FixedCredentials())), transport)
    }

    @Test func nothingBeforeTheListIsRead() async {
        let (model, transport) = model([])
        #expect(model.rows.isEmpty && model.unreadCount == 0)
        await model.markAllRead()
        #expect(await transport.requests.isEmpty)
    }

    @Test func listsNewestFirstAndFiltersTheUnread() async throws {
        let (model, transport) = model([try page()])
        await model.load()
        #expect(model.rows.map(\.id) == ["n1", "n2"])
        #expect(model.unreadCount == 1)
        model.filter = .unread
        #expect(model.rows.map(\.id) == ["n1"])
        #expect(model.rows.first?.title == "Work completed — Bedroom AC")
        let asked = try #require(await transport.requests.first?.url)
        #expect(query(asked.path() + "?" + (asked.query() ?? "")).values == ["limit": ["100"], "sort": ["occurredAt:desc"]])
    }

    @Test func theEmptyListSaysWhatTheFilterFoundAsOnTheWeb() async throws {
        let none = Page<CoreNotification>(items: [], nextCursor: nil, total: 0, snapshotVersion: 1)
        let (model, _) = model([try page(true), try answer(none)])
        #expect(model.emptyText == nil, "nothing is said while the list loads")
        await model.load()
        #expect(model.emptyText == nil, "the list has rows")
        model.filter = .unread
        #expect(model.rows.isEmpty)
        #expect(model.emptyText == "No unread notifications")
        await model.load()
        #expect(model.emptyText == "No unread notifications")
        model.filter = .all
        #expect(model.emptyText == "No notifications")
    }

    @Test func readingOneMarksItWithItsVersion() async throws {
        let marked = NotificationTextTests().notice(.completion, readAt: "2026-11-20T02:00:00Z")
        let (model, transport) = model([try page(), try answer(marked), try page(true)])
        await model.load()
        let row = try #require(model.rows.first)
        await model.markRead(row)
        let request = try #require(await transport.requests.dropFirst().first)
        #expect(request.url?.path() == "/v1/notifications/n1/mark-read")
        #expect(request.value(forHTTPHeaderField: "X-Expected-Version") == "3")
        #expect(model.unreadCount == 0)
        #expect(model.note == nil)
        await model.markRead(try #require(model.rows.first))
        #expect(await transport.requests.count == 3, "a read one is not marked again")
    }

    @Test(arguments: [
        ("CONFLICT", 409, "This notification changed — the latest state is shown."),
        ("NOT_FOUND", 404, "This notification is no longer available."),
        ("FORBIDDEN", 403, "You don't have permission for this action"),
    ])
    func refusedMarkSaysWhy(code: String, status: Int, words: String) async throws {
        let (model, _) = model([try page(), refusal(code, status), try page()])
        await model.load()
        await model.markRead(try #require(model.rows.first))
        #expect(model.note == words)
    }

    @Test func markAllReadIsOneWrite() async throws {
        let (model, transport) = model([try page(), try answer(NotificationsMarked(markedIds: ["n1"], skippedIds: [])), try page(true)])
        await model.load()
        await model.markAllRead()
        #expect(model.note == "1 notification marked as read")
        let request = try #require(await transport.requests.dropFirst().first)
        #expect(request.httpBody == Data(#"{"ids":["n1"]}"#.utf8))
        await model.markAllRead()
        #expect(await transport.requests.count == 3, "nothing unread, nothing sent")
    }

    @Test func markAllReadCountsAndFailures() async throws {
        let (many, _) = model([try page(), try answer(NotificationsMarked(markedIds: ["n1", "n3"], skippedIds: [])), try page(true)])
        await many.load()
        await many.markAllRead()
        #expect(many.note == "2 notifications marked as read")
        let (failed, _) = model([try page(), .failure(URLError(.timedOut)), try page()])
        await failed.load()
        await failed.markAllRead()
        #expect(failed.note == "The request took too long — check whether the change was saved, then try again")
    }

    @Test func aSecondTapWhileAMarkRunsSendsNothing() async throws {
        let marked = NotificationTextTests().notice(.completion, readAt: "2026-11-20T02:00:00Z")
        let (one, slow) = model([try page(), try answer(marked), try page(true)], delay: .milliseconds(200))
        await one.load()
        let row = try #require(one.rows.first)
        #expect(!one.isWriting)
        let first = Task { await one.markRead(row) }
        for _ in 0..<100 where !one.isWriting { await Task.yield() }
        #expect(one.isWriting, "the rows and Mark all as read are disabled while the mark runs")
        await one.markRead(row)
        await one.markAllRead()
        await first.value
        #expect(!one.isWriting)
        #expect(await slow.requests.count == 3, "one mark and its reload, nothing for the second tap")
        #expect(one.note == nil && one.unreadCount == 0)

        let markedAll = try answer(NotificationsMarked(markedIds: ["n1"], skippedIds: []))
        let (all, slowAll) = model([try page(), markedAll, try page(true)], delay: .milliseconds(200))
        await all.load()
        let firstAll = Task { await all.markAllRead() }
        for _ in 0..<100 where !all.isWriting { await Task.yield() }
        await all.markAllRead()
        await all.markRead(row)
        await firstAll.value
        #expect(await slowAll.requests.count == 3, "one write and its reload")
        #expect(all.note == "1 notification marked as read", "the second tap does not say 0 marked")
    }

    @Test func dayOfAnInstant() {
        let zone = TimeZone(identifier: "Asia/Kuala_Lumpur") ?? .gmt
        Localizer.with(.en) {
            let day = Format.date("2026-11-30T20:00:00Z", timeZone: zone)
            #expect(day.contains("Dec") && day.contains("2026") && !day.contains(":"))
        }
        #expect(Format.date("soon") == "soon")
    }
}
