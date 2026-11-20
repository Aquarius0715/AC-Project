import ACKit
import Foundation

/// A notification in words, as the web apps' inbox words it (shared/lib/notifications.ts, FR-X07, IR258, IR481): its
/// title by its template, and its detail line from its message, reason and status in the display language; a text no
/// source writes stays as recorded.
public enum NotificationText {
    public static func title(_ notification: CoreNotification) -> String {
        let name = ["name": notification.params.targetName]
        if notification.type == .cleaningDue { return tr("Filter cleaning due on {name}", name) }
        switch notification.templateKey {
        case .alert: return tr("Alert on {name}", name)
        case .quality: return tr("Air quality alert on {name}", name)
        case .scheduleChange: return tr("Schedule changed — {name}", name)
        case .reportReturn: return tr("Work report returned — {name}", name)
        case .completion: return tr("Work completed — {name}", name)
        case .payment: return tr("Payment update — {name}", name)
        case .paymentReminder: return tr("Payment reminder — {name}", name)
        case .restriction: return tr("Service restriction — {name}", name)
        case .inquiry: return tr("Inquiry update — {name}", name)
        case .jobUpdate: return tr("Job update — {name}", name)
        case .deviceOperation: return tr("Device operation — {name}", name)
        case .invite: return tr("Invitation — {name}", name)
        case .certificateExpiry: return tr("Certificate expiring — {name}", name)
        case .warrantyExpiring: return tr("Warranty ending — {name}", name)
        }
    }

    /// A text of a table, worded when it is shown (in the display language of that moment).
    typealias Words = @Sendable ([String: String]) -> String

    /// The detail line: a reminder's own line (a certificate's or a warranty's day, a payment's amount), else the message
    /// (or the reason) and the status, then the channel when it is not the app's.
    public static func detail(_ notification: CoreNotification) -> String {
        let params = notification.params
        let text: String
        switch notification.templateKey {
        case .certificateExpiry:
            text = tr("Expires {date}", ["date": Format.date(params.at)])
        case .warrantyExpiring:
            text = tr("Warranty ends {date} · no maintenance contract", ["date": Format.date(params.at)])
        case .paymentReminder:
            text =
                params.amountMinor.flatMap { minor in
                    params.currency.map { tr("Amount due {amount}", ["amount": money(minor, $0.rawValue)]) }
                } ?? ""
        default:
            // a status the API filled in from the type (the notification had none) is left out
            let status = params.status == notification.type.rawValue ? nil : statusWords(params.status)
            let message = params.message.map(Self.message) ?? params.reason.map(reasonWords)
            text = [message, status].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
        }
        return [text, channel(notification.channel)].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// "2,110.00 MYR": minor units with two decimals and thousands separators, as the web writes an amount.
    static func money(_ minor: Double, _ currency: String) -> String {
        let amount = (minor / 100).formatted(.number.precision(.fractionLength(2)).grouping(.automatic).locale(Locale(identifier: "en_MY")))
        return "\(amount) \(currency)"
    }

    static func channel(_ channel: Channel) -> String? {
        switch channel {
        case .inApp: nil
        case .email: tr("email")
        case .whatsapp: "WhatsApp"
        }
    }

    /// The status codes the sources store, in words; another code stays as stored.
    static func statusWords(_ code: String) -> String {
        statuses[code]?([:]) ?? code
    }

    private static let statuses: [String: Words] = [
        "open": { _ in tr("Unresolved") },
        "requested": { _ in tr("Requested") },
        "time_proposed": { _ in tr("Time proposed") },
        "offered": { _ in tr("Offered") },
        "accepted": { _ in tr("Accepted") },
        "declined": { _ in tr("Declined") },
        "assigned": { _ in tr("Assigned") },
        "in_progress": { _ in tr("In progress") },
        "submitted": { _ in tr("Submitted") },
        "returned": { _ in tr("Returned for rework") },
        "rework_requested": { _ in tr("Rework requested") },
        "completed": { _ in tr("Completed") },
        "cancelled": { _ in tr("Cancelled") },
        "on_hold": { _ in tr("On hold") },
        "scheduled": { _ in tr("Scheduled") },
        "applied": { _ in tr("Applied") },
        "release_requested": { _ in tr("Release requested") },
        "released": { _ in tr("Released") },
        "received": { _ in tr("Received") },
        "answered": { _ in tr("Answered") },
        "confirmed": { _ in tr("Payment confirmed") },
        "failed": { _ in tr("Failed") },
        "escalated": { _ in tr("Escalated — nobody acknowledged it") },
        "expired": { _ in tr("Expired") },
        "reschedule_requested": { _ in tr("Reschedule requested") },
        "time_change_proposed": { _ in tr("Time change proposed") },
        "time_change_kept": { _ in tr("Time change not accepted") },
        "cant_make": { _ in tr("Can’t make the visit") },
        "restriction_bypass": { _ in tr("Restriction may be bypassed — HQ checks the unit") },
    ]

    /// Why an assignment was released (IR439).
    static func reasonWords(_ code: String) -> String {
        switch code {
        case "technician_access_ended": tr("Technician’s access ended — to be reassigned")
        case "technician_qualification_ended": tr("Technician’s qualification ended — to be reassigned")
        default: code
        }
    }

    private static let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

    /// A message: a firmware campaign's notice is dated again in the display language (IR339); any other is the
    /// evidence of the alert it comes from (IR480).
    static func message(_ text: String) -> String {
        let firmware =
            #/
            Firmware\ update\ from\ (\d{1,2})\ ([A-Z][a-z]{2})\ (\d{4}),\ ([^\s–]+)–([^\s–]+)
            \ local\ time\.\ The\ AC\ may\ restart\ for\ about\ 2\ minutes\.
            /#
        guard let match = text.wholeMatch(of: firmware), let month = months.firstIndex(of: String(match.2)) else {
            return Evidence.text(text)
        }
        let dayOfMonth = match.1.count == 1 ? "0\(match.1)" : String(match.1)
        let day = String(format: "%@-%02d-%@T00:00:00Z", String(match.3), month + 1, dayOfMonth)
        return tr(
            "Firmware update from {date}, {from}–{to} local time. The AC may restart for about 2 minutes.",
            ["date": Format.date(day, timeZone: .gmt), "from": String(match.4), "to": String(match.5)]
        )
    }
}
