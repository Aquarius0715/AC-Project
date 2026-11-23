import ACKit
import ACShared
import Foundation

/// What the phone answered when asked for its position for a check-in.
public enum PositionAnswer: Sendable, Equatable {
    case fix(CheckInPosition)
    /// The app may not know the position (the permission was refused).
    case refused
    /// The phone gave no fix.
    case unavailable
}

/// The phone's position for a check-in (DEC-109, IR825): Core Location in the app (CoreLocationPositionPlatform.swift),
/// a stand-in in the tests. The Core API measures the fix against the site's pin; nothing else is sent or kept.
public protocol PositionSource: Sendable {
    func locate() async -> PositionAnswer
}

/// A field of the check-in form, top to bottom: a refusal names one and the form shows its line there.
public enum CheckInField: String, Sendable, CaseIterable {
    case position, qr, reason
}

/// Check in at site (FR-T13, DD-T13): at the site with the phone's fix and the unit's scanned QR label, or manually with
/// a reason (a fix, when there is one, records the distance). Checking in starts the work.
public struct CheckInForm: Sendable, Equatable {
    public var method: JobsCheckInInputMethod = .locationQr
    public var position: CheckInPosition?
    public var qr = ""
    public var reason = ""

    public init() {
        // an empty form
    }

    /// The coarsest fix a location check-in takes, in metres (the Core API's limit).
    static let maxAccuracy = 100.0
    /// The longest reason the Core API takes.
    static let maxReason = 1000

    /// A reason's length as the Core API counts it: the trimmed text's code points (Go's utf8.RuneCountInString), not
    /// its characters, so a letter with a combining mark counts as two.
    static func length(of reason: String) -> Int {
        reason.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count
    }

    /// The form's own checks (techJob.ts checkInErrors): a location check-in needs a fix good enough to tell 200 m and
    /// the scanned label; a manual one a reason of 1–1000 characters. How far the fix is from the site is the server's to
    /// say.
    public var errors: [CheckInField: String] {
        if method == .manual {
            let count = Self.length(of: reason)
            guard count < 1 || count > Self.maxReason else { return [:] }
            return [.reason: tr("A reason is required for a manual check-in (1–1000 characters)")]
        }
        var errors: [CheckInField: String] = [:]
        if let position {
            if position.accuracyMeters > Self.maxAccuracy {
                errors[.position] = tr(
                    "Your location is only known to ±{n} m — move into the open and try again, or check in manually with a reason",
                    ["n": String(Int(position.accuracyMeters.rounded()))]
                )
            }
        } else {
            errors[.position] = tr("Get your location first — or check in manually with a reason")
        }
        if qr.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { errors[.qr] = tr("Scan the unit’s QR label") }
        return errors
    }

    /// The line under the location button: the fix's accuracy, or where the server expects the phone.
    public var positionHint: String {
        guard let position else { return tr("Within 200 m of the site, as HQ set it on the map") }
        return tr(
            "Found, accurate to ±{n} m — the server checks you are within 200 m of the site",
            ["n": String(Int(position.accuracyMeters.rounded()))]
        )
    }

    /// The fields of a Core API refusal (the check-in's position, qrUnitId and reason) under the form's fields.
    static func fields(of error: DomainError) -> [CheckInField: String] {
        let names: [String: CheckInField] = ["position": .position, "qrUnitId": .qr, "reason": .reason]
        var fields: [CheckInField: String] = [:]
        for (name, key) in error.fieldErrors {
            guard let field = names[name] else { continue }
            var one = error
            one.fieldErrors = [name: key]
            fields[field] = ErrorText.of(one)
        }
        return fields
    }
}

/// Can't make this time (DD-T13 step 0): the reason and, optionally, a time the technician could do instead.
public struct CantMakeForm: Sendable, Equatable {
    public var reason = ""
    /// The day of the other time, if one is offered.
    public var date: Date?
    public var window = Self.windows[1]

    public init() {
        // an empty form
    }

    /// The two-hour windows a technician can offer, as on the web.
    public static let windows = ["08:00-10:00", "10:00-12:00", "12:00-14:00", "14:00-16:00", "16:00-18:00"]

    public var error: String? {
        let count = CheckInForm.length(of: reason)
        return count < 1 || count > CheckInForm.maxReason ? tr("Tell your coordinator why (1–1000 characters)") : nil
    }

    /// The other time in the display time zone (NFR-08), or nil when none is offered. The day is the one the date
    /// picker showed, in the phone's time zone (`pickedIn`), which can differ from the membership's: the web's date
    /// field is a day too, and its window is in the display time zone.
    public func slot(in timeZone: TimeZone = Localizer.timeZone, pickedIn picker: TimeZone = .current) -> Slot? {
        guard let date else { return nil }
        var pickerCalendar = Calendar(identifier: .gregorian)
        pickerCalendar.timeZone = picker
        let day = pickerCalendar.dateComponents([.year, .month, .day], from: date)
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let instants = window.split(separator: "-").compactMap { time -> String? in
            let parts = time.split(separator: ":").compactMap { Int($0) }
            guard parts.count == 2 else { return nil }
            var at = day
            at.hour = parts[0]
            at.minute = parts[1]
            return calendar.date(from: at)?.ISO8601Format()
        }
        guard instants.count == 2 else { return nil }
        return Slot(startAt: instants[0], endAt: instants[1])
    }
}
