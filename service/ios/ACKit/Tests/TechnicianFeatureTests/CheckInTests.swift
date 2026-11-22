import ACShared
import Foundation
import Testing

@testable import ACKit
@testable import TechnicianFeature

@Suite struct CheckInFormTests {
    private let fix = CheckInPosition(latitude: 3.139, longitude: 101.6869, accuracyMeters: 12.4)

    @Test func aLocationCheckInNeedsAFixAndTheLabel() {
        var form = CheckInForm()
        #expect(
            form.errors == [
                .position: "Get your location first — or check in manually with a reason", .qr: "Scan the unit’s QR label",
            ]
        )
        #expect(form.positionHint == "Within 200 m of the site, as HQ set it on the map")
        form.position = CheckInPosition(latitude: 3.139, longitude: 101.6869, accuracyMeters: 140.6)
        form.qr = "  "
        #expect(
            form.errors[.position]
                == "Your location is only known to ±141 m — move into the open and try again, or check in manually with a reason"
        )
        #expect(form.errors[.qr] != nil, "blanks are no label")
        form.position = fix
        form.qr = "ac-unit:u1"
        #expect(form.errors.isEmpty)
        #expect(form.positionHint == "Found, accurate to ±12 m — the server checks you are within 200 m of the site")
    }

    @Test func aManualCheckInNeedsAReasonOfAtMost1000Characters() {
        var form = CheckInForm()
        form.method = .manual
        let missing = [CheckInField.reason: "A reason is required for a manual check-in (1–1000 characters)"]
        form.reason = " \n"
        #expect(form.errors == missing)
        form.reason = String(repeating: "x", count: 1001)
        #expect(form.errors == missing)
        form.reason = "No GPS signal in the basement plant room"
        #expect(form.errors.isEmpty, "a manual check-in needs neither a fix nor a label")
    }

    @Test func aRefusalOfAFieldIsShownUnderIt() {
        let refusal = DomainError(
            code: .validation,
            messageKey: "error.validation",
            fieldErrors: ["position": "errors.too_far", "qrUnitId": "errors.qr_unit_mismatch", "jobId": "error.required"],
            correlationId: "c",
            retryAfterSeconds: nil
        )
        let fields = CheckInForm.fields(of: refusal)
        #expect(fields.count == 2, "a field the form does not have is not shown under one")
        #expect(
            fields[.position] == "You are more than 200 m from the site — move closer and try again, or check in manually with a reason"
        )
        #expect(
            fields[.qr]
                == "The scanned QR label is not this job's unit — scan the label on this AC, or check in manually with a reason"
        )
    }
}

@Suite struct CantMakeFormTests {
    @Test func theCoordinatorIsToldWhy() {
        var form = CantMakeForm()
        #expect(form.error == "Tell your coordinator why (1–1000 characters)")
        form.reason = String(repeating: "x", count: 1001)
        #expect(form.error != nil)
        form.reason = "Medical appointment"
        #expect(form.error == nil)
    }

    @Test func anotherTimeIsTheWindowOfItsDayInTheDisplayTimeZone() throws {
        var form = CantMakeForm()
        #expect(form.slot() == nil, "no other time offered")
        let malaysia = try #require(TimeZone(identifier: "Asia/Kuala_Lumpur"))
        form.date = try #require(Format.parse("2026-12-02T20:00:00Z"))  // 3 Dec, 04:00 in Kuala Lumpur
        form.window = "14:00-16:00"
        #expect(
            form.slot(in: malaysia, pickedIn: malaysia) == Slot(startAt: "2026-12-03T06:00:00Z", endAt: "2026-12-03T08:00:00Z")
        )
        form.window = "14:00"
        #expect(form.slot(in: malaysia, pickedIn: malaysia) == nil, "a window that is no span")
        form.window = "14-16"
        #expect(form.slot(in: malaysia, pickedIn: malaysia) == nil, "a time that is no hour and minute")
        #expect(CantMakeForm.windows.contains(CantMakeForm().window))
    }

    @Test func theDayIsTheOneThePickerShowed() throws {
        // the phone is in Tokyo, the membership shows Kuala Lumpur: 00:30 on 3 Dec in Tokyo is still 2 Dec there
        let malaysia = try #require(TimeZone(identifier: "Asia/Kuala_Lumpur"))
        let tokyo = try #require(TimeZone(identifier: "Asia/Tokyo"))
        var form = CantMakeForm()
        form.date = try #require(Format.parse("2026-12-02T15:30:00Z"))
        form.window = "14:00-16:00"
        #expect(
            form.slot(in: malaysia, pickedIn: tokyo) == Slot(startAt: "2026-12-03T06:00:00Z", endAt: "2026-12-03T08:00:00Z"),
            "the day the picker showed (3 Dec), its window in the display time zone"
        )
    }
}
