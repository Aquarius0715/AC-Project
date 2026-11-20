import Foundation
import Testing

@testable import ACShared

@Suite struct ClockAndSpanTests {
    private let malaysia = TimeZone(identifier: "Asia/Kuala_Lumpur") ?? .gmt

    @Test func aTimeOfDayInTheDisplayTimeZone() {
        Localizer.with(.en) {
            // the space before AM is the locale's (CLDR writes a narrow one), so only the parts are checked
            let clock = Format.clock("2026-12-01T01:05:00Z", timeZone: malaysia)
            #expect(clock.hasPrefix("9:05") && clock.hasSuffix("AM") && !clock.contains("Dec"))
            #expect(Format.clock("later") == "later", "a text that is no instant is shown as it is")
        }
    }

    @Test func aSpanNamesItsDayOnceWhenBothEndsAreOnIt() {
        Localizer.with(.en) {
            let span = Format.span("2026-12-01T01:05:00Z", "2026-12-01T04:35:00Z", timeZone: malaysia)
            #expect(span.contains("9:05") && span.contains("12:35"))
            #expect(span.components(separatedBy: "2026").count == 2, "the day is written once: \(span)")
            // backwards or unreadable: both ends as they are
            #expect(Format.span("2026-12-01T04:35:00Z", "2026-12-01T01:05:00Z").contains(" – "))
            #expect(Format.span("soon", "later") == "soon – later")
        }
    }
}
